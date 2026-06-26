package pool

import (
	"testing"

	"github.com/Samsepik9/PolyProxy/internal/config"
)

func TestNew(t *testing.T) {
	cfg := &config.PoolConfig{
		Strategy: "random",
		Proxies: []config.ProxyEntry{
			{Name: "direct", Type: "direct"},
			{Name: "p1", Type: "http", Server: "10.0.0.1", Port: 8080},
			{Name: "p2", Type: "socks5", Server: "10.0.0.2", Port: 1080},
		},
	}
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if p.Strategy() != "random" {
		t.Errorf("strategy = %q, want random", p.Strategy())
	}
	list := p.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 upstreams, got %d", len(list))
	}
	if list[0].Name != "direct" {
		t.Errorf("first = %q, want direct", list[0].Name)
	}
	if list[1].Addr != "10.0.0.1:8080" {
		t.Errorf("p1 addr = %q, want 10.0.0.1:8080", list[1].Addr)
	}
	if list[2].Addr != "10.0.0.2:1080" {
		t.Errorf("p2 addr = %q, want 10.0.0.2:1080", list[2].Addr)
	}
	// All should be healthy by default
	for _, u := range list {
		if !u.Healthy() {
			t.Errorf("%s should be healthy", u.Name)
		}
	}
}

func TestAdd(t *testing.T) {
	cfg := &config.PoolConfig{
		Strategy: "random",
		Proxies:  []config.ProxyEntry{{Name: "direct", Type: "direct"}},
	}
	p, _ := New(cfg)

	err := p.Add(&Upstream{Name: "new-http", Type: "http", Addr: "1.2.3.4:8080"})
	if err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if len(p.List()) != 2 {
		t.Errorf("expected 2 upstreams, got %d", len(p.List()))
	}

	// Duplicate name
	err = p.Add(&Upstream{Name: "new-http", Type: "http", Addr: "5.6.7.8:8080"})
	if err == nil {
		t.Error("expected error for duplicate name")
	}
}

func TestRemove(t *testing.T) {
	cfg := &config.PoolConfig{
		Strategy: "random",
		Proxies:  []config.ProxyEntry{{Name: "direct", Type: "direct"}},
	}
	p, _ := New(cfg)
	p.Add(&Upstream{Name: "to-remove", Type: "http", Addr: "1.2.3.4:8080"})

	if !p.Remove("to-remove") {
		t.Error("Remove() returned false for existing proxy")
	}
	if len(p.List()) != 1 {
		t.Errorf("expected 1 upstream after remove, got %d", len(p.List()))
	}
	if p.Remove("nonexistent") {
		t.Error("Remove() returned true for nonexistent proxy")
	}
	// Cannot remove direct
	// (API layer prevents this, but pool allows it)
}

func TestSetStrategy(t *testing.T) {
	cfg := &config.PoolConfig{
		Strategy: "random",
		Proxies:  []config.ProxyEntry{{Name: "direct", Type: "direct"}},
	}
	p, _ := New(cfg)

	for _, s := range []string{"random", "round-robin", "hash", "name"} {
		if err := p.SetStrategy(s); err != nil {
			t.Errorf("SetStrategy(%q) error: %v", s, err)
		}
		if p.Strategy() != s {
			t.Errorf("strategy after SetStrategy(%q) = %q", s, p.Strategy())
		}
	}

	if err := p.SetStrategy("invalid"); err == nil {
		t.Error("expected error for invalid strategy")
	}
}

func TestSelect_Random(t *testing.T) {
	cfg := &config.PoolConfig{
		Strategy: "random",
		Proxies: []config.ProxyEntry{
			{Name: "direct", Type: "direct"},
			{Name: "p1", Type: "http", Server: "10.0.0.1", Port: 8080},
			{Name: "p2", Type: "http", Server: "10.0.0.2", Port: 8080},
		},
	}
	p, _ := New(cfg)

	// Select many times, should always succeed
	for i := 0; i < 100; i++ {
		u, err := p.Select("", "")
		if err != nil {
			t.Fatalf("Select() iteration %d: %v", i, err)
		}
		if u == nil {
			t.Fatal("Select() returned nil")
		}
	}
}

