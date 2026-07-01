package provision

import "path/filepath"

// Filesystem layout provisioning works against. Everything hangs off the repo
// root; base/ is the single shared, read-only CS2 install every server overlays.
//
//	<root>/hooks/pre.sh               -> optional boot hook override
//
//	<root>/base/                      -> the cs2-dedicated install root (the big
//	                                     shared overlay lowerdir). steamcmd writes
//	                                     here; the base mods (MetaMod + CSS) are
//	                                     baked in here too.
//	<root>/base/game/csgo/            -> the game's csgo dir (addons live under here)
//	<root>/base/steamapps/...         -> steam appmanifests
//	<root>/base/.csfleet-versions.json-> the install receipt (what we installed)
//	<root>/base/pre.sh                -> baked in-container boot hook
type paths struct {
	root        string
	base        string // == /home/steam/cs2-dedicated inside the container
	gameCSGO    string // base/game/csgo
	cssDir      string // gameCSGO/addons/counterstrikesharp
	appManifest string // base/steamapps/appmanifest_730.acf
	preHook     string // optional hooks/pre.sh override
	bakedHook   string // base/pre.sh
	receipt     string // base/.csfleet-versions.json
}

func newPaths(root string) paths {
	base := filepath.Join(root, "base")
	gameCSGO := filepath.Join(base, "game", "csgo")
	cssDir := filepath.Join(gameCSGO, "addons", "counterstrikesharp")
	return paths{
		root:        root,
		base:        base,
		gameCSGO:    gameCSGO,
		cssDir:      cssDir,
		appManifest: filepath.Join(base, "steamapps", "appmanifest_730.acf"),
		preHook:     filepath.Join(root, "hooks", "pre.sh"),
		bakedHook:   filepath.Join(base, "pre.sh"),
		receipt:     filepath.Join(base, ".csfleet-versions.json"),
	}
}
