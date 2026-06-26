// Package proxy — socks5.go: minimal SOCKS5 server (no-auth and username/password).
package proxy

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/Samsepik9/PolyProxy/internal/conntrack"
	"github.com/Samsepik9/PolyProxy/internal/pool"
)

// SOCKS5Server is a local SOCKS5 proxy that forwards through the upstream pool.
//
// Authentication: when a username is supplied in the SOCKS5 handshake, the
// username is interpreted as the *preferred upstream name* (same convention as
// the HTTP proxy). Password is ignored (the upstream itself handles its own
// credentials, if any).
type SOCKS5Server struct {
	Pool   *pool.Pool
	Dialer *Dialer
	Cm     *conntrack.Manager
}

// ListenAndServe binds to addr and serves until ctx is cancelled.
func (s *SOCKS5Server) ListenAndServe(ctx context.Context, addr string) error {
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

func (s *SOCKS5Server) handle(client net.Conn) {
	defer safeClose(client)
	_ = client.SetReadDeadline(time.Now().Add(30 * time.Second))

	// Greeting
	head := make([]byte, 2)
	if _, err := io.ReadFull(client, head); err != nil {
		log.Printf("[socks5] read greeting: %v", err)
		return
	}
	if head[0] != 0x05 {
		log.Printf("[socks5] bad version %d", head[0])
		return
	}
	nMethods := int(head[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(client, methods); err != nil {
		log.Printf("[socks5] read methods: %v", err)
		return
	}
	log.Printf("[socks5] from %s methods=%x", client.RemoteAddr(), methods)

	preferred := ""
	password := ""
	var chosen byte = 0xFF

	for _, m := range methods {
		switch m {
		case 0x00:
			chosen = 0x00
		case 0x02:
			if chosen != 0x00 {
				chosen = 0x02
			}
		}
	}
	if chosen == 0xFF {
		_, _ = client.Write([]byte{0x05, 0xFF})
		return
	}
	if _, err := client.Write([]byte{0x05, chosen}); err != nil {
		return
	}

	if chosen == 0x02 {
		// Username/password auth (RFC 1929). Username = preferred upstream.
		// Frame: ver(1) ulen(1) uname(ulen) plen(1) passwd(plen)
		log.Printf("[socks5] doing user/pass auth")
		ver := make([]byte, 1)
		if _, err := io.ReadFull(client, ver); err != nil {
			log.Printf("[socks5] read auth ver: %v", err)
			return
		}
		if ver[0] != 0x01 {
			log.Printf("[socks5] bad auth ver %d", ver[0])
			return
		}
		ul := make([]byte, 1)
		if _, err := io.ReadFull(client, ul); err != nil {
			log.Printf("[socks5] read ul: %v", err)
			return
		}
		userBuf := make([]byte, ul[0])
		if _, err := io.ReadFull(client, userBuf); err != nil {
			log.Printf("[socks5] read userBuf: %v", err)
			return
		}
		preferred = string(userBuf)

		pl := make([]byte, 1)
		if _, err := io.ReadFull(client, pl); err != nil {
			log.Printf("[socks5] read pl: %v", err)
			return
		}
		passBuf := make([]byte, pl[0])
		if _, err := io.ReadFull(client, passBuf); err != nil {
			log.Printf("[socks5] read passBuf: %v", err)
			return
		}
		password = string(passBuf)
		log.Printf("[socks5] auth user=%q pass-len=%d", preferred, len(password))

		// Always report success — the user is local, we only use the username
		// to decide which upstream to forward through.
		if _, err := client.Write([]byte{0x01, 0x00}); err != nil {
			log.Printf("[socks5] write auth reply: %v", err)
			return
		}
		_ = password
	}

	// Request
	reqHead := make([]byte, 4)
	if _, err := io.ReadFull(client, reqHead); err != nil {
		return
	}
	if reqHead[0] != 0x05 || reqHead[1] != 0x01 {
		_, _ = client.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	var host string
	switch reqHead[3] {
	case 0x01:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(client, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 0x03:
		l := make([]byte, 1)
		if _, err := io.ReadFull(client, l); err != nil {
			return
		}
		buf := make([]byte, l[0])
		if _, err := io.ReadFull(client, buf); err != nil {
			return
		}
		host = string(buf)
	case 0x04:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(client, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	default:
		_, _ = client.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	portB := make([]byte, 2)
	if _, err := io.ReadFull(client, portB); err != nil {
		return
	}
	port := uint16(portB[0])<<8 | uint16(portB[1])

	entry := &conntrack.Entry{
		ID:         newID(),
		StartTime:  time.Now(),
		InboundNet: conntrack.NetTCP,
		Inbound:    conntrack.ProtoSOCKS5,
		Source:     client.RemoteAddr().String(),
		Host:       host,
		Target:     net.JoinHostPort(host, strconv.Itoa(int(port))),
	}
	s.Cm.Register(entry)
	defer s.Cm.Unregister(entry.ID)
	defer entry.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	upstream, proxyName, err := dialWithFailover(ctx, s.Pool, s.Dialer, preferred, host, uint16(port))
	if err != nil {
		log.Printf("[socks5] all proxies failed for %s: %v", host, err)
		_, _ = client.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	entry.Proxy = proxyName
	defer safeClose(upstream)

	// Success reply: bind to 0.0.0.0:0
	if _, err := client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	pipe(client, upstream, entry)
}