package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"csfleet/orchestrator/database"
	"csfleet/orchestrator/plugin"
	"csfleet/orchestrator/provision"
	"csfleet/orchestrator/proxy"
	"csfleet/orchestrator/server"
)

const (
	mariaImage  = "mariadb:11"  // db img name
	mariaDBName = "cs2-mariadb" // db docker container name

	netName    = "csfleet"       // user-defined bridge every container shares
	netSubnet  = "172.30.0.0/24" // fixed subnet so we can assign static IPs
	netGateway = "172.30.0.1"
	mariaIP    = "172.30.0.2" // maria's static address on the bridge
)

// ensureNetwork creates the shared bridge with a fixed subnet if it's missing,
// so containers can be given static IPs the proxy can DNAT to. A pre-existing
// network whose subnet no longer matches (e.g. a crash skipped teardown and the
// constants have since changed) is torn down and recreated.
func ensureNetwork(ctx context.Context, cli *client.Client, name, subnet, gateway string) error {
	if existing, err := cli.NetworkInspect(ctx, name, network.InspectOptions{}); err == nil {
		if networkHasSubnet(existing, subnet) {
			return nil
		}
		log.Printf("[orchestrator] network %s has stale subnet, recreating as %s", name, subnet)
		if err := removeNetwork(ctx, cli, name); err != nil {
			return fmt.Errorf("remove stale network %s: %w", name, err)
		}
	}
	_, err := cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
		IPAM: &network.IPAM{
			Config: []network.IPAMConfig{{Subnet: subnet, Gateway: gateway}},
		},
	})
	return err
}

// networkHasSubnet reports whether the network already uses the wanted subnet.
func networkHasSubnet(n network.Inspect, subnet string) bool {
	for _, c := range n.IPAM.Config {
		if c.Subnet == subnet {
			return true
		}
	}
	return false
}

// removeNetwork tears down the shared bridge. Any still-attached endpoints are
// force-disconnected first
func removeNetwork(ctx context.Context, cli *client.Client, name string) error {
	existing, err := cli.NetworkInspect(ctx, name, network.InspectOptions{})
	if err != nil {
		return nil // already gone
	}
	for id := range existing.Containers {
		if err := cli.NetworkDisconnect(ctx, name, id, true); err != nil {
			log.Printf("[orchestrator] disconnect %s from %s: %v", id, name, err)
		}
	}
	return cli.NetworkRemove(ctx, name)
}

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

	// TODO: when we have server lifecycle, we need to make sure any orphaned containers
	// are killed here before ensureNetwork

	if err := ensureNetwork(ctx, cli, netName, netSubnet, netGateway); err != nil {
		log.Fatalf("[orchestrator] network: %v", err)
	}
	// Registered here so LIFO runs it after every container-removing defer below
	// (store.Close, inst.Stop) but before cli.Close — the bridge is empty by then.
	defer func() {
		if err := removeNetwork(ctx, cli, netName); err != nil {
			log.Printf("[orchestrator] remove network %s: %v", netName, err)
		}
	}()

	store, err := database.Start(ctx, cli, database.Config{
		Image:     mariaImage,
		Container: mariaDBName,
		Network:   netName,
		IP:        mariaIP,
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

	env, err := store.LoadEnv("")
	if err != nil {
		log.Fatalf("[orchestrator] load env: %v", err)
	}
	dbPort, _ := strconv.Atoi(env["db.port"])
	ds := plugin.Datasource{
		Host: env["db.host"],
		Port: dbPort,
		Name: env["db.name"],
		User: env["db.user"],
		Pass: env["db.pass"],
	}

	// --- UDP proxy: put the test server behind one external port ---
	const backendIP = "172.30.0.10" // the test server's static bridge address
	const extPort = 4567            // external udp port clients connect to

	px := proxy.New(proxy.Config{Table: netName, Subnet: netSubnet})
	if err := px.Start(ctx); err != nil {
		log.Fatalf("[orchestrator] proxy: %v", err)
	}
	defer px.Stop()

	// Clean slate before the container binds the address, so no stale conntrack
	// entry can NAT a client straight into a fresh instance on the same IP.
	if err := px.FlushConntrack(backendIP); err != nil {
		log.Printf("[orchestrator] flush conntrack %s: %v", backendIP, err)
	}

	inst, err := server.Start(ctx, cli, root, server.Definition{
		Name:       "test",
		Map:        "de_dust2",
		Network:    netName,
		IP:         backendIP,
		LAN:        true,
		GameMode:   1,
		MaxPlayers: 10,
	}, nil, nil, ds, store.LoadManifest)
	if err != nil {
		log.Fatalf("[orchestrator] server: %v", err)
	}
	defer inst.Stop(context.Background(), cli)

	// Register the live backend: new UDP flows to extPort now DNAT to backendIP:27015.
	if err := px.AddBackend(extPort, backendIP); err != nil {
		log.Fatalf("[orchestrator] add backend: %v", err)
	}
	log.Printf("[orchestrator] 'test' reachable on udp/%d -> %s:27015", extPort, backendIP)

	log.Println("[orchestrator] ready — waiting for signal")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("[orchestrator] shutting down")
}
