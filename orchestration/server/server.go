package server

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"csfleet/orchestrator/plugin"
	"csfleet/orchestrator/serverconfig"
)

const (
	cs2Image = "joedwards32/cs2:latest"

	// Every container has its own bridge IP, so the internal ports are constant.
	// External reachability is handled by the proxy, not by these.
	cs2GamePort = 27015
	cs2GOTVPort = 27020
)

type Definition struct {
	Name    string
	Network string            // bridge to attach to
	IP      string            // static address on that bridge (the proxy's DNAT target)
	Env     map[string]string // game settings + other env variables, resolved from env_variables
}

type ConfigPayload struct {
	Name    string
	Content string
}

type Instance struct {
	Name        string
	ContainerID string
	IP          string
	overlay     overlayDirs
}

func containerName(name string) string {
	return "csfleet-" + name
}

func Start(ctx context.Context, cli *client.Client, root string, def Definition,
	pluginNames []string, configs []ConfigPayload,
	loadManifest func(string) (string, error)) (*Instance, error) {

	base := filepath.Join(root, "base")
	ov := newOverlay(root, def.Name)

	if err := ov.mount(base); err != nil {
		return nil, fmt.Errorf("overlay: %w", err)
	}

	csgo := filepath.Join(ov.merged, "game", "csgo")

	manifests, err := plugin.ResolveOrder(pluginNames, loadManifest)
	if err != nil {
		ov.unmount()
		return nil, fmt.Errorf("resolve plugins: %w", err)
	}
	for _, toml := range manifests {
		if err := plugin.Apply(csgo, toml, def.Env); err != nil {
			ov.unmount()
			return nil, fmt.Errorf("plugin: %w", err)
		}
	}

	for _, c := range configs {
		if err := serverconfig.Apply(csgo, c.Name, c.Content); err != nil {
			ov.unmount()
			return nil, fmt.Errorf("config %s: %w", c.Name, err)
		}
	}

	id, err := createContainer(ctx, cli, def, ov.merged)
	if err != nil {
		ov.unmount()
		return nil, fmt.Errorf("container: %w", err)
	}

	log.Printf("[server/%s] started (container %s, ip %s)", def.Name, id[:12], def.IP)
	return &Instance{
		Name:        def.Name,
		ContainerID: id,
		IP:          def.IP,
		overlay:     ov,
	}, nil
}

func (inst *Instance) Stop(ctx context.Context, cli *client.Client) error {
	log.Printf("[server/%s] stopping", inst.Name)

	timeout := 10
	cli.ContainerStop(ctx, inst.ContainerID, container.StopOptions{Timeout: &timeout})
	cli.ContainerRemove(ctx, inst.ContainerID, container.RemoveOptions{Force: true})

	return inst.overlay.unmount()
}

func createContainer(ctx context.Context, cli *client.Client, def Definition, merged string) (string, error) {
	port := strconv.Itoa(cs2GamePort)
	gotvPort := strconv.Itoa(cs2GOTVPort)
	name := containerName(def.Name)

	// Resolved game settings come from def.Env. The
	// routing vars are appended last so they always win

	// CS2_SERVERNAME leads, so a server-scoped env var can override the default display name.
	env := make([]string, 0, len(def.Env)+4)
	env = append(env, "CS2_SERVERNAME="+def.Name)

	for k, v := range def.Env {
		env = append(env, k+"="+v)
	}
	env = append(env,
		"CS2_PORT="+port,
		"CS2_RCON_PORT="+port,
		"TV_PORT="+gotvPort,
	)

	cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: cs2Image,
		Env:   env,
	}, &container.HostConfig{
		NetworkMode: container.NetworkMode(def.Network),
		// docker's default seccomp profile blocks an i386 syscall that the 32 bit steamcmd needs...
		SecurityOpt: []string{"seccomp=unconfined"},
		Binds:       []string{merged + ":/home/steam/cs2-dedicated"},
	}, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			def.Network: {IPAMConfig: &network.EndpointIPAMConfig{IPv4Address: def.IP}},
		},
	}, nil, name)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", name, err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("start %s: %w", name, err)
	}

	return resp.ID, nil
}
