package main

import (
	"os"
	"strconv"
)

// Bootstrap values the orchestrator needs before it can reach the database
// Everything else is stored in the DB tables and is seeded from these on first run
const networkName = "csfleet"

type Config struct {
	DBHost     string
	DBPort     int
	DBName     string
	DBUser     string
	DBPass     string
	DBRootPass string
}

func configFromEnv() Config {
	port, _ := strconv.Atoi(envOr("DB_PORT", "3306"))
	return Config{
		DBHost:     envOr("DB_HOST", "127.0.0.1"),
		DBPort:     port,
		DBName:     envOr("DB_NAME", "csfleet"),
		DBUser:     envOr("DB_USER", "csfleet"),
		DBPass:     envOr("DB_PASS", "csfleet"),
		DBRootPass: envOr("DB_ROOT_PASS", "csfleet"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
