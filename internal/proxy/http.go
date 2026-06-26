package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Samsepik9/PolyProxy/internal/conntrack"
	"github.com/Samsepik9/PolyProxy/internal/pool"
)

// HTTPServer is a local HTTP proxy. It accepts:
//   - CONNECT host:port for arbitrary TCP (HTTPS-style tunnelling)
//   - GET/POST http://host/...  (plain HTTP forward through upstream)
type HTTPServer struct {
	Pool   *pool.Pool
	Dialer *Dialer
	Cm     *conntrack.Manager
}

// ListenAndServe binds to addr and serves until ctx is cancelled.
func (s *HTTPServer) ListenAndServe(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.handle(conn)
	}
}

// dialWithFailover tries to connect through proxies in sequence.
// If preferred is set (user-specified proxy), only that proxy is tried.
// Otherwise it attempts each healthy proxy until one succeeds.
func dialWithFailover(ctx context.Context, p *pool.Pool, d *Dialer, preferred, host string, port uint16) (net.Conn, string, error) {
	if preferred != "" {
		// User specified a specific proxy — no failover, respect the choice
		up, err := p.Select(preferred, host)
		if err != nil {
			return nil, "", fmt.Errorf("pool select %q: %w", preferred, err)
		}
		conn, err := d.Dial(ctx, up, host, port)
		if err != nil {
			return nil, up.Name, fmt.Errorf("dial %s (%s): %w", up.Name, up.Addr, err)
		}
		return conn, up.Name, nil
	}

	// No preferred proxy — try each healthy upstream in strategy order.
	attempted := map[string]bool{}
	lastErr := errors.New("no healthy proxies available")
	for attempts := 0; attempts < len(p.List())*2; attempts++ {
		up, err := p.Select("", host)
		if err != nil {
			return nil, "", err
		}
		if attempted[up.Name] {
			continue
		}
		attempted[up.Name] = true
		if up.Type == "direct" {
			conn, err := d.Dial(ctx, up, host, port)
			if err == nil {
				return conn, up.Name, nil
			}
			lastErr = fmt.Errorf("direct dial: %w", err)
			continue
		}
		if !up.Healthy() {
			lastErr = fmt.Errorf("proxy %s (%s) marked unhealthy", up.Name, up.Addr)
			continue
		}
		conn, err := d.Dial(ctx, up, host, port)
		if err == nil {
			return conn, up.Name, nil
		}
		log.Printf("[proxy] proxy %s (%s) failed: %v, trying next", up.Name, up.Addr, err)
		lastErr = fmt.Errorf("dial %s (%s): %w", up.Name, up.Addr, err)
	}
	return nil, "", lastErr
}

func (s *HTTPServer) handle(client net.Conn) {
	defer safeClose(client)
	_ = client.SetReadDeadline(time.Now().Add(30 * time.Second))
	br := bufio.NewReader(client)
	req, err := http.ReadRequest(br)
	if err != nil {
		log.Printf("[http] bad request from %s: %v", client.RemoteAddr(), err)
		return
	}
	_ = client.SetReadDeadline(time.Time{})

	// Username in proxy auth = preferred upstream name.
	preferred := ""
	if req.URL.User != nil {
		preferred = req.URL.User.Username()
	}

	switch req.Method {
	case http.MethodConnect:
		s.handleConnect(client, req, preferred)
	default:
		s.handleForward(client, br, req, preferred)
	}
}

func (s *HTTPServer) handleConnect(client net.Conn, req *http.Request, preferred string) {
	host, portStr := splitHostPort(req.Host)
	port := parsePort(portStr, 443)

	entry := &conntrack.Entry{
		ID:         newID(),
		StartTime:  time.Now(),
		InboundNet: conntrack.NetTCP,
		Inbound:    conntrack.ProtoHTTP,
		Source:     client.RemoteAddr().String(),
		Host:       host,
		Target:     joinedHost(host, uint16(port)),
	}
	s.Cm.Register(entry)
	defer s.Cm.Unregister(entry.ID)
	defer entry.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	upstream, proxyName, err := dialWithFailover(ctx, s.Pool, s.Dialer, preferred, host, uint16(port))
	if err != nil {
		log.Printf("[http] all proxies failed for %s: %v", host, err)
		_, _ = client.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	entry.Proxy = proxyName
	defer safeClose(upstream)

	if _, err := client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return
	}
	pipe(client, upstream, entry)
}

func (s *HTTPServer) handleForward(client net.Conn, br *bufio.Reader, req *http.Request, preferred string) {
	host := req.URL.Hostname()
	port := parsePort(req.URL.Port(), 80)

	entry := &conntrack.Entry{
		ID:         newID(),
		StartTime:  time.Now(),
		InboundNet: conntrack.NetTCP,
		Inbound:    conntrack.ProtoHTTP,
		Source:     client.RemoteAddr().String(),
		Host:       host,
		Target:     joinedHost(host, uint16(port)),
	}
	s.Cm.Register(entry)
	defer s.Cm.Unregister(entry.ID)
	defer entry.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	upstream, proxyName, err := dialWithFailover(ctx, s.Pool, s.Dialer, preferred, host, uint16(port))
	if err != nil {
		log.Printf("[http] all proxies failed for %s: %v", host, err)
		_, _ = client.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	entry.Proxy = proxyName
	defer safeClose(upstream)

	// Forward the original request via the upstream.
	if err := req.Write(upstream); err != nil {
		return
	}
	pipe(client, upstream, entry)
}

// pipe shuttles bytes between client and upstream, updating counters and
// aborting when the conntrack entry is closed by the user via the API.
func pipe(client, upstream net.Conn, entry *conntrack.Entry) {
	errCh := make(chan struct{}, 2)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := client.Read(buf)
			if n > 0 {
				entry.AddUpload(n)
				if _, werr := upstream.Write(buf[:n]); werr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		errCh <- struct{}{}
	}()
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := upstream.Read(buf)
			if n > 0 {
				entry.AddDownload(n)
				if _, werr := client.Write(buf[:n]); werr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		errCh <- struct{}{}
	}()
	select {
	case <-entry.Done():
		_ = client.Close()
		_ = upstream.Close()
	case <-errCh:
		_ = client.Close()
		_ = upstream.Close()
		<-errCh // wait for the other side
	}
	_ = io.Discard
}

func splitHostPort(addr string) (string, string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, ""
	}
	return host, port
}

func parsePort(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 || n > 65535 {
		return def
	}
	return n
}

var _ = strings.HasPrefix // keep import for future use