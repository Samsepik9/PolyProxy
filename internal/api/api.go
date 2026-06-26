// Package api exposes the REST API used by the embedded UI.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Samsepik9/PolyProxy/internal/config"
	"github.com/Samsepik9/PolyProxy/internal/conntrack"
	"github.com/Samsepik9/PolyProxy/internal/freeproxy"
	"github.com/Samsepik9/PolyProxy/internal/pool"
	"github.com/Samsepik9/PolyProxy/internal/web"
)

// Server wires the API handlers.
type Server struct {
	Cm      *conntrack.Manager
	Pool    *pool.Pool
	FreeCfg *config.FreeProxyConfig
	CfgPath string // path to config.yaml for save

	mu          sync.Mutex
	cached      []freeproxy.ProxyEntry
	validateTask *freeproxy.ValidateTask

	dynamicMu sync.Mutex
	dynamic   *DynamicRunner
}

// Handler returns an http.Handler with routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/healthz", s.handleHealth)
	mux.HandleFunc("/api/connections", s.handleConnections)
	mux.HandleFunc("/api/connections/", s.handleConnectionItem)
	mux.HandleFunc("/api/proxies/fetch", s.handleFetch)
	mux.HandleFunc("/api/proxies/validate", s.handleValidate)
	mux.HandleFunc("/api/proxies/validate/", s.handleValidateProgress)
	mux.HandleFunc("/api/proxies/", s.handleProxyItem)
	mux.HandleFunc("/api/proxies", s.handleProxies)
	mux.HandleFunc("/api/pool/add", s.handlePoolAdd)
	mux.HandleFunc("/api/pool/save", s.handlePoolSave)
	mux.HandleFunc("/api/pool/strategy", s.handlePoolStrategy)
	mux.HandleFunc("/api/proxies/dynamic", s.handleDynamic)
	mux.HandleFunc("/api/proxies/auto-pool", s.handleAutoPool)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/logs", s.handleLogs)
	return mux
}

// WebHandler returns the web UI handler (mounted separately in main).
func WebHandler() http.Handler {
	return web.Handler()
}

// --- Health ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Connections ---

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
	if r.Method == http.MethodDelete {
		closed := false
		for _, v := range s.Cm.Snapshot() {
			if v.ID == id {
				if e := s.Cm.Get(id); e != nil {
					e.Close()
					closed = true
				}
				break
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"closed": closed})
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// --- Proxies ---

func (s *Server) handleProxies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.Pool.Snapshot())
}

func (s *Server) handleProxyItem(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/proxies/")
	if name == "" || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodDelete {
		if name == "direct" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot remove the 'direct' proxy"})
			return
		}
		ok := s.Pool.Remove(name)
		if l := freeproxy.GetLogger(); l != nil {
			l.Info("pool", "removed proxy: %s", name)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"removed": ok})
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// --- Fetch ---

func (s *Server) handleFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.FreeCfg == nil || !s.FreeCfg.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "freeproxy is not enabled"})
		return
	}

	crawlTimeout := time.Duration(s.FreeCfg.CrawlTimeout) * time.Second
	if crawlTimeout <= 0 {
		crawlTimeout = 30 * time.Second
	}

	// Merge builtin + custom sources
	sources := freeproxy.BuiltinSources()
	// TODO: parse custom sources from s.FreeCfg.Sources

	if l := freeproxy.GetLogger(); l != nil {
		l.Info("crawl", "starting crawl from %d sources", len(sources))
	}

	ctx, cancel := context.WithTimeout(r.Context(), crawlTimeout+10*time.Second)
	defer cancel()

	result := freeproxy.Crawl(ctx, sources, crawlTimeout)

	s.mu.Lock()
	s.cached = result.Entries
	s.mu.Unlock()

	if l := freeproxy.GetLogger(); l != nil {
		l.Info("crawl", "done: %d proxies from %d sources, %d errors", result.Total, len(result.Sources), len(result.Errors))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":   result.Total,
		"sources": result.Sources,
		"errors":  result.Errors,
	})
}

