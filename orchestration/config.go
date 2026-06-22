package main

import (
	"os"
	"strconv"
)

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
		DBHost:     envOr("DB_HOST", mariaIP),
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
