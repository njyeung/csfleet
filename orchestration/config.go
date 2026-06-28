package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
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
	APIAddr    string // HTTP control-plane listen address

	AdminUser string // ADMIN_USER
	AdminPass string // ADMIN_PASS
	JWTSecret string // JWT_SECRET: HS256 signing key
}

func configFromEnv() Config {
	port, _ := strconv.Atoi(envOr("DB_PORT", "3306"))
	cfg := Config{
		DBHost:     envOr("DB_HOST", mariaIP),
		DBPort:     port,
		DBName:     envOr("DB_NAME", "csfleet"),
		DBUser:     envOr("DB_USER", "csfleet"),
		DBPass:     envOr("DB_PASS", "csfleet"),
		DBRootPass: envOr("DB_ROOT_PASS", "csfleet"),
		APIAddr:    envOr("API_ADDR", ":8080"),
		AdminUser:  strings.ToLower(envOr("ADMIN_USER", "admin")),
		AdminPass:  os.Getenv("ADMIN_PASS"),
		JWTSecret:  os.Getenv("JWT_SECRET"),
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
