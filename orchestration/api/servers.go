package api

import (
	"fmt"
	"log"
	"net/http"

	"csfleet/orchestrator/database"
	"csfleet/orchestrator/fleet"
)

// host-octet range for auto-allocated server IPs. .1/.2/.3 (gateway, db, nginx)
// fall below the floor and are never handed out.
const (
	ipAllocMin = 10
	ipAllocMax = 254
)

func (s *Server) listServers(w http.ResponseWriter, r *http.Request) {
	statuses, err := s.serverStatuses()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.serverResponses(statuses))
}

func (s *Server) getServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	row, err := s.store.GetServer(name)
	if err != nil {
		dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.serverResponse(s.statusFor(row)))
}

func (s *Server) createServer(w http.ResponseWriter, r *http.Request) {
	var req createServerRequest
	if err := readJSON(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	row := req.toRow()
	if err := validateReachable(row); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if row.DesiredState == "" {
		row.DesiredState = "running"
	}
	// ip is orchestrator-managed: always allocated here, never taken from the body.
	ip, err := s.allocateIP()
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	row.IP = ip

	// CreateServer writes the row and the immutable server-scope plugins/configs/env
	// in one tx, so the worker never sees the row before its overrides.
	if err := s.store.CreateServer(row, req.Plugins, req.Configs, req.Env); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error()) // unique/FK/check → client error
		return
	}
	s.mgr.Nudge() // spawn a worker for the new row

	created, err := s.store.GetServer(row.Name)
	if err != nil {
		dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.serverResponse(s.statusFor(created)))
}

func (s *Server) updateServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	existing, err := s.store.GetServer(name)
	if err != nil {
		dbErr(w, err)
		return
	}

	var req updateServerRequest
	if err := readJSON(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}

	// applyTo preserves ip and cluster membership and enforces the standalone-only
	// port rule; ip/plugins/configs/env aren't in the contract, so they can't move.
	row := existing
	if err := req.applyTo(&row); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.UpdateServer(name, row); err != nil {
		writeErr(w, http.StatusConflict, err.Error()) // port collision → client error
		return
	}
	s.mgr.Nudge() // a changed port makes the worker rebind on reconcile

	updated, err := s.store.GetServer(name)
	if err != nil {
		dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.serverResponse(s.statusFor(updated)))
}

func (s *Server) deleteServer(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteServer(r.PathValue("name")); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.mgr.Nudge() // the worker reaps itself once its row is gone
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) startServer(w http.ResponseWriter, r *http.Request) {
	s.setDesired(w, r, "running")
}

func (s *Server) stopServer(w http.ResponseWriter, r *http.Request) {
	s.setDesired(w, r, "stopped")
}

func (s *Server) setDesired(w http.ResponseWriter, r *http.Request, state string) {
	if err := s.store.UpdateServerDesiredState(r.PathValue("name"), state); err != nil {
		dbErr(w, err)
		return
	}
	s.mgr.Nudge()
	w.WriteHeader(http.StatusNoContent)
}

// --- per-server plugin / config / env assignments (read-only) ---
//
// A server's plugins, configs and env are fixed at creation (baked into the
// container at start), so these are inspection-only — there is no PUT. Change
// them by recreating the server or editing the cluster it inherits from. Each
// returns the effective set the server actually runs (resolved global < cluster
// < server), not just what the server scope itself defines.

func (s *Server) getServerPlugins(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	row, err := s.store.GetServer(name)
	if err != nil {
		dbErr(w, err)
		return
	}
	own, err := s.store.ServerPlugins(name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items, err := s.store.EffectivePlugins(name, clusterOf(row))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, effectivePluginsResponse{Overridden: own.Overridden, Items: items})
}

func (s *Server) getServerConfigs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	row, err := s.store.GetServer(name)
	if err != nil {
		dbErr(w, err)
		return
	}
	own, err := s.store.ServerConfigs(name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items, err := s.store.EffectiveConfigs(name, clusterOf(row))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, effectiveConfigsResponse{Overridden: own.Overridden, Items: items})
}

func (s *Server) getServerEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	row, err := s.store.GetServer(name)
	if err != nil {
		dbErr(w, err)
		return
	}
	rows, err := s.store.EffectiveEnv(name, clusterOf(row))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEnvVarResponses(rows))
}

// --- helpers ---

// serverResponse builds a server's DTO and overlays the effective plugin/config/
// env sets it runs. serverResponses does the same for a list (GET /api/servers
// and the SSE push), so a consumer sees what each server runs without a per-server
// follow-up call.
func (s *Server) serverResponse(st fleet.ServerStatus) serverResponse {
	return s.withEffective(toServerResponse(st), st.ServerRow)
}

