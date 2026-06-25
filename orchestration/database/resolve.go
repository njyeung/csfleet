package database

import "fmt"

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
	eff.AutoToken = resolveBool(row.AutoToken, clustered, cl.AutoToken, true)
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
		"SELECT `key`, value FROM env_variables "+
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
