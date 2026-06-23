#!/usr/bin/env python3
"""Tiny test echo servers used by the proxypool e2e harness."""
import socket
import threading
import sys

def echo_socks5_server(port: int):
    """Minimal SOCKS5 (no-auth) echo server: whatever bytes you send, it sends back."""
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind(("127.0.0.1", port))
    s.listen(64)
    print(f"[echo-socks5] listening on 127.0.0.1:{port}", flush=True)
    while True:
        c, addr = s.accept()
        threading.Thread(target=_handle_socks5, args=(c, addr), daemon=True).start()

def _handle_socks5(c, addr):
    try:
        # greeting
        head = c.recv(2)
        if len(head) < 2 or head[0] != 0x05:
            return
        nmethods = head[1]
        c.recv(nmethods)
        # reply: no-auth
        c.sendall(b"\x05\x00")
        # request: VER CMD RSV ATYP ...
        req = c.recv(4)
        if len(req) < 4:
            return
        atyp = req[3]
        if atyp == 0x01:
            c.recv(4)
        elif atyp == 0x03:
            l = c.recv(1)[0]
            c.recv(l)
        elif atyp == 0x04:
            c.recv(16)
        else:
            return
        c.recv(2)  # port
        # success reply, bind 0.0.0.0:0
        c.sendall(b"\x05\x00\x00\x01\x00\x00\x00\x00\x00\x00")
        # echo until client closes
        while True:
            data = c.recv(4096)
            if not data:
                break
            c.sendall(data)
    except Exception as e:
        print(f"[echo-socks5] {addr}: {e}", file=sys.stderr, flush=True)
    finally:
        c.close()

def echo_http_server(port: int):
    """Tiny HTTP CONNECT-tunnel echo: responds to CONNECT with 200, then echoes bytes."""
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind(("127.0.0.1", port))
    s.listen(64)
    print(f"[echo-http]   listening on 127.0.0.1:{port}", flush=True)
    while True:
        c, addr = s.accept()
        threading.Thread(target=_handle_http, args=(c, addr), daemon=True).start()

def _handle_http(c, addr):
    try:
        # read request line
        buf = b""
        while b"\r\n\r\n" not in buf:
            chunk = c.recv(4096)
            if not chunk:
                return
            buf += chunk
        request_line = buf.split(b"\r\n", 1)[0].decode(errors="replace")
        if request_line.startswith("CONNECT"):
            c.sendall(b"HTTP/1.1 200 Connection Established\r\n\r\n")
            # echo
            while True:
                data = c.recv(4096)
                if not data:
                    break
                c.sendall(data)
        else:
            # plain HTTP — read full request and echo the headers + body marker
            c.sendall(b"HTTP/1.1 200 OK\r\nContent-Length: 12\r\nConnection: close\r\n\r\nECHO-HTTP-OK")
    except Exception as e:
        print(f"[echo-http] {addr}: {e}", file=sys.stderr, flush=True)
    finally:
        c.close()

if __name__ == "__main__":
    t1 = threading.Thread(target=echo_socks5_server, args=(11080,), daemon=True)
    t2 = threading.Thread(target=echo_http_server, args=(11081,), daemon=True)
    t1.start(); t2.start()
    try:
        threading.Event().wait()  # run forever
    except KeyboardInterrupt:
        pass