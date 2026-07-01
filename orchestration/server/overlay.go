package server

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// steamUID is the uid/gid the cs2 image's container process runs as (the
// unprivileged "steam" user); it must match provision.steamUID. The container
// writes into the merged overlay as this user, so the upperdir that receives
// copy-up writes has to be owned by it — otherwise, with a root orchestrator
// (the common server case), every write from the server EACCESes.
const steamUID = 1000

type overlayDirs struct {
	instanceDir string
	upper       string
	work        string
	merged      string
}

func newOverlay(repoRoot, serverName string) overlayDirs {
	dir := filepath.Join(repoRoot, "instances", serverName)
	return overlayDirs{
		instanceDir: dir,
		upper:       filepath.Join(dir, "upper"),
		work:        filepath.Join(dir, "work"),
		merged:      filepath.Join(dir, "merged"),
	}
}

func (ov overlayDirs) mount(lowerdir string) error {
	for _, d := range []string{ov.upper, ov.work, ov.merged} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	// When the orchestrator runs as root the dirs are root-owned; hand the
	// upperdir (and its work companion) to the steam user so the container's
	// copy-up writes succeed. A non-root orchestrator already owns them.
	if os.Geteuid() == 0 {
		for _, d := range []string{ov.upper, ov.work} {
			if err := os.Chown(d, steamUID, steamUID); err != nil {
				return fmt.Errorf("chown %s: %w", d, err)
			}
		}
	}
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerdir, ov.upper, ov.work)
	if err := syscall.Mount("overlay", ov.merged, "overlay", 0, opts); err != nil {
		return fmt.Errorf("mount overlay at %s: %w", ov.merged, err)
	}
	return nil
}

func (ov overlayDirs) unmount() error {
	if err := syscall.Unmount(ov.merged, 0); err != nil {
		return fmt.Errorf("unmount %s: %w", ov.merged, err)
	}
	return os.RemoveAll(ov.instanceDir)
}