// --- Validate ---

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.FreeCfg == nil || !s.FreeCfg.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "freeproxy is not enabled"})
		return
	}

	s.mu.Lock()
	entries := s.cached
	s.mu.Unlock()

	if len(entries) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no cached proxies — run fetch first"})
		return
	}

	timeout := time.Duration(s.FreeCfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	concurrency := s.FreeCfg.Concurrency
	if concurrency <= 0 {
		concurrency = 20
	}

	task := &freeproxy.ValidateTask{
		ID: fmt.Sprintf("task-%d", time.Now().UnixNano()),
	}

	s.mu.Lock()
	s.validateTask = task
	s.mu.Unlock()

	if l := freeproxy.GetLogger(); l != nil {
		l.Info("validate", "starting validation of %d proxies (task %s)", len(entries), task.ID)
	}

	// Run async
	go freeproxy.ValidateAsync(context.Background(), task, entries, s.FreeCfg.TestURLs, timeout, concurrency)

	writeJSON(w, http.StatusOK, map[string]any{
		"task_id": task.ID,
		"total":   len(entries),
	})
}

func (s *Server) handleValidateProgress(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimPrefix(r.URL.Path, "/api/proxies/validate/")
	if taskID == "" {
		http.NotFound(w, r)
		return
	}

	s.mu.Lock()
	task := s.validateTask
	s.mu.Unlock()

	if task == nil || task.ID != taskID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}

	// Return only valid results (thread-safe via Snapshot)
	validResults := task.Snapshot()

	writeJSON(w, http.StatusOK, map[string]any{
		"task_id": task.ID,
		"total":   task.Total,
		"done":    task.Done,
		"valid":   task.Valid,
		"running": task.Running,
		"results": validResults,
	})
}

// --- Pool management ---

func (s *Server) handlePoolAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Proxies []struct {
			Addr string `json:"addr"`
			Type string `json:"type"`
		} `json:"proxies"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	added := 0
	for _, p := range req.Proxies {
		name := fmt.Sprintf("free-%s-%s", p.Type, strings.Replace(p.Addr, ":", "-", 1))
		up := &pool.Upstream{
			Name: name,
			Type: p.Type,
			Addr: p.Addr,
		}
		if err := s.Pool.Add(up); err != nil {
			// Try with suffix
			up.Name = name + "-" + fmt.Sprintf("%d", time.Now().UnixNano()%1000)
			if err2 := s.Pool.Add(up); err2 != nil {
				continue
			}
		}
		added++
	}

	if l := freeproxy.GetLogger(); l != nil {
		l.Info("pool", "added %d proxies to pool", added)
	}

	writeJSON(w, http.StatusOK, map[string]any{"added": added})
}

func (s *Server) handlePoolSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.CfgPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no config path configured"})
		return
	}

	// Build config from current pool state
	cfg, err := config.Load(s.CfgPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Rebuild proxies list from pool
	var proxies []config.ProxyEntry
	for _, u := range s.Pool.List() {
		if u.Type == "direct" {
			proxies = append(proxies, config.ProxyEntry{Name: "direct", Type: "direct"})
			continue
		}
		host, portStr, _ := netSplitHostPort(u.Addr)
		port := 0
		fmt.Sscanf(portStr, "%d", &port)
		proxies = append(proxies, config.ProxyEntry{
			Name:     u.Name,
			Type:     u.Type,
			Server:   host,
			Port:     port,
			Username: u.Username,
			Password: u.Password,
		})
	}
	cfg.Pool.Proxies = proxies
	cfg.Pool.Strategy = s.Pool.Strategy()

	if err := cfg.Save(s.CfgPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if l := freeproxy.GetLogger(); l != nil {
		l.Info("pool", "saved %d proxies to config", len(proxies))
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handlePoolStrategy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Strategy   string `json:"strategy"`
		PinnedName string `json:"pinned_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if err := s.Pool.SetStrategy(req.Strategy); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// If switching to "name" strategy, set the pinned name (may be empty to clear).
	if req.Strategy == "name" {
		s.Pool.SetPinnedName(req.PinnedName)
	}

	if l := freeproxy.GetLogger(); l != nil {
		if req.Strategy == "name" {
			l.Info("pool", "strategy changed to %s, pinned=%q", req.Strategy, req.PinnedName)
		} else {
			l.Info("pool", "strategy changed to %s", req.Strategy)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"strategy":    req.Strategy,
		"pinned_name": s.Pool.PinnedName(),
	})
}

// --- Dynamic proxy ---

