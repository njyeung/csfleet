package database

import (
	"database/sql"
	"fmt"
)

// --- Servers ---

func (s *Store) ListServers() ([]ServerRow, error) {
	rows, err := s.DB.Query(`SELECT name, ip, port, cluster,
		accepting_connections, restart_after_hrs, stop_after_hrs, desired_state, updated_at
		FROM csfleet_servers ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	defer rows.Close()

	var out []ServerRow
	for rows.Next() {
		var r ServerRow
		if err := rows.Scan(&r.Name, &r.IP, &r.Port, &r.Cluster,
			&r.AcceptingConns, &r.RestartAfterHrs, &r.StopAfterHrs, &r.DesiredState, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan server: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetServer(name string) (ServerRow, error) {
	var r ServerRow
	err := s.DB.QueryRow(`SELECT name, ip, port, cluster,
		accepting_connections, restart_after_hrs, stop_after_hrs, desired_state, updated_at
		FROM csfleet_servers WHERE name = ?`, name).Scan(
		&r.Name, &r.IP, &r.Port, &r.Cluster,
		&r.AcceptingConns, &r.RestartAfterHrs, &r.StopAfterHrs, &r.DesiredState, &r.UpdatedAt)
	if err != nil {
		return r, fmt.Errorf("get server %q: %w", name, err)
	}
	return r, nil
}

// CreateServer inserts a server row together with its create-time, immutable
// server-scope plugin/config/env assignments in one transaction, so a worker
// never observes the row before its overrides. A nil plugins/configs means
// "inherit" (no override marker written); a non-nil slice (even empty) is an
// explicit override. The duplicate-name PK violation aborts the whole tx.
func (s *Store) CreateServer(r ServerRow, plugins, configs *[]string, env map[string]string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO csfleet_servers
		(name, ip, port, cluster, accepting_connections,
		 restart_after_hrs, stop_after_hrs, desired_state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Name, r.IP, r.Port, r.Cluster, r.AcceptingConns,
		r.RestartAfterHrs, r.StopAfterHrs, r.DesiredState); err != nil {
		return fmt.Errorf("create server %q: %w", r.Name, err)
	}
	if plugins != nil {
		if err := setPluginsTx(tx, "server", r.Name, *plugins); err != nil {
			return err
		}
	}
	if configs != nil {
		if err := setConfigsTx(tx, "server", r.Name, *configs); err != nil {
			return err
		}
	}
	for k, v := range env {
		if err := setEnvTx(tx, k, v, "server", r.Name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpdateServer(name string, r ServerRow) error {
	_, err := s.DB.Exec(`UPDATE csfleet_servers SET
		ip = ?, port = ?, cluster = ?, accepting_connections = ?,
		restart_after_hrs = ?, stop_after_hrs = ?, desired_state = ?
		WHERE name = ?`,
		r.IP, r.Port, r.Cluster, r.AcceptingConns,
		r.RestartAfterHrs, r.StopAfterHrs, r.DesiredState,
		name)
	if err != nil {
		return fmt.Errorf("update server %q: %w", name, err)
	}
	return nil
}

// DeleteServer removes the server and any env vars / plugin / config
// assignments scoped to it (those have no FK to csfleet_servers, so clean them here).
func (s *Store) DeleteServer(name string) error {
	return s.deleteScopedServer(name)
}

func (s *Store) UpdateServerDesiredState(name, state string) error {
	_, err := s.DB.Exec("UPDATE csfleet_servers SET desired_state = ? WHERE name = ?", state, name)
	if err != nil {
		return fmt.Errorf("update desired_state for %q: %w", name, err)
	}
	return nil
}

// --- Clusters ---

func (s *Store) ListClusters() ([]ClusterRow, error) {
	rows, err := s.DB.Query(`SELECT name, port, accepting_connections,
		restart_after_hrs, stop_after_hrs, lb_policy, updated_at FROM csfleet_clusters ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	defer rows.Close()

	var out []ClusterRow
	for rows.Next() {
		var r ClusterRow
		if err := rows.Scan(&r.Name, &r.Port, &r.AcceptingConns,
			&r.RestartAfterHrs, &r.StopAfterHrs, &r.LBPolicy, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetCluster(name string) (ClusterRow, error) {
	var r ClusterRow
	err := s.DB.QueryRow(`SELECT name, port, accepting_connections,
		restart_after_hrs, stop_after_hrs, lb_policy, updated_at FROM csfleet_clusters WHERE name = ?`, name).
		Scan(&r.Name, &r.Port, &r.AcceptingConns,
			&r.RestartAfterHrs, &r.StopAfterHrs, &r.LBPolicy, &r.UpdatedAt)
	if err != nil {
		return r, fmt.Errorf("get cluster %q: %w", name, err)
	}
	return r, nil
}

// CreateCluster inserts a cluster row together with its create-time, immutable
// cluster-scope plugin/config/env assignments in one transaction. Mirrors
// CreateServer; see its doc for the nil-vs-empty plugins/configs semantics.
func (s *Store) CreateCluster(r ClusterRow, plugins, configs *[]string, env map[string]string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO csfleet_clusters
		(name, port, accepting_connections, restart_after_hrs, stop_after_hrs, lb_policy)
		VALUES (?, ?, ?, ?, ?, ?)`,
		r.Name, r.Port, r.AcceptingConns, r.RestartAfterHrs, r.StopAfterHrs, r.LBPolicy); err != nil {
		return fmt.Errorf("create cluster %q: %w", r.Name, err)
	}
	if plugins != nil {
		if err := setPluginsTx(tx, "cluster", r.Name, *plugins); err != nil {
			return err
		}
	}
	if configs != nil {
		if err := setConfigsTx(tx, "cluster", r.Name, *configs); err != nil {
			return err
		}
	}
	for k, v := range env {
		if err := setEnvTx(tx, k, v, "cluster", r.Name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpdateCluster(name string, r ClusterRow) error {
	_, err := s.DB.Exec(`UPDATE csfleet_clusters SET
		port = ?, accepting_connections = ?,
		restart_after_hrs = ?, stop_after_hrs = ?, lb_policy = ?
		WHERE name = ?`,
		r.Port, r.AcceptingConns, r.RestartAfterHrs, r.StopAfterHrs, r.LBPolicy, name)
	if err != nil {
		return fmt.Errorf("update cluster %q: %w", name, err)
	}
	return nil
}

// DeleteCluster removes the cluster and any cluster-scoped env vars / plugin /
// config assignments. (csfleet_servers.cluster FK blocks deletion while members exist.)
func (s *Store) DeleteCluster(name string) error {
	return s.deleteScopedCluster(name)
}

// deleteScopedServer deletes a server row plus the env/plugin/config
// assignments scoped to it, in one transaction.
func (s *Store) deleteScopedServer(name string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmts := []string{
		"DELETE FROM csfleet_servers WHERE name = ?",
		"DELETE FROM csfleet_env_variables WHERE scope = 'server' AND scope_name = ?",
		"DELETE FROM csfleet_plugin_assignments WHERE scope = 'server' AND scope_name = ?",
		"DELETE FROM csfleet_plugin_overrides WHERE scope = 'server' AND scope_name = ?",
		"DELETE FROM csfleet_config_assignments WHERE scope = 'server' AND scope_name = ?",
		"DELETE FROM csfleet_config_overrides WHERE scope = 'server' AND scope_name = ?",
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q, name); err != nil {
			return fmt.Errorf("delete server %q: %w", name, err)
		}
	}
	return tx.Commit()
}

// deleteScopedCluster deletes a cluster row plus the env/plugin/config
// assignments scoped to it, in one transaction.
func (s *Store) deleteScopedCluster(name string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmts := []string{
		"DELETE FROM csfleet_clusters WHERE name = ?",
		"DELETE FROM csfleet_env_variables WHERE scope = 'cluster' AND scope_name = ?",
		"DELETE FROM csfleet_plugin_assignments WHERE scope = 'cluster' AND scope_name = ?",
		"DELETE FROM csfleet_plugin_overrides WHERE scope = 'cluster' AND scope_name = ?",
		"DELETE FROM csfleet_config_assignments WHERE scope = 'cluster' AND scope_name = ?",
		"DELETE FROM csfleet_config_overrides WHERE scope = 'cluster' AND scope_name = ?",
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q, name); err != nil {
			return fmt.Errorf("delete cluster %q: %w", name, err)
		}
	}
	return tx.Commit()
}

// --- Plugin manifests ---

func (s *Store) ListManifests() ([]ManifestRow, error) {
	rows, err := s.DB.Query("SELECT name, manifest, updated_at FROM csfleet_plugin_manifests ORDER BY name")
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
	_, err := s.DB.Exec(`INSERT INTO csfleet_plugin_manifests (name, manifest) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE manifest = VALUES(manifest)`, name, manifest)
	if err != nil {
		return fmt.Errorf("upsert manifest %q: %w", name, err)
	}
	return nil
}

func (s *Store) DeleteManifest(name string) error {
	_, err := s.DB.Exec("DELETE FROM csfleet_plugin_manifests WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete manifest %q: %w", name, err)
	}
	return nil
}

// --- Config files ---

func (s *Store) ListConfigFiles() ([]ConfigFileRow, error) {
	rows, err := s.DB.Query("SELECT name, filename, content, updated_at FROM csfleet_config_files ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list config files: %w", err)
	}
	defer rows.Close()

	var out []ConfigFileRow
	for rows.Next() {
		var r ConfigFileRow
		if err := rows.Scan(&r.Name, &r.Filename, &r.Content, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan config file: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetConfigFile(name string) (ConfigFileRow, error) {
	var r ConfigFileRow
	err := s.DB.QueryRow("SELECT name, filename, content, updated_at FROM csfleet_config_files WHERE name = ?", name).
		Scan(&r.Name, &r.Filename, &r.Content, &r.UpdatedAt)
	if err != nil {
		return r, fmt.Errorf("get config file %q: %w", name, err)
	}
	return r, nil
}

func (s *Store) UpsertConfigFile(name, filename, content string) error {
	_, err := s.DB.Exec(`INSERT INTO csfleet_config_files (name, filename, content) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE filename = VALUES(filename), content = VALUES(content)`, name, filename, content)
	if err != nil {
		return fmt.Errorf("upsert config file %q: %w", name, err)
	}
	return nil
}

func (s *Store) DeleteConfigFile(name string) error {
	_, err := s.DB.Exec("DELETE FROM csfleet_config_files WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete config file %q: %w", name, err)
	}
	return nil
}

// --- Environment variables (scoped global|cluster|server) ---

func (s *Store) ListEnvVars(scope, scopeName string) ([]EnvVarRow, error) {
	rows, err := s.DB.Query(
		"SELECT `key`, value, scope, scope_name FROM csfleet_env_variables "+
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
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := setEnvTx(tx, key, value, scope, scopeName); err != nil {
		return err
	}
	return tx.Commit()
}

// setEnvTx upserts one env var within a transaction. Shared by SetEnvVar and the
// CreateServer/CreateCluster transactions so a created resource's scope env lands
// atomically with its row.
func setEnvTx(tx *sql.Tx, key, value, scope, scopeName string) error {
	_, err := tx.Exec(
		"INSERT INTO csfleet_env_variables (`key`, value, scope, scope_name) VALUES (?, ?, ?, ?) "+
			"ON DUPLICATE KEY UPDATE value = VALUES(value)",
		key, value, scope, scopeName)
	if err != nil {
		return fmt.Errorf("set env var %q: %w", key, err)
	}
	return nil
}

func (s *Store) DeleteEnvVar(key, scope, scopeName string) error {
	_, err := s.DB.Exec(
		"DELETE FROM csfleet_env_variables WHERE `key` = ? AND scope = ? AND scope_name = ?",
		key, scope, scopeName)
	if err != nil {
		return fmt.Errorf("delete env var %q: %w", key, err)
	}
	return nil
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
