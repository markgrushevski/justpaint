package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSPA lays out a minimal built frontend: a shell and one fingerprinted asset.
func writeSPA(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>justpaint</title>"), 0o600); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o700); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "index-abc123.js"), []byte("console.log(1)"), 0o600); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	return dir
}

// TestSPA covers the whole contract of serving a client-routed app: real files
// win, unknown paths fall back to the shell (so a deep link like /play works on a
// hard refresh), fingerprinted assets cache forever while the shell never does,
// and a directory never lists itself.
func TestSPA(t *testing.T) {
	handler, err := SPA(writeSPA(t))
	if err != nil {
		t.Fatalf("SPA: %v", err)
	}

	tests := []struct {
		name      string
		method    string
		path      string
		wantCode  int
		wantBody  string
		wantCache string
	}{
		{name: "shell at root", method: http.MethodGet, path: "/", wantCode: http.StatusOK, wantBody: "justpaint", wantCache: "no-cache"},
		{name: "client route falls back to the shell", method: http.MethodGet, path: "/play", wantCode: http.StatusOK, wantBody: "justpaint", wantCache: "no-cache"},
		{name: "nested client route falls back too", method: http.MethodGet, path: "/matches/1/anything", wantCode: http.StatusOK, wantBody: "justpaint"},
		{name: "real asset is served and cached hard", method: http.MethodGet, path: "/assets/index-abc123.js", wantCode: http.StatusOK, wantBody: "console.log(1)", wantCache: "public, max-age=31536000, immutable"},
		{name: "missing asset does NOT 404 into the shell as an asset", method: http.MethodGet, path: "/assets/gone.js", wantCode: http.StatusOK, wantBody: "justpaint"},
		{name: "directory does not list", method: http.MethodGet, path: "/assets/", wantCode: http.StatusOK, wantBody: "justpaint"},
		{name: "non-GET is not a resource here", method: http.MethodPost, path: "/play", wantCode: http.StatusNotFound, wantBody: "not_found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.wantBody)
			}
			if tt.wantCache != "" {
				if got := rec.Header().Get("Cache-Control"); got != tt.wantCache {
					t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
				}
			}
		})
	}
}

// TestSPA_MissingIndex pins the boot-time failure: an image that shipped without
// the built frontend must refuse to start rather than 500 on every page view.
func TestSPA_MissingIndex(t *testing.T) {
	if _, err := SPA(t.TempDir()); err == nil {
		t.Fatal("expected an error when the static dir has no index.html")
	}
}

// TestSPA_NoEscape pins that a traversal cannot reach outside the served dir.
func TestSPA_NoEscape(t *testing.T) {
	dir := writeSPA(t)
	secret := filepath.Join(filepath.Dir(dir), "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP-SECRET"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	handler, err := SPA(dir)
	if err != nil {
		t.Fatalf("SPA: %v", err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/../secret.txt", nil))

	if strings.Contains(rec.Body.String(), "TOP-SECRET") {
		t.Fatal("traversal escaped the static dir")
	}
}
