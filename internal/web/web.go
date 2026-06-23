// Package web embeds the static UI files served by the API server.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static
var staticFS embed.FS

// Handler returns an http.Handler that serves the embedded UI.
// Layout:
//   GET /              -> static/index.html
//   GET /static/*      -> static/<file>
//   (the api package mounts /api/* separately)
func Handler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API routes are registered by the api package; never shadow them.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		// Strip leading slash and optional "static/" prefix to get the fs path.
		p := strings.TrimPrefix(r.URL.Path, "/")
		p = strings.TrimPrefix(p, "static/")
		if p == "" {
			p = "index.html"
		}
		f, err := sub.Open(p)
		if err != nil {
			// SPA fallback
			f, err = sub.Open("index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			p = "index.html"
		}
		defer f.Close()
		// Set Content-Type based on extension.
		switch path.Ext(p) {
		case ".html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case ".css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case ".js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		case ".svg":
			w.Header().Set("Content-Type", "image/svg+xml")
		case ".png":
			w.Header().Set("Content-Type", "image/png")
		}
		// Use ServeContent to handle caching + Range.
		if info, err := f.Stat(); err == nil {
			http.ServeContent(w, r, p, info.ModTime(), f.(readSeeker))
			return
		}
		_, _ = f.Read(make([]byte, 0))
	})
}

// readSeeker is the subset of interfaces needed by http.ServeContent.
type readSeeker interface {
	Read(p []byte) (n int, err error)
	Seek(offset int64, whence int) (int64, error)
}