package provision

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

const (
	mmsourceBase   = "https://mms.alliedmods.net/mmsdrop/2.0/"
	mmsourceLatest = mmsourceBase + "mmsource-latest-linux"
	cssRepo        = "roflmuffin/CounterStrikeSharp"
	wpRepo         = "Nereziel/cs2-WeaponPaints"
)

// WeaponPaints' runtime dependencies, by NickFox007. Each ships one GitHub
// release asset that unzips with an addons/ prefix.
var depRepos = []struct{ key, repo, asset string }{
	{"AnyBaseLib", "NickFox007/AnyBaseLibCS2", "AnyBaseLib.zip"},
	{"PlayerSettings", "NickFox007/PlayerSettingsCS2", "PlayerSettings.zip"},
	{"MenuManager", "NickFox007/MenuManagerCS2", "MenuManager.zip"},
}

// modSources is the resolved set of latest mod downloads: the version we'd
// record in the receipt plus the URL to fetch it from.
type modSources struct {
	metamodFile, metamodURL string
	cssTag, cssURL          string
	wpTag, wpURL            string
	deps                    []struct{ key, tag, url string }
}

func (m modSources) bundle() modBundle {
	deps := make(map[string]string, len(m.deps))
	for _, d := range m.deps {
		deps[d.key] = d.tag
	}
	return modBundle{
		MetaMod:      m.metamodFile,
		CSS:          m.cssTag,
		WeaponPaints: m.wpTag,
		Deps:         deps,
	}
}

// reconcileMods treats the mod bundle as one unit: MetaMod, CSS, WeaponPaints
// and the deps all unzip into the shared addons/ tree with overlapping paths, so
// a partial update risks stale files. If any version drifts from the receipt (or
// addons/ is missing) we clear addons/ and re-lay the whole bundle.
func reconcileMods(p paths, rec receipt) (modBundle, error) {
	src, err := resolveMods()
	if err != nil {
		return rec.ModBundle, err
	}
	want := src.bundle()

	if _, statErr := os.Stat(filepath.Join(p.gameCSGO, "addons")); statErr == nil &&
		reflect.DeepEqual(want, rec.ModBundle) {
		logf("mod bundle up to date (css %s, weaponpaints %s)", want.CSS, want.WeaponPaints)
		return rec.ModBundle, nil
	}

	logf("rebuilding mod bundle")
	if err := buildBundle(p, src); err != nil {
		return rec.ModBundle, err
	}
	return want, nil
}

func resolveMods() (modSources, error) {
	var m modSources

	file, err := fetchString(mmsourceLatest)
	if err != nil {
		return m, fmt.Errorf("metamod latest: %w", err)
	}
	m.metamodFile, m.metamodURL = file, mmsourceBase+file

	css, err := githubLatestRelease(cssRepo)
	if err != nil {
		return m, err
	}
	if m.cssURL, err = css.assetURL("with-runtime-linux"); err != nil {
		return m, err
	}
	m.cssTag = css.TagName

	wp, err := githubLatestRelease(wpRepo)
	if err != nil {
		return m, err
	}
	if m.wpURL, err = wp.assetURL("WeaponPaints.zip"); err != nil {
		return m, err
	}
	m.wpTag = wp.TagName

	for _, d := range depRepos {
		rel, err := githubLatestRelease(d.repo)
		if err != nil {
			return m, err
		}
		url, err := rel.assetURL(d.asset)
		if err != nil {
			return m, err
		}
		m.deps = append(m.deps, struct{ key, tag, url string }{d.key, rel.TagName, url})
	}
	return m, nil
}

// buildBundle clears addons/ and
// re-lays every mod, leaving the rest of the game install untouched.
func buildBundle(p paths, src modSources) error {
	if err := os.RemoveAll(filepath.Join(p.gameCSGO, "addons")); err != nil {
		return err
	}
	for _, d := range []string{
		p.pluginsDst,
		filepath.Join(p.cssDir, "gamedata"),
		filepath.Join(p.cssDir, "configs"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	work, err := os.MkdirTemp("", "csfleet-mods-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	// MetaMod, CSS and the deps all unpack with an addons/ prefix into csgo.
	logf("MetaMod %s", src.metamodFile)
	if err := fetchExtractTarGz(src.metamodURL, p.gameCSGO, filepath.Join(work, "metamod.tar.gz")); err != nil {
		return err
	}
	logf("CounterStrikeSharp %s", src.cssTag)
	if err := fetchExtractZip(src.cssURL, p.gameCSGO, filepath.Join(work, "css.zip")); err != nil {
		return err
	}
	for _, d := range src.deps {
		logf("plugin %s %s", d.key, d.tag)
		if err := fetchExtractZip(d.url, p.gameCSGO, filepath.Join(work, d.key+".zip")); err != nil {
			return err
		}
	}

	// WeaponPaints is a bare zip: its root is the plugin folder itself plus
	// gamedata/ and configs/ with NO addons/ prefix, so we place its parts by hand.
	logf("WeaponPaints %s", src.wpTag)
	wpEx := filepath.Join(work, "weaponpaints.d")
	if err := fetchExtractZip(src.wpURL, wpEx, filepath.Join(work, "weaponpaints.zip")); err != nil {
		return err
	}
	if err := layWeaponPaints(p, wpEx); err != nil {
		return err
	}

	return bakeCoreJSON(p)
}

func fetchExtractZip(url, dest, tmp string) error {
	if err := download(url, tmp); err != nil {
		return err
	}
	return extractZip(tmp, dest)
}

func fetchExtractTarGz(url, dest, tmp string) error {
	if err := download(url, tmp); err != nil {
		return err
	}
	return extractTarGz(tmp, dest)
}

func layWeaponPaints(p paths, wpEx string) error {
	for _, name := range []string{"gamedata", "configs"} {
		src := filepath.Join(wpEx, name)
		if _, err := os.Stat(src); err == nil {
			if err := copyTree(src, filepath.Join(p.cssDir, name)); err != nil {
				return err
			}
		}
	}
	entries, err := os.ReadDir(wpEx)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		switch e.Name() {
		case "gamedata", "configs", "lang":
			continue
		}
		if err := copyTree(filepath.Join(wpEx, e.Name()), filepath.Join(p.pluginsDst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// bakeCoreJSON writes CounterStrikeSharp's core.json host-side, disabling CS2
// server guidelines so arbitrary paint/float/seed work. CSS would otherwise
// generate this on first load; baking it means pre.sh needs no in-container jq.
func bakeCoreJSON(p paths) error {
	dest := filepath.Join(p.cssDir, "configs", "core.json")
	logf("writing core.json (FollowCS2ServerGuidelines=false)")
	const core = `{
  "PublicChatTrigger": [ "!" ],
  "SilentChatTrigger": [ "/" ],
  "FollowCS2ServerGuidelines": false,
  "ServerLanguage": "en"
}
`
	return atomicWrite(dest, []byte(core))
}
