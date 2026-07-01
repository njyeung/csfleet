package api

import "net/http"

// --- Plugin manifests ---
//
// The catalog of installable plugins. Editing a manifest does not touch running
// servers; the new definition is picked up the next time a server using it starts.

func (s *Server) listManifests(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListManifests()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toManifestResponses(rows))
}

func (s *Server) getManifest(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	manifest, err := s.store.LoadManifest(name)
	if err != nil {
		dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, manifestResponse{Name: name, Manifest: manifest})
}

func (s *Server) putManifest(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body putManifestRequest
	if err := readJSON(w, r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if err := s.store.UpsertManifest(name, body.Manifest); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteManifest(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteManifest(r.PathValue("name")); err != nil {
		// csfleet_plugin_assignments FK blocks deletion while a scope still assigns it
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Config files ---

func (s *Server) listConfigFiles(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListConfigFiles()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toConfigFileResponses(rows))
}

func (s *Server) getConfigFile(w http.ResponseWriter, r *http.Request) {
	row, err := s.store.GetConfigFile(r.PathValue("name"))
	if err != nil {
		dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toConfigFileResponse(row))
}

func (s *Server) putConfigFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body putConfigFileRequest
	if err := readJSON(w, r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if body.Filename == "" {
		writeErr(w, http.StatusBadRequest, "filename is required")
		return
	}
	if err := s.store.UpsertConfigFile(name, body.Filename, body.Content); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteConfigFile(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteConfigFile(r.PathValue("name")); err != nil {
		// csfleet_config_assignments FK blocks deletion while a scope still assigns it
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Global plugin / config assignment ---
//
// Global is the base of the global < cluster < server inheritance chain and the
// only editable assignment tier (cluster/server sets are fixed at creation). An
// empty items list is an explicit "none" that members can still override.

func (s *Server) getGlobalPlugins(w http.ResponseWriter, r *http.Request) {
	set, err := s.store.GlobalPlugins()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toPluginSetResponse(set))
}

func (s *Server) setGlobalPlugins(w http.ResponseWriter, r *http.Request) {
	items, err := readItems(w, r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if err := s.store.SetGlobalPlugins(items); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error()) // unknown plugin → FK error
		return
	}
	w.WriteHeader(http.StatusNoContent) // applies on each server's next start
}

func (s *Server) getGlobalConfigs(w http.ResponseWriter, r *http.Request) {
	set, err := s.store.GlobalConfigs()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toConfigSetResponse(set))
}

func (s *Server) setGlobalConfigs(w http.ResponseWriter, r *http.Request) {
	items, err := readItems(w, r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if err := s.store.SetGlobalConfigs(items); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error()) // unknown config → FK error
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Env variables ---
//
// GET reads any scope (global|cluster|server) for inspection. Writes (PUT/DELETE)
// are restricted to global scope: cluster/server env is set at the resource's
// creation and immutable after, since env is injected into the container at start.

func (s *Server) listEnv(w http.ResponseWriter, r *http.Request) {
	scope, scopeName := envScope(r)
	rows, err := s.store.ListEnvVars(scope, scopeName)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEnvVarResponses(rows))
}

func (s *Server) setEnv(w http.ResponseWriter, r *http.Request) {
	var req setEnvRequest
	if err := readJSON(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if req.Key == "" {
		writeErr(w, http.StatusBadRequest, "key is required")
		return
	}
	if req.Scope == "" {
		req.Scope = "global"
	}
	if req.Scope != "global" {
		writeErr(w, http.StatusBadRequest, "only global env is editable; cluster/server env is set at creation")
		return
	}
	if err := s.store.SetEnvVar(req.Key, req.Value, req.Scope, req.ScopeName); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteEnv(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeErr(w, http.StatusBadRequest, "key query param is required")
		return
	}
	scope, scopeName := envScope(r)
	if scope != "global" {
		writeErr(w, http.StatusBadRequest, "only global env is editable; cluster/server env is set at creation")
		return
	}
	if err := s.store.DeleteEnvVar(key, scope, scopeName); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// envScope reads the scope/scope_name query params, defaulting scope to "global".
func envScope(r *http.Request) (scope, scopeName string) {
	scope = r.URL.Query().Get("scope")
	if scope == "" {
		scope = "global"
	}
	return scope, r.URL.Query().Get("scope_name")
}
