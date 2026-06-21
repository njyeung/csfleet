package server

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

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
