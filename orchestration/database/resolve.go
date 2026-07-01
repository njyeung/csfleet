package database

import (
	"fmt"
	"sort"
)

// ResolveServer collapses a server row and its cluster into the
// EffectiveServer the fleet runs against.
func (s *Store) ResolveServer(name string) (EffectiveServer, error) {
	row, err := s.GetServer(name)
	if err != nil {
		return EffectiveServer{}, err // wraps sql.ErrNoRows for the caller's reap path
	}

	var (
		cl        ClusterRow
		clustered = row.Cluster != nil
	)
	if clustered {
		if cl, err = s.GetCluster(*row.Cluster); err != nil {
			return EffectiveServer{}, err
		}
	}

	eff := EffectiveServer{Row: row}

	// Structural: own port wins; a member takes the cluster's port and lb policy.
	switch {
	case row.Port != nil:
		eff.Port, eff.Standalone = uint16(*row.Port), true
	case clustered:
		eff.Port, eff.LBPolicy = uint16(cl.Port), cl.LBPolicy
	default:
		return EffectiveServer{}, fmt.Errorf("resolve %q: neither port nor cluster set", name)
	}

	// Override tier: server value wins, else cluster's, else the built-in default.
	eff.Accepting = resolveBool(row.AcceptingConns, clustered, cl.AcceptingConns, true)
	eff.RestartHrs = resolveHrs(row.RestartAfterHrs, cl.RestartAfterHrs)
	eff.StopHrs = resolveHrs(row.StopAfterHrs, cl.StopAfterHrs)
	return eff, nil
}

// --- Env overlay (global < cluster < server; most specific key wins) ---
//
// The db.* keys seeded by seedDefaults are ordinary env vars injected into the container.
func (s *Store) LoadEnv(server, cluster string) (map[string]string, error) {
	rows, err := s.DB.Query(
		"SELECT `key`, value FROM csfleet_env_variables "+
			"WHERE scope = 'global' "+
			"OR (scope = 'cluster' AND scope_name = ?) "+
			"OR (scope = 'server' AND scope_name = ?) "+
			"ORDER BY FIELD(scope, 'global', 'cluster', 'server')",
		cluster, server,
	)
	if err != nil {
		return nil, fmt.Errorf("load env: %w", err)
	}
	defer rows.Close()

	env := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("load env: %w", err)
		}
		env[k] = v // later (more specific) scope overwrites
	}
	return env, rows.Err()
}

// EffectiveEnv returns the env a server actually runs as one row per key, keeping
// the scope that won each key (global < cluster < server, most specific wins).
// Unlike LoadEnv, which collapses to a map for container injection, this preserves
// the winning scope so a UI can show where each value came from. Rows are sorted
// by key for stable output.
func (s *Store) EffectiveEnv(server, cluster string) ([]EnvVarRow, error) {
	rows, err := s.DB.Query(
		"SELECT `key`, value, scope, scope_name FROM csfleet_env_variables "+
			"WHERE scope = 'global' "+
			"OR (scope = 'cluster' AND scope_name = ?) "+
			"OR (scope = 'server' AND scope_name = ?) "+
			"ORDER BY FIELD(scope, 'global', 'cluster', 'server')",
		cluster, server,
	)
	if err != nil {
		return nil, fmt.Errorf("effective env: %w", err)
	}
	defer rows.Close()

	winning := map[string]EnvVarRow{}
	for rows.Next() {
		var r EnvVarRow
		if err := rows.Scan(&r.Key, &r.Value, &r.Scope, &r.ScopeName); err != nil {
			return nil, fmt.Errorf("effective env: %w", err)
		}
		winning[r.Key] = r // later (more specific) scope overwrites
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]EnvVarRow, 0, len(winning))
	for _, r := range winning {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// resolveBool applies the override tier to a tri-state boolean: the server's own
// value wins, else the cluster's (when the server is in one), else def.
func resolveBool(server *bool, clustered, cluster, def bool) bool {
	switch {
	case server != nil:
		return *server
	case clustered:
		return cluster
	default:
		return def
	}
}

// resolveHrs applies the override tier to a lifecycle hour field. nil means
// inherit, so the server's value wins, else the cluster's; with neither set it
// falls back to -1 ("no limit"). The <0 == no-limit convention is preserved, so
// callers treat anything <= 0 as "never". cluster is the zero value's nil when the
// server is standalone, which collapses to the -1 fallback as intended.
func resolveHrs(server, cluster *float64) float64 {
	v := server
	if v == nil {
		v = cluster
	}
	if v == nil {
		return -1
	}
	return *v
}
