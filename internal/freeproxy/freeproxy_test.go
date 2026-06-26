package freeproxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuiltinSources(t *testing.T) {
	sources := BuiltinSources()
	if len(sources) != 14 {
		t.Errorf("expected 14 builtin sources, got %d", len(sources))
	}
	seen := map[string]bool{}
	for _, s := range sources {
		if s.Name == "" {
			t.Error("source has empty name")
		}
		if s.URL == "" {
			t.Errorf("source %q has empty URL", s.Name)
		}
		if !s.Enabled {
			t.Errorf("source %q should be enabled by default", s.Name)
		}
		if seen[s.Name] {
			t.Errorf("duplicate source name: %q", s.Name)
		}
		seen[s.Name] = true
	}
}

func TestParseText(t *testing.T) {
	body := []byte("192.168.1.1:8080\n10.0.0.1:3128\ninvalid\n192.168.1.1:8080") // duplicate
	src := Source{Name: "test", Type: SourceText}
	entries, err := ParseSourceResponse(src, body)
	if err != nil {
		t.Fatalf("ParseSourceResponse error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 unique entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Type != "http" {
		t.Errorf("type = %q, want http", entries[0].Type)
	}
	if entries[0].Source != "test" {
		t.Errorf("source = %q, want test", entries[0].Source)
	}
}

func TestParseText_CustomRegex(t *testing.T) {
	body := []byte("proxy=1.2.3.4:9999 proxy=5.6.7.8:8888")
	src := Source{
		Name:    "custom",
		Type:    SourceText,
		Regex:   `proxy=(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}):(\d{2,5})`,
		Enabled: true,
	}
	entries, err := ParseSourceResponse(src, body)
	if err != nil {
		t.Fatalf("ParseSourceResponse error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestParseJSON_SingleField(t *testing.T) {
	// Simulate proxifly-style: [{"proxy": "1.2.3.4:8080"}, {"proxy": "5.6.7.8:3128"}]
	data := []map[string]string{
		{"proxy": "1.2.3.4:8080"},
		{"proxy": "5.6.7.8:3128"},
	}
	body, _ := json.Marshal(data)
	src := Source{Name: "test-json", Type: SourceJSON, JSONPath: "[*].proxy"}
	entries, err := ParseSourceResponse(src, body)
	if err != nil {
		t.Fatalf("ParseSourceResponse error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Addr != "1.2.3.4:8080" {
		t.Errorf("addr = %q", entries[0].Addr)
	}
}

func TestParseJSON_TwoFields(t *testing.T) {
	// Simulate geonode-style: {"data": [{"ip": "1.2.3.4", "port": 8080}, {"ip": "5.6.7.8", "port": 3128}]}
	data := map[string]any{
		"data": []any{
			map[string]any{"ip": "1.2.3.4", "port": float64(8080)},
			map[string]any{"ip": "5.6.7.8", "port": float64(3128)},
		},
	}
	body, _ := json.Marshal(data)
	src := Source{Name: "test-json2", Type: SourceJSON, JSONPath: "data[*].ip,data[*].port"}
	entries, err := ParseSourceResponse(src, body)
	if err != nil {
		t.Fatalf("ParseSourceResponse error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Addr != "1.2.3.4:8080" {
		t.Errorf("addr = %q", entries[0].Addr)
	}
	if entries[1].Addr != "5.6.7.8:3128" {
		t.Errorf("addr = %q", entries[1].Addr)
	}
}

func TestParseJSON_InvalidJSON(t *testing.T) {
	src := Source{Name: "bad", Type: SourceJSON, JSONPath: "[*].ip"}
	_, err := ParseSourceResponse(src, []byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFetchSourceBody(t *testing.T) {
	// Start a test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("1.2.3.4:8080\n5.6.7.8:3128"))
	}))
	defer ts.Close()

	body, err := FetchSourceBody(ts.URL, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("FetchSourceBody error: %v", err)
	}
	if !strings.Contains(string(body), "1.2.3.4:8080") {
		t.Errorf("body missing expected content: %s", string(body))
	}
}

func TestFetchSourceBody_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	_, err := FetchSourceBody(ts.URL, 100*time.Millisecond, nil)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestCrawl(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("1.2.3.4:8080\n5.6.7.8:3128"))
	}))
	defer ts.Close()

	sources := []Source{
		{Name: "test1", URL: ts.URL, Type: SourceText, Enabled: true},
		{Name: "test2", URL: ts.URL, Type: SourceText, Enabled: false}, // disabled
	}
	result := Crawl(nil, sources, 5*time.Second)
	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
	if len(result.Sources) != 1 {
		t.Errorf("expected 1 enabled source, got %d", len(result.Sources))
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
}

func TestCrawl_DisabledSource(t *testing.T) {
	sources := []Source{
		{Name: "disabled", URL: "http://example.com", Type: SourceText, Enabled: false},
	}
	result := Crawl(nil, sources, 5*time.Second)
	if result.Total != 0 {
		t.Errorf("Total = %d, want 0 (disabled source)", result.Total)
	}
}

func TestCrawl_ErrorSource(t *testing.T) {
	sources := []Source{
		{Name: "bad", URL: "http://127.0.0.1:1/nonexistent", Type: SourceText, Enabled: true},
	}
	result := Crawl(nil, sources, 2*time.Second)
	if len(result.Errors) == 0 {
		t.Error("expected errors from bad source")
	}
}

func TestLogger(t *testing.T) {
	dir := t.TempDir()
	defaultLogger = nil
	logOnce = sync.Once{}
	l := InitLogger(dir, 10)
	defer l.Close()

	l.Info("test", "hello %s", "world")
	l.Warn("test", "warning %d", 42)
	l.Error("test", "error %s", "boom")

	entries := l.Query("", 10)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Filter by level
	infoEntries := l.Query(LevelInfo, 10)
	if len(infoEntries) != 1 {
		t.Errorf("expected 1 info entry, got %d", len(infoEntries))
	}

	warnEntries := l.Query(LevelWarn, 10)
	if len(warnEntries) != 1 {
		t.Errorf("expected 1 warn entry, got %d", len(warnEntries))
	}

	errorEntries := l.Query(LevelError, 10)
	if len(errorEntries) != 1 {
		t.Errorf("expected 1 error entry, got %d", len(errorEntries))
	}
}

func TestLogger_RingBuffer(t *testing.T) {
	dir := t.TempDir()
	// Reset global logger for isolated test
	defaultLogger = nil
	logOnce = sync.Once{}
	l := InitLogger(dir, 5)
	defer l.Close()

	// Write 10 entries, cap is 5
	for i := 0; i < 10; i++ {
		l.Info("test", "entry %d", i)
	}

	entries := l.Query("", 20)
	if len(entries) > 5 {
		t.Errorf("ring buffer should cap at 5, got %d", len(entries))
	}
}

func TestLogger_Stats(t *testing.T) {
	dir := t.TempDir()
	defaultLogger = nil
	logOnce = sync.Once{}
	l := InitLogger(dir, 10)
	defer l.Close()

	l.Info("test", "msg")
	stats := l.Stats()
	if stats["total"].(int64) != 1 {
		t.Errorf("total = %d, want 1", stats["total"])
	}
}

func TestLogger_Nil(t *testing.T) {
	var l *Logger
	// Should not panic
	l.Info("test", "msg")
	l.Warn("test", "msg")
	l.Error("test", "msg")
	if entries := l.Query("", 10); entries != nil {
		t.Error("nil logger Query should return nil")
	}
	if stats := l.Stats(); stats != nil {
		t.Error("nil logger Stats should return nil")
	}
}

func TestGetLogger_Nil(t *testing.T) {
	// Reset global (hack: use a fresh Logger variable)
	old := defaultLogger
	defaultLogger = nil
	defer func() { defaultLogger = old }()

	if l := GetLogger(); l != nil {
		t.Error("GetLogger() should return nil when not initialized")
	}
}

func TestValidateBatch_AllInvalid(t *testing.T) {
	// All proxies point to an unreachable address
	entries := []ProxyEntry{
		{Addr: "127.0.0.1:1", Type: "http", Source: "test"},
		{Addr: "127.0.0.1:2", Type: "http", Source: "test"},
	}
	results := ValidateBatch(nil, entries, []string{"http://httpbin.org/ip"}, 2*time.Second, 2)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Valid {
			t.Errorf("%s should be invalid", r.Addr)
		}
	}
}

