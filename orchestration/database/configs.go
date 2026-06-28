package database

import (
	"database/sql"
	"fmt"
)

// Configs assigned to a scope (global/cluster/server). A server applies the set
// resolved most-specific-first (see EffectiveConfigs)

// GlobalConfigs / ClusterConfigs / ServerConfigs return the config set defined at
// one scope. Overridden==false means the scope defines nothing and inherits its
// parent; an empty Items with Overridden==true means "apply no configs".

func (s *Store) GlobalConfigs() (ScopedConfig, error) {
	return s.scopedConfigs("global", "")
}

func (s *Store) ClusterConfigs(cluster string) (ScopedConfig, error) {
	return s.scopedConfigs("cluster", cluster)
}

func (s *Store) ServerConfigs(server string) (ScopedConfig, error) {
	return s.scopedConfigs("server", server)
}

// SetGlobalConfigs / SetClusterConfigs / SetServerConfigs make a scope apply
// exactly configs. An empty slice is an explicit "apply none" that stops inheritance.

func (s *Store) SetGlobalConfigs(configs []string) error {
	return s.setScopedConfigs("global", "", configs)
}

func (s *Store) SetClusterConfigs(cluster string, configs []string) error {
	return s.setScopedConfigs("cluster", cluster, configs)
}

func (s *Store) SetServerConfigs(server string, configs []string) error {
	return s.setScopedConfigs("server", server, configs)
}

// ClearClusterConfigs / ClearServerConfigs drop a scope's config set so it
// inherits from its parent again. Global is the root and has no clearer.

func (s *Store) ClearClusterConfigs(cluster string) error {
	return s.clearScopedConfigs("cluster", cluster)
}

func (s *Store) ClearServerConfigs(server string) error {
	return s.clearScopedConfigs("server", server)
}

// EffectiveConfigs returns the config set a server actually applies: the set from
// the most specific scope that overrides, walking server -> cluster -> global. An
// overriding scope stops the walk even when its set is empty ("apply none"). Pass
// "" for cluster when the server is standalone.
func (s *Store) EffectiveConfigs(server, cluster string) ([]string, error) {
	if set, err := s.scopedConfigs("server", server); err != nil || set.Overridden {
		return set.Items, err
	}
	if cluster != "" {
		if set, err := s.scopedConfigs("cluster", cluster); err != nil || set.Overridden {
			return set.Items, err
		}
	}
	set, err := s.scopedConfigs("global", "")
	return set.Items, err
}

// scopedConfigs reads the config set stored at one scope. A marker row in
// csfleet_config_overrides means the scope overrides, so its csfleet_config_assignments are read;
// no marker means the scope inherits its parent.
func (s *Store) scopedConfigs(scope, scopeName string) (ScopedConfig, error) {
	var marker int
	switch err := s.DB.QueryRow(
		"SELECT 1 FROM csfleet_config_overrides WHERE scope = ? AND scope_name = ?",
		scope, scopeName).Scan(&marker); err {
	case sql.ErrNoRows:
		return ScopedConfig{}, nil // no marker: inherit
	case nil:
		// marker present: read the configs below
	default:
		return ScopedConfig{}, fmt.Errorf("read config override: %w", err)
	}

	rows, err := s.DB.Query(
		"SELECT config FROM csfleet_config_assignments WHERE scope = ? AND scope_name = ? ORDER BY config",
		scope, scopeName)
	if err != nil {
		return ScopedConfig{}, fmt.Errorf("read configs: %w", err)
	}
	defer rows.Close()

	configs, err := scanStrings(rows)
	if err != nil {
		return ScopedConfig{}, err
	}
	return ScopedConfig{Overridden: true, Items: configs}, nil
}

// setScopedConfigs makes a scope apply exactly configs (which may be empty): it
// sets the marker and replaces the config rows in one transaction.
func (s *Store) setScopedConfigs(scope, scopeName string, configs []string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := setConfigsTx(tx, scope, scopeName, configs); err != nil {
		return err
	}
	return tx.Commit()
}

// setConfigsTx sets the override marker and replaces the config assignment rows
// for a scope, within tx. Shared by setScopedConfigs and the CreateServer/
// CreateCluster transactions so a created resource's configs land atomically.
func setConfigsTx(tx *sql.Tx, scope, scopeName string, configs []string) error {
	if _, err := tx.Exec(
		"INSERT IGNORE INTO csfleet_config_overrides (scope, scope_name) VALUES (?, ?)",
		scope, scopeName); err != nil {
		return fmt.Errorf("mark config override: %w", err)
	}
	if _, err := tx.Exec(
		"DELETE FROM csfleet_config_assignments WHERE scope = ? AND scope_name = ?",
		scope, scopeName); err != nil {
		return fmt.Errorf("clear configs: %w", err)
	}
	for _, c := range configs {
		if _, err := tx.Exec(
			"INSERT INTO csfleet_config_assignments (config, scope, scope_name) VALUES (?, ?, ?)",
			c, scope, scopeName); err != nil {
			return fmt.Errorf("assign config %q: %w", c, err)
		}
	}
	return nil
}

// clearScopedConfigs drops a scope's config set so it inherits its parent again:
// it removes the marker and the config rows in one transaction.
func (s *Store) clearScopedConfigs(scope, scopeName string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		"DELETE FROM csfleet_config_overrides WHERE scope = ? AND scope_name = ?",
		scope, scopeName); err != nil {
		return fmt.Errorf("clear config override: %w", err)
	}
	if _, err := tx.Exec(
		"DELETE FROM csfleet_config_assignments WHERE scope = ? AND scope_name = ?",
		scope, scopeName); err != nil {
		return fmt.Errorf("clear configs: %w", err)
	}
	return tx.Commit()
}
