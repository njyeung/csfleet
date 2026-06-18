package provision

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// download GETs url and writes the body to dest, retrying transient failures.
// Replaces the old curl shell-out; redirects (e.g. GitHub asset URLs) are
// followed by the default client.
func download(url, dest string) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		lastErr = downloadOnce(url, dest)
		if lastErr == nil {
			return nil
		}
		logf("download %s failed (attempt %d/3): %v", url, attempt, lastErr)
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

// fetchString GETs url and returns the trimmed body. Used for AlliedModders'
// mmsource-latest-linux pointer file.
func fetchString(url string) (string, error) {
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

// fetchJSON GETs url and decodes the JSON body into v. Sends a GitHub token
// from $GITHUB_TOKEN when present so release lookups aren't rate-limited.
func fetchJSON(url string, v any) error {
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

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// githubLatestRelease returns the latest release's tag and its assets for a
// "owner/repo".
func githubLatestRelease(repo string) (ghRelease, error) {
	var rel ghRelease
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	if err := fetchJSON(url, &rel); err != nil {
		return rel, fmt.Errorf("latest release for %s: %w", repo, err)
	}
	if rel.TagName == "" {
		return rel, fmt.Errorf("latest release for %s: no tag_name", repo)
	}
	return rel, nil
}

// assetURL returns the download URL of the first asset whose name contains
// match (so we don't have to hardcode version-stamped filenames).
func (r ghRelease) assetURL(match string) (string, error) {
	for _, a := range r.Assets {
		if strings.Contains(a.Name, match) {
			return a.URL, nil
		}
	}
	return "", fmt.Errorf("no asset matching %q in release %s", match, r.TagName)
}

// githubFileCommit returns the SHA of the latest commit touching path in repo's
// default branch — our freshness key for the CSGO-API skin data.
func githubFileCommit(repo, path string) (string, error) {
	var commits []struct {
		SHA string `json:"sha"`
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/commits?path=%s&per_page=1", repo, path)
	if err := fetchJSON(url, &commits); err != nil {
		return "", err
	}
	if len(commits) == 0 {
		return "", fmt.Errorf("no commits for %s in %s", path, repo)
	}
	return commits[0].SHA, nil
}
