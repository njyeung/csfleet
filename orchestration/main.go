package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"csfleet/orchestrator/database"
	"csfleet/orchestrator/provision"
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

	ensureNetwork(ctx, cli)

	store, err := database.Start(ctx, cli, database.Config{
		Image:     mariaImage,
		Container: mariaDBName,
		Network:   networkName,
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

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("[orchestrator] shutting down")
		store.Close(context.Background())
		os.Exit(0)
	}()

	if err := provision.Run(ctx, root, cli); err != nil {
		log.Fatalf("[orchestrator] provision: %v", err)
	}
}

func ensureNetwork(ctx context.Context, cli *client.Client) {
	cli.NetworkCreate(ctx, networkName, network.CreateOptions{})
}
