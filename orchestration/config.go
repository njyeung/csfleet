package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// loadDotenv reads KEY=VALUE pairs from a .env file into the process
// environment so configFromEnv (and anything else using os.Getenv) picks them
// up. A real environment variable always wins, so the file is only a default.
// A missing file is fine — the env may be set some other way.
func loadDotenv(path string) {
	f, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[config] reading %s: %v", path, err)
		}
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "" {
			continue
		}
		if _, set := os.LookupEnv(key); set {
			continue // real environment overrides the file
		}
		os.Setenv(key, val)
	}
	if err := sc.Err(); err != nil {
		log.Printf("[config] reading %s: %v", path, err)
	}
}

type Config struct {
	DBHost     string
	DBPort     int
	DBName     string
	DBUser     string
	DBPass     string
	DBRootPass string
	APIAddr    string // primary control-plane listen address (HTTPS when TLS is on)
	HTTPAddr   string // plain-HTTP listener for the ACME challenge + HTTPS redirect

	AdminUser string // ADMIN_USER
	AdminPass string // ADMIN_PASS
	JWTSecret string // JWT_SECRET: HS256 signing key

	TLSDomains  []string // TLS_DOMAINS: hostnames autocert issues certs for; empty disables TLS
	TLSCacheDir string   // TLS_CACHE_DIR: where autocert caches certs
	TLSEmail    string   // TLS_EMAIL: optional ACME account contact
}

func configFromEnv(root string) Config {
	port, _ := strconv.Atoi(envOr("DB_PORT", "3306"))

	// TLS is on when TLS_DOMAINS is set,
	// the primary listener then defaults to :443.
	tlsDomains := splitList(os.Getenv("TLS_DOMAINS"))
	apiDefault := ":80"
	if len(tlsDomains) > 0 {
		apiDefault = ":443"
	}

	cfg := Config{
		DBHost:      envOr("DB_HOST", "172.30.0.2"),
		DBPort:      port,
		DBName:      envOr("DB_NAME", "csfleet"),
		DBUser:      envOr("DB_USER", "csfleet"),
		DBPass:      envOr("DB_PASS", "csfleet"),
		DBRootPass:  envOr("DB_ROOT_PASS", "csfleet"),
		APIAddr:     envOr("API_ADDR", apiDefault),
		HTTPAddr:    ":80",
		AdminUser:   strings.ToLower(envOr("ADMIN_USER", "admin")),
		AdminPass:   os.Getenv("ADMIN_PASS"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		TLSDomains:  tlsDomains,
		TLSCacheDir: envOr("TLS_CACHE_DIR", filepath.Join(root, "cache", "autocert")),
		TLSEmail:    os.Getenv("TLS_EMAIL"),
	}

	if cfg.AdminPass == "" {
		log.Fatal("[config] ADMIN_PASS is required; refusing to start with an empty admin password")
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = randomSecret()
		log.Print("[config] JWT_SECRET not set — generated a random one; sessions reset on restart")
	}
	return cfg
}

func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("[config] generate JWT secret: %v", err)
	}
	return hex.EncodeToString(b)
}

// splitList parses a comma-separated env value into a trimmed, non-empty slice.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
