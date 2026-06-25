package api

import "net/http"

func (s *Server) listClusters(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListClusters()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toClusterResponses(rows))
}

func (s *Server) getCluster(w http.ResponseWriter, r *http.Request) {
	row, err := s.store.GetCluster(r.PathValue("name"))
	if err != nil {
		dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toClusterResponse(row))
}

func (s *Server) createCluster(w http.ResponseWriter, r *http.Request) {
	var req createClusterRequest
	if err := readJSON(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Port == 0 {
		writeErr(w, http.StatusBadRequest, "port is required")
		return
	}
	// CreateCluster writes the row and the immutable cluster-scope plugins/configs/env
	// in one tx.
	if err := s.store.CreateCluster(req.toRow(), req.Plugins, req.Configs, req.Env); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error()) // unique port / bad name
		return
	}
	created, err := s.store.GetCluster(req.Name)
	if err != nil {
		dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toClusterResponse(created))
}

func (s *Server) updateCluster(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	existing, err := s.store.GetCluster(name)
	if err != nil {
		dbErr(w, err)
		return
	}

	var req updateClusterRequest
	if err := readJSON(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if req.Port == 0 {
		writeErr(w, http.StatusBadRequest, "port is required")
		return
	}
	// plugins/configs/env aren't in updateClusterRequest, so they stay as created.
	row := existing
	req.applyTo(&row)
	if err := s.store.UpdateCluster(name, row); err != nil {
		writeErr(w, http.StatusConflict, err.Error()) // port collision → client error
		return
	}
	s.mgr.Nudge() // a changed port/lb policy makes members rebind on reconcile

	updated, err := s.store.GetCluster(name)
	if err != nil {
		dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toClusterResponse(updated))
}

func (s *Server) deleteCluster(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteCluster(r.PathValue("name")); err != nil {
		// servers.cluster FK blocks deletion while members exist
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- per-cluster plugin / config / env assignments (read-only) ---
//
// Set at cluster creation and immutable after (members bake them in at start), so
// these are inspection-only. Members inherit them unless they override at create.

func (s *Server) getClusterPlugins(w http.ResponseWriter, r *http.Request) {
	set, err := s.store.ClusterPlugins(r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toPluginSetResponse(set))
}

func (s *Server) getClusterConfigs(w http.ResponseWriter, r *http.Request) {
	set, err := s.store.ClusterConfigs(r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toConfigSetResponse(set))
}

func (s *Server) getClusterEnv(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListEnvVars("cluster", r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEnvVarResponses(rows))
}
