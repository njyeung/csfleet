// Package plugin is stage 2 of building a server's filesystem: inserting plugins
// into a single server's overlay.
//
// A manifest is a plugin recipe (*.toml, see plugins/README.md). It is passed
// as a string: template bodies are inline (TemplateRule.Body).
package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"csfleet/orchestrator/internal/install"

	"github.com/pelletier/go-toml/v2"
)

// Manifest is a decoded plugin recipe. Fields mirror plugins/README.md.
//
// A manifest carries no name of its own: a plugin's identity is its catalog key
// (the name it's stored under in csfleet_plugin_manifests, also what requires references).
// The caller passes that key in for diagnostics.
type Manifest struct {
	Requires []string       `toml:"requires"`
	Ignore   []string       `toml:"ignore"`
	Source   Source         `toml:"source"`
	Layout   []LayoutRule   `toml:"layout"`
	Template []TemplateRule `toml:"template"`
}

// Source is where the plugin's bytes come from.
type Source struct {
	Type    string `toml:"type"`    // github_release | url | local
	Repo    string `toml:"repo"`    // owner/name (github_release)
	Asset   string `toml:"asset"`   // regex selecting the asset (github_release)
	URL     string `toml:"url"`     // direct download URL (url)
	Version string `toml:"version"` // "" / "latest", or a pinned tag (github_release)
	Path    string `toml:"path"`    // directory to copy from (local)
}

// LayoutRule copies the contents of From (a glob under the source root) into To
// (a dir relative to game/csgo/).
type LayoutRule struct {
	From string `toml:"from"`
	To   string `toml:"to"`
}

// TemplateRule renders one config file into the overlay at Path (relative to
// game/csgo/). Body is the file contents as an inline TOML multiline literal;
// any ${key} in it is substituted with the matching env variable (e.g.
// ${db.host}, ${CS2_PW}).
type TemplateRule struct {
	Body string `toml:"body"`
	Path string `toml:"path"`
}

// ParseManifest decodes a manifest from TOML text.
func ParseManifest(tomlText string) (Manifest, error) {
	var m Manifest
	if err := toml.Unmarshal([]byte(tomlText), &m); err != nil {
		return m, fmt.Errorf("parse manifest: %w", err)
	}
	return m, nil
}

// Apply installs the plugin described by manifestTOML into the overlay whose
// game/csgo/ is overlayCSGO. name is the plugin's catalog key, used only for
// diagnostics.
func Apply(overlayCSGO, name, manifestTOML string, env map[string]string) error {
	m, err := ParseManifest(manifestTOML)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return m.applyTo(name, overlayCSGO, env)
}

// applyTo is Apply for an already-parsed manifest. name is the catalog key for
// diagnostics.
func (m Manifest) applyTo(name, overlayCSGO string, env map[string]string) error {
	root, cleanup, err := m.fetchSource()
	if err != nil {
		return fmt.Errorf("%s: source: %w", name, err)
	}
	defer cleanup()

	if err := applyIgnore(root, m.Ignore); err != nil {
		return fmt.Errorf("%s: ignore: %w", name, err)
	}
	if err := m.layout(root, overlayCSGO); err != nil {
		return fmt.Errorf("%s: layout: %w", name, err)
	}
	if err := m.templates(overlayCSGO, env); err != nil {
		return fmt.Errorf("%s: template: %w", name, err)
	}
	return nil
}

// fetchSource resolves the source into a local directory whose tree is laid out
// as the archive root (so layout rules resolve the same way for every source
// type). cleanup removes any temp dir; it's a no-op for a local source.
func (m Manifest) fetchSource() (root string, cleanup func(), err error) {
	noop := func() {}
	switch m.Source.Type {
	case "local":
		if m.Source.Path == "" {
			return "", noop, fmt.Errorf("local source needs a path")
		}
		if !filepath.IsAbs(m.Source.Path) {
			return "", noop, fmt.Errorf("local source path %q must be absolute", m.Source.Path)
		}
		return m.Source.Path, noop, nil

	case "github_release":
		url, name, err := m.releaseAsset()
		if err != nil {
			return "", noop, err
		}
		return downloadExtract(url, name)

	case "url":
		if m.Source.URL == "" {
			return "", noop, fmt.Errorf("url source needs a url")
		}
		return downloadExtract(m.Source.URL, m.Source.URL)

	default:
		return "", noop, fmt.Errorf("unknown source type %q", m.Source.Type)
	}
}

