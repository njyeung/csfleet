package provision

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	skinsRepo = "ByMykel/CSGO-API"
	skinsPath = "public/api/en/skins.json"
	skinsURL  = "https://raw.githubusercontent.com/" + skinsRepo + "/main/" + skinsPath
)

// reconcileSkins refreshes the InspectGive skin-name lookup when CSGO-API's
// skins.json has changed since we last built it. The freshness key is the file's
// latest commit SHA.
func reconcileSkins(p paths, rec receipt) (skinsReceipt, error) {
	dest := filepath.Join(p.pluginSrc, "SkinNames.json")

	sha, err := githubFileCommit(skinsRepo, skinsPath)
	if err != nil {
		return rec.Skins, fmt.Errorf("resolve skins commit: %w", err)
	}

	if _, statErr := os.Stat(dest); statErr == nil && sha == rec.Skins.Commit {
		logf("skin data up to date (%s)", short(sha))
		return rec.Skins, nil
	}

	logf("refreshing skin-name lookup from CSGO-API (%s)", short(sha))
	if err := buildSkinNames(dest); err != nil {
		return rec.Skins, err
	}
	return skinsReceipt{Commit: sha}, nil
}

// buildSkinNames downloads skins.json and flattens it to a compact
// "weaponid_paintindex" -> name lookup embedded into the plugin DLL.
func buildSkinNames(dest string) error {
	tmp, err := os.CreateTemp("", "csfleet-skins-*.json")
	if err != nil {
		return err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if err := download(skinsURL, tmp.Name()); err != nil {
		return err
	}

	raw, err := os.ReadFile(tmp.Name())
	if err != nil {
		return err
	}
	var skins []struct {
		Weapon struct {
			WeaponID json.Number `json:"weapon_id"`
		} `json:"weapon"`
		PaintIndex json.Number `json:"paint_index"`
		Name       string      `json:"name"`
	}
	if err := json.Unmarshal(raw, &skins); err != nil {
		return err
	}

	lookup := make(map[string]string, len(skins))
	for _, s := range skins {
		lookup[s.Weapon.WeaponID.String()+"_"+s.PaintIndex.String()] = s.Name
	}
	if len(lookup) == 0 {
		return fmt.Errorf("no skins parsed from %s", skinsURL)
	}

	out, err := json.MarshalIndent(lookup, "", "  ")
	if err != nil {
		return err
	}
	logf("  -> %d skins", len(lookup))
	return atomicWrite(dest, out)
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
