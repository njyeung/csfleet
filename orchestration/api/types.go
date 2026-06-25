package api

import (
	"fmt"
	"time"

	"csfleet/orchestrator/database"
	"csfleet/orchestrator/fleet"
)

// This file is the API's wire contract. The handlers speak these DTOs and map to
// and from the internal database/fleet structs, so the HTTP surface is decoupled
// from the DB schema. The split also encodes what a client may set: ip is
// orchestrator-managed (response only); a server/cluster's plugins, configs and
// env are baked into the container at start, so they appear only on create, never
// on update.

// --- Servers ---

// serverResponse is a server's spec plus its live state.
type serverResponse struct {
	Name            string    `json:"name"`
	IP              string    `json:"ip"`
	Port            *int      `json:"port"`
	Cluster         *string   `json:"cluster"`
	AutoToken       *bool     `json:"auto_token"`
	AcceptingConns  *bool     `json:"accepting_connections"`
	RestartAfterHrs *float64  `json:"restart_after_hrs"`
	StopAfterHrs    *float64  `json:"stop_after_hrs"`
	DesiredState    string    `json:"desired_state"`
	ActualState     string    `json:"actual_state"`
	LastError       string    `json:"last_error,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func toServerResponse(st fleet.ServerStatus) serverResponse {
	r := st.ServerRow
	return serverResponse{
		Name:            r.Name,
		IP:              r.IP,
		Port:            r.Port,
		Cluster:         r.Cluster,
		AutoToken:       r.AutoToken,
		AcceptingConns:  r.AcceptingConns,
		RestartAfterHrs: r.RestartAfterHrs,
		StopAfterHrs:    r.StopAfterHrs,
		DesiredState:    r.DesiredState,
		ActualState:     st.ActualState,
		LastError:       st.LastError,
		UpdatedAt:       r.UpdatedAt,
	}
}

func toServerResponses(statuses []fleet.ServerStatus) []serverResponse {
	out := make([]serverResponse, len(statuses))
	for i, st := range statuses {
		out[i] = toServerResponse(st)
	}
	return out
}

// createServerRequest is the body of POST /api/servers. There is no ip — the
// orchestrator allocates it. Membership (exactly one of Port or Cluster) and the
// Plugins/Configs/Env sets are fixed here. Plugins/Configs are tri-state: nil
// inherits, a non-nil slice (even empty) overrides — empty meaning "run none".
type createServerRequest struct {
	Name            string            `json:"name"`
	Port            *int              `json:"port"`
	Cluster         *string           `json:"cluster"`
	AutoToken       *bool             `json:"auto_token"`
	AcceptingConns  *bool             `json:"accepting_connections"`
	RestartAfterHrs *float64          `json:"restart_after_hrs"`
	StopAfterHrs    *float64          `json:"stop_after_hrs"`
	DesiredState    string            `json:"desired_state"`
	Plugins         *[]string         `json:"plugins"`
	Configs         *[]string         `json:"configs"`
	Env             map[string]string `json:"env"`
}

// toRow maps the request to a ServerRow. IP is left blank for the handler to
// allocate; Plugins/Configs/Env travel separately into CreateServer.
func (req createServerRequest) toRow() database.ServerRow {
	return database.ServerRow{
		Name:            req.Name,
		Port:            req.Port,
		Cluster:         req.Cluster,
		AutoToken:       req.AutoToken,
		AcceptingConns:  req.AcceptingConns,
		RestartAfterHrs: req.RestartAfterHrs,
		StopAfterHrs:    req.StopAfterHrs,
		DesiredState:    req.DesiredState,
	}
}

// updateServerRequest is the body of PUT /api/servers/{name}: the live-mutable
// fields only. Membership (cluster), ip, plugins, configs and env are create-only
// and absent here. A standalone server's port may change (rebinds live).
type updateServerRequest struct {
	Port            *int     `json:"port"`
	AutoToken       *bool    `json:"auto_token"`
	AcceptingConns  *bool    `json:"accepting_connections"`
	RestartAfterHrs *float64 `json:"restart_after_hrs"`
	StopAfterHrs    *float64 `json:"stop_after_hrs"`
	DesiredState    string   `json:"desired_state"`
}

// applyTo overlays the update onto an existing row, preserving ip and cluster
// membership. A non-nil Port retargets a standalone server's external port (nil
// leaves it; clearing would break the port-XOR-cluster invariant); a Port on a
// cluster member is rejected, since a member's port is the cluster's. An empty
// DesiredState preserves the current one (use /start and /stop to change it).
func (req updateServerRequest) applyTo(row *database.ServerRow) error {
	if req.Port != nil {
		if row.Cluster != nil {
			return fmt.Errorf("port is managed by the cluster for a member server")
		}
		row.Port = req.Port
	}
	row.AutoToken = req.AutoToken
	row.AcceptingConns = req.AcceptingConns
	row.RestartAfterHrs = req.RestartAfterHrs
	row.StopAfterHrs = req.StopAfterHrs
	if req.DesiredState != "" {
		row.DesiredState = req.DesiredState
	}
	return nil
}

// --- Clusters ---

type clusterResponse struct {
	Name            string    `json:"name"`
	Port            int       `json:"port"`
	AutoToken       bool      `json:"auto_token"`
	AcceptingConns  bool      `json:"accepting_connections"`
	RestartAfterHrs *float64  `json:"restart_after_hrs"`
	StopAfterHrs    *float64  `json:"stop_after_hrs"`
	LBPolicy        string    `json:"lb_policy"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func toClusterResponse(r database.ClusterRow) clusterResponse {
	return clusterResponse{
		Name:            r.Name,
		Port:            r.Port,
		AutoToken:       r.AutoToken,
		AcceptingConns:  r.AcceptingConns,
		RestartAfterHrs: r.RestartAfterHrs,
		StopAfterHrs:    r.StopAfterHrs,
		LBPolicy:        r.LBPolicy,
		UpdatedAt:       r.UpdatedAt,
	}
}

func toClusterResponses(rows []database.ClusterRow) []clusterResponse {
	out := make([]clusterResponse, len(rows))
	for i, r := range rows {
		out[i] = toClusterResponse(r)
	}
	return out
}

// createClusterRequest is the body of POST /api/clusters. AutoToken/AcceptingConns
// are pointers so an omitted field falls back to the schema default (TRUE) rather
// than decoding as false. Plugins/Configs/Env are set here and immutable after.
type createClusterRequest struct {
	Name            string            `json:"name"`
	Port            int               `json:"port"`
	AutoToken       *bool             `json:"auto_token"`
	AcceptingConns  *bool             `json:"accepting_connections"`
	RestartAfterHrs *float64          `json:"restart_after_hrs"`
	StopAfterHrs    *float64          `json:"stop_after_hrs"`
	LBPolicy        string            `json:"lb_policy"`
	Plugins         *[]string         `json:"plugins"`
	Configs         *[]string         `json:"configs"`
	Env             map[string]string `json:"env"`
}

// toRow maps the request to a ClusterRow, applying the inheritable defaults
// (auto_token/accepting TRUE, round-robin) a body may omit.
func (req createClusterRequest) toRow() database.ClusterRow {
	row := database.ClusterRow{
		Name:            req.Name,
		Port:            req.Port,
		AutoToken:       true,
		AcceptingConns:  true,
		RestartAfterHrs: req.RestartAfterHrs,
		StopAfterHrs:    req.StopAfterHrs,
		LBPolicy:        req.LBPolicy,
	}
	if req.AutoToken != nil {
		row.AutoToken = *req.AutoToken
	}
	if req.AcceptingConns != nil {
		row.AcceptingConns = *req.AcceptingConns
	}
	if row.LBPolicy == "" {
		row.LBPolicy = database.LBRoundRobin
	}
	return row
}

// updateClusterRequest is the body of PUT /api/clusters/{name}: the structural and
// override-tier knobs. Plugins/configs/env are create-only and absent. A changed
// Port rebinds every member (worker reconcile -> rebind).
type updateClusterRequest struct {
	Port            int      `json:"port"`
	AutoToken       *bool    `json:"auto_token"`
	AcceptingConns  *bool    `json:"accepting_connections"`
	RestartAfterHrs *float64 `json:"restart_after_hrs"`
	StopAfterHrs    *float64 `json:"stop_after_hrs"`
	LBPolicy        string   `json:"lb_policy"`
}

// applyTo overlays the update onto an existing cluster row. A nil AutoToken/
// AcceptingConns or empty LBPolicy preserves the current value.
func (req updateClusterRequest) applyTo(row *database.ClusterRow) {
	row.Port = req.Port
	if req.AutoToken != nil {
		row.AutoToken = *req.AutoToken
	}
	if req.AcceptingConns != nil {
		row.AcceptingConns = *req.AcceptingConns
	}
	row.RestartAfterHrs = req.RestartAfterHrs
	row.StopAfterHrs = req.StopAfterHrs
	if req.LBPolicy != "" {
		row.LBPolicy = req.LBPolicy
	}
}

// --- Plugin / config assignment sets (read-only on server/cluster, editable on global) ---

type pluginSetResponse struct {
	Overridden bool     `json:"overridden"`
	Items      []string `json:"items"`
}

func toPluginSetResponse(p database.ScopedPlugin) pluginSetResponse {
	return pluginSetResponse{Overridden: p.Overridden, Items: p.Items}
}

type configSetResponse struct {
	Overridden bool     `json:"overridden"`
	Items      []string `json:"items"`
}

func toConfigSetResponse(c database.ScopedConfig) configSetResponse {
	return configSetResponse{Overridden: c.Overridden, Items: c.Items}
}

// --- Plugin manifests (catalog) ---

type manifestResponse struct {
	Name      string    `json:"name"`
	Manifest  string    `json:"manifest"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toManifestResponse(r database.ManifestRow) manifestResponse {
	return manifestResponse{Name: r.Name, Manifest: r.Manifest, UpdatedAt: r.UpdatedAt}
}

func toManifestResponses(rows []database.ManifestRow) []manifestResponse {
	out := make([]manifestResponse, len(rows))
	for i, r := range rows {
		out[i] = toManifestResponse(r)
	}
	return out
}

type putManifestRequest struct {
	Manifest string `json:"manifest"`
}

// --- Config files (catalog) ---

type configFileResponse struct {
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toConfigFileResponse(r database.ConfigFileRow) configFileResponse {
	return configFileResponse{Name: r.Name, Content: r.Content, UpdatedAt: r.UpdatedAt}
}

func toConfigFileResponses(rows []database.ConfigFileRow) []configFileResponse {
	out := make([]configFileResponse, len(rows))
	for i, r := range rows {
		out[i] = toConfigFileResponse(r)
	}
	return out
}

type putConfigFileRequest struct {
	Content string `json:"content"`
}

// --- Env variables ---

type envVarResponse struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Scope     string `json:"scope"`
	ScopeName string `json:"scope_name"`
}

func toEnvVarResponses(rows []database.EnvVarRow) []envVarResponse {
	out := make([]envVarResponse, len(rows))
	for i, r := range rows {
		out[i] = envVarResponse{Key: r.Key, Value: r.Value, Scope: r.Scope, ScopeName: r.ScopeName}
	}
	return out
}

// setEnvRequest is the body of PUT /api/env. Only global scope is editable here;
// cluster/server env is create-only (immutable after the resource is created).
type setEnvRequest struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Scope     string `json:"scope"`
	ScopeName string `json:"scope_name"`
}
