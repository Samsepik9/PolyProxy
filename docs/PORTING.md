# Porting guide

PolyProxy is written in **pure Go** with no cgo. That means any platform Go
supports can build and run it without modifications — no libc, no glibc
quirks, no system headers.

## 1. Cross-compile the matrix

```bash
./scripts/build.sh all
```

This produces:

```
bin/
├── polyproxy-linux-amd64
├── polyproxy-linux-arm64
├── polyproxy-linux-arm       # ARMv6/v7 (Raspberry Pi, OpenWrt, etc.)
├── polyproxy-darwin-amd64    # Intel Mac
├── polyproxy-darwin-arm64    # Apple Silicon
├── polyproxy-windows-amd64.exe
└── polyproxy-windows-arm64.exe
```

## 2. One-off build for an exotic target

```bash
# FreeBSD amd64
GOOS=freebsd GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' \
  -o bin/polyproxy-freebsd-amd64 ./cmd/proxypool

# OpenWrt (mipsel)
GOOS=linux GOARCH=mipsle CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' \
  -o bin/polyproxy-openwrt-mipsel ./cmd/proxypool

# RISC-V64
GOOS=linux GOARCH=riscv64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' \
  -o bin/polyproxy-linux-riscv64 ./cmd/proxypool
```

`GOOS=js GOARCH=wasm` will technically build, but won't bind TCP sockets —
use one of the targets above.

## 3. Configuration paths per OS

The binary picks the right per-user config path automatically:

| GOOS         | Path                                                |
|--------------|-----------------------------------------------------|
| `linux`      | `$XDG_CONFIG_HOME/PolyProxy/config.yaml` (default `~/.config/PolyProxy/`) |
| `darwin`     | `~/Library/Application Support/PolyProxy/config.yaml` |
| `windows`    | `%APPDATA%\PolyProxy\config.yaml`                   |
| `freebsd`    | `~/.config/PolyProxy/config.yaml`                   |
| `openbsd`    | `~/.config/PolyProxy/config.yaml`                   |
| `netbsd`     | `~/.config/PolyProxy/config.yaml`                   |
| everything else | `~/.config/PolyProxy/config.yaml`                |

If you need to override the path for testing on a new platform, pass
`-config /full/path/to/config.yaml`.

## 4. Running on routers / OpenWrt

1. `scp bin/polyproxy-openwrt-mipsel root@router:/usr/bin/polyproxy`
2. `chmod +x /usr/bin/polyproxy`
3. Drop your config at `/etc/PolyProxy/config.yaml`
4. Start it: `/usr/bin/polyproxy -config /etc/PolyProxy/config.yaml &`
5. To survive reboot, add an init script under `/etc/init.d/` or use
   `/etc/rc.local`.

The web dashboard listens on `127.0.0.1:9090` by default — if you want to
expose it on the LAN, change `api_listen` to `0.0.0.0:9090` and add a
firewall rule.

## 5. Adding a new platform in code

No code change is needed. If you want the binary to **prefer** a different
config path on, say, FreeBSD, edit `config.DefaultConfigPath()` in
`internal/config/config.go`:

```go
case "freebsd":
    return filepath.Join(home, ".config", "PolyProxy", "config.yaml")
```

That's it — the rest of the binary is pure stdlib.

## 6. Memory / CPU budget

Rough sizing on commodity hardware:

| Connections | RSS   | CPU  |
|-------------|-------|------|
| 100         | ~12 MB | <1% |
| 1 000       | ~20 MB | ~3% |
| 10 000      | ~80 MB | ~25%|

The `pipe()` goroutine pair uses a 32 KB buffer per direction; very small
allocations. There's no per-connection GC churn.

## 7. Hardening checklist

- Bind the API to `127.0.0.1` (default). If you must expose it, put it behind
  a reverse proxy with auth.
- Use `unix` sockets instead of TCP for local-only deployments: pass
  `http_listen: "unix:/tmp/polyproxy.sock"` (requires a 3-line patch to
  `ListenAndServe` — happy to add it).
- For high-throughput, raise the file-descriptor limit:
  `ulimit -n 65535` (Linux / BSD) before starting.
