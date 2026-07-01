package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"csfleet/orchestrator/api"
	"csfleet/orchestrator/database"
	"csfleet/orchestrator/internal/install"
	"csfleet/orchestrator/fleet"
	"csfleet/orchestrator/provision"
	"csfleet/orchestrator/proxy"
)

const (
	mariaImage  = "mariadb:11"  // db img name
	mariaDBName = "cs2-mariadb" // db docker container name

	netName    = "csfleet"       // user-defined bridge every container shares
	netSubnet  = "172.30.0.0/24" // fixed subnet so we can assign static IPs
	netGateway = "172.30.0.1"
	mariaIP    = "172.30.0.2" // maria's static address on the bridge

	// hostIPPrefix is the /24 host prefix the API auto-allocates server IPs from.
	// Reserved low addresses (.1 gateway, .2 maria, .3 nginx) sit below its floor.
	hostIPPrefix = "172.30.0."
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

// killOrphans removes csfleet-* game server containers left from a previous
// crash, similar to how ensureNetwork recreates a stale bridge.
func killOrphans(ctx context.Context, cli *client.Client) {
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", "csfleet-")),
	})
	if err != nil {
		log.Printf("[orchestrator] list containers for orphan cleanup: %v", err)
		return
	}
	for _, c := range containers {
		for _, n := range c.Names {
			name := strings.TrimPrefix(n, "/")
			if strings.HasPrefix(name, "csfleet-") {
				log.Printf("[orchestrator] killing orphan container %s (%s)", name, c.ID[:12])
				t := 10
				cli.ContainerStop(ctx, c.ID, container.StopOptions{Timeout: &t})
				cli.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true})
			}
		}
	}
}

// repoRoot is the working directory the binary runs from (the repo root),
// overridable with CSFLEET_ROOT.
func repoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("cannot resolve working directory (set CSFLEET_ROOT): %v", err)
	}
	return wd
}

func main() {
	root := repoRoot()
	loadDotenv(filepath.Join(root, ".env"))
	install.ConfigureCache(root)
	cfg := configFromEnv(root)
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("[orchestrator] docker client: %v", err)
	}
	defer cli.Close()

	// NewClientWithOpts doesn't touch the daemon; ping so a missing/stopped
	// Docker fails fast with an actionable message instead of deep in provisioning.
	if _, err := cli.Ping(ctx); err != nil {
		log.Fatalf("[orchestrator] cannot reach the Docker daemon (%v)\n"+
			"  Docker must be installed and running. On most distros:\n"+
			"    install: https://docs.docker.com/engine/install/\n"+
			"    start:   sudo systemctl enable --now docker", err)
	}

	killOrphans(ctx, cli)

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

	px := proxy.New(proxy.Config{Table: netName, Subnet: netSubnet})
	if err := px.Start(ctx); err != nil {
		log.Fatalf("[orchestrator] proxy: %v", err)
	}
	defer px.Stop()

	mgr := fleet.New(store, px, cli, root)

	// Serve the built SPA from the same listener as the API.
	staticDir := filepath.Join(root, "frontend", "build")
	if _, err := os.Stat(staticDir); err != nil {
		log.Printf("[orchestrator] SPA build not found at %s — serving API only (run `npm run build` in frontend/)", staticDir)
		staticDir = ""
	}
	// Seed admin lives only in memory; only its bcrypt hash reaches the API.
	adminHash, err := api.HashPassword(cfg.AdminPass)
	if err != nil {
		log.Fatalf("[orchestrator] hash admin password: %v", err)
	}
	apiSrv := api.New(api.Config{
		Addr:          cfg.APIAddr,
		IPPrefix:      hostIPPrefix,
		StaticDir:     staticDir,
		AdminUser:     cfg.AdminUser,
		AdminPassHash: adminHash,
		JWTSecret:     cfg.JWTSecret,
		TLSDomains:    cfg.TLSDomains,
		TLSCacheDir:   cfg.TLSCacheDir,
		TLSEmail:      cfg.TLSEmail,
		HTTPAddr:      cfg.HTTPAddr,
	}, store, mgr)

	// runCtx cancels on the first SIGINT/SIGTERM; the long-running services derive
	// from it, while setup and the teardown defers above keep using the background
	// ctx so cleanup still runs after a signal.
	runCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mgrDone := make(chan struct{})
	go func() {
		defer close(mgrDone)
		if err := mgr.Run(runCtx); err != nil {
			log.Printf("[orchestrator] manager: %v", err)
		}
	}()
	go func() {
		if err := apiSrv.Run(runCtx); err != nil {
			log.Printf("[orchestrator] api: %v", err)
		}
	}()

	log.Println("[orchestrator] ready — waiting for signal")
	<-runCtx.Done()
	log.Println("[orchestrator] shutting down")
	mgr.Stop()
	<-mgrDone // let workers tear down their containers before the defers remove the network
}