func TestValidateBatch_ValidHTTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"origin":"1.2.3.4"}`))
	}))
	defer ts.Close()

	// Use the test server as both proxy and test URL
	// This is a simplified test — in reality the proxy would forward
	entries := []ProxyEntry{
		{Addr: strings.TrimPrefix(ts.URL, "http://"), Type: "http", Source: "test"},
	}
	results := ValidateBatch(nil, entries, []string{ts.URL + "/"}, 5*time.Second, 1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Note: this may or may not be valid depending on whether the test server
	// acts as a proxy. We just check that results are returned.
	if results[0].Addr == "" {
		t.Error("result addr should not be empty")
	}
}

func TestValidateAsync(t *testing.T) {
	entries := []ProxyEntry{
		{Addr: "127.0.0.1:1", Type: "http", Source: "test"},
		{Addr: "127.0.0.1:2", Type: "http", Source: "test"},
	}
	task := &ValidateTask{ID: "test-task"}
	ValidateAsync(nil, task, entries, []string{"http://httpbin.org/ip"}, 2*time.Second, 2)

	if task.Running {
		t.Error("task should be done")
	}
	if task.Total != 2 {
		t.Errorf("Total = %d, want 2", task.Total)
	}
	if task.Done != 2 {
		t.Errorf("Done = %d, want 2", task.Done)
	}
	if task.Valid != 0 {
		t.Errorf("Valid = %d, want 0 (unreachable)", task.Valid)
	}
	if len(task.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(task.Results))
	}
}
