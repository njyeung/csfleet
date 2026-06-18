package provision

import (
	"fmt"
	"os"
	"regexp"
)

const (
	steamAppID   = "730"
	cs2Image     = "cs2-skin-inspect:local"
	steamInfoURL = "https://api.steamcmd.net/v1/info/" + steamAppID
)

// ensureGame brings base/ up to the build Valve currently ships and returns the
// installed buildid for the receipt.
func ensureGame(p paths, rec receipt) (gameReceipt, error) {
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

	if err := steamUpdate(p); err != nil {
		return rec.Game, err
	}
	id, ok := currentBuildID(p)
	if !ok {
		return rec.Game, fmt.Errorf("game still reports incomplete after update")
	}

	logf("game at buildid %s", id)

	return gameReceipt{BuildID: id}, nil
}

// latestBuildID reads the current public-branch buildid from api.steamcmd.net,
// a lightweight mirror of `steamcmd +app_info_print 730` (no SteamCMD run).
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
	if err := fetchJSON(steamInfoURL, &info); err != nil {
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
func steamUpdate(p paths) error {
	if err := os.MkdirAll(p.base, 0o755); err != nil {
		return err
	}
	// The image's default user is uid 1000 (steam), matching the host user, so
	// bind-mounted files land owned by us with no --user override (which would
	// also blank $HOME and break SteamCMD's ~/.steam).
	script := `
set -e
bash "${STEAMCMDDIR}/steamcmd.sh" \
  +force_install_dir "${STEAMAPPDIR}" \
  +@bClientTryRequestManifestWithoutCode 1 \
  +login anonymous \
  +app_update ` + steamAppID + ` \
  +quit
`
	if err := run("docker", "run", "--rm", "-i",
		"--name", "csfleet-game-update",
		"--security-opt", "seccomp=unconfined",
		"-v", p.base+":/home/steam/cs2-dedicated",
		"--entrypoint", "bash",
		cs2Image, "-c", script,
	); err != nil {
		return fmt.Errorf("steamcmd update: %w", err)
	}
	return nil
}

// stateFullyInstalled matches `"StateFlags" "4"`
// SteamCMD only sets that once the install is complete and verified.
var (
	stateFullyInstalled = regexp.MustCompile(`"StateFlags"\s*"4"`)
	buildIDField        = regexp.MustCompile(`"buildid"\s*"(\d+)"`)
)

// currentBuildID returns the installed buildid from the appmanifest. ok is false
// when the game isn't fully installed (no manifest, or a mid-download state).
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
