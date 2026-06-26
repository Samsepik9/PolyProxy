package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg == nil {
		t.Fatal("Default() returned nil")
	}
	if cfg.Server.HTTPListen != "127.0.0.1:7890" {
		t.Errorf("HTTPListen = %q, want 127.0.0.1:7890", cfg.Server.HTTPListen)
	}
	if cfg.Pool.Strategy != "random" {
		t.Errorf("Strategy = %q, want random", cfg.Pool.Strategy)
	}
	if len(cfg.Pool.Proxies) != 1 || cfg.Pool.Proxies[0].Name != "direct" {
		t.Errorf("default proxies = %+v, want [direct]", cfg.Pool.Proxies)
	}
	if !cfg.FreeProxy.Enabled {
		t.Error("FreeProxy.Enabled should be true by default")
	}
}

func TestLoad_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
server:
  http_listen: "0.0.0.0:7890"
  socks5_listen: "0.0.0.0:7891"
  api_listen: "0.0.0.0:9090"
  api_enable: true
pool:
  strategy: round-robin
  health_check: true
  proxies:
    - { name: direct, type: direct }
    - { name: my-http, type: http, server: "10.0.0.1", port: 8080 }
freeproxy:
  enabled: true
  test_url: "https://example.com/test"
  timeout: 30
  concurrency: 50
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.HTTPListen != "0.0.0.0:7890" {
		t.Errorf("HTTPListen = %q", cfg.Server.HTTPListen)
	}
	if cfg.Pool.Strategy != "round-robin" {
		t.Errorf("Strategy = %q", cfg.Pool.Strategy)
	}
	if !cfg.Pool.HealthCheck {
		t.Error("HealthCheck should be true")
	}
	if len(cfg.Pool.Proxies) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(cfg.Pool.Proxies))
	}
	if cfg.FreeProxy.Timeout != 30 {
		t.Errorf("Timeout = %d, want 30", cfg.FreeProxy.Timeout)
	}
	if cfg.FreeProxy.Concurrency != 50 {
		t.Errorf("Concurrency = %d, want 50", cfg.FreeProxy.Concurrency)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoad_EmptyPath(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load(\"\") returned nil")
	}
}

func TestValidate_NoProxies(t *testing.T) {
	cfg := Default()
	cfg.Pool.Proxies = nil
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for empty proxies")
	}
}

func TestValidate_DuplicateNames(t *testing.T) {
	cfg := Default()
	cfg.Pool.Proxies = []ProxyEntry{
		{Name: "direct", Type: "direct"},
		{Name: "direct", Type: "http", Server: "1.2.3.4", Port: 8080},
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for duplicate proxy names")
	}
}

func TestValidate_UnknownStrategy(t *testing.T) {
	cfg := Default()
	cfg.Pool.Strategy = "invalid"
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for unknown strategy")
	}
}

func TestValidate_UnknownProxyType(t *testing.T) {
	cfg := Default()
	cfg.Pool.Proxies = []ProxyEntry{
		{Name: "direct", Type: "direct"},
		{Name: "bad", Type: "socks4", Server: "1.2.3.4", Port: 1080},
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for unknown proxy type")
	}
}

func TestValidate_MissingServer(t *testing.T) {
	cfg := Default()
	cfg.Pool.Proxies = []ProxyEntry{
		{Name: "direct", Type: "direct"},
		{Name: "no-server", Type: "http", Port: 8080},
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for missing server")
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := Default()
	cfg.Pool.Proxies = []ProxyEntry{
		{Name: "direct", Type: "direct"},
		{Name: "bad-port", Type: "http", Server: "1.2.3.4", Port: 99999},
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid port")
	}
}

func TestValidate_EmptyName(t *testing.T) {
	cfg := Default()
	cfg.Pool.Proxies = []ProxyEntry{
		{Name: "", Type: "direct"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for empty proxy name")
	}
}

func TestValidate_AllStrategies(t *testing.T) {
	for _, s := range []string{"random", "round-robin", "hash", "name"} {
		cfg := Default()
		cfg.Pool.Strategy = s
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() for strategy %q: %v", s, err)
		}
	}
}

func TestSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "saved.yaml")
	cfg := Default()
	cfg.Pool.Strategy = "hash"
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	// Reload and verify
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("re-Load() error: %v", err)
	}
	if loaded.Pool.Strategy != "hash" {
		t.Errorf("reloaded strategy = %q, want hash", loaded.Pool.Strategy)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	if path == "" {
		t.Error("DefaultConfigPath() returned empty")
	}
	// Should end with config.yaml
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("DefaultConfigPath() base = %q, want config.yaml", filepath.Base(path))
	}
}
