package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	_ "github.com/go-sql-driver/mysql"
)

type Config struct {
	Image     string // MariaDB Docker image
	Container string // container name
	Network   string // user-defined bridge the container attaches to
	IP        string // static address on that bridge
	Host      string // host the orchestrator dials (maria's bridge IP)
	Name      string // database name
	User      string
	Pass      string
	RootPass  string
	Port      int // MariaDB port, default 3306
}

type Store struct {
	DB   *sql.DB
	cfg  Config
	cli  *client.Client
	once sync.Once
}

func Start(ctx context.Context, cli *client.Client, cfg Config) (*Store, error) {
	if cfg.Port == 0 {
		cfg.Port = 3306
	}

	info, err := cli.ContainerInspect(ctx, cfg.Container)
	if err == nil && info.State.Running {
		log.Printf("[database] %s already running", cfg.Container)
	} else {
		cli.ContainerRemove(ctx, cfg.Container, container.RemoveOptions{Force: true})

		log.Printf("[database] pulling %s", cfg.Image)
		if err := pullImage(ctx, cli, cfg.Image); err != nil {
			return nil, fmt.Errorf("pull %s: %w", cfg.Image, err)
		}

		log.Printf("[database] starting %s", cfg.Container)
		resp, err := cli.ContainerCreate(ctx, &container.Config{
			Image: cfg.Image,
			Env: []string{
				"MARIADB_DATABASE=" + cfg.Name,
				"MARIADB_USER=" + cfg.User,
				"MARIADB_PASSWORD=" + cfg.Pass,
				"MARIADB_ROOT_PASSWORD=" + cfg.RootPass,
			},
		}, &container.HostConfig{
			NetworkMode: container.NetworkMode(cfg.Network),
		}, &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				cfg.Network: {IPAMConfig: &network.EndpointIPAMConfig{IPv4Address: cfg.IP}},
			},
		}, nil, cfg.Container)
		if err != nil {
			return nil, fmt.Errorf("create %s: %w", cfg.Container, err)
		}
		if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
			return nil, fmt.Errorf("start %s: %w", cfg.Container, err)
		}
	}

	log.Println("[database] waiting for mariadb to be ready")
	if err := waitReady(ctx, cli, cfg); err != nil {
		return nil, err
	}
	log.Println("[database] mariadb ready")

	db, err := openDB(cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	s := &Store{DB: db, cfg: cfg, cli: cli}

	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := s.seedDefaults(); err != nil {
		db.Close()
		return nil, fmt.Errorf("seed: %w", err)
	}

	log.Println("[database] ready")
	return s, nil
}

func (s *Store) Close(ctx context.Context) {
	s.once.Do(func() {
		log.Println("[database] shutting down")
		if s.DB != nil {
			s.DB.Close()
		}
		s.cli.ContainerStop(ctx, s.cfg.Container, container.StopOptions{})
		s.cli.ContainerRemove(ctx, s.cfg.Container, container.RemoveOptions{})
	})
}

func pullImage(ctx context.Context, cli *client.Client, ref string) error {
	r, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer r.Close()
	dec := json.NewDecoder(r)
	for dec.More() {
		var msg map[string]any
		if err := dec.Decode(&msg); err != nil {
			break
		}
		if status, ok := msg["status"].(string); ok {
			if id, ok := msg["id"].(string); ok {
				fmt.Printf("%s: %s\n", id, status)
			} else {
				fmt.Println(status)
			}
		}
	}
	return nil
}

