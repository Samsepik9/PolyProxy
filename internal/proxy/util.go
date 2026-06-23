// Package proxy implements the local HTTP and SOCKS5 proxy servers that
// forward traffic through the configured upstream pool.
package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"time"
)

// newID returns a short random hex identifier for a connection entry.
func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// joinedHost returns host:port suitable for dialling.
// If host is already an IP it is returned verbatim, otherwise it goes through
// net.JoinHostPort which handles IPv6 correctly.
func joinedHost(host string, port uint16) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(host, uint16ToStr(port))
}

func uint16ToStr(p uint16) string {
	// avoid strconv import for one call site
	if p == 0 {
		return "0"
	}
	var buf [5]byte
	i := len(buf)
	for p > 0 {
		i--
		buf[i] = byte('0' + p%10)
		p /= 10
	}
	return string(buf[i:])
}

// safeClose closes c and logs nothing — used in defer paths.
func safeClose(c net.Conn) {
	if c != nil {
		_ = c.Close()
	}
}

// copyDeadline returns the deadline the io.Copy should respect, honouring the
// caller's overall timeout.
func copyDeadline(timeout time.Duration) time.Time {
	if timeout <= 0 {
		return time.Time{}
	}
	return time.Now().Add(timeout)
}