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
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// Download GETs url and writes the body to dest, retrying transient failures.
// Redirects (e.g. GitHub asset URLs) are followed by the default client.
func Download(url, dest string) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		lastErr = downloadOnce(url, dest)
		if lastErr == nil {
			return nil
		}
		log.Printf("[install] download %s failed (attempt %d/3): %v", url, attempt, lastErr)
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	return fmt.Errorf("download %s: %w", url, lastErr)
}

func downloadOnce(url, dest string) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s", resp.Status)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// FetchString GETs url and returns the trimmed body. Used for AlliedModders'
// mmsource-latest-linux pointer file.
func FetchString(url string) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: status %s", url, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

// FetchJSON GETs url and decodes the JSON body into v. Sends a GitHub token from
// $GITHUB_TOKEN when present so release lookups aren't rate-limited.
func FetchJSON(url string, v any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" && strings.Contains(url, "api.github.com") {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
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
