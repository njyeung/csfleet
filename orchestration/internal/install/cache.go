package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"
)

// cacheTTL is how long a cached response is served before we revalidate. Past
// it we make a conditional request; a 304 refreshes the entry without
// re-downloading (and GitHub does not count 304s against the API rate limit,
// so this is what keeps release lookups from hitting the 60/hr limit). Entries
// without an ETag are refetched on expiry — fine, they rarely change and it
// keeps callers from having to reason about per-URL freshness.
const cacheTTL = 60 * time.Minute

// cacheMaxDefault bounds total blob bytes; least-recently-used entries are
// evicted past it. Override with CSFLEET_CACHE_MAX (bytes).
const cacheMaxDefault = 5 << 30 // 5 GiB

// Default is the process-wide cache the package-level Download/FetchJSON/
// FetchString helpers fetch through.
var Default = NewCache(cacheDirFromEnv(""), cacheMaxDefault)

// Cache is a disk-backed HTTP GET cache keyed by URL. Small JSON/text and large
// archives share one store: each entry is a <sha256(url)>.blob body plus a
// .meta sidecar holding the validators (ETag/Last-Modified) and fetch time.
type Cache struct {
	dir       string
	ttl       time.Duration
	maxBytes  int64
	startedAt time.Time
	group     singleflight.Group
}

// NewCache returns a cache rooted at dir that keeps total blob size under
// maxBytes. Entries left by earlier runs are revalidated on first use this run
// (see fetch), so restarting the orchestrator re-checks every upstream for
// updates — matching the provisioning step, which re-checks versions rather
// than rebuilding blindly, without re-downloading assets that haven't changed.
func NewCache(dir string, maxBytes int64) *Cache {
	return &Cache{dir: dir, ttl: cacheTTL, maxBytes: maxBytes, startedAt: time.Now()}
}

// cacheMeta is the .meta sidecar for one cached response.
type cacheMeta struct {
	URL          string    `json:"url"`
	FetchedAt    time.Time `json:"fetched_at"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	Size         int64     `json:"size"`
}

// Get returns the path to a locally cached copy of url's body, fetching or
// revalidating it as freshness requires. Concurrent calls for the same url
// share a single fetch, so parallel installs never double-download an asset.
func (c *Cache) Get(url string) (string, error) {
	v, err, _ := c.group.Do(url, func() (any, error) { return c.fetch(url) })
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (c *Cache) fetch(url string) (string, error) {
	blob, metaPath := c.paths(url)
	meta, haveMeta := c.readMeta(metaPath)

	// Fresh hit: serve without touching the network, bumping the blob's mtime
	// so LRU eviction treats it as recently used. Entries fetched by an earlier
	// run (before startedAt) are never fresh, so the first use each run makes a
	// conditional request — picking up new releases while a 304 keeps the blob.
	if haveMeta && meta.FetchedAt.After(c.startedAt) && time.Since(meta.FetchedAt) < c.ttl {
		if _, err := os.Stat(blob); err == nil {
			touch(blob)
			return blob, nil
		}
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if strings.Contains(url, "api.github.com") {
		req.Header.Set("Accept", "application/vnd.github+json")
		if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	if haveMeta {
		if meta.ETag != "" {
			req.Header.Set("If-None-Match", meta.ETag)
		}
		if meta.LastModified != "" {
			req.Header.Set("If-Modified-Since", meta.LastModified)
		}
	}

	resp, err := c.do(req)
	if err != nil {
		if blobUsable(blob, haveMeta) {
			log.Printf("[install] cache: %s fetch failed (%v); serving stale", url, err)
			return blob, nil
		}
		return "", err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotModified && haveMeta:
		meta.FetchedAt = time.Now()
		c.writeMeta(metaPath, meta)
		touch(blob)
		return blob, nil
	case resp.StatusCode == http.StatusOK:
		return c.store(url, blob, metaPath, resp)
	default:
		if blobUsable(blob, haveMeta) {
			log.Printf("[install] cache: %s returned %s; serving stale", url, resp.Status)
			return blob, nil
		}
		return "", statusError(url, resp)
	}
}

// do issues req, retrying transient transport failures a few times.
func (c *Cache) do(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err = httpClient.Do(req)
		if err == nil {
			return resp, nil
		}
		log.Printf("[install] fetch %s failed (attempt %d/3): %v", req.URL, attempt, err)
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	return nil, err
}

// store streams a 200 response body to the blob and records its validators.
func (c *Cache) store(url, blob, metaPath string, resp *http.Response) (string, error) {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(c.dir, ".dl-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	size, err := io.Copy(tmp, resp.Body)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, blob); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	c.writeMeta(metaPath, cacheMeta{
		URL:          url,
		FetchedAt:    time.Now(),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		Size:         size,
	})
	c.evict()
	return blob, nil
}

// evict removes least-recently-used blobs until total size fits maxBytes.
func (c *Cache) evict() {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type blob struct {
		path string
		size int64
		mod  time.Time
	}
	var blobs []blob
	var total int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".blob") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		blobs = append(blobs, blob{filepath.Join(c.dir, e.Name()), info.Size(), info.ModTime()})
		total += info.Size()
	}
	if total <= c.maxBytes {
		return
	}
	sort.Slice(blobs, func(i, j int) bool { return blobs[i].mod.Before(blobs[j].mod) })
	for _, b := range blobs {
		if total <= c.maxBytes {
			return
		}
		os.Remove(b.path)
		os.Remove(strings.TrimSuffix(b.path, ".blob") + ".meta")
		total -= b.size
		log.Printf("[install] cache: evicted %s (%d bytes)", filepath.Base(b.path), b.size)
	}
}

func (c *Cache) paths(url string) (blob, meta string) {
	sum := sha256.Sum256([]byte(url))
	base := filepath.Join(c.dir, hex.EncodeToString(sum[:]))
	return base + ".blob", base + ".meta"
}

func (c *Cache) readMeta(path string) (cacheMeta, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cacheMeta{}, false
	}
	var m cacheMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return cacheMeta{}, false
	}
	return m, true
}

func (c *Cache) writeMeta(path string, m cacheMeta) {
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	if err := AtomicWrite(path, data); err != nil {
		log.Printf("[install] cache: write meta %s: %v", path, err)
	}
}

// blobUsable reports whether we can fall back to the on-disk blob.
func blobUsable(blob string, haveMeta bool) bool {
	if !haveMeta {
		return false
	}
	_, err := os.Stat(blob)
	return err == nil
}

func touch(path string) {
	now := time.Now()
	os.Chtimes(path, now, now)
}

// statusError turns a non-2xx GitHub response into a diagnosable error,
// unmasking a rate-limit 403 (which looks like a permission error otherwise).
func statusError(url string, resp *http.Response) error {
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		reset := resp.Header.Get("X-RateLimit-Reset")
		if os.Getenv("GITHUB_TOKEN") == "" {
			return fmt.Errorf("GET %s: github rate limit exceeded (unauthenticated 60/hr/IP; resets at epoch %s); set $GITHUB_TOKEN to raise it to 5000/hr", url, reset)
		}
		return fmt.Errorf("GET %s: github rate limit exceeded (resets at epoch %s)", url, reset)
	}
	return fmt.Errorf("GET %s: status %s", url, resp.Status)
}

// ConfigureCache points Default at <root>/cache/assets (unless
// CSFLEET_CACHE_DIR overrides). Call once at startup, before any fetch, so the
// download cache lives inside the repo like base/ and instances/ rather than a
// machine-wide path.
func ConfigureCache(root string) {
	Default = NewCache(cacheDirFromEnv(root), cacheMaxDefault)
}

func cacheDirFromEnv(root string) string {
	if d := os.Getenv("CSFLEET_CACHE_DIR"); d != "" {
		return d
	}
	return filepath.Join(root, "cache", "assets")
}
