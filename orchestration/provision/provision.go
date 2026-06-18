package provision

import (
	"fmt"
	"os"
)

// Run is the orchestrator's startup provisioning phase.
//
// On startup, it reconciles the shared base/ install to upstream before the server starts
// Receipt is stored in base/.csfleet-versions.json
// It also pulls the latest version of the joedwards32/CS2 dedicated server docker image since
// we use it for SteamCMD to install the latest version of counter strike and it is needed later
// to spawn server instances
func Run(root string) error {
	p := newPaths(root)
	if err := os.MkdirAll(p.base, 0o755); err != nil {
		return err
	}

	logf("provisioning shared base at %s", p.base)

	rec := loadReceipt(p.receipt)

	// pull cs2 dedicated server docker img
	if err := ensureImage(p); err != nil {
		return fmt.Errorf("image: %w", err)
	}

	// download cs2 binary
	game, err := ensureGame(p, rec)
	if err != nil {
		return fmt.Errorf("game: %w", err)
	}
	rec.Game = game

	// InspectGive plugin requires a "weaponid_paintindex" -> skin name mapping
	skins, err := reconcileSkins(p, rec)
	if err != nil {
		return fmt.Errorf("skins: %w", err)
	}
	rec.Skins = skins

	// Metamod, Counter strike sharp, WeaponPaints
	mods, err := reconcileMods(p, rec)
	if err != nil {
		return fmt.Errorf("mods: %w", err)
	}
	rec.ModBundle = mods

	// InspectGive
	plugin, err := reconcilePlugin(p, rec, rec.ModBundle.CSS)
	if err != nil {
		return fmt.Errorf("plugin: %w", err)
	}
	rec.Plugin = plugin

	// pre.sh runs before entry.sh and registers our mods within
	// the server's container
	if err := bakeHook(p); err != nil {
		return fmt.Errorf("hook: %w", err)
	}

	// Save receipt to bookmark versions for next time
	if err := saveReceipt(p.receipt, rec); err != nil {
		return fmt.Errorf("receipt: %w", err)
	}
	logf("provisioning complete")
	return nil
}