func waitReady(ctx context.Context, cli *client.Client, cfg Config) error {
	for range 30 {
		execCfg, err := cli.ContainerExecCreate(ctx, cfg.Container, container.ExecOptions{
			Cmd:          []string{"mariadb", "-u", cfg.User, "-p" + cfg.Pass, "-e", "SELECT 1"},
			AttachStdout: true,
		})
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		attach, err := cli.ContainerExecAttach(ctx, execCfg.ID, container.ExecAttachOptions{})
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		io.Copy(io.Discard, attach.Reader)
		attach.Close()

		inspect, err := cli.ContainerExecInspect(ctx, execCfg.ID)
		if err == nil && inspect.ExitCode == 0 {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("mariadb did not become ready in 30s")
}

func openDB(cfg Config) (*sql.DB, error) {
	// Bootstrap the database on a throwaway connection that isn't scoped to any
	// schema. We can't put cfg.Name in the DSN yet because it may not exist.
	bootDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/?parseTime=true&multiStatements=true",
		cfg.User, cfg.Pass, cfg.Host, cfg.Port)
	boot, err := sql.Open("mysql", bootDSN)
	if err != nil {
		return nil, err
	}
	if _, err := boot.Exec("CREATE DATABASE IF NOT EXISTS " + cfg.Name); err != nil {
		boot.Close()
		return nil, fmt.Errorf("create database: %w", err)
	}
	boot.Close()

	// Open the real pool with the database in the DSN so every pooled connection
	// is scoped to it. A per-connection "USE" wouldn't work: database/sql may
	// hand later queries a different connection that never ran it.
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&multiStatements=true",
		cfg.User, cfg.Pass, cfg.Host, cfg.Port, cfg.Name)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

func (s *Store) migrate() error {
	_, err := s.DB.Exec(`
		CREATE TABLE IF NOT EXISTS csfleet_plugin_manifests (
			name       VARCHAR(255) NOT NULL PRIMARY KEY,
			manifest   MEDIUMTEXT   NOT NULL,
			updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		);

		-- name is the catalog identifier (the PK assignments reference); filename is
		-- the file's path under game/csgo/cfg/, usually a bare name
		-- (e.g. gamemode_competitive_server.cfg).

		CREATE TABLE IF NOT EXISTS csfleet_config_files (
			name       VARCHAR(255) NOT NULL PRIMARY KEY,
			filename   VARCHAR(255) NOT NULL,
			content    MEDIUMTEXT   NOT NULL,
			updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS csfleet_gslt_tokens (
			token      VARCHAR(255) NOT NULL PRIMARY KEY,
			updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		);

		-- A cluster is the shared spec its members inherit. port is structural (the
		-- single external ingress every member sits behind); auto_token,
		-- accepting_connections and restart/stop are the inheritable override-tier
		-- defaults; lb_policy is how the proxy spreads new sessions across members.
		CREATE TABLE IF NOT EXISTS csfleet_clusters (
			name                  VARCHAR(255) NOT NULL PRIMARY KEY,
			port                  INT          NOT NULL UNIQUE,
			auto_token            BOOLEAN      NOT NULL DEFAULT TRUE,
			accepting_connections BOOLEAN      NOT NULL DEFAULT TRUE,
			restart_after_hrs     FLOAT        DEFAULT NULL,
			stop_after_hrs        FLOAT        DEFAULT NULL,
			lb_policy             VARCHAR(32)  NOT NULL DEFAULT 'round_robin',
			updated_at            TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		);

		-- ip: static bridge address, the proxy's DNAT target
		-- port: external host port clients reach this server on when standalone
		--
		-- A member inherits its cluster on the override tier: auto_token,
		-- accepting_connections (drain), restart_after_hrs, stop_after_hrs. NULL on
		-- the server means "inherit the cluster's value"; a non-NULL overrides it.
		-- For the hour fields a value < 0 means "no limit" (distinct from NULL), so a
		-- server can opt out of a cluster cadence. The hour fields default to -1 (no
		-- limit) so a fresh server never silently inherits a restart/stop.
		CREATE TABLE IF NOT EXISTS csfleet_servers (
			name                  VARCHAR(255) NOT NULL PRIMARY KEY,
			ip                    VARCHAR(45)  NOT NULL UNIQUE,
			port                  INT          UNIQUE,
			cluster               VARCHAR(255) DEFAULT NULL,
			auto_token            BOOLEAN      DEFAULT NULL,
			accepting_connections BOOLEAN      DEFAULT NULL,
			restart_after_hrs     FLOAT        DEFAULT -1,
			stop_after_hrs        FLOAT        DEFAULT -1,
			desired_state         ENUM('running','stopped') NOT NULL DEFAULT 'running',
			updated_at            TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			CONSTRAINT server_reachable CHECK ((cluster IS NULL) <> (port IS NULL)),
			FOREIGN KEY (cluster) REFERENCES csfleet_clusters(name)
		);

		-- scope is global|cluster|server; scope_name is '' for global, else the
		-- cluster or server name. A server resolves global < cluster < server
		

		CREATE TABLE IF NOT EXISTS csfleet_env_variables (
` + "			`key`" + `      VARCHAR(255) NOT NULL,
			value      TEXT         NOT NULL,
			scope      ENUM('global','cluster','server') NOT NULL DEFAULT 'global',
			scope_name VARCHAR(255) NOT NULL DEFAULT '',
` + "			PRIMARY KEY (`key`, scope, scope_name)" + `
		);

		CREATE TABLE IF NOT EXISTS csfleet_plugin_assignments (
			plugin     VARCHAR(255) NOT NULL,
			scope      ENUM('global','cluster','server') NOT NULL DEFAULT 'server',
			scope_name VARCHAR(255) NOT NULL DEFAULT '',
			PRIMARY KEY (plugin, scope, scope_name),
			FOREIGN KEY (plugin) REFERENCES csfleet_plugin_manifests(name)
		);

		-- Marks a scope that holds an explicit plugin set. Presence means "this
		-- scope overrides" even with 0 rows in csfleet_plugin_assignments above. Absence
		-- means inherit from the parent scope (server < cluster < global).

		CREATE TABLE IF NOT EXISTS csfleet_plugin_overrides (
			scope      ENUM('global','cluster','server') NOT NULL,
			scope_name VARCHAR(255) NOT NULL DEFAULT '',
			PRIMARY KEY (scope, scope_name)
		);

		CREATE TABLE IF NOT EXISTS csfleet_config_assignments (
			config     VARCHAR(255) NOT NULL,
			scope      ENUM('global','cluster','server') NOT NULL DEFAULT 'server',
			scope_name VARCHAR(255) NOT NULL DEFAULT '',
			PRIMARY KEY (config, scope, scope_name),
			FOREIGN KEY (config) REFERENCES csfleet_config_files(name)
		);

		-- Same as csfleet_plugin_overrides, for csfleet_config_assignments

		CREATE TABLE IF NOT EXISTS csfleet_config_overrides (
			scope      ENUM('global','cluster','server') NOT NULL,
			scope_name VARCHAR(255) NOT NULL DEFAULT '',
			PRIMARY KEY (scope, scope_name)
		);

		-- Web UI accounts. The seed admin (ADMIN_USER) is reconciled here on every
		-- boot from .env; all other rows are managed through the UI. bcrypt hashes only.
		CREATE TABLE IF NOT EXISTS csfleet_web_users (
			username   VARCHAR(255) NOT NULL PRIMARY KEY,
			pass_hash  VARCHAR(255) NOT NULL,
			created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return err
	}
	return s.ensurePortTriggers()
}

// ensurePortTriggers keeps the external host ports in csfleet_servers.port and
// csfleet_clusters.port from colliding.
//
// UNIQUE already guards each column on its own;
// these cover the cross-table case.
func (s *Store) ensurePortTriggers() error {
	add := func(name, event, other, msg string) string {
		return fmt.Sprintf(`CREATE OR REPLACE TRIGGER %s %s FOR EACH ROW
			BEGIN
				IF EXISTS (SELECT 1 FROM %s WHERE port = NEW.port) THEN
					SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = '%s';
				END IF;
			END`, name, event, other, msg)
	}
	triggers := []string{
		add("csfleet_servers_port_insert", "BEFORE INSERT ON csfleet_servers", "csfleet_clusters", "server port collides with a cluster port"),
		add("csfleet_servers_port_update", "BEFORE UPDATE ON csfleet_servers", "csfleet_clusters", "server port collides with a cluster port"),
		add("csfleet_clusters_port_insert", "BEFORE INSERT ON csfleet_clusters", "csfleet_servers", "cluster port collides with a server port"),
		add("csfleet_clusters_port_update", "BEFORE UPDATE ON csfleet_clusters", "csfleet_servers", "cluster port collides with a server port"),
	}
	for _, t := range triggers {
		if _, err := s.DB.Exec(t); err != nil {
			return fmt.Errorf("port trigger: %w", err)
		}
	}
	return nil
}

func (s *Store) seedDefaults() error {
	defaults := []struct{ key, value string }{
		{"db.host", s.cfg.Container},
		{"db.port", strconv.Itoa(s.cfg.Port)},
		{"db.name", s.cfg.Name},
		{"db.user", s.cfg.User},
		{"db.pass", s.cfg.Pass},
		{"db.rootpass", s.cfg.RootPass},
	}
	for _, d := range defaults {
		_, err := s.DB.Exec(
			"INSERT IGNORE INTO csfleet_env_variables (`key`, value) VALUES (?, ?)",
			d.key, d.value,
		)
		if err != nil {
			return fmt.Errorf("seed %q: %w", d.key, err)
		}
	}
	return nil
}

func (s *Store) LoadManifest(name string) (string, error) {
	var manifest string
	err := s.DB.QueryRow("SELECT manifest FROM csfleet_plugin_manifests WHERE name = ?", name).Scan(&manifest)
	if err != nil {
		return "", fmt.Errorf("load manifest %q: %w", name, err)
	}
	return manifest, nil
}

func (s *Store) LoadConfigFile(name string) (string, error) {
	var content string
	err := s.DB.QueryRow("SELECT content FROM csfleet_config_files WHERE name = ?", name).Scan(&content)
	if err != nil {
		return "", fmt.Errorf("load config %q: %w", name, err)
	}
	return content, nil
}
