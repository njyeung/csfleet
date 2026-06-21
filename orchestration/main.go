package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/docker/docker/client"

	"csfleet/orchestrator/database"
	"csfleet/orchestrator/plugin"
	"csfleet/orchestrator/provision"
	"csfleet/orchestrator/server"
)

const (
	mariaImage  = "mariadb:11"   // db img name
	mariaDBName = "cs2-mariadb"  // db docker container name
)

func repoRoot() string {
	if env := os.Getenv("CSFLEET_ROOT"); env != "" {
		return env
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("cannot resolve source location (set CSFLEET_ROOT)")
	}
	return filepath.Dir(filepath.Dir(file))
}

func main() {
	root := repoRoot()
	cfg := configFromEnv()
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("[orchestrator] docker client: %v", err)
	}
	defer cli.Close()

	store, err := database.Start(ctx, cli, database.Config{
		Image:     mariaImage,
		Container: mariaDBName,
		Host:      cfg.DBHost,
		Name:      cfg.DBName,
		User:      cfg.DBUser,
		Pass:      cfg.DBPass,
		RootPass:  cfg.DBRootPass,
		Port:      cfg.DBPort,
	})
	if err != nil {
		log.Fatalf("[orchestrator] database: %v", err)
	}
	defer store.Close(context.Background())

	if err := provision.Run(ctx, root, cli); err != nil {
		log.Fatalf("[orchestrator] provision: %v", err)
	}

	inst, err := server.Start(ctx, cli, root, server.Definition{
		Name:           "test",
		Map:            "de_dust2",
		Port:           27015,
		GOTVPortOffset: 5,
		LAN:            true,
		GameMode:       1,
		MaxPlayers:     10,
	}, nil, nil, plugin.Datasource{}, store.LoadManifest)
	if err != nil {
		log.Fatalf("[orchestrator] server: %v", err)
	}
	defer inst.Stop(context.Background(), cli)

	log.Println("[orchestrator] ready — waiting for signal")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("[orchestrator] shutting down")
}

