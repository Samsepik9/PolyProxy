// Package freeproxy — validator.go: async proxy validation with progress tracking.
package freeproxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// ValidateTask tracks an async validation operation.
type ValidateTask struct {
	ID        string           `json:"id"`
	Total     int              `json:"total"`
	Done      int64            `json:"done"` // atomic
	Valid     int64            `json:"valid"` // atomic
	Running   bool             `json:"running"`
	Results   []ValidateResult `json:"results,omitempty"`
	StartTime time.Time        `json:"start_time"`
	mu        sync.Mutex
}

// Snapshot returns a safe copy of the task's valid results under lock.
func (t *ValidateTask) Snapshot() []ValidateResult {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]ValidateResult, 0, len(t.Results))
	for _, r := range t.Results {
		if r.Valid {
			out = append(out, r)
		}
	}
	return out
}

// ValidateResult holds the outcome of validating a single proxy.
type ValidateResult struct {
	Addr    string `json:"addr"`
	Type    string `json:"type"`
	Source  string `json:"source"`
	Valid   bool   `json:"valid"`
	Latency int64  `json:"latency_ms"`
	Error   string `json:"error,omitempty"`
}

// ValidateBatch runs validation synchronously and returns all results.
func ValidateBatch(ctx context.Context, entries []ProxyEntry, testURLs []string, timeout time.Duration, concurrency int) []ValidateResult {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if concurrency <= 0 {
		concurrency = 20
	}
	if len(testURLs) == 0 {
		testURLs = []string{"http://myip.ipip.net"}
	}

	var (
		mu      sync.Mutex
		results []ValidateResult
		sem     = make(chan struct{}, concurrency)
		wg      sync.WaitGroup
	)

	for _, entry := range entries {
		wg.Add(1)
		go func(e ProxyEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r := ValidateResult{
				Addr:   e.Addr,
				Type:   e.Type,
				Source: e.Source,
			}

			start := time.Now()
			err := testProxyMulti(ctx, e, testURLs, timeout)
			r.Latency = time.Since(start).Milliseconds()

			if err != nil {
				r.Valid = false
				r.Error = err.Error()
			} else {
				r.Valid = true
			}

			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}(entry)
	}
	wg.Wait()
	return results
}

// ValidateAsync runs validation in the background and updates the task progress.
func ValidateAsync(ctx context.Context, task *ValidateTask, entries []ProxyEntry, testURLs []string, timeout time.Duration, concurrency int) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if concurrency <= 0 {
		concurrency = 20
	}
	if len(testURLs) == 0 {
		testURLs = []string{"http://myip.ipip.net"}
	}

	task.Running = true
	task.Total = len(entries)
	task.StartTime = time.Now()

	var (
		mu  sync.Mutex
		sem = make(chan struct{}, concurrency)
		wg  sync.WaitGroup
	)

	for _, entry := range entries {
		wg.Add(1)
		go func(e ProxyEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r := ValidateResult{
				Addr:   e.Addr,
				Type:   e.Type,
				Source: e.Source,
			}

			start := time.Now()
			err := testProxyMulti(ctx, e, testURLs, timeout)
			r.Latency = time.Since(start).Milliseconds()

			if err != nil {
				r.Valid = false
				r.Error = err.Error()
			} else {
				r.Valid = true
				atomic.AddInt64(&task.Valid, 1)
			}

			mu.Lock()
			task.Results = append(task.Results, r)
			mu.Unlock()

			atomic.AddInt64(&task.Done, 1)
		}(entry)
	}
	wg.Wait()
	task.Running = false

	log.Printf("[validate] task %s done: %d/%d valid", task.ID, task.Valid, task.Total)
}

// testProxyMulti validates one proxy via TCP pre-check + sequential test URL probes.
// 1. TCP three-way handshake on entry.Addr — fail fast on dead endpoints.
// 2. Try each test URL in order; first 2xx-3xx response = valid.
func testProxyMulti(parentCtx context.Context, entry ProxyEntry, testURLs []string, timeout time.Duration) error {
	// Defensive: nil context is a programming error in production, but tests
	// (and sloppy callers) pass nil — fall back to Background so we don't panic.
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	// Step 1: TCP pre-check (fast path — most dead proxies fail here in <1s)
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", entry.Addr)
	if err != nil {
		return fmt.Errorf("tcp dial: %w", err)
	}
	conn.Close()

	// Step 2: HTTP test against each URL in order
	var lastErr error
	for _, testURL := range testURLs {
		err := testProxy(ctx, entry, testURL, timeout)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return fmt.Errorf("all test urls failed: %w", lastErr)
	}
	return nil
}

