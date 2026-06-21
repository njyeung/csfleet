// Package serverconfig is stage 2b of building a server's filesystem: writing
// CS2 config files (cvars, exec scripts, gamemode overrides, etc.) into a
// single server's overlay. Like plugin apply, this runs once per server
// spin-up, after the overlay is mounted.
//
// A config file is a (name, content) tuple stored in the config_files DB
// table. "name" is the game-relative path (e.g. "cfg/server.cfg") and
// "content" is the raw file body. The orchestrator loads the tuple and passes
// it here.
package serverconfig

import (
	"fmt"
	"path/filepath"
	"strings"

	"csfleet/orchestrator/internal/install"
)

// Apply writes a single config file into the overlay. name is the
// game-relative path (e.g. "cfg/server.cfg") doubling as the config's
// identifier. It rejects paths that try to escape the overlay root.
func Apply(overlayCSGO, name, content string) error {
	if name == "" {
		return fmt.Errorf("config file has no name")
	}

	dest := filepath.Join(overlayCSGO, filepath.FromSlash(name))

	rel, err := filepath.Rel(overlayCSGO, dest)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("config path %q escapes overlay root", name)
	}

	return install.AtomicWrite(dest, []byte(content))
}
