package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Samsepik9/PolyProxy/internal/config"
	"github.com/Samsepik9/PolyProxy/internal/conntrack"
	"github.com/Samsepik9/PolyProxy/internal/pool"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.PoolConfig{
		Strategy: "random",
		Proxies: []config.ProxyEntry{
			{Name: "direct", Type: "direct"},
			{Name: "p1", Type: "http", Server: "10.0.0.1", Port: 8080},
		},
	}
	p, err := pool.New(cfg)
	if err != nil {
		t.Fatalf("pool.New: %v", err)
	}
	return &Server{
		Cm:      conntrack.NewManager(),
		Pool:    p,
		FreeCfg: &config.FreeProxyConfig{Enabled: true, Timeout: 5, Concurrency: 2},
	}
}

func doRequest(t *testing.T, s *Server, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func TestHealthz(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "GET", "/api/healthz", "")
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %q", resp["status"])
	}
}

func TestConnections_Get(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "GET", "/api/connections", "")
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var conns []any
	json.NewDecoder(w.Body).Decode(&conns)
	// Should be empty initially
}

func TestConnections_Delete(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "DELETE", "/api/connections", "")
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestConnections_DeleteItem_NotFound(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "DELETE", "/api/connections/nonexistent", "")
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestProxies_Get(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "GET", "/api/proxies", "")
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var proxies []map[string]any
	json.NewDecoder(w.Body).Decode(&proxies)
	if len(proxies) != 2 {
		t.Errorf("expected 2 proxies, got %d", len(proxies))
	}
}

func TestProxies_Delete(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "DELETE", "/api/proxies/p1", "")
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	// Verify removed
	w = doRequest(t, s, "GET", "/api/proxies", "")
	var proxies []map[string]any
	json.NewDecoder(w.Body).Decode(&proxies)
	if len(proxies) != 1 {
		t.Errorf("expected 1 proxy after delete, got %d", len(proxies))
	}
}

func TestProxies_DeleteDirect(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "DELETE", "/api/proxies/direct", "")
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestProxies_DeleteNotFound(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "DELETE", "/api/proxies/nonexistent", "")
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestFetch_Disabled(t *testing.T) {
	s := newTestServer(t)
	s.FreeCfg.Enabled = false
	w := doRequest(t, s, "POST", "/api/proxies/fetch", "")
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestValidate_NoCache(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "POST", "/api/proxies/validate", "")
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestValidateProgress_NotFound(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "GET", "/api/proxies/validate/nonexistent", "")
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestPoolAdd(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "POST", "/api/pool/add", `{"proxies":[{"addr":"1.2.3.4:8080","type":"http"}]}`)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["added"].(float64) != 1 {
		t.Errorf("added = %v, want 1", resp["added"])
	}
}

func TestPoolAdd_InvalidJSON(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "POST", "/api/pool/add", `not json`)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPoolSave_NoConfigPath(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "POST", "/api/pool/save", "")
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPoolStrategy(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "PUT", "/api/pool/strategy", `{"strategy":"round-robin"}`)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["strategy"] != "round-robin" {
		t.Errorf("strategy = %q", resp["strategy"])
	}
}

func TestPoolStrategy_Invalid(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "PUT", "/api/pool/strategy", `{"strategy":"invalid"}`)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPoolStrategy_InvalidJSON(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "PUT", "/api/pool/strategy", `bad json`)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestStats(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "GET", "/api/stats", "")
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["active"].(float64) != 0 {
		t.Errorf("active = %v, want 0", resp["active"])
	}
	if resp["strategy"].(string) != "random" {
		t.Errorf("strategy = %q", resp["strategy"])
	}
	if resp["proxy_count"].(float64) != 2 {
		t.Errorf("proxy_count = %v, want 2", resp["proxy_count"])
	}
}

func TestLogs(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "GET", "/api/logs", "")
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestLogs_WithLevel(t *testing.T) {
	s := newTestServer(t)
	w := doRequest(t, s, "GET", "/api/logs?level=info&limit=10", "")
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	s := newTestServer(t)
	tests := []struct {
		method string
		path   string
	}{
		{"POST", "/api/healthz"},
		{"POST", "/api/connections"},
		{"POST", "/api/proxies"},
		{"GET", "/api/proxies/fetch"},
		{"GET", "/api/proxies/validate"},
		{"GET", "/api/pool/add"},
		{"GET", "/api/pool/save"},
		{"POST", "/api/stats"},
		{"POST", "/api/logs"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			w := doRequest(t, s, tt.method, tt.path, "")
			if w.Code != 405 {
				t.Errorf("status = %d, want 405", w.Code)
			}
		})
	}
}
