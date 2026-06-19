package provision

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"

	"csfleet/orchestrator/internal/install"
)

const (
	steamAppID = "730"
	cs2Image   = "joedwards32/cs2:latest"
	steamInfoURL = "https://api.steamcmd.net/v1/info/" + steamAppID
)

func ensureGame(ctx context.Context, cli *client.Client, p paths, rec receipt) (gameReceipt, error) {
	installed, present := currentBuildID(p)

	latest, err := latestBuildID()

	switch {
	case err != nil:
		logf("could not resolve latest game buildid (%v) — running SteamCMD", err)
	case present && rec.Game.BuildID == latest && installed == latest:
		logf("game up to date (buildid %s)", latest)
		return rec.Game, nil
	}

	logf("updating CS2 (appid %s) at %s via SteamCMD", steamAppID, p.base)

	if err := steamUpdate(ctx, cli, p); err != nil {
		return rec.Game, err
	}
	id, ok := currentBuildID(p)
	if !ok {
		return rec.Game, fmt.Errorf("game still reports incomplete after update")
	}

	logf("game at buildid %s", id)

	return gameReceipt{BuildID: id}, nil
}

func latestBuildID() (string, error) {
	var info struct {
		Data map[string]struct {
			Depots struct {
				Branches struct {
					Public struct {
						BuildID string `json:"buildid"`
					} `json:"public"`
				} `json:"branches"`
			} `json:"depots"`
		} `json:"data"`
	}
	if err := install.FetchJSON(steamInfoURL, &info); err != nil {
		return "", err
	}
	app, ok := info.Data[steamAppID]
	if !ok || app.Depots.Branches.Public.BuildID == "" {
		return "", fmt.Errorf("no public buildid in %s", steamInfoURL)
	}
	return app.Depots.Branches.Public.BuildID, nil
}

// seccomp:unconfined: Docker's default seccomp profile blocks an i386 syscall
// the 32-bit Steam client needs, which otherwise makes SteamCMD falsely report
// "needs to be online to update" (same fix as docker-compose.yml).
func steamUpdate(ctx context.Context, cli *client.Client, p paths) error {
	if err := os.MkdirAll(p.base, 0o755); err != nil {
		return err
	}
	script := `
set -e
bash "${STEAMCMDDIR}/steamcmd.sh" \
  +force_install_dir "${STEAMAPPDIR}" \
  +@bClientTryRequestManifestWithoutCode 1 \
  +login anonymous \
  +app_update ` + steamAppID + ` \
  +quit
`
	if err := runEphemeral(ctx, cli, &container.Config{
		Image:      cs2Image,
		Entrypoint: []string{"bash"},
		Cmd:        []string{"-c", script},
	}, &container.HostConfig{
		SecurityOpt: []string{"seccomp=unconfined"},
		Binds:       []string{p.base + ":/home/steam/cs2-dedicated"},
	}, "csfleet-game-update"); err != nil {
		return fmt.Errorf("steamcmd update: %w", err)
	}
	return nil
}

var (
	stateFullyInstalled = regexp.MustCompile(`"StateFlags"\s*"4"`)
	buildIDField        = regexp.MustCompile(`"buildid"\s*"(\d+)"`)
)

func currentBuildID(p paths) (id string, ok bool) {
	manifest, err := os.ReadFile(p.appManifest)
	if err != nil || !stateFullyInstalled.Match(manifest) {
		return "", false
	}
	m := buildIDField.FindSubmatch(manifest)
	if m == nil {
		return "", false
	}
	return string(m[1]), true
}
