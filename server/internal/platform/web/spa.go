package web

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// SPA serves a built single-page app (apps/web's `dist/`) from disk, with the
// history fallback a client-side router needs: a request for /play or
// /leaderboard is not a file, but it must answer with index.html rather than
// 404, because the route only exists once the bundle boots.
//
// Why the Go binary serves the frontend at all: the session cookie and the
// WebSocket upgrade are same-origin by design (docs/API.md, docs/DESIGN-PHASE3-LIVE.md
// §3.4 — the origin check never allows "*"), and the service sets no CORS
// headers whatsoever. Serving the SPA from a second origin would therefore break
// every authenticated request unless something else re-unified the origins. One
// binary serving both keeps the deployment a single unit: no proxy to configure,
// nothing to get wrong.
//
// In development this is unused — Vite serves the SPA on :7777 and proxies /api
// here — so dir is empty and the route is never registered.
func SPA(dir string) (http.Handler, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("web: static dir %q: %w", dir, err)
	}
	index := filepath.Join(root, "index.html")
	// Fail at boot, not on the first page view: an image that forgot to copy the
	// build would otherwise serve a 500 to every visitor while looking healthy.
	if _, err := os.Stat(index); err != nil {
		return nil, fmt.Errorf("web: static dir %q has no index.html (build it with `npm run build -w @justpaint/web`): %w", root, err)
	}

	files := http.Dir(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only document fetches reach the shell. A POST to some non-API path is
		// not a resource here, and docs/API.md §3 freezes the error-code set with
		// no method_not_allowed member — so this answers with the honest
		// "nothing here" rather than inventing a code outside the contract.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			Error(w, http.StatusNotFound, CodeNotFound, "not found")
			return
		}

		name := path.Clean("/" + r.URL.Path)
		f, err := files.Open(name)
		if err != nil {
			// Anything that is not a real file is a client-side route: hand back
			// the shell and let the router decide, including its own 404.
			if errors.Is(err, fs.ErrNotExist) {
				serveShell(w, r, index)
				return
			}
			Error(w, http.StatusInternalServerError, CodeInternal, "static asset unavailable")
			return
		}
		defer func() { _ = f.Close() }()

		info, err := f.Stat()
		if err != nil {
			Error(w, http.StatusInternalServerError, CodeInternal, "static asset unavailable")
			return
		}
		// A directory request ("/", "/assets/") is not a file to send: "/" is the
		// shell, and a bare directory must never render an index listing.
		if info.IsDir() {
			serveShell(w, r, index)
			return
		}

		// Vite fingerprints everything under /assets/ (index-BTs-MvWJ.js), so those
		// are safe to cache forever; the shell that references them must never be,
		// or a deploy would keep serving the previous build's asset names.
		if strings.HasPrefix(name, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	}), nil
}

// serveShell writes index.html for a client-side route. Status stays 200: the SPA
// owns routing, so a bad path is its 404 to render, not ours to guess.
func serveShell(w http.ResponseWriter, r *http.Request, index string) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, index)
}
