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

	"github.com/jeff/proxypool/internal/config"
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
//   - name        : falls back to first healthy
//
// Falls back to the first upstream if no healthy candidates remain.
func (p *Pool) Select(preferredName, key string) (*Upstream, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.all) == 0 {
		return nil, errors.New("proxy pool is empty")
	}

	if preferredName != "" {
		if u, ok := p.byName[preferredName]; ok {
			return u, nil
		}
		return nil, fmt.Errorf("proxy %q not found in pool", preferredName)
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

