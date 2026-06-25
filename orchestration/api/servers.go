package api

import (
	"fmt"
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
	writeJSON(w, http.StatusOK, toServerResponses(statuses))
}

func (s *Server) getServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	row, err := s.store.GetServer(name)
	if err != nil {
		dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toServerResponse(s.statusFor(row)))
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
	writeJSON(w, http.StatusCreated, toServerResponse(s.statusFor(created)))
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
	writeJSON(w, http.StatusOK, toServerResponse(s.statusFor(updated)))
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
// them by recreating the server or editing the cluster it inherits from.

func (s *Server) getServerPlugins(w http.ResponseWriter, r *http.Request) {
	set, err := s.store.ServerPlugins(r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toPluginSetResponse(set))
}

func (s *Server) getServerConfigs(w http.ResponseWriter, r *http.Request) {
	set, err := s.store.ServerConfigs(r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toConfigSetResponse(set))
}

func (s *Server) getServerEnv(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListEnvVars("server", r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEnvVarResponses(rows))
}

// --- helpers ---

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
