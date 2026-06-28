package database

import (
	"database/sql"
	"fmt"
)

// Plugins assigned to a scope (global/cluster/server). A server runs the set
// resolved most-specific-first (see EffectivePlugins)

// GlobalPlugins / ClusterPlugins / ServerPlugins return the plugin set defined at
// one scope. Overridden==false means the scope defines nothing and inherits its
// parent; an empty Items with Overridden==true means "run no plugins".

func (s *Store) GlobalPlugins() (ScopedPlugin, error) {
	return s.scopedPlugins("global", "")
}

func (s *Store) ClusterPlugins(cluster string) (ScopedPlugin, error) {
	return s.scopedPlugins("cluster", cluster)
}

func (s *Store) ServerPlugins(server string) (ScopedPlugin, error) {
	return s.scopedPlugins("server", server)
}

// SetGlobalPlugins / SetClusterPlugins / SetServerPlugins make a scope run exactly
// plugins. An empty slice is an explicit "run none" that stops inheritance.

func (s *Store) SetGlobalPlugins(plugins []string) error {
	return s.setScopedPlugins("global", "", plugins)
}

func (s *Store) SetClusterPlugins(cluster string, plugins []string) error {
	return s.setScopedPlugins("cluster", cluster, plugins)
}

func (s *Store) SetServerPlugins(server string, plugins []string) error {
	return s.setScopedPlugins("server", server, plugins)
}

// ClearClusterPlugins / ClearServerPlugins drop a scope's plugin set so it
// inherits from its parent again. Global is the root and has no clearer.

func (s *Store) ClearClusterPlugins(cluster string) error {
	return s.clearScopedPlugins("cluster", cluster)
}

func (s *Store) ClearServerPlugins(server string) error {
	return s.clearScopedPlugins("server", server)
}

// EffectivePlugins returns the plugin set a server actually runs: the set from
// the most specific scope that overrides, walking server -> cluster -> global. An
// overriding scope stops the walk even when its set is empty ("run none"). Pass
// "" for cluster when the server is standalone.
func (s *Store) EffectivePlugins(server, cluster string) ([]string, error) {
	if set, err := s.scopedPlugins("server", server); err != nil || set.Overridden {
		return set.Items, err
	}
	if cluster != "" {
		if set, err := s.scopedPlugins("cluster", cluster); err != nil || set.Overridden {
			return set.Items, err
		}
	}
	set, err := s.scopedPlugins("global", "")
	return set.Items, err
}

// scopedPlugins reads the plugin set stored at one scope. A marker row in
// csfleet_plugin_overrides means the scope overrides, so its csfleet_plugin_assignments are read;
// no marker means the scope inherits its parent.
func (s *Store) scopedPlugins(scope, scopeName string) (ScopedPlugin, error) {
	var marker int
	switch err := s.DB.QueryRow(
		"SELECT 1 FROM csfleet_plugin_overrides WHERE scope = ? AND scope_name = ?",
		scope, scopeName).Scan(&marker); err {
	case sql.ErrNoRows:
		return ScopedPlugin{}, nil // no marker: inherit
	case nil:
		// marker present: read the plugins below
	default:
		return ScopedPlugin{}, fmt.Errorf("read plugin override: %w", err)
	}

	rows, err := s.DB.Query(
		"SELECT plugin FROM csfleet_plugin_assignments WHERE scope = ? AND scope_name = ? ORDER BY plugin",
		scope, scopeName)
	if err != nil {
		return ScopedPlugin{}, fmt.Errorf("read plugins: %w", err)
	}
	defer rows.Close()

	plugins, err := scanStrings(rows)
	if err != nil {
		return ScopedPlugin{}, err
	}
	return ScopedPlugin{Overridden: true, Items: plugins}, nil
}

// setScopedPlugins makes a scope run exactly plugins (which may be empty): it sets
// the marker and replaces the plugin rows in one transaction.
func (s *Store) setScopedPlugins(scope, scopeName string, plugins []string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := setPluginsTx(tx, scope, scopeName, plugins); err != nil {
		return err
	}
	return tx.Commit()
}

// setPluginsTx sets the override marker and replaces the plugin assignment rows
// for a scope, within tx. Shared by setScopedPlugins and the CreateServer/
// CreateCluster transactions so a created resource's plugins land atomically.
func setPluginsTx(tx *sql.Tx, scope, scopeName string, plugins []string) error {
	if _, err := tx.Exec(
		"INSERT IGNORE INTO csfleet_plugin_overrides (scope, scope_name) VALUES (?, ?)",
		scope, scopeName); err != nil {
		return fmt.Errorf("mark plugin override: %w", err)
	}
	if _, err := tx.Exec(
		"DELETE FROM csfleet_plugin_assignments WHERE scope = ? AND scope_name = ?",
		scope, scopeName); err != nil {
		return fmt.Errorf("clear plugins: %w", err)
	}
	for _, p := range plugins {
		if _, err := tx.Exec(
			"INSERT INTO csfleet_plugin_assignments (plugin, scope, scope_name) VALUES (?, ?, ?)",
			p, scope, scopeName); err != nil {
			return fmt.Errorf("assign plugin %q: %w", p, err)
		}
	}
	return nil
}

// clearScopedPlugins drops a scope's plugin set so it inherits its parent again:
// it removes the marker and the plugin rows in one transaction.
func (s *Store) clearScopedPlugins(scope, scopeName string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		"DELETE FROM csfleet_plugin_overrides WHERE scope = ? AND scope_name = ?",
		scope, scopeName); err != nil {
		return fmt.Errorf("clear plugin override: %w", err)
	}
	if _, err := tx.Exec(
		"DELETE FROM csfleet_plugin_assignments WHERE scope = ? AND scope_name = ?",
		scope, scopeName); err != nil {
		return fmt.Errorf("clear plugins: %w", err)
	}
	return tx.Commit()
}
