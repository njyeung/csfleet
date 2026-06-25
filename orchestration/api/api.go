// Package api is the orchestrator's HTTP control plane. Handlers write intent to
// the database (the source of truth) and Nudge the fleet manager to reconcile;
// they never touch containers or the proxy directly. Live state is read back from
// the manager's in-memory snapshot and streamed to clients over SSE.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"csfleet/orchestrator/database"
	"csfleet/orchestrator/fleet"
)

const maxBodyBytes = 1 << 20 // 1 MiB cap on request bodies

// Config is the API's runtime configuration.
type Config struct {
	Addr     string // listen address, e.g. ":8080"
	IPPrefix string // /24 host prefix for auto-allocating server IPs, e.g. "172.30.0."
}

// Server is the HTTP control plane. It holds the Store it writes intent to and
// the Manager it nudges and snapshots.
type Server struct {
	cfg   Config
	store *database.Store
	mgr   *fleet.Manager
	hub   *sseHub
	http  *http.Server
}

func New(cfg Config, store *database.Store, mgr *fleet.Manager) *Server {
	s := &Server{cfg: cfg, store: store, mgr: mgr, hub: newSSEHub()}

	mux := http.NewServeMux()
	s.routes(mux)
	s.http = &http.Server{Addr: cfg.Addr, Handler: mux}
	return s
}

// Run starts the SSE fan-out and the HTTP server, blocking until ctx is
// cancelled, then drains in-flight requests. Mirrors fleet.Manager.Run.
func (s *Server) Run(ctx context.Context) error {
	go s.hub.run(ctx, s.mgr.Changes(), s.snapshot)

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.http.Shutdown(shutCtx); err != nil {
			log.Printf("[api] shutdown: %v", err)
		}
	}()

	log.Printf("[api] listening on %s", s.cfg.Addr)
	if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// routes wires the HTTP surface. Go 1.22+ method+pattern routing; {name...}
// captures config paths that contain slashes (e.g. "cfg/server.cfg").
func (s *Server) routes(mux *http.ServeMux) {
	// Servers
	mux.HandleFunc("GET /api/servers", s.listServers)
	mux.HandleFunc("POST /api/servers", s.createServer)
	mux.HandleFunc("GET /api/servers/{name}", s.getServer)
	mux.HandleFunc("PUT /api/servers/{name}", s.updateServer)
	mux.HandleFunc("DELETE /api/servers/{name}", s.deleteServer)
	mux.HandleFunc("POST /api/servers/{name}/start", s.startServer)
	mux.HandleFunc("POST /api/servers/{name}/stop", s.stopServer)
	// Server plugins/configs/env are fixed at creation, so these are read-only.
	mux.HandleFunc("GET /api/servers/{name}/plugins", s.getServerPlugins)
	mux.HandleFunc("GET /api/servers/{name}/configs", s.getServerConfigs)
	mux.HandleFunc("GET /api/servers/{name}/env", s.getServerEnv)

	// Clusters
	mux.HandleFunc("GET /api/clusters", s.listClusters)
	mux.HandleFunc("POST /api/clusters", s.createCluster)
	mux.HandleFunc("GET /api/clusters/{name}", s.getCluster)
	mux.HandleFunc("PUT /api/clusters/{name}", s.updateCluster)
	mux.HandleFunc("DELETE /api/clusters/{name}", s.deleteCluster)
	// Cluster plugins/configs/env are fixed at creation, so these are read-only.
	mux.HandleFunc("GET /api/clusters/{name}/plugins", s.getClusterPlugins)
	mux.HandleFunc("GET /api/clusters/{name}/configs", s.getClusterConfigs)
	mux.HandleFunc("GET /api/clusters/{name}/env", s.getClusterEnv)

	// Global-scope plugin / config assignment (the only editable assignment tier)
	mux.HandleFunc("GET /api/global/plugins", s.getGlobalPlugins)
	mux.HandleFunc("PUT /api/global/plugins", s.setGlobalPlugins)
	mux.HandleFunc("GET /api/global/configs", s.getGlobalConfigs)
	mux.HandleFunc("PUT /api/global/configs", s.setGlobalConfigs)

	// Plugin manifests
	mux.HandleFunc("GET /api/plugins", s.listManifests)
	mux.HandleFunc("GET /api/plugins/{name}", s.getManifest)
	mux.HandleFunc("PUT /api/plugins/{name}", s.putManifest)
	mux.HandleFunc("DELETE /api/plugins/{name}", s.deleteManifest)

	// Config files
	mux.HandleFunc("GET /api/configs", s.listConfigFiles)
	mux.HandleFunc("GET /api/configs/{name...}", s.getConfigFile)
	mux.HandleFunc("PUT /api/configs/{name...}", s.putConfigFile)
	mux.HandleFunc("DELETE /api/configs/{name...}", s.deleteConfigFile)

	// GSLT token pool
	mux.HandleFunc("GET /api/gslt-tokens", s.listTokens)
	mux.HandleFunc("POST /api/gslt-tokens", s.addToken)
	mux.HandleFunc("DELETE /api/gslt-tokens", s.deleteToken)

	// Env variables: GET reads any scope; PUT/DELETE edit global only
	// (cluster/server env is set at creation).
	mux.HandleFunc("GET /api/env", s.listEnv)
	mux.HandleFunc("PUT /api/env", s.setEnv)
	mux.HandleFunc("DELETE /api/env", s.deleteEnv)

	// Live status stream
	mux.HandleFunc("GET /api/events", s.handleSSE)
}

// --- shared helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			log.Printf("[api] encode response: %v", err)
		}
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// readJSON decodes a capped request body into dst.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

// readItems decodes a {"items": [...]} body — the shape the plugin and config
// assignment endpoints accept. A nil/empty items is a valid "override with none".
func readItems(w http.ResponseWriter, r *http.Request) ([]string, error) {
	var body struct {
		Items []string `json:"items"`
	}
	if err := readJSON(w, r, &body); err != nil {
		return nil, err
	}
	return body.Items, nil
}

// dbErr maps a store error to a status: a missing row is 404, anything else 500.
func dbErr(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeErr(w, http.StatusInternalServerError, err.Error())
}
