package database

import "time"

// ServerRow is the raw spec for one server. The override-tier fields
// (AutoToken, AcceptingConns, RestartAfterHrs, StopAfterHrs) are nullable: nil
// means "inherit from the cluster" (else a built-in default). For the hour
// fields a non-nil value < 0 means "no limit" — distinct from nil/inherit, so a
// server can explicitly opt out of a cluster-wide cadence. See ResolveServer.
type ServerRow struct {
	Name            string
	IP              string
	Port            *int
	Cluster         *string
	AutoToken       *bool
	AcceptingConns  *bool
	RestartAfterHrs *float64
	StopAfterHrs    *float64
	DesiredState    string
	UpdatedAt       time.Time
}

// ClusterRow is the shared spec a cluster's members inherit. Port is structural
// (every member ingresses on it); AutoToken/AcceptingConns/RestartAfterHrs/
// StopAfterHrs are the inheritable override-tier defaults; LBPolicy is
// cluster-owned (how the proxy spreads new sessions across members).
type ClusterRow struct {
	Name            string
	Port            int
	AutoToken       bool
	AcceptingConns  bool
	RestartAfterHrs *float64
	StopAfterHrs    *float64
	LBPolicy        string
	UpdatedAt       time.Time
}

// Load-balancing policies a cluster can request. Only LBRoundRobin is honored by
// the proxy today; the rest are accepted and persisted for later.
const (
	LBRoundRobin = "round_robin"
	LBPacking    = "packing" // fill one backend before spilling to the next
	LBSparse     = "sparse"  // spread to minimize per-backend population
)

// EffectiveServer is a ServerRow with cluster inheritance applied: the override
// tier resolved most-specific-wins and the structural/cluster-owned fields filled
// in. The fleet reconciles against this instead of re-deriving precedence inline.
type EffectiveServer struct {
	Row        ServerRow // the raw server spec
	Port       uint16    // external port: the server's own, else its cluster's
	Standalone bool      // carries its own port (vs being a cluster member)
	AutoToken  bool
	Accepting  bool
	RestartHrs float64 // <= 0 means no limit
	StopHrs    float64 // <= 0 means no limit
	LBPolicy   string  // cluster's policy; "" when standalone
}

type ManifestRow struct {
	Name      string
	Manifest  string
	UpdatedAt time.Time
}

type ConfigFileRow struct {
	Name      string
	Filename  string
	Content   string
	UpdatedAt time.Time
}

// EnvVarRow is one row of csfleet_env_variables. scope is global|cluster|server and
// scope_name is ” for global, otherwise the cluster or server name.
type EnvVarRow struct {
	Key       string
	Value     string
	Scope     string
	ScopeName string
}

// ScopedPlugin is the set of plugins defined at one scope. Overridden
// distinguishes an explicit set: possibly an empty Items, meaning "run none".
type ScopedPlugin struct {
	Overridden bool
	Items      []string
}

// ScopedConfig is the set of configs defined at one scope. It carries the same
// inherit/override/run-none semantics as ScopedPlugin.
type ScopedConfig struct {
	Overridden bool
	Items      []string
}

// User is a web UI account. The bcrypt hash never leaves the DB layer.
type User struct {
	Username  string
	CreatedAt time.Time
}
