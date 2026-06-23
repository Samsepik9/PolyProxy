// Package conntrack tracks every active connection flowing through the proxy.
package conntrack

import (
	"sync"
	"sync/atomic"
	"time"
)

// Protocol is the inbound proxy protocol that the client used.
type Protocol string

const (
	ProtoHTTP   Protocol = "HTTP"
	ProtoSOCKS5 Protocol = "SOCKS5"
)

// Network is the transport-layer network.
type Network string

const (
	NetTCP Network = "TCP"
	NetUDP Network = "UDP"
)

// Entry describes one live connection through the pool.
//
// Counters are accessed atomically; the struct MUST NOT be copied after
// registration. Always pass *Entry around.
type Entry struct {
	ID        string    `json:"id"`
	StartTime time.Time `json:"start_time"`

	// Inbound
	InboundNet Network `json:"inbound_net"`
	Inbound    Protocol `json:"inbound"`

	// Source / Target
	Source string `json:"source"`  // client IP
	Host   string `json:"host"`    // destination host (SNI / Host header / SOCKS5 domain)
	Target string `json:"target"`  // resolved ip:port actually dialled

	// Chosen upstream
	Proxy string `json:"proxy"`

	// Counters (atomic; not safe to copy)
	Upload   atomic.Int64 `json:"upload"`
	Download atomic.Int64 `json:"download"`

	// Lifecycle
	cancel  chan struct{}
	closed  atomic.Bool
}

// Closed reports whether Close has been called.
func (e *Entry) Closed() bool { return e.closed.Load() }

// AddUpload records n bytes uploaded (client -> target).
func (e *Entry) AddUpload(n int) { e.Upload.Add(int64(n)) }

// AddDownload records n bytes downloaded (target -> client).
func (e *Entry) AddDownload(n int) { e.Download.Add(int64(n)) }

// Close marks the entry as closed and cancels any in-flight copy.
func (e *Entry) Close() {
	if e.closed.CompareAndSwap(false, true) {
		close(e.cancel)
	}
}

// Manager is a thread-safe registry of active connections.
type Manager struct {
	mu      sync.RWMutex
	entries map[string]*Entry
}

// NewManager creates a Manager.
func NewManager() *Manager {
	return &Manager{entries: map[string]*Entry{}}
}

// Register adds an entry and returns it. The cancel channel is internal.
func (m *Manager) Register(e *Entry) *Entry {
	if e.cancel == nil {
		e.cancel = make(chan struct{})
	}
	m.mu.Lock()
	m.entries[e.ID] = e
	m.mu.Unlock()
	return e
}

// Unregister removes an entry. Safe to call multiple times.
func (m *Manager) Unregister(id string) {
	m.mu.Lock()
	delete(m.entries, id)
	m.mu.Unlock()
}

// Get returns the live *Entry by id. Returns nil if not present.
func (m *Manager) Get(id string) *Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.entries[id]
}

// Snapshot returns a JSON-safe copy of every active entry (closed or not).
func (m *Manager) Snapshot() []View {
	m.mu.RLock()
	all := make([]*Entry, 0, len(m.entries))
	for _, e := range m.entries {
		all = append(all, e)
	}
	m.mu.RUnlock()

	out := make([]View, 0, len(all))
	for _, e := range all {
		out = append(out, View{
			ID:        e.ID,
			StartTime: e.StartTime,
			Inbound:   string(e.Inbound),
			Network:   string(e.InboundNet),
			Source:    e.Source,
			Host:      e.Host,
			Target:    e.Target,
			Proxy:     e.Proxy,
			Upload:    e.Upload.Load(),
			Download:  e.Download.Load(),
			Closed:    e.Closed(),
		})
	}
	return out
}

// CloseAll cancels every entry. Used at shutdown.
func (m *Manager) CloseAll() {
	m.mu.RLock()
	all := make([]*Entry, 0, len(m.entries))
	for _, e := range m.entries {
		all = append(all, e)
	}
	m.mu.RUnlock()
	for _, e := range all {
		e.Close()
	}
}

// View is the API-facing shape of an Entry.
type View struct {
	ID        string    `json:"id"`
	StartTime time.Time `json:"start_time"`
	Inbound   string    `json:"inbound"`
	Network   string    `json:"network"`
	Source    string    `json:"source"`
	Host      string    `json:"host"`
	Target    string    `json:"target"`
	Proxy     string    `json:"proxy"`
	Upload    int64     `json:"upload"`
	Download  int64     `json:"download"`
	Closed    bool      `json:"closed"`
}

// Done returns the cancel channel for the entry (for the proxy goroutines
// to listen on).
func (e *Entry) Done() <-chan struct{} { return e.cancel }