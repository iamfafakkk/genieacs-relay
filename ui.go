package main

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// webDistDir is the built admin UI, produced by `make web-build` (pnpm --dir web build).
const webDistDir = "web/dist"

// uiCSP replaces the API's `default-src 'none'`. The SvelteKit shell boots from an
// inline script and inline style, and the jobs page opens a same-origin WebSocket.
const uiCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'"

// backendPaths belong to the API router. An unknown endpoint under them must keep
// its JSON 404 rather than receive the SPA shell from the catch-all route.
var backendPaths = []string{
	"/api", "/health", "/healthz", "/ready", "/readyz",
	"/version", "/metrics", "/swagger",
}

func isBackendPath(p string) bool {
	for _, prefix := range backendPaths {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

// newUIHandler serves the built SPA so the admin panel lives on the same port as
// the API. Unmatched paths return index.html, letting the client router resolve
// hard reloads of deep links such as /devices/{id}.
//
// Returns nil when web/dist has not been built, so an API-only checkout still starts.
func newUIHandler(distDir string) http.Handler {
	if _, err := os.Stat(filepath.Join(distDir, "index.html")); err != nil {
		return nil
	}

	root := http.Dir(distDir)
	files := http.FileServer(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		if isBackendPath(p) {
			sendError(w, r, http.StatusNotFound, ErrCodeNotFound, nil)
			return
		}

		h := w.Header()
		h.Set("Content-Security-Policy", uiCSP)
		h.Set("Cache-Control", "no-cache")

		if f, err := root.Open(strings.TrimPrefix(p, "/")); err == nil {
			info, statErr := f.Stat()
			f.Close()
			if statErr == nil && !info.IsDir() {
				// Hashed filenames never change content, so they can be cached forever.
				if strings.HasPrefix(p, "/_app/immutable/") {
					h.Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}

		// Clone: rewriting r in place would make the access log record "/" instead
		// of the deep link the user actually requested.
		shell := r.Clone(r.Context())
		shell.URL.Path = "/"
		shell.URL.RawPath = ""
		files.ServeHTTP(w, shell)
	})
}