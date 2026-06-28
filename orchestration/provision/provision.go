package provision

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/docker/client"
)

// Run is the orchestrator's startup provisioning phase: stage 1 of building a
// server's filesystem. It reconciles the shared, read-only base/ install (the
// overlay lowerdir) to upstream — game + MetaMod + CounterStrikeSharp only.
// Plugins are NOT baked into base; they're inserted into each server's overlay
// per-instance (stage 2, ApplyManifest).
//
// Receipt is stored in base/.csfleet-versions.json. It also pulls the latest
// joedwards32/CS2 dedicated server docker image, which we use both for SteamCMD
// (to install CS2) and later to spawn server instances.
func Run(ctx context.Context, root string, cli *client.Client) error {
	p := newPaths(root)
	if err := os.MkdirAll(p.base, 0o755); err != nil {
		return err
	}
	if err := ensureBaseOwnership(p.base); err != nil {
		return fmt.Errorf("base ownership: %w", err)
	}

	logf("provisioning shared base at %s", p.base)

	rec := loadReceipt(p.receipt)

	if err := ensureImage(ctx, cli); err != nil {
		return fmt.Errorf("image: %w", err)
	}

	game, err := ensureGame(ctx, cli, p, rec)
	if err != nil {
		return fmt.Errorf("game: %w", err)
	}
	rec.Game = game

	mods, err := reconcileMods(p, rec)
	if err != nil {
		return fmt.Errorf("mods: %w", err)
	}
	rec.ModBundle = mods

	if err := bakeHook(p); err != nil {
		return fmt.Errorf("hook: %w", err)
	}

	if err := saveReceipt(p.receipt, rec); err != nil {
		return fmt.Errorf("receipt: %w", err)
	}
	logf("provisioning complete")
	return nil
}
