package database

import (
	"database/sql"
	"fmt"
)

// --- Servers ---

func (s *Store) ListServers() ([]ServerRow, error) {
	rows, err := s.DB.Query(`SELECT name, ip, port, cluster, auto_token,
		accepting_connections, restart_after_hrs, stop_after_hrs, desired_state, updated_at
		FROM servers ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	defer rows.Close()

	var out []ServerRow
	for rows.Next() {
		var r ServerRow
		if err := rows.Scan(&r.Name, &r.IP, &r.Port, &r.Cluster, &r.AutoToken,
			&r.AcceptingConns, &r.RestartAfterHrs, &r.StopAfterHrs, &r.DesiredState, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan server: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetServer(name string) (ServerRow, error) {
	var r ServerRow
	err := s.DB.QueryRow(`SELECT name, ip, port, cluster, auto_token,
		accepting_connections, restart_after_hrs, stop_after_hrs, desired_state, updated_at
		FROM servers WHERE name = ?`, name).Scan(
		&r.Name, &r.IP, &r.Port, &r.Cluster, &r.AutoToken,
		&r.AcceptingConns, &r.RestartAfterHrs, &r.StopAfterHrs, &r.DesiredState, &r.UpdatedAt)
	if err != nil {
		return r, fmt.Errorf("get server %q: %w", name, err)
	}
	return r, nil
}

func (s *Store) CreateServer(r ServerRow) error {
	_, err := s.DB.Exec(`INSERT INTO servers
		(name, ip, port, cluster, auto_token, accepting_connections,
		 restart_after_hrs, stop_after_hrs, desired_state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Name, r.IP, r.Port, r.Cluster, r.AutoToken, r.AcceptingConns,
		r.RestartAfterHrs, r.StopAfterHrs, r.DesiredState)
	if err != nil {
		return fmt.Errorf("create server %q: %w", r.Name, err)
	}
	return nil
}

func (s *Store) UpdateServer(name string, r ServerRow) error {
	_, err := s.DB.Exec(`UPDATE servers SET
		ip = ?, port = ?, cluster = ?, auto_token = ?, accepting_connections = ?,
		restart_after_hrs = ?, stop_after_hrs = ?, desired_state = ?
		WHERE name = ?`,
		r.IP, r.Port, r.Cluster, r.AutoToken, r.AcceptingConns,
		r.RestartAfterHrs, r.StopAfterHrs, r.DesiredState,
		name)
	if err != nil {
		return fmt.Errorf("update server %q: %w", name, err)
	}
	return nil
}

// DeleteServer removes the server and any env vars / plugin / config
// assignments scoped to it (those have no FK to servers, so clean them here).
func (s *Store) DeleteServer(name string) error {
	return s.deleteScoped("servers", "server", name)
}

func (s *Store) UpdateServerDesiredState(name, state string) error {
	_, err := s.DB.Exec("UPDATE servers SET desired_state = ? WHERE name = ?", state, name)
	if err != nil {
		return fmt.Errorf("update desired_state for %q: %w", name, err)
	}
	return nil
}

// --- Clusters ---

func (s *Store) ListClusters() ([]ClusterRow, error) {
	rows, err := s.DB.Query(`SELECT name, port, auto_token, accepting_connections,
		restart_after_hrs, stop_after_hrs, lb_policy, updated_at FROM clusters ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	defer rows.Close()

	var out []ClusterRow
	for rows.Next() {
		var r ClusterRow
		if err := rows.Scan(&r.Name, &r.Port, &r.AutoToken, &r.AcceptingConns,
			&r.RestartAfterHrs, &r.StopAfterHrs, &r.LBPolicy, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetCluster(name string) (ClusterRow, error) {
	var r ClusterRow
	err := s.DB.QueryRow(`SELECT name, port, auto_token, accepting_connections,
		restart_after_hrs, stop_after_hrs, lb_policy, updated_at FROM clusters WHERE name = ?`, name).
		Scan(&r.Name, &r.Port, &r.AutoToken, &r.AcceptingConns,
			&r.RestartAfterHrs, &r.StopAfterHrs, &r.LBPolicy, &r.UpdatedAt)
	if err != nil {
		return r, fmt.Errorf("get cluster %q: %w", name, err)
	}
	return r, nil
}

func (s *Store) CreateCluster(r ClusterRow) error {
	_, err := s.DB.Exec(`INSERT INTO clusters
		(name, port, auto_token, accepting_connections, restart_after_hrs, stop_after_hrs, lb_policy)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.Name, r.Port, r.AutoToken, r.AcceptingConns, r.RestartAfterHrs, r.StopAfterHrs, r.LBPolicy)
	if err != nil {
		return fmt.Errorf("create cluster %q: %w", r.Name, err)
	}
	return nil
}

func (s *Store) UpdateCluster(name string, r ClusterRow) error {
	_, err := s.DB.Exec(`UPDATE clusters SET
		port = ?, auto_token = ?, accepting_connections = ?,
		restart_after_hrs = ?, stop_after_hrs = ?, lb_policy = ?
		WHERE name = ?`,
		r.Port, r.AutoToken, r.AcceptingConns, r.RestartAfterHrs, r.StopAfterHrs, r.LBPolicy, name)
	if err != nil {
		return fmt.Errorf("update cluster %q: %w", name, err)
	}
	return nil
}

// DeleteCluster removes the cluster and any cluster-scoped env vars / plugin /
// config assignments. (servers.cluster FK blocks deletion while members exist.)
func (s *Store) DeleteCluster(name string) error {
	return s.deleteScoped("clusters", "cluster", name)
}

// deleteScoped deletes a server or cluster row plus the env/plugin/config
// assignments scoped to it, in one transaction.
func (s *Store) deleteScoped(table, scope, name string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmts := []string{
		"DELETE FROM " + table + " WHERE name = ?",
		"DELETE FROM env_variables WHERE scope = '" + scope + "' AND scope_name = ?",
		"DELETE FROM plugin_assignments WHERE scope = '" + scope + "' AND scope_name = ?",
		"DELETE FROM config_assignments WHERE scope = '" + scope + "' AND scope_name = ?",
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q, name); err != nil {
			return fmt.Errorf("delete %s %q: %w", scope, name, err)
		}
	}
	return tx.Commit()
}

