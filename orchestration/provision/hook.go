package provision

import (
	_ "embed"
	"errors"
	"fmt"
	"os"

	"csfleet/orchestrator/internal/install"
)

//go:embed pre.sh
var defaultPreHook []byte

// bakeHook installs pre.sh at base/pre.sh, the path the base image's
// entry.sh sources before launch ($STEAMAPPDIR/pre.sh). The image only copies
// its own default when none exists, so baking ours in means every server runs
// it with no per-container bind mount. It's tiny, so we always refresh it.
func bakeHook(p paths) error {
	if _, err := os.Stat(p.preHook); err == nil {
		logf("baking pre.sh -> %s", p.bakedHook)
		return install.CopyFile(p.preHook, p.bakedHook)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat hook override %s: %w", p.preHook, err)
	}

	logf("baking embedded pre.sh -> %s", p.bakedHook)
	if err := install.AtomicWrite(p.bakedHook, defaultPreHook); err != nil {
		return err
	}
	return os.Chmod(p.bakedHook, 0o755)
}
