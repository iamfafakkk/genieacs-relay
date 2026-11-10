package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uiFixture writes a minimal dist tree plus a sibling file that must never be served.
func uiFixture(t *testing.T) (dist string, parent string) {
	t.Helper()
	parent = t.TempDir()
	dist = filepath.Join(parent, "dist")
	require.NoError(t, os.MkdirAll(filepath.Join(dist, "_app", "immutable"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dist, "index.html"), []byte("<html>shell</html>"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dist, "robots.txt"), []byte("User-agent: *"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dist, "_app", "immutable", "app.abc123.js"), []byte("console.log(1)"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("TOP SECRET"), 0o644))
	return dist, parent
}

func TestNewUIHandler_MissingDist(t *testing.T) {
	assert.Nil(t, newUIHandler(filepath.Join(t.TempDir(), "dist")))
}

func TestNewUIHandler_ServesFilesAndFallsBack(t *testing.T) {
	dist, _ := uiFixture(t)
	h := newUIHandler(dist)
	require.NotNil(t, h)

	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantBody    string
		wantCache   string
		wantJSON404 bool
	}{
		{"root serves shell", "/", http.StatusOK, "<html>shell</html>", "no-cache", false},
		{"real file served verbatim", "/robots.txt", http.StatusOK, "User-agent: *", "no-cache", false},
		{"hashed asset is immutable", "/_app/immutable/app.abc123.js", http.StatusOK, "console.log(1)", "public, max-age=31536000, immutable", false},
		{"deep link falls back to shell", "/devices/ABC123", http.StatusOK, "<html>shell</html>", "no-cache", false},
		{"login falls back to shell", "/login", http.StatusOK, "<html>shell</html>", "no-cache", false},
		{"nested unknown falls back to shell", "/devices/ABC123/wlan", http.StatusOK, "<html>shell</html>", "no-cache", false},
		{"directory falls back to shell", "/_app/immutable/", http.StatusOK, "<html>shell</html>", "no-cache", false},
		{"unknown api stays JSON 404", "/api/v1/genieacs/nope", http.StatusNotFound, "", "no-cache", true},
		{"api root stays JSON 404", "/api", http.StatusNotFound, "", "no-cache", true},
		{"unknown probe stays JSON 404", "/healthz/extra", http.StatusNotFound, "", "no-cache", true},
		{"metrics subpath stays JSON 404", "/metrics/foo", http.StatusNotFound, "", "no-cache", true},
		{"swagger subpath stays JSON 404", "/swagger/nope", http.StatusNotFound, "", "no-cache", true},
		{"version subpath stays JSON 404", "/version/foo", http.StatusNotFound, "", "no-cache", true},
		{"traversal cannot escape dist", "/../secret.txt", http.StatusOK, "<html>shell</html>", "no-cache", false},
		{"dotdot inside path is cleaned", "/devices/../../secret.txt", http.StatusOK, "<html>shell</html>", "no-cache", false},
		{"lookalike ui route is not backend", "/healthcare", http.StatusOK, "<html>shell</html>", "no-cache", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantJSON404 {
				// Backend 404s keep the strict API headers from
				// securityHeadersMiddleware; the handler must not restyle them.
				assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
				assert.Contains(t, rec.Body.String(), ErrCodeNotFound)
				assert.NotContains(t, rec.Body.String(), "<html>")
				return
			}

			assert.Equal(t, tt.wantCache, rec.Header().Get("Cache-Control"))
			assert.Equal(t, uiCSP, rec.Header().Get("Content-Security-Policy"))
			assert.Equal(t, tt.wantBody, rec.Body.String())
			assert.NotContains(t, rec.Body.String(), "TOP SECRET")
		})
	}
}

func TestNewUIHandler_HeadAndRange(t *testing.T) {
	dist, _ := uiFixture(t)
	h := newUIHandler(dist)

	t.Run("head returns no body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/robots.txt", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, rec.Body.String())
	})

	t.Run("range request is honoured", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
		req.Header.Set("Range", "bytes=0-3")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusPartialContent, rec.Code)
		assert.Equal(t, "User", rec.Body.String())
	})
}

// The catch-all must coexist with the API, probe and Swagger routes without
// stealing them, and must not mutate the path the access logger sees.
func TestUIHandler_ChiRoutePrecedence(t *testing.T) {
	dist, _ := uiFixture(t)

	r := chi.NewRouter()

	var loggedPath string
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req)
			loggedPath = req.URL.Path
		})
	})

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	})
	r.Get("/swagger/*", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("swagger"))
	})
	r.Route("/api/v1/genieacs", func(r chi.Router) {
		r.Get("/devices", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("devices"))
		})
	})
	r.Handle("/*", newUIHandler(dist))

	tests := []struct {
		path     string
		wantBody string
	}{
		{"/health", `{"status":"healthy"}`},
		{"/swagger/index.html", "swagger"},
		{"/api/v1/genieacs/devices", "devices"},
		{"/", "<html>shell</html>"},
		{"/devices/ABC123", "<html>shell</html>"},
		{"/jobs", "<html>shell</html>"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tt.wantBody, rec.Body.String())
			assert.Equal(t, tt.path, loggedPath, "access log must keep the original path")
		})
	}

	t.Run("unknown api endpoint never becomes the shell", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/genieacs/unknown", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.NotContains(t, rec.Body.String(), "<html>")
	})
}

func TestIsBackendPath(t *testing.T) {
	for _, p := range []string{"/api", "/api/v1/genieacs/ssid/1.2.3.4", "/health", "/healthz", "/ready", "/readyz", "/version", "/metrics", "/swagger/doc.json"} {
		assert.True(t, isBackendPath(p), p)
	}
	for _, p := range []string{"/", "/login", "/devices", "/jobs", "/healthcare", "/apiv2", "/versions", "/robots.txt"} {
		assert.False(t, isBackendPath(p), p)
	}
}