// --- Plugin manifests ---

func (s *Store) ListManifests() ([]ManifestRow, error) {
	rows, err := s.DB.Query("SELECT name, manifest, updated_at FROM plugin_manifests ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list manifests: %w", err)
	}
	defer rows.Close()

	var out []ManifestRow
	for rows.Next() {
		var r ManifestRow
		if err := rows.Scan(&r.Name, &r.Manifest, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan manifest: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpsertManifest(name, manifest string) error {
	_, err := s.DB.Exec(`INSERT INTO plugin_manifests (name, manifest) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE manifest = VALUES(manifest)`, name, manifest)
	if err != nil {
		return fmt.Errorf("upsert manifest %q: %w", name, err)
	}
	return nil
}

func (s *Store) DeleteManifest(name string) error {
	_, err := s.DB.Exec("DELETE FROM plugin_manifests WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete manifest %q: %w", name, err)
	}
	return nil
}

// --- Config files ---

func (s *Store) ListConfigFiles() ([]ConfigFileRow, error) {
	rows, err := s.DB.Query("SELECT name, content, updated_at FROM config_files ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list config files: %w", err)
	}
	defer rows.Close()

	var out []ConfigFileRow
	for rows.Next() {
		var r ConfigFileRow
		if err := rows.Scan(&r.Name, &r.Content, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan config file: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetConfigFile(name string) (ConfigFileRow, error) {
	var r ConfigFileRow
	err := s.DB.QueryRow("SELECT name, content, updated_at FROM config_files WHERE name = ?", name).
		Scan(&r.Name, &r.Content, &r.UpdatedAt)
	if err != nil {
		return r, fmt.Errorf("get config file %q: %w", name, err)
	}
	return r, nil
}

func (s *Store) UpsertConfigFile(name, content string) error {
	_, err := s.DB.Exec(`INSERT INTO config_files (name, content) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE content = VALUES(content)`, name, content)
	if err != nil {
		return fmt.Errorf("upsert config file %q: %w", name, err)
	}
	return nil
}

func (s *Store) DeleteConfigFile(name string) error {
	_, err := s.DB.Exec("DELETE FROM config_files WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete config file %q: %w", name, err)
	}
	return nil
}

// --- GSLT tokens (the claim pool) ---

func (s *Store) ListGSLTTokens() ([]string, error) {
	rows, err := s.DB.Query("SELECT token FROM gslt_tokens ORDER BY token")
	if err != nil {
		return nil, fmt.Errorf("list gslt tokens: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scan gslt token: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) AddGSLTToken(token string) error {
	_, err := s.DB.Exec("INSERT IGNORE INTO gslt_tokens (token) VALUES (?)", token)
	if err != nil {
		return fmt.Errorf("add gslt token: %w", err)
	}
	return nil
}

func (s *Store) DeleteGSLTToken(token string) error {
	_, err := s.DB.Exec("DELETE FROM gslt_tokens WHERE token = ?", token)
	if err != nil {
		return fmt.Errorf("delete gslt token: %w", err)
	}
	return nil
}

// --- Environment variables (scoped global|cluster|server) ---

func (s *Store) ListEnvVars(scope, scopeName string) ([]EnvVarRow, error) {
	rows, err := s.DB.Query(
		"SELECT `key`, value, scope, scope_name FROM env_variables "+
			"WHERE scope = ? AND scope_name = ? ORDER BY `key`", scope, scopeName)
	if err != nil {
		return nil, fmt.Errorf("list env vars: %w", err)
	}
	defer rows.Close()

	var out []EnvVarRow
	for rows.Next() {
		var r EnvVarRow
		if err := rows.Scan(&r.Key, &r.Value, &r.Scope, &r.ScopeName); err != nil {
			return nil, fmt.Errorf("scan env var: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) SetEnvVar(key, value, scope, scopeName string) error {
	_, err := s.DB.Exec(
		"INSERT INTO env_variables (`key`, value, scope, scope_name) VALUES (?, ?, ?, ?) "+
			"ON DUPLICATE KEY UPDATE value = VALUES(value)",
		key, value, scope, scopeName)
	if err != nil {
		return fmt.Errorf("set env var %q: %w", key, err)
	}
	return nil
}

func (s *Store) DeleteEnvVar(key, scope, scopeName string) error {
	_, err := s.DB.Exec(
		"DELETE FROM env_variables WHERE `key` = ? AND scope = ? AND scope_name = ?",
		key, scope, scopeName)
	if err != nil {
		return fmt.Errorf("delete env var %q: %w", key, err)
	}
	return nil
}

// --- Plugin assignments (scoped global|cluster|server) ---

func (s *Store) PluginsFor(scope, scopeName string) ([]string, error) {
	return s.assignmentsFor("plugin", "plugin_assignments", scope, scopeName)
}

func (s *Store) SetPlugins(scope, scopeName string, plugins []string) error {
	return s.setAssignments("plugin", "plugin_assignments", scope, scopeName, plugins)
}

// --- Config assignments (scoped global|cluster|server) ---

func (s *Store) ConfigsFor(scope, scopeName string) ([]string, error) {
	return s.assignmentsFor("config", "config_assignments", scope, scopeName)
}

func (s *Store) SetConfigs(scope, scopeName string, configs []string) error {
	return s.setAssignments("config", "config_assignments", scope, scopeName, configs)
}

// --- Shared assignment helpers ---

func (s *Store) assignmentsFor(col, table, scope, scopeName string) ([]string, error) {
	rows, err := s.DB.Query(
		"SELECT "+col+" FROM "+table+" WHERE scope = ? AND scope_name = ? ORDER BY "+col,
		scope, scopeName)
	if err != nil {
		return nil, fmt.Errorf("%s assignments: %w", col, err)
	}
	defer rows.Close()
	return scanStrings(rows)
}

func (s *Store) setAssignments(col, table, scope, scopeName string, items []string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		"DELETE FROM "+table+" WHERE scope = ? AND scope_name = ?", scope, scopeName); err != nil {
		return fmt.Errorf("clear %s assignments: %w", col, err)
	}
	for _, it := range items {
		if _, err := tx.Exec(
			"INSERT INTO "+table+" ("+col+", scope, scope_name) VALUES (?, ?, ?)",
			it, scope, scopeName); err != nil {
			return fmt.Errorf("insert %s %q: %w", col, it, err)
		}
	}
	return tx.Commit()
}

// scanStrings collects a single-column string result into a slice.
func scanStrings(rows *sql.Rows) ([]string, error) {
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
