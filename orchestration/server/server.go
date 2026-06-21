package server

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"

	"csfleet/orchestrator/plugin"
	"csfleet/orchestrator/serverconfig"
)

const cs2Image = "joedwards32/cs2:latest"

type Definition struct {
	Name           string
	Map            string
	Port           int
	GOTVPortOffset int
	GSLTToken      string
	RconPassword   string
	ServerPassword string
	LAN            bool
	GameType       int
	GameMode       int
	MaxPlayers     int
	BotQuota       int
	Env            map[string]string
}

type ConfigPayload struct {
	Name    string
	Content string
}

type Instance struct {
	Name        string
	ContainerID string
	Port        int
	overlay     overlayDirs
}

func containerName(name string) string {
	return "csfleet-" + name
}

func Start(ctx context.Context, cli *client.Client, root string, def Definition,
	pluginNames []string, configs []ConfigPayload, ds plugin.Datasource,
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
		if err := plugin.Apply(csgo, toml, "", ds); err != nil {
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

	log.Printf("[server/%s] started (container %s, port %d)", def.Name, id[:12], def.Port)
	return &Instance{
		Name:        def.Name,
		ContainerID: id,
		Port:        def.Port,
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
	port := strconv.Itoa(def.Port)
	gotvPort := strconv.Itoa(def.Port + def.GOTVPortOffset)
	name := containerName(def.Name)

	env := []string{
		"CS2_SERVERNAME=" + def.Name,
		"CS2_PORT=" + port,
		"CS2_RCON_PORT=" + port,
		"CS2_RCONPW=" + def.RconPassword,
		"CS2_PW=" + def.ServerPassword,
		"CS2_LAN=" + boolStr(def.LAN),
		"CS2_GAMETYPE=" + strconv.Itoa(def.GameType),
		"CS2_GAMEMODE=" + strconv.Itoa(def.GameMode),
		"CS2_MAXPLAYERS=" + strconv.Itoa(def.MaxPlayers),
		"CS2_STARTMAP=" + def.Map,
		"CS2_BOT_QUOTA=" + strconv.Itoa(def.BotQuota),
		"TV_PORT=" + gotvPort,
	}

	if def.GSLTToken != "" {
		env = append(env, "SRCDS_TOKEN="+def.GSLTToken)
	}
	for k, v := range def.Env {
		env = append(env, k+"="+v)
	}

	cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: cs2Image,
		Env:   env,
	}, &container.HostConfig{
		NetworkMode: "host",
		// docker's default seccomp profile blocks an i386 syscall that the 32 bit steamcmd needs...
		SecurityOpt: []string{"seccomp=unconfined"},
		Binds:       []string{merged + ":/home/steam/cs2-dedicated"},
	}, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", name, err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("start %s: %w", name, err)
	}

	return resp.ID, nil
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