func TestSelect_RoundRobin(t *testing.T) {
	cfg := &config.PoolConfig{
		Strategy: "round-robin",
		Proxies: []config.ProxyEntry{
			{Name: "direct", Type: "direct"},
			{Name: "p1", Type: "http", Server: "10.0.0.1", Port: 8080},
		},
	}
	p, _ := New(cfg)

	// Two selects should cycle
	u1, _ := p.Select("", "")
	u2, _ := p.Select("", "")
	if u1 == nil || u2 == nil {
		t.Fatal("Select() returned nil")
	}
}

func TestSelect_Hash(t *testing.T) {
	cfg := &config.PoolConfig{
		Strategy: "hash",
		Proxies: []config.ProxyEntry{
			{Name: "direct", Type: "direct"},
			{Name: "p1", Type: "http", Server: "10.0.0.1", Port: 8080},
			{Name: "p2", Type: "http", Server: "10.0.0.2", Port: 8080},
		},
	}
	p, _ := New(cfg)

	// Same key should return same upstream
	u1, _ := p.Select("", "example.com:443")
	u2, _ := p.Select("", "example.com:443")
	if u1.Name != u2.Name {
		t.Errorf("hash not sticky: %q vs %q", u1.Name, u2.Name)
	}
}

func TestSelect_PreferredName(t *testing.T) {
	cfg := &config.PoolConfig{
		Strategy: "random",
		Proxies: []config.ProxyEntry{
			{Name: "direct", Type: "direct"},
			{Name: "p1", Type: "http", Server: "10.0.0.1", Port: 8080},
		},
	}
	p, _ := New(cfg)

	u, err := p.Select("p1", "")
	if err != nil {
		t.Fatalf("Select(p1) error: %v", err)
	}
	if u.Name != "p1" {
		t.Errorf("preferred name: got %q, want p1", u.Name)
	}

	_, err = p.Select("nonexistent", "")
	if err == nil {
		t.Error("expected error for nonexistent preferred name")
	}
}

func TestSelect_EmptyPool(t *testing.T) {
	p := &Pool{byName: map[string]*Upstream{}}
	_, err := p.Select("", "")
	if err == nil {
		t.Error("expected error for empty pool")
	}
}

func TestSnapshot(t *testing.T) {
	cfg := &config.PoolConfig{
		Strategy: "random",
		Proxies: []config.ProxyEntry{
			{Name: "direct", Type: "direct"},
			{Name: "p1", Type: "http", Server: "10.0.0.1", Port: 8080},
		},
	}
	p, _ := New(cfg)

	snap := p.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 in snapshot, got %d", len(snap))
	}
	if snap[0].Name != "direct" {
		t.Errorf("snap[0].Name = %q", snap[0].Name)
	}
	if !snap[0].Healthy {
		t.Error("direct should be healthy")
	}
}

func TestGet(t *testing.T) {
	cfg := &config.PoolConfig{
		Strategy: "random",
		Proxies:  []config.ProxyEntry{{Name: "direct", Type: "direct"}},
	}
	p, _ := New(cfg)

	u, ok := p.Get("direct")
	if !ok {
		t.Fatal("Get(direct) returned false")
	}
	if u.Name != "direct" {
		t.Errorf("got %q", u.Name)
	}

	_, ok = p.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) should return false")
	}
}

func TestUpstream_Healthy(t *testing.T) {
	u := &Upstream{Name: "test"}
	if u.Healthy() {
		t.Error("new upstream should default to false (atomic.Bool zero value)")
	}
	u.SetHealthy(true)
	if !u.Healthy() {
		t.Error("after SetHealthy(true), should be healthy")
	}
	u.SetHealthy(false)
	if u.Healthy() {
		t.Error("after SetHealthy(false), should not be healthy")
	}
}
