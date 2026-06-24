package database

import "fmt"

// ResolveServer collapses a server row and its cluster (if any) into the
// EffectiveServer the fleet runs against. Three inheritance models meet here:
//
//   - structural: the external port is the server's own when standalone, else the
//     cluster's. The server_reachable CHECK guarantees exactly one is set.
//   - override: auto_token / accepting / restart / stop resolve most-specific-wins
//     — the server's value if set, else the cluster's, else a built-in default.
//     For the hour fields a stored value < 0 means "no limit", distinct from NULL
//     ("inherit"): that's what lets a server opt out of a cluster-wide cadence.
//   - cluster-owned: lb_policy comes straight from the cluster (meaningless for a
//     standalone server, left "").
//
// The list-valued settings — env vars (LoadEnv) and plugin/config assignments
// (EffectivePlugins / EffectiveConfigs) — resolve the same most-specific-wins way
// and live just below; the raw per-scope reads/writes they build on stay in crud.go.
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

// LoadEnv returns the env_variables visible to a server: globals overlaid with
// the server's cluster scope, then the server's own scope (most specific key
// wins). Pass "" for cluster when the server is standalone.
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

// --- Plugin/config assignments (most specific scope wins, whole-list) ---
//
// EffectivePlugins / EffectiveConfigs return the assignment list from the most
// specific scope that defines one — the server's own, else its cluster's, else
// global — as a whole-list replacement rather than a union, so the model matches
// every other setting: the most specific definition wins wholesale. Trade-off: an
// empty list at a scope reads as "inherit", not "explicitly none", and tweaking a
// single entry at the server scope means redeclaring the whole list.
//
// The per-scope reads/writes (PluginsFor / SetPlugins / ...) and the shared
// assignmentsFor / setAssignments / scanStrings helpers stay in crud.go.
func (s *Store) EffectivePlugins(server, cluster string) ([]string, error) {
	return s.effectiveAssignments("plugin", "plugin_assignments", server, cluster)
}

func (s *Store) EffectiveConfigs(server, cluster string) ([]string, error) {
	return s.effectiveAssignments("config", "config_assignments", server, cluster)
}

func (s *Store) effectiveAssignments(col, table, server, cluster string) ([]string, error) {
	if items, err := s.assignmentsFor(col, table, "server", server); err != nil || len(items) > 0 {
		return items, err
	}
	if cluster != "" {
		if items, err := s.assignmentsFor(col, table, "cluster", cluster); err != nil || len(items) > 0 {
			return items, err
		}
	}
	return s.assignmentsFor(col, table, "global", "")
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
