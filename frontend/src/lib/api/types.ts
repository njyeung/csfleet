// Wire types for the orchestrator control plane. These mirror the DTOs in
// orchestration/api/types.go — keep them in sync. Field nullability follows the
// Go pointer fields: a `T | null` here is a `*T` there (omittable / nullable).
//
// Tri-state plugins/configs on create requests: omit (undefined) = inherit from
// the parent scope; [] = explicitly override with none; [..] = override with these.

// --- Orchestrator Info ---

// Live host utilization, resampled by the orchestrator's background sampler and
// pushed on the SSE `host` event. per_core_percent is indexed by logical core
// (length cpu_cores).
export interface HostStats {
	cpu_percent: number;
	per_core_percent: number[];
	load_avg: [number, number, number];
	mem_used_bytes: number;
	mem_available_bytes: number;
	swap_used_bytes: number;
	disk_used_bytes: number;
	disk_total_bytes: number;
	uptime_seconds: number;
	sampled_at: string;
}

// The host the orchestrator runs on. Identity/hardware fields are static (sampled
// once at startup); host_stats carries the live, periodically-resampled metrics.
export interface OrchestratorInfo {
	local_ip: string;
	public_ip: string;
	hostname: string;
	cpu_model: string;
	cpu_cores: number;
	mem_total_bytes: number;
	host_stats: HostStats;
}

// --- Servers ---

export interface Server {
	name: string;
	ip: string;
	port: number | null;
	cluster: string | null;
	auto_token: boolean | null;
	accepting_connections: boolean | null;
	restart_after_hrs: number | null;
	stop_after_hrs: number | null;
	desired_state: string;
	actual_state: string;
	last_error?: string;
	// Effective sets the server actually runs (resolved global < cluster < server),
	// embedded in the list/SSE payload for a quick view without a follow-up call.
	// These are the lean views: no override flag, and env is collapsed to key→value
	// with no source scope. For the detail panel's richer display (override badge,
	// per-var scope) use the dedicated GET .../plugins|configs|env endpoints.
	// Best-effort on the server side, so a field may be null on a resolution error.
	plugins: string[] | null;
	configs: string[] | null;
	env: Record<string, string> | null;
	updated_at: string; // RFC3339
}

export interface CreateServerRequest {
	name: string;
	// Exactly one of port (standalone) or cluster (member) must be set.
	port?: number | null;
	cluster?: string | null;
	auto_token?: boolean | null;
	accepting_connections?: boolean | null;
	restart_after_hrs?: number | null;
	stop_after_hrs?: number | null;
	desired_state?: string;
	plugins?: string[] | null;
	configs?: string[] | null;
	env?: Record<string, string>;
}

// Live-mutable fields only. Membership, ip, plugins, configs and env are
// create-only and absent here. A standalone server's port may be retargeted.
export interface UpdateServerRequest {
	port?: number | null;
	auto_token?: boolean | null;
	accepting_connections?: boolean | null;
	restart_after_hrs?: number | null;
	stop_after_hrs?: number | null;
	desired_state?: string;
}

// --- Clusters ---

export type LBPolicy = 'round_robin' | 'packing' | 'sparse';

export interface Cluster {
	name: string;
	port: number;
	auto_token: boolean;
	accepting_connections: boolean;
	restart_after_hrs: number | null;
	stop_after_hrs: number | null;
	lb_policy: string;
	updated_at: string;
}

export interface CreateClusterRequest {
	name: string;
	port: number;
	auto_token?: boolean | null;
	accepting_connections?: boolean | null;
	restart_after_hrs?: number | null;
	stop_after_hrs?: number | null;
	lb_policy?: LBPolicy;
	plugins?: string[] | null;
	configs?: string[] | null;
	env?: Record<string, string>;
}

// Structural / override-tier knobs. Port is required (> 0). Plugins/configs/env
// are create-only and absent.
export interface UpdateClusterRequest {
	port: number;
	auto_token?: boolean | null;
	accepting_connections?: boolean | null;
	restart_after_hrs?: number | null;
	stop_after_hrs?: number | null;
	lb_policy?: LBPolicy;
}

// --- Plugin / config assignment sets ---
// Read-only on server/cluster (fixed at creation); editable only at global scope.

export interface AssignmentSet {
	overridden: boolean;
	items: string[] | null; // nil slice on the Go side marshals as null, e.g. an inherited/empty set
}

// --- Plugin manifests (catalog) ---

export interface Manifest {
	name: string;
	manifest: string; // TOML source
	updated_at: string;
}

// --- Config files (catalog) ---

export interface ConfigFile {
	name: string; // catalog identifier (PK); what assignments reference. May contain slashes.
	filename: string; // path under game/csgo/cfg/, e.g. "gamemode_competitive_server.cfg"
	content: string;
	updated_at: string;
}

// --- Env variables ---

export type EnvScope = 'global' | 'cluster' | 'server';

export interface EnvVar {
	key: string;
	value: string;
	scope: string;
	scope_name: string;
}

// Only global scope is editable; cluster/server env is create-only.
export interface SetEnvRequest {
	key: string;
	value: string;
	scope: EnvScope;
	scope_name: string;
}
