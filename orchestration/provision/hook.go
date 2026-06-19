package provision

import "csfleet/orchestrator/internal/install"

// bakeHook installs hooks/pre.sh at base/pre.sh, the path the base image's
// entry.sh sources before launch ($STEAMAPPDIR/pre.sh). The image only copies
// its own default when none exists, so baking ours in means every server runs
// it with no per-container bind mount. It's tiny, so we always refresh it.
func bakeHook(p paths) error {
	logf("baking pre.sh -> %s", p.bakedHook)
	return install.CopyFile(p.preHook, p.bakedHook)
}