func testProxy(ctx context.Context, entry ProxyEntry, testURL string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr := entry.Addr

	var transport *http.Transport

	switch entry.Type {
	case "http":
		proxyURL, err := url.Parse("http://" + addr)
		if err != nil {
			return fmt.Errorf("bad proxy url: %w", err)
		}
		transport = &http.Transport{
			Proxy:               http.ProxyURL(proxyURL),
			DialContext:         (&net.Dialer{Timeout: timeout}).DialContext,
			TLSHandshakeTimeout: timeout,
		}
	case "socks5":
		transport = &http.Transport{
			DialContext: func(ctx context.Context, network, target string) (net.Conn, error) {
				conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", addr)
				if err != nil {
					return nil, err
				}
				host, portStr, err := net.SplitHostPort(target)
				if err != nil {
					conn.Close()
					return nil, err
				}
				port, err := strconv.Atoi(portStr)
				if err != nil {
					conn.Close()
					return nil, err
				}
				if err := socks5Handshake(conn, host, uint16(port), "", ""); err != nil {
					conn.Close()
					return nil, err
				}
				return conn, nil
			},
			TLSHandshakeTimeout: timeout,
		}
	default:
		return fmt.Errorf("unknown proxy type: %s", entry.Type)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ProxyPool-Validator/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("bad status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// --- SOCKS5 handshake (standalone) ---

func socks5Handshake(conn net.Conn, host string, port uint16, user, pass string) error {
	var greeting []byte
	if user != "" || pass != "" {
		greeting = []byte{0x05, 0x01, 0x02}
	} else {
		greeting = []byte{0x05, 0x01, 0x00}
	}
	if _, err := conn.Write(greeting); err != nil {
		return err
	}
	method := make([]byte, 2)
	if _, err := io.ReadFull(conn, method); err != nil {
		return err
	}
	if method[0] != 0x05 {
		return fmt.Errorf("socks5: bad server version %d", method[0])
	}
	switch method[1] {
	case 0x00:
	case 0x02:
		if user == "" {
			return fmt.Errorf("socks5 upstream requires username/password")
		}
		if err := socks5UserPass(conn, user, pass); err != nil {
			return err
		}
	case 0xFF:
		return fmt.Errorf("socks5: no acceptable auth method")
	default:
		return fmt.Errorf("socks5: unknown method 0x%02x", method[1])
	}
	var req []byte
	req = append(req, 0x05, 0x01, 0x00)
	ip := net.ParseIP(host)
	switch {
	case ip == nil:
		if len(host) > 255 {
			return fmt.Errorf("socks5: domain too long")
		}
		req = append(req, 0x03, byte(len(host)))
		req = append(req, []byte(host)...)
	case ip.To4() != nil:
		req = append(req, 0x01)
		req = append(req, ip.To4()...)
	default:
		req = append(req, 0x04)
		req = append(req, ip.To16()...)
	}
	req = append(req, byte(port>>8), byte(port))
	if _, err := conn.Write(req); err != nil {
		return err
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return err
	}
	if head[0] != 0x05 {
		return fmt.Errorf("socks5: bad reply version %d", head[0])
	}
	if head[1] != 0x00 {
		return fmt.Errorf("socks5: connect failed, rep=0x%02x", head[1])
	}
	var addrLen int
	switch head[3] {
	case 0x01:
		addrLen = 4
	case 0x04:
		addrLen = 16
	case 0x03:
		l := make([]byte, 1)
		if _, err := io.ReadFull(conn, l); err != nil {
			return err
		}
		addrLen = int(l[0])
	default:
		return fmt.Errorf("socks5: unknown atyp 0x%02x", head[3])
	}
	if addrLen > 0 {
		rest := make([]byte, addrLen+2)
		if _, err := io.ReadFull(conn, rest); err != nil {
			return err
		}
	}
	return nil
}

func socks5UserPass(conn net.Conn, user, pass string) error {
	if len(user) > 255 || len(pass) > 255 {
		return fmt.Errorf("socks5: credentials too long")
	}
	pkt := []byte{0x01, byte(len(user))}
	pkt = append(pkt, []byte(user)...)
	pkt = append(pkt, byte(len(pass)))
	pkt = append(pkt, []byte(pass)...)
	if _, err := conn.Write(pkt); err != nil {
		return err
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return err
	}
	if reply[1] != 0x00 {
		return fmt.Errorf("socks5: user/pass authentication failed")
	}
	return nil
}

// --- HTTP CONNECT helpers ---

func httpConnectTunnel(ctx context.Context, proxyAddr, target string, timeout time.Duration) (net.Conn, error) {
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, err
	}
	req := "CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, err
	}
	parts := splitFields(statusLine)
	if len(parts) < 2 {
		conn.Close()
		return nil, fmt.Errorf("malformed status line: %q", statusLine)
	}
	code, _ := strconv.Atoi(parts[1])
	if code != 200 {
		conn.Close()
		return nil, fmt.Errorf("CONNECT failed: %s", statusLine)
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	return conn, nil
}

func splitFields(s string) []string {
	var out []string
	cur := ""
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
