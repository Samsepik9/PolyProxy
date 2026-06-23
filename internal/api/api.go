// Package api exposes a small REST + WebSocket-free API used by the embedded UI.
//
// Endpoints:
//
//	GET  /api/connections     -> active connections snapshot
//	DELETE /api/connections/:id -> close a single connection
//	GET  /api/proxies         -> upstream pool snapshot
//	GET  /api/stats           -> aggregate counters (up/down totals + connection count)
//	GET  /api/healthz         -> liveness probe
//
// All other GET requests are served from the embedded web UI.
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jeff/proxypool/internal/conntrack"
	"github.com/jeff/proxypool/internal/pool"
	"github.com/jeff/proxypool/internal/web"
)

// Server wires the API handlers.
type Server struct {
	Cm   *conntrack.Manager
	Pool *pool.Pool
}

// Handler returns an http.Handler with routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/healthz", s.handleHealth)
	mux.HandleFunc("/api/connections", s.handleConnections)
	mux.HandleFunc("/api/connections/", s.handleConnectionItem) // /api/connections/:id
	mux.HandleFunc("/api/proxies", s.handleProxies)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.Handle("/", web.Handler()) // serves embedded UI for everything else
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.Cm.Snapshot())
	case http.MethodDelete:
		s.Cm.CloseAll()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleConnectionItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/connections/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodDelete:
		// Look up by id and close.
		closed := false
		for _, v := range s.Cm.Snapshot() {
			if v.ID == id {
				// Snapshot doesn't expose *Entry; fetch by re-iterating manager state.
				if e := findEntry(s.Cm, id); e != nil {
					e.Close()
					closed = true
				}
				break
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"closed": closed})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProxies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.Pool.Snapshot())
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snap := s.Cm.Snapshot()
	var up, down int64
	for _, e := range snap {
		up += e.Upload
		down += e.Download
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active":     len(snap),
		"upload":     up,
		"download":   down,
		"strategy":   s.Pool.Strategy(),
		"proxy_count": len(s.Pool.Snapshot()),
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// findEntry is a tiny accessor that returns the live *Entry by id.
// It avoids leaking the internal map.
func findEntry(m *conntrack.Manager, id string) *conntrack.Entry {
	return m.Get(id)
}