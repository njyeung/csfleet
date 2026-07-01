// Package install holds the shared primitives both provisioning stages use to
// pull upstream artifacts and lay them onto disk: HTTP/GitHub fetching, archive
// extraction, tree copies and atomic writes. Stage 1 (provision) reconciles the
// shared base with these; stage 2 (plugin) inserts plugins into a server overlay
// with the same toolkit.
package install

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// Download copies url's cached body to dest, fetching through Default. Big
// assets keyed by a version-stamped URL are downloaded once and reused across
// installs. Redirects (e.g. GitHub asset URLs) are followed by the client.
func Download(url, dest string) error {
	return Default.File(url, dest)
}

// FetchString GETs url (through Default) and returns the trimmed body. Used for
// AlliedModders' mmsource-latest-linux pointer file.
func FetchString(url string) (string, error) {
	return Default.String(url)
}

// FetchJSON GETs url (through Default) and decodes the JSON body into v.
func FetchJSON(url string, v any) error {
	return Default.JSON(url, v)
}

// File copies the cached body for url to dest, hardlinking when possible so a
// multi-gigabyte asset isn't duplicated on the same filesystem.
func (c *Cache) File(url, dest string) error {
	path, err := c.Get(url)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	os.Remove(dest)
	if os.Link(path, dest) == nil {
		return nil
	}
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// String returns the cached body for url, trimmed.
func (c *Cache) String(url string) (string, error) {
	path, err := c.Get(url)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// JSON decodes the cached body for url into v.
func (c *Cache) JSON(url string, v any) error {
	path, err := c.Get(url)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(v)
}

// Release is a GitHub release and its downloadable assets.
type Release struct {
	TagName string         `json:"tag_name"`
	Assets  []ReleaseAsset `json:"assets"`
}

// ReleaseAsset is one downloadable file attached to a release.
type ReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// GithubLatestRelease returns the latest release for an "owner/repo".
func GithubLatestRelease(repo string) (Release, error) {
	var rel Release
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	if err := FetchJSON(url, &rel); err != nil {
		return rel, fmt.Errorf("latest release for %s: %w", repo, err)
	}
	if rel.TagName == "" {
		return rel, fmt.Errorf("latest release for %s: no tag_name", repo)
	}
	return rel, nil
}

// GithubReleaseByTag returns a specific tagged release — used when a manifest
// pins a version instead of tracking latest.
func GithubReleaseByTag(repo, tag string) (Release, error) {
	var rel Release
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, tag)
	if err := FetchJSON(url, &rel); err != nil {
		return rel, fmt.Errorf("release %s for %s: %w", tag, repo, err)
	}
	if rel.TagName == "" {
		return rel, fmt.Errorf("release %s for %s: no tag_name", tag, repo)
	}
	return rel, nil
}

// AssetURL returns the download URL of the first asset whose name contains match
// (so we don't have to hardcode version-stamped filenames).
func (r Release) AssetURL(match string) (string, error) {
	for _, a := range r.Assets {
		if strings.Contains(a.Name, match) {
			return a.URL, nil
		}
	}
	return "", fmt.Errorf("no asset matching %q in release %s", match, r.TagName)
}