// CycleResult captures one run of the dynamic proxy loop.
type CycleResult struct {
	Time    time.Time `json:"time"`
	Fetched int       `json:"fetched"`
	Valid   int       `json:"valid"`
	Added   int       `json:"added"`
	Error   string    `json:"error,omitempty"`
}

// DynamicRunner runs a periodic fetch→validate→(optionally add to pool) loop.
type DynamicRunner struct {
	Enabled  bool          `json:"enabled"`
	Running  bool          `json:"running"`
	Interval int           `json:"interval"` // seconds
	AutoPool bool          `json:"auto_pool"`
	LastRun  *CycleResult  `json:"last_run,omitempty"`
	TaskID   string        `json:"task_id,omitempty"`
	cancel   context.CancelFunc
	mu       sync.Mutex
}

func (s *Server) handleDynamic(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.dynamicMu.Lock()
		d := s.dynamic
		s.dynamicMu.Unlock()
		if d == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"enabled": false, "running": false, "interval": 0, "auto_pool": false,
			})
			return
		}
		d.mu.Lock()
		resp := map[string]any{
			"enabled": d.Enabled, "running": d.Running, "interval": d.Interval,
			"auto_pool": d.AutoPool, "last_run": d.LastRun, "task_id": d.TaskID,
		}
		d.mu.Unlock()
		writeJSON(w, http.StatusOK, resp)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Action   string `json:"action"` // "start" or "stop"
		Interval int    `json:"interval,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	switch req.Action {
	case "start":
		if s.FreeCfg == nil || !s.FreeCfg.Enabled {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "freeproxy is not enabled"})
			return
		}
		interval := req.Interval
		if interval < 10 {
			interval = 30 // minimum 10s, default 30
		}

		// Cancel existing loop if any
		s.dynamicMu.Lock()
		if s.dynamic != nil && s.dynamic.Enabled {
			s.dynamic.mu.Lock()
			if s.dynamic.cancel != nil {
				s.dynamic.cancel()
			}
			s.dynamic.Enabled = false
			s.dynamic.Running = false
			s.dynamic.mu.Unlock()
		}

		ctx, cancel := context.WithCancel(context.Background())
		d := &DynamicRunner{
			Enabled:  true,
			Running:  false,
			Interval: interval,
			AutoPool: false,
			cancel:   cancel,
		}
		s.dynamic = d
		s.dynamicMu.Unlock()

		go s.runDynamicLoop(ctx, d)
		writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "interval": interval})

	case "stop":
		s.dynamicMu.Lock()
		if s.dynamic != nil {
			s.dynamic.mu.Lock()
			if s.dynamic.cancel != nil {
				s.dynamic.cancel()
			}
			s.dynamic.Enabled = false
			s.dynamic.Running = false
			s.dynamic.mu.Unlock()
		}
		s.dynamicMu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})

	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
	}
}

func (s *Server) handleAutoPool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	s.dynamicMu.Lock()
	if s.dynamic != nil {
		s.dynamic.mu.Lock()
		s.dynamic.AutoPool = req.Enabled
		s.dynamic.mu.Unlock()
	}
	s.dynamicMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"auto_pool": req.Enabled})
}

func (s *Server) runDynamicLoop(ctx context.Context, d *DynamicRunner) {
	if l := freeproxy.GetLogger(); l != nil {
		l.Info("dynamic", "dynamic proxy loop started (interval=%ds)", d.Interval)
	}

	ticker := time.NewTicker(time.Duration(d.Interval) * time.Second)
	defer ticker.Stop()

	// Run immediately on start
	s.runDynamicCycle(ctx, d)

	for {
		select {
		case <-ctx.Done():
			d.mu.Lock()
			d.Running = false
			d.mu.Unlock()
			if l := freeproxy.GetLogger(); l != nil {
				l.Info("dynamic", "dynamic proxy loop stopped")
			}
			return
		case <-ticker.C:
			s.runDynamicCycle(ctx, d)
		}
	}
}

func (s *Server) runDynamicCycle(ctx context.Context, d *DynamicRunner) {
	d.mu.Lock()
	d.Running = true
	d.mu.Unlock()

	result := &CycleResult{Time: time.Now()}

	// 1. Fetch
	sources := freeproxy.BuiltinSources()
	crawlTimeout := time.Duration(30) * time.Second
	if s.FreeCfg != nil && s.FreeCfg.CrawlTimeout > 0 {
		crawlTimeout = time.Duration(s.FreeCfg.CrawlTimeout) * time.Second
	}
	crawlCtx, crawlCancel := context.WithTimeout(ctx, crawlTimeout+10*time.Second)
	crawlResult := freeproxy.Crawl(crawlCtx, sources, crawlTimeout)
	crawlCancel()
	result.Fetched = crawlResult.Total

	s.mu.Lock()
	s.cached = crawlResult.Entries
	s.mu.Unlock()

	if ctx.Err() != nil {
		result.Error = "cancelled"
		d.mu.Lock()
		d.LastRun = result
		d.Running = false
		d.mu.Unlock()
		return
	}

	// 2. Validate
	if len(crawlResult.Entries) == 0 {
		result.Error = "no proxies fetched"
		d.mu.Lock()
		d.LastRun = result
		d.Running = false
		d.mu.Unlock()
		return
	}

	timeout := time.Duration(10) * time.Second
	concurrency := 20
	if s.FreeCfg != nil {
		if s.FreeCfg.Timeout > 0 {
			timeout = time.Duration(s.FreeCfg.Timeout) * time.Second
		}
		if s.FreeCfg.Concurrency > 0 {
			concurrency = s.FreeCfg.Concurrency
		}
	}

	testURLs := []string{"http://myip.ipip.net"}
	if s.FreeCfg != nil && len(s.FreeCfg.TestURLs) > 0 {
		testURLs = s.FreeCfg.TestURLs
	}

	validResults := freeproxy.ValidateBatch(ctx, crawlResult.Entries, testURLs, timeout, concurrency)
	var validCount int64
	var validProxies []freeproxy.ValidateResult
	for _, r := range validResults {
		if r.Valid {
			validCount++
			validProxies = append(validProxies, r)
		}
	}
	result.Valid = int(validCount)

	// Store for frontend polling - use a simple task ID
	taskID := fmt.Sprintf("dynamic-%d", time.Now().UnixNano())
	d.mu.Lock()
	d.TaskID = taskID
	d.mu.Unlock()

	if l := freeproxy.GetLogger(); l != nil {
		l.Info("dynamic", "cycle done: %d fetched, %d valid", result.Fetched, result.Valid)
	}

	if ctx.Err() != nil {
		result.Error = "cancelled"
		d.mu.Lock()
		d.LastRun = result
		d.Running = false
		d.mu.Unlock()
		return
	}

	// 3. Auto-add to pool if enabled
	d.mu.Lock()
	autoPool := d.AutoPool
	d.mu.Unlock()

	if autoPool && len(validProxies) > 0 {
		added := 0
		for _, p := range validProxies {
			name := fmt.Sprintf("dynamic-%s-%s", p.Type, strings.Replace(p.Addr, ":", "-", 1))
			up := &pool.Upstream{
				Name: name,
				Type: p.Type,
				Addr: p.Addr,
			}
			if err := s.Pool.Add(up); err != nil {
				// name conflict — try with suffix
				up.Name = name + "-" + fmt.Sprintf("%d", time.Now().UnixNano()%1000)
				if err2 := s.Pool.Add(up); err2 != nil {
					continue
				}
			}
			added++
		}
		result.Added = added
		if l := freeproxy.GetLogger(); l != nil {
			l.Info("dynamic", "auto-added %d proxies to pool", added)
		}
	}

	d.mu.Lock()
	d.LastRun = result
	d.Running = false
	d.mu.Unlock()
}

// --- Stats ---

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
		"active":      len(snap),
		"upload":      up,
		"download":    down,
		"strategy":    s.Pool.Strategy(),
		"pinned_name": s.Pool.PinnedName(),
		"proxy_count": len(s.Pool.Snapshot()),
	})
}

// --- Logs ---

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	level := freeproxy.LogLevel(r.URL.Query().Get("level"))
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	l := freeproxy.GetLogger()
	if l == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	entries := l.Query(level, limit)
	writeJSON(w, http.StatusOK, entries)
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func netSplitHostPort(addr string) (host, port string, err error) {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("invalid addr: %s", addr)
	}
	return addr[:idx], addr[idx+1:], nil
}
