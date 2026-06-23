// Package proxy — dialer.go: dial out to the chosen upstream.
//
// Supports three upstream types:
//   - direct : plain TCP / UDP dial
//   - http   : HTTP CONNECT tunnel
//   - socks5 : SOCKS5 handshake
package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/jeff/proxypool/internal/pool"
)

// Dialer dials through the chosen upstream.
type Dialer struct {
	timeout time.Duration
}

// NewDialer creates a Dialer with the given dial timeout.
func NewDialer(timeout time.Duration) *Dialer {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Dialer{timeout: timeout}
}

// Dial connects to host:port through the supplied upstream.
// For "direct" upstreams it dials host:port directly.
// For "http" upstreams it sends an HTTP CONNECT and returns the tunnelled
// connection.
// For "socks5" upstreams it performs the SOCKS5 handshake.
func (d *Dialer) Dial(ctx context.Context, up *pool.Upstream, host string, port uint16) (net.Conn, error) {
	if up == nil {
		return nil, errors.New("upstream is nil")
	}
	switch up.Type {
	case "direct":
		addr := joinedHost(host, port)
		return (&net.Dialer{Timeout: d.timeout}).DialContext(ctx, "tcp", addr)
	case "http":
		return d.dialHTTP(ctx, up, host, port)
	case "socks5":
		return d.dialSOCKS5(ctx, up, host, port)
	default:
		return nil, fmt.Errorf("unsupported upstream type %q", up.Type)
	}
}

// dialHTTP sends `CONNECT host:port HTTP/1.1` to the HTTP upstream and returns
// the tunnelled connection on 200 OK.
func (d *Dialer) dialHTTP(ctx context.Context, up *pool.Upstream, host string, port uint16) (net.Conn, error) {
	raw, err := (&net.Dialer{Timeout: d.timeout}).DialContext(ctx, "tcp", up.Addr)
	if err != nil {
		return nil, fmt.Errorf("dial http upstream %s: %w", up.Addr, err)
	}
	req := &httpConnectRequest{
		Host: joinedHost(host, port),
		Auth: basicAuthHeader(up.Username, up.Password),
	}
	if err := req.Write(raw); err != nil {
		_ = raw.Close()
		return nil, err
	}
	resp, err := readHTTPResponse(raw)
	if err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("read upstream response: %w", err)
	}
	if resp.StatusCode != 200 {
		_ = raw.Close()
		return nil, fmt.Errorf("upstream CONNECT failed: %s", resp.Status)
	}
	return raw, nil
}

// dialSOCKS5 performs a SOCKS5 handshake against the upstream and returns
// the tunnelled connection. Supports username/password auth (RFC 1929).
func (d *Dialer) dialSOCKS5(ctx context.Context, up *pool.Upstream, host string, port uint16) (net.Conn, error) {
	raw, err := (&net.Dialer{Timeout: d.timeout}).DialContext(ctx, "tcp", up.Addr)
	if err != nil {
		return nil, fmt.Errorf("dial socks5 upstream %s: %w", up.Addr, err)
	}
	if err := socks5Handshake(raw, host, port, up.Username, up.Password); err != nil {
		_ = raw.Close()
		return nil, err
	}
	return raw, nil
}

// --- HTTP CONNECT helpers ---

type httpConnectRequest struct {
	Host string
	Auth string // optional "Basic xxx"
}

func (r *httpConnectRequest) Write(w io.Writer) error {
	var b []byte
	b = append(b, "CONNECT "...)
	b = append(b, r.Host...)
	b = append(b, " HTTP/1.1\r\nHost: "...)
	b = append(b, r.Host...)
	b = append(b, "\r\n"...)
	if r.Auth != "" {
		b = append(b, "Proxy-Authorization: "...)
		b = append(b, r.Auth...)
		b = append(b, "\r\n"...)
	}
	b = append(b, "\r\n"...)
	_, err := w.Write(b)
	return err
}

type httpResponse struct {
	Status     string
	StatusCode int
}

func readHTTPResponse(r io.Reader) (*httpResponse, error) {
	br := bufio.NewReader(r)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		return nil, err
	}
	parts := splitFields(statusLine)
	if len(parts) < 2 {
		return nil, fmt.Errorf("malformed status line: %q", statusLine)
	}
	code, _ := strconv.Atoi(parts[1])
	// drain headers
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	return &httpResponse{Status: statusLine, StatusCode: code}, nil
}

func basicAuthHeader(user, pass string) string {
	if user == "" && pass == "" {
		return ""
	}
	u := &url.URL{User: url.UserPassword(user, pass)}
	return "Basic " + u.String()[len("//"):]
}

// --- SOCKS5 helpers ---

func socks5Handshake(conn net.Conn, host string, port uint16, user, pass string) error {
	// Greeting: ver, nmethods, methods...
	var greeting []byte
	if user != "" || pass != "" {
		greeting = []byte{0x05, 0x01, 0x02} // ver=5, nmethods=1, method=username/password
	} else {
		greeting = []byte{0x05, 0x01, 0x00} // ver=5, nmethods=1, method=no-auth
	}
	if _, err := conn.Write(greeting); err != nil {
		return err
	}
	// Reply: ver, method
	method := make([]byte, 2)
	if _, err := io.ReadFull(conn, method); err != nil {
		return err
	}
	if method[0] != 0x05 {
		return fmt.Errorf("socks5: bad server version %d", method[0])
	}
	switch method[1] {
	case 0x00:
		// no auth
	case 0x02:
		if user == "" {
			return errors.New("socks5 upstream requires username/password")
		}
		if err := socks5UserPass(conn, user, pass); err != nil {
			return err
		}
	case 0xFF:
		return errors.New("socks5: no acceptable auth method")
	default:
		return fmt.Errorf("socks5: unknown method 0x%02x", method[1])
	}
	// Request: ver, cmd=CONNECT, rsv, atyp, addr, port
	var req []byte
	req = append(req, 0x05, 0x01, 0x00)
	ip := net.ParseIP(host)
	switch {
	case ip == nil:
		// domain
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
	// Reply: ver, rep, rsv, atyp, addr, port
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
		return errors.New("socks5: credentials too long")
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
		return errors.New("socks5: user/pass authentication failed")
	}
	return nil
}

// splitFields splits on ASCII whitespace.
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