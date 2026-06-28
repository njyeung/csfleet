package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// spaFileServer serves the built SvelteKit SPA from dir. A request for an
// existing file returns that file; anything else falls back to index.html so the
// client-side router can take over. The /api/* routes are registered as more
// specific patterns, so they never reach this catch-all.
func spaFileServer(dir string) http.HandlerFunc {
	root := filepath.Clean(dir)
	index := filepath.Join(root, "index.html")
	return func(w http.ResponseWriter, r *http.Request) {
		// Resolve the request path under root and reject any traversal escape.
		p := filepath.Join(root, filepath.Clean("/"+r.URL.Path))
		if p != root && !strings.HasPrefix(p, root+string(os.PathSeparator)) {
			http.NotFound(w, r)
			return
		}
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			http.ServeFile(w, r, p)
			return
		}
		http.ServeFile(w, r, index) // SPA fallback
	}
}
