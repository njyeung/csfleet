package api

import (
	"fmt"
	"time"

	"csfleet/orchestrator/database"
	"csfleet/orchestrator/fleet"
)

// --- Auth ---

// userResponse is one web UI account, never including the password hash. Seed
// marks the in-memory admin (from .env), which can't be deleted or reset via the UI.
type userResponse struct {
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	Seed      bool      `json:"seed"`
}

// --- Orchestrator ---

// orchestratorInfoResponse describes the host the orchestrator runs on, not any
// one server or cluster. The identity/hardware fields are static (sampled once at
// startup); Host carries the live, periodically-resampled utilization metrics.
type orchestratorInfoResponse struct {
	LocalIP  string    `json:"local_ip"`
	PublicIP string    `json:"public_ip"`
	Hostname string    `json:"hostname"`
	CPUModel string    `json:"cpu_model"`
	CPUCores int       `json:"cpu_cores"`
	MemTotal uint64    `json:"mem_total_bytes"`
	Host     hostStats `json:"host_stats"`
}

// hostStats is the live slice of orchestratorInfoResponse: utilization figures
// refreshed by the background sampler.
type hostStats struct {
	CPUPercent     float64    `json:"cpu_percent"`
	PerCorePercent []float64  `json:"per_core_percent"`
	LoadAvg        [3]float64 `json:"load_avg"`
	MemUsed        uint64     `json:"mem_used_bytes"`
	MemAvailable   uint64     `json:"mem_available_bytes"`
	SwapUsed       uint64     `json:"swap_used_bytes"`
	DiskUsed       uint64     `json:"disk_used_bytes"`
	DiskTotal      uint64     `json:"disk_total_bytes"`
	UptimeSeconds  uint64     `json:"uptime_seconds"`
	SampledAt      time.Time  `json:"sampled_at"`
}

// --- Servers ---

// serverResponse is a server's spec plus its live state and the effective
// plugin/config/env sets it actually runs (resolved global < cluster < server),
// so a list or SSE consumer sees what a server runs without a follow-up call per
// server. The dedicated /plugins, /configs and /env endpoints carry the same sets
// with extra detail (override flag, env source scope).
type serverResponse struct {
	Name            string            `json:"name"`
	IP              string            `json:"ip"`
	Port            *int              `json:"port"`
	Cluster         *string           `json:"cluster"`
	AutoToken       *bool             `json:"auto_token"`
	AcceptingConns  *bool             `json:"accepting_connections"`
	RestartAfterHrs *float64          `json:"restart_after_hrs"`
	StopAfterHrs    *float64          `json:"stop_after_hrs"`
	DesiredState    string            `json:"desired_state"`
	ActualState     string            `json:"actual_state"`
	LastError       string            `json:"last_error,omitempty"`
	Plugins         []string          `json:"plugins"`
	Configs         []string          `json:"configs"`
	Env             map[string]string `json:"env"`
	UpdatedAt       time.Time         `json:"updated_at"`
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

// createServerRequest is the body of POST /api/servers. Membership and the
// Plugins/Configs/Env sets are fixed here. Plugins/Configs are tri-state: nil
// inherits, a non-nil slice overrides. Empty means "run none".
// Env variables are scoped overlay from global, cluster, then server.
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
// fields only.
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

// --- Effective per-server plugin / config sets ---
//
// Unlike the scope set responses above (which report what one scope defines),
// these report the resolved set a server actually runs (global < cluster <
// server). Overridden flags whether the server's own scope supplied the set
// (vs inheriting it from a cluster or global).

type effectivePluginsResponse struct {
	Overridden bool     `json:"overridden"`
	Items      []string `json:"items"`
}

type effectiveConfigsResponse struct {
	Overridden bool     `json:"overridden"`
	Items      []string `json:"items"`
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
	Filename  string    `json:"filename"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toConfigFileResponse(r database.ConfigFileRow) configFileResponse {
	return configFileResponse{Name: r.Name, Filename: r.Filename, Content: r.Content, UpdatedAt: r.UpdatedAt}
}

func toConfigFileResponses(rows []database.ConfigFileRow) []configFileResponse {
	out := make([]configFileResponse, len(rows))
	for i, r := range rows {
		out[i] = toConfigFileResponse(r)
	}
	return out
}

type putConfigFileRequest struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
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
