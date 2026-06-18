package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"csfleet/orchestrator/provision"
)

// repoRoot resolves the repo root. CSFLEET_ROOT overrides it (e.g. when the
// binary runs containerized where the source path no longer exists); otherwise
// it derives from this source file's location.
func repoRoot() string {
	if env := os.Getenv("CSFLEET_ROOT"); env != "" {
		return env
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("cannot resolve source location (set CSFLEET_ROOT)")
	}
	// file = <root>/orchestration/main.go -> Dir twice == <root>
	return filepath.Dir(filepath.Dir(file))
}

// CSFleet orchestrator. Every startup begins by provisioning the shared base
// install (game + mods + plugin) to upstream-latest; the long-running daemon
// (port pool, overlays, container lifecycle) follows.
func main() {
	if err := provision.Run(repoRoot()); err != nil {
		log.Fatalf("[orchestrator] provision: %v", err)
	}
	// TODO: daemon runtime (spawn + reconcile the configured servers).
}