func (s *Server) serverResponses(statuses []fleet.ServerStatus) []serverResponse {
	out := make([]serverResponse, len(statuses))
	for i, st := range statuses {
		out[i] = s.serverResponse(st)
	}
	return out
}

// withEffective fills the effective plugin/config/env sets onto a server DTO.
// Best-effort: a resolution error leaves that field empty and is logged, so one
// bad server never blanks the whole list or an SSE push.
func (s *Server) withEffective(resp serverResponse, row database.ServerRow) serverResponse {
	cluster := clusterOf(row)
	// if a member has no own port, overlay its cluster's.
	if resp.Port == nil && row.Cluster != nil {
		if cl, err := s.store.GetCluster(*row.Cluster); err != nil {
			log.Printf("[api] effective port for %q: %v", row.Name, err)
		} else {
			resp.Port = &cl.Port
		}
	}
	if plugins, err := s.store.EffectivePlugins(row.Name, cluster); err != nil {
		log.Printf("[api] effective plugins for %q: %v", row.Name, err)
	} else {
		resp.Plugins = plugins
	}
	if configs, err := s.store.EffectiveConfigs(row.Name, cluster); err != nil {
		log.Printf("[api] effective configs for %q: %v", row.Name, err)
	} else {
		resp.Configs = configs
	}
	if env, err := s.store.LoadEnv(row.Name, cluster); err != nil {
		log.Printf("[api] effective env for %q: %v", row.Name, err)
	} else {
		resp.Env = env
	}
	return resp
}

// clusterOf returns a server's cluster name, or "" when it is standalone — the
// form the resolve helpers expect.
func clusterOf(row database.ServerRow) string {
	if row.Cluster != nil {
		return *row.Cluster
	}
	return ""
}

// serverStatuses merges the configured fleet (DB rows, authoritative for the
// spec) with live worker phases (manager, authoritative for actual_state). A
// server the manager has not spawned a worker for yet reads as "stopped".
func (s *Server) serverStatuses() ([]fleet.ServerStatus, error) {
	rows, err := s.store.ListServers()
	if err != nil {
		return nil, err
	}
	live := make(map[string]fleet.ServerStatus, len(rows))
	for _, st := range s.mgr.Status() {
		live[st.Name] = st
	}
	out := make([]fleet.ServerStatus, 0, len(rows))
	for _, row := range rows {
		out = append(out, mergeStatus(row, live[row.Name]))
	}
	return out, nil
}

// statusFor builds the merged status for a single server row.
func (s *Server) statusFor(row database.ServerRow) fleet.ServerStatus {
	for _, st := range s.mgr.Status() {
		if st.Name == row.Name {
			return mergeStatus(row, st)
		}
	}
	return mergeStatus(row, fleet.ServerStatus{})
}

// mergeStatus keeps the DB row as the spec and overlays the worker's live state;
// with no worker (zero live) the server reads as "stopped".
func mergeStatus(row database.ServerRow, live fleet.ServerStatus) fleet.ServerStatus {
	state := live.ActualState
	if state == "" {
		state = "stopped"
	}
	return fleet.ServerStatus{ServerRow: row, ActualState: state, LastError: live.LastError}
}

// allocateIP returns the lowest free host address in the configured /24.
func (s *Server) allocateIP() (string, error) {
	if s.cfg.IPPrefix == "" {
		return "", fmt.Errorf("ip auto-allocation not configured; provide an explicit ip")
	}
	rows, err := s.store.ListServers()
	if err != nil {
		return "", err
	}
	used := make(map[string]bool, len(rows))
	for _, r := range rows {
		used[r.IP] = true
	}
	for i := ipAllocMin; i <= ipAllocMax; i++ {
		if ip := fmt.Sprintf("%s%d", s.cfg.IPPrefix, i); !used[ip] {
			return ip, nil
		}
	}
	return "", fmt.Errorf("no free ip in %s%d-%d", s.cfg.IPPrefix, ipAllocMin, ipAllocMax)
}

// validateReachable mirrors the servers.server_reachable CHECK: exactly one of
// port (standalone) or cluster (member) must be set.
func validateReachable(row database.ServerRow) error {
	if (row.Port == nil) == (row.Cluster == nil) {
		return fmt.Errorf("set exactly one of port (standalone) or cluster (member)")
	}
	return nil
}
