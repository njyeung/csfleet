// Package serverconfig is stage 2b of building a server's filesystem: writing
// CS2 config files (cvars, exec scripts, gamemode overrides, etc.) into a
// single server's overlay. Like plugin apply, this runs once per server
// spin-up, after the overlay is mounted.
//
// A config file is a (name, filename, content) tuple stored in the csfleet_config_files
// DB table. "name" is the catalog identifier (the PK assignments reference);
// "filename" is the file's path under game/csgo/cfg/ — usually just a bare name
// like gamemode_competitive_server.cfg; "content" is the raw file body.
//
// A server may resolve to several configs. Each is written to its own filename,
// so distinct filenames coexist. Two configs that resolve to the same filename
// just overwrite in apply order. Last one wins.

package serverconfig

import (
	"fmt"
	"path/filepath"

	"csfleet/orchestrator/internal/install"
)

const cfgDir = "cfg"

// Apply writes one config body into the overlay under game/csgo/cfg/, at the
// config's filename. name is the config's catalog identifier, used only for
// diagnostics.
func Apply(overlayCSGO, name, filename, content string) error {
	if filename == "" {
		return fmt.Errorf("config file %q has no filename", name)
	}

	dest := filepath.Join(overlayCSGO, cfgDir, filepath.FromSlash(filename))
	return install.AtomicWrite(dest, []byte(content))
}