func (m Manifest) releaseAsset() (url, name string, err error) {
	var rel install.Release
	if v := m.Source.Version; v != "" && v != "latest" {
		rel, err = install.GithubReleaseByTag(m.Source.Repo, v)
	} else {
		rel, err = install.GithubLatestRelease(m.Source.Repo)
	}
	if err != nil {
		return "", "", err
	}
	re, err := regexp.Compile(m.Source.Asset)
	if err != nil {
		return "", "", fmt.Errorf("asset regex %q: %w", m.Source.Asset, err)
	}
	for _, a := range rel.Assets {
		if re.MatchString(a.Name) {
			return a.URL, a.Name, nil
		}
	}
	return "", "", fmt.Errorf("no asset matching %q in %s release %s", m.Source.Asset, m.Source.Repo, rel.TagName)
}

// downloadExtract fetches an archive and unpacks it into a fresh temp dir, whose
// path is returned as the source root. name only picks the archive format.
func downloadExtract(url, name string) (string, func(), error) {
	work, err := os.MkdirTemp("", "csfleet-plugin-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(work) }

	archive := filepath.Join(work, "archive")
	if err := install.Download(url, archive); err != nil {
		cleanup()
		return "", func() {}, err
	}

	root := filepath.Join(work, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") {
		err = install.ExtractTarGz(archive, root)
	} else {
		err = install.ExtractZip(archive, root)
	}
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	return root, cleanup, nil
}

// applyIgnore deletes entries under root matching any glob (matched against
// slash-separated, root-relative paths). Supports * (within a segment), ? and **
// (any depth); a trailing /** also matches the directory itself.
func applyIgnore(root string, patterns []string) error {
	if len(patterns) == 0 {
		return nil
	}
	res := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := globToRegexp(p)
		if err != nil {
			return fmt.Errorf("ignore %q: %w", p, err)
		}
		res = append(res, re)
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel := filepath.ToSlash(mustRel(root, path))
		for _, re := range res {
			if re.MatchString(rel) {
				if err := os.RemoveAll(path); err != nil {
					return err
				}
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		return nil
	})
}

func mustRel(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

// globToRegexp compiles a path glob to an anchored regexp. ** matches across
// slashes, * stays within a segment, ? is a single non-slash char. A trailing
// /** is optional so it matches the named directory as well as its contents.
func globToRegexp(glob string) (*regexp.Regexp, error) {
	suffix := "$"
	if rest, ok := strings.CutSuffix(glob, "/**"); ok {
		glob = rest
		suffix = "(/.*)?$"
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString(suffix)
	return regexp.Compile(b.String())
}

// layout copies the source tree into the overlay per the manifest's rules,
// defaulting to "extract straight into csgo/" when none are given.
func (m Manifest) layout(root, overlayCSGO string) error {
	rules := m.Layout
	if len(rules) == 0 {
		rules = []LayoutRule{{From: ".", To: "."}}
	}
	for _, r := range rules {
		from, to := r.From, r.To
		if from == "" {
			from = "."
		}
		if to == "" {
			to = "."
		}
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(from)))
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return fmt.Errorf("no source dir matches %q", from)
		}
		dest := filepath.Join(overlayCSGO, filepath.FromSlash(to))
		for _, src := range matches {
			if err := install.CopyTree(src, dest); err != nil {
				return err
			}
		}
	}
	return nil
}

// templates renders each [[template]] into the overlay, substituting ${key}
// with the matching env variable.
func (m Manifest) templates(overlayCSGO string, env map[string]string) error {
	rep := envReplacer(env)
	for _, t := range m.Template {
		if t.Path == "" {
			return fmt.Errorf("template needs a path")
		}
		if t.Body == "" {
			return fmt.Errorf("template %q needs a body", t.Path)
		}
		dest := filepath.Join(overlayCSGO, filepath.FromSlash(t.Path))
		if err := install.AtomicWrite(dest, []byte(rep.Replace(t.Body))); err != nil {
			return err
		}
	}
	return nil
}

// envReplacer substitutes ${key} with its value for every env variable.
func envReplacer(env map[string]string) *strings.Replacer {
	pairs := make([]string, 0, len(env)*2)
	for k, v := range env {
		pairs = append(pairs, "${"+k+"}", v)
	}
	return strings.NewReplacer(pairs...)
}
