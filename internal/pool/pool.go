// Package pool manages a set of upstream proxies and a selection strategy.
package pool

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Samsepik9/PolyProxy/internal/config"
)

// Upstream is a normalised upstream proxy ready to dial.
type Upstream struct {
	Name     string
	Type     string // http | socks5 | direct
	Addr     string // host:port (empty for direct)
	Username string
	Password string

	// health
	healthy atomic.Bool
}

// Healthy reports whether the upstream passed the latest health check (always true if disabled).
func (u *Upstream) Healthy() bool { return u.healthy.Load() }

// SetHealthy toggles the health flag.
func (u *Upstream) SetHealthy(v bool) { u.healthy.Store(v) }

// Pool holds a collection of upstreams and a selection strategy.
type Pool struct {
	mu       sync.RWMutex
	all      []*Upstream
	byName   map[string]*Upstream
	strategy string
	pinned   string // pinned upstream name when strategy == "name"

	// round-robin state
	rrCounter atomic.Uint64
}

// New builds a Pool from config.
func New(cfg *config.PoolConfig) (*Pool, error) {
	p := &Pool{
		byName:   map[string]*Upstream{},
		strategy: cfg.Strategy,
	}
	for _, e := range cfg.Proxies {
		u := &Upstream{
			Name:     e.Name,
			Type:     e.Type,
			Username: e.Username,
			Password: e.Password,
		}
		if e.Type != "direct" {
			u.Addr = net.JoinHostPort(e.Server, fmt.Sprintf("%d", e.Port))
		}
		u.SetHealthy(true)
		p.all = append(p.all, u)
		p.byName[u.Name] = u
	}
	return p, nil
}

// Strategy returns the active strategy name.
func (p *Pool) Strategy() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.strategy
}

// List returns a snapshot of all upstreams.
func (p *Pool) List() []*Upstream {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*Upstream, len(p.all))
	copy(out, p.all)
	return out
}

// Add adds a new upstream to the pool dynamically. Returns an error if the name
// already exists.
func (p *Pool) Add(u *Upstream) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.byName[u.Name]; ok {
		return fmt.Errorf("proxy %q already exists in pool", u.Name)
	}
	u.SetHealthy(true)
	p.all = append(p.all, u)
	p.byName[u.Name] = u
	return nil
}

// Remove removes an upstream from the pool by name. Returns false if not found.
func (p *Pool) Remove(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	u, ok := p.byName[name]
	if !ok {
		return false
	}
	delete(p.byName, name)
	for i, v := range p.all {
		if v == u {
			p.all = append(p.all[:i], p.all[i+1:]...)
			break
		}
	}
	return true
}

// SetStrategy changes the selection strategy at runtime.
func (p *Pool) SetStrategy(s string) error {
	switch s {
	case "random", "round-robin", "hash", "name":
	default:
		return fmt.Errorf("unknown strategy %q (random|round-robin|hash|name)", s)
	}
	p.mu.Lock()
	p.strategy = s
	p.mu.Unlock()
	return nil
}

// PinnedName returns the currently pinned upstream name (used when strategy == "name").
func (p *Pool) PinnedName() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.pinned
}

// SetPinnedName sets the upstream name to use when strategy == "name".
// Pass an empty string to clear the pin.
func (p *Pool) SetPinnedName(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pinned = name
}

// Snapshot returns a JSON-safe view of all upstreams.
func (p *Pool) Snapshot() []UpstreamView {
	ups := p.List()
	out := make([]UpstreamView, 0, len(ups))
	for _, u := range ups {
		out = append(out, UpstreamView{
			Name:    u.Name,
			Type:    u.Type,
			Addr:    u.Addr,
			Healthy: u.Healthy(),
		})
	}
	return out
}

// UpstreamView is the API-facing view of an upstream.
type UpstreamView struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Addr    string `json:"addr"`
	Healthy bool   `json:"healthy"`
}

// Get returns the upstream by name.
func (p *Pool) Get(name string) (*Upstream, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	u, ok := p.byName[name]
	return u, ok
}

// Select picks an upstream based on the strategy and an optional preferred name.
//
// If preferredName is non-empty and exists in the pool, it is returned directly
// (this is the "user-specified proxy" path).
//
// Otherwise the configured strategy is applied:
//   - random      : uniform random among healthy upstreams
//   - round-robin : cyclical
//   - hash        : hash(key) -> index, sticky to the same upstream per key
//   - name        : uses the pool's pinned name (if set), else falls back to first healthy
//
// Falls back to the first upstream if no healthy candidates remain.
func (p *Pool) Select(preferredName, key string) (*Upstream, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.all) == 0 {
		return nil, errors.New("proxy pool is empty")
	}

	// Effective preferred name: explicit preferred (from URL auth) OR pinned (from "name" strategy)
	effective := preferredName
	if effective == "" && p.strategy == "name" && p.pinned != "" {
		effective = p.pinned
	}

	if effective != "" {
		if u, ok := p.byName[effective]; ok {
			return u, nil
		}
		return nil, fmt.Errorf("pinned proxy %q not found in pool", effective)
	}

	healthy := make([]*Upstream, 0, len(p.all))
	for _, u := range p.all {
		if u.Healthy() {
			healthy = append(healthy, u)
		}
	}
	if len(healthy) == 0 {
		// fall back to all
		healthy = p.all
	}

	switch p.strategy {
	case "random":
		return healthy[fastRandN(len(healthy))], nil
	case "round-robin":
		idx := p.rrCounter.Add(1) - 1
		return healthy[int(idx%uint64(len(healthy)))], nil
	case "hash":
		if key == "" {
			key = "default"
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(key))
		return healthy[int(h.Sum32()%uint32(len(healthy)))], nil
	default:
		return healthy[0], nil
	}
}

// fastRandN returns a uniform random number in [0, n) using math/rand/v2.
func fastRandN(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.IntN(n)
}

// StartHealthCheck spawns a goroutine that periodically TCP-dials every
// non-direct upstream. It is a no-op when health_check is disabled.
func (p *Pool) StartHealthCheck(enabled bool, interval time.Duration) {
	if !enabled {
		return
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			p.mu.RLock()
			targets := append([]*Upstream(nil), p.all...)
			p.mu.RUnlock()
			for _, u := range targets {
				if u.Type == "direct" {
					u.SetHealthy(true)
					continue
				}
				conn, err := net.DialTimeout("tcp", u.Addr, 5*time.Second)
				if err != nil {
					u.SetHealthy(false)
					continue
				}
				_ = conn.Close()
				u.SetHealthy(true)
			}
		}
	}()
}

