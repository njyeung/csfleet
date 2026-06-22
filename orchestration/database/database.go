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
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?parseTime=true&multiStatements=true",
		cfg.User, cfg.Pass, cfg.Host, cfg.Port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)

	if _, err := db.Exec("CREATE DATABASE IF NOT EXISTS " + cfg.Name); err != nil {
		db.Close()
		return nil, fmt.Errorf("create database: %w", err)
	}
	if _, err := db.Exec("USE " + cfg.Name); err != nil {
		db.Close()
		return nil, fmt.Errorf("use database: %w", err)
	}
	return db, nil
}

func (s *Store) migrate() error {
	_, err := s.DB.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_manifests (
			name       VARCHAR(255) NOT NULL PRIMARY KEY,
			manifest   MEDIUMTEXT   NOT NULL,
			updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS config_files (
			name       VARCHAR(255) NOT NULL PRIMARY KEY,
			content    MEDIUMTEXT   NOT NULL,
			updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS gslt_tokens (
			token      VARCHAR(255) NOT NULL PRIMARY KEY,
			updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS clusters (
			name       VARCHAR(255) NOT NULL PRIMARY KEY,
			port       INT          NOT NULL UNIQUE,
			updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		);

		-- ip: static bridge address, the proxy's DNAT target. port: external host
		-- port clients reach this server on when standalone, NULL when it's a cluster
		-- backend (reached via clusters.port). Internal game/GOTV ports are constants.
		CREATE TABLE IF NOT EXISTS servers (
			name              VARCHAR(255) NOT NULL PRIMARY KEY,
			map_name          VARCHAR(255) NOT NULL,
			ip                VARCHAR(45)  NOT NULL UNIQUE,
			port              INT          UNIQUE,
			cluster           VARCHAR(255) DEFAULT NULL,
			gslt_token        VARCHAR(255),
			rcon_password     VARCHAR(255),
			server_password   VARCHAR(255),
			lan               BOOLEAN      NOT NULL,
			game_type         INT,
			game_mode         INT,
			max_players       INT,
			bot_quota         INT,
			restart_after_hrs FLOAT,
			stop_after_hrs    FLOAT,
			desired_state     ENUM('running','stopped','disabled') NOT NULL DEFAULT 'running',
			updated_at        TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			CONSTRAINT server_reachable CHECK ((cluster IS NULL) <> (port IS NULL)),
			FOREIGN KEY (gslt_token) REFERENCES gslt_tokens(token),
			FOREIGN KEY (cluster) REFERENCES clusters(name)
		);

		CREATE TABLE IF NOT EXISTS env_variables (
` + "			`key`" + `   VARCHAR(255) NOT NULL,
			value    TEXT         NOT NULL,
			server   VARCHAR(255) NOT NULL DEFAULT '',
` + "			PRIMARY KEY (`key`, server)" + `
		);

		CREATE TABLE IF NOT EXISTS server_plugins (
			server     VARCHAR(255) NOT NULL,
			plugin     VARCHAR(255) NOT NULL,
			PRIMARY KEY (server, plugin),
			FOREIGN KEY (server) REFERENCES servers(name) ON DELETE CASCADE,
			FOREIGN KEY (plugin) REFERENCES plugin_manifests(name)
		);

		CREATE TABLE IF NOT EXISTS server_configs (
			server     VARCHAR(255) NOT NULL,
			config     VARCHAR(255) NOT NULL,
			PRIMARY KEY (server, config),
			FOREIGN KEY (server) REFERENCES servers(name) ON DELETE CASCADE,
			FOREIGN KEY (config) REFERENCES config_files(name)
		);
	`)
	if err != nil {
		return err
	}
	return s.ensurePortTriggers()
}

// ensurePortTriggers keeps the external host ports in servers.port and
// clusters.port from colliding — they share the host's port space. UNIQUE
// already guards each column on its own; these cover the cross-table case.
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
		add("servers_port_insert", "BEFORE INSERT ON servers", "clusters", "server port collides with a cluster port"),
		add("servers_port_update", "BEFORE UPDATE ON servers", "clusters", "server port collides with a cluster port"),
		add("clusters_port_insert", "BEFORE INSERT ON clusters", "servers", "cluster port collides with a server port"),
		add("clusters_port_update", "BEFORE UPDATE ON clusters", "servers", "cluster port collides with a server port"),
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
	}
	for _, d := range defaults {
		_, err := s.DB.Exec(
			"INSERT IGNORE INTO env_variables (`key`, value) VALUES (?, ?)",
			d.key, d.value,
		)
		if err != nil {
			return fmt.Errorf("seed %q: %w", d.key, err)
		}
	}
	return nil
}

// LoadEnv returns the env_variables visible to a server: the globals
// (server = '') overlaid with that server's own rows. Pass "" for just the
// globals. The db.* keys seeded by seedDefaults feed the plugin Datasource.
func (s *Store) LoadEnv(server string) (map[string]string, error) {
	rows, err := s.DB.Query(
		"SELECT `key`, value FROM env_variables WHERE server = '' OR server = ? ORDER BY server",
		server,
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
		env[k] = v // specific server rows sort after '' and overwrite globals
	}
	return env, rows.Err()
}

func (s *Store) LoadManifest(name string) (string, error) {
	var manifest string
	err := s.DB.QueryRow("SELECT manifest FROM plugin_manifests WHERE name = ?", name).Scan(&manifest)
	if err != nil {
		return "", fmt.Errorf("load manifest %q: %w", name, err)
	}
	return manifest, nil
}

func (s *Store) LoadConfigFile(name string) (string, error) {
	var content string
	err := s.DB.QueryRow("SELECT content FROM config_files WHERE name = ?", name).Scan(&content)
	if err != nil {
		return "", fmt.Errorf("load config %q: %w", name, err)
	}
	return content, nil
}
