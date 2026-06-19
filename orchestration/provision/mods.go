package provision

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"csfleet/orchestrator/internal/install"
)

const (
	mmsourceBase   = "https://mms.alliedmods.net/mmsdrop/2.0/"
	mmsourceLatest = mmsourceBase + "mmsource-latest-linux"
	cssRepo        = "roflmuffin/CounterStrikeSharp"
)

// modSources is the resolved set of latest mod downloads: the version we'd
// record in the receipt plus the URL to fetch it from.
type modSources struct {
	metamodFile, metamodURL string
	cssTag, cssURL          string
}

func (m modSources) bundle() modBundle {
	return modBundle{MetaMod: m.metamodFile, CSS: m.cssTag}
}

// reconcileMods lays the shared base mod layer: MetaMod + CounterStrikeSharp.
// Both unzip into the shared addons/ tree with overlapping paths, so a partial
// update risks stale files — if either version drifts from the receipt (or
// addons/ is missing) we clear addons/ and re-lay both. Plugins are not part of
// this layer; they go into each server's overlay per-instance (see ApplyManifest).
func reconcileMods(p paths, rec receipt) (modBundle, error) {
	src, err := resolveMods()
	if err != nil {
		return rec.ModBundle, err
	}
	want := src.bundle()

	if _, statErr := os.Stat(filepath.Join(p.gameCSGO, "addons")); statErr == nil &&
		reflect.DeepEqual(want, rec.ModBundle) {
		logf("base mods up to date (metamod %s, css %s)", want.MetaMod, want.CSS)
		return rec.ModBundle, nil
	}

	logf("rebuilding base mods")
	if err := buildBundle(p, src); err != nil {
		return rec.ModBundle, err
	}
	return want, nil
}

func resolveMods() (modSources, error) {
	var m modSources

	file, err := install.FetchString(mmsourceLatest)
	if err != nil {
		return m, fmt.Errorf("metamod latest: %w", err)
	}
	m.metamodFile, m.metamodURL = file, mmsourceBase+file

	css, err := install.GithubLatestRelease(cssRepo)
	if err != nil {
		return m, err
	}
	if m.cssURL, err = css.AssetURL("with-runtime-linux"); err != nil {
		return m, err
	}
	m.cssTag = css.TagName

	return m, nil
}

// buildBundle clears addons/ and re-lays MetaMod + CSS, leaving the rest of the
// game install untouched. Both unpack with an addons/ prefix into csgo.
func buildBundle(p paths, src modSources) error {
	if err := os.RemoveAll(filepath.Join(p.gameCSGO, "addons")); err != nil {
		return err
	}

	work, err := os.MkdirTemp("", "csfleet-mods-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	logf("MetaMod %s", src.metamodFile)
	if err := fetchExtractTarGz(src.metamodURL, p.gameCSGO, filepath.Join(work, "metamod.tar.gz")); err != nil {
		return err
	}
	logf("CounterStrikeSharp %s", src.cssTag)
	if err := fetchExtractZip(src.cssURL, p.gameCSGO, filepath.Join(work, "css.zip")); err != nil {
		return err
	}

	return bakeCoreJSON(p)
}

func fetchExtractZip(url, dest, tmp string) error {
	if err := install.Download(url, tmp); err != nil {
		return err
	}
	return install.ExtractZip(tmp, dest)
}

func fetchExtractTarGz(url, dest, tmp string) error {
	if err := install.Download(url, tmp); err != nil {
		return err
	}
	return install.ExtractTarGz(tmp, dest)
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
	return install.AtomicWrite(dest, []byte(core))
}
