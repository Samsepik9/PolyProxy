# ProxyPool

> A local HTTP + SOCKS5 proxy with a **pool of upstream proxies**, **real-time
> connection monitoring**, and a **web dashboard** — written in Go, single
> static binary, runs on Linux / macOS / Windows.

Inspired by Clash / Clash Verge's *Connections* panel. Built for one job: give
you a single local port that fans out across N upstream proxies, with a live
view of every byte that flows through it.

```
┌────────────────────────┐      ┌─────────────────────────────┐
│  Your apps / curl /    │      │     ProxyPool (this tool)   │
│  browser / OS proxy    │ ───▶ │  ┌──────────┐  ┌────────┐  │
│                        │      │  │ HTTP     │  │ Web UI │  │
│  127.0.0.1:7890 (HTTP) │      │  │ :7890    │  │ :9090  │  │
│  127.0.0.1:7891 (SOCKS5)│     │  ├──────────┤  └────────┘  │
└────────────────────────┘      │  │ SOCKS5   │              │
                                │  │ :7891    │              │
                                │  └────┬─────┘              │
                                │       │                    │
                                │   ┌───▼──────────┐         │
                                │   │ Proxy pool   │         │
                                │   │ random / rr / │         │
                                │   │ hash / name  │         │
                                │   └─┬───┬───┬─────┘         │
                                └─────┼───┼───┼───────────────┘
                                      │   │   │
                          ┌───────────┘   │   └────────────┐
                          ▼               ▼                ▼
                    upstream-A       upstream-B        upstream-C
                     (HTTP)           (SOCKS5)         (direct)
```

---

## Table of contents

1. [Features](#1-features)
2. [Quick start (60 seconds)](#2-quick-start-60-seconds)
3. [Detailed walkthrough](#3-detailed-walkthrough)
   - 3.1 [Prerequisites](#31-prerequisites)
   - 3.2 [Build](#32-build)
   - 3.3 [Configure](#33-configure)
   - 3.4 [Run](#34-run)
   - 3.5 [Use the proxy](#35-use-the-proxy)
   - 3.6 [Open the dashboard](#36-open-the-dashboard)
4. [Selection strategies](#4-selection-strategies)
5. [Pinning a specific upstream (per-request)](#5-pinning-a-specific-upstream-per-request)
6. [Web dashboard reference](#6-web-dashboard-reference)
7. [REST API reference](#7-rest-api-reference)
8. [Cross-platform guide](#8-cross-platform-guide)
9. [Health checking](#9-health-checking)
10. [Logging & troubleshooting](#10-logging--troubleshooting)
11. [Security checklist](#11-security-checklist)
12. [Project layout](#12-project-layout)
13. [Development](#13-development)
14. [Roadmap](#14-roadmap)
15. [FAQ](#15-faq)

---

## 1. Features

| Capability | Detail |
|---|---|
| **Local proxy ports** | HTTP (`7890`, both `CONNECT` and plain `GET` forward) + SOCKS5 (`7891`) |
| **Proxy pool** | Mix `direct`, `http`, `socks5` upstreams freely |
| **Selection strategies** | `random` (default), `round-robin`, `hash` (sticky per host), `name` (first) |
| **Per-request pinning** | Proxy-auth username = upstream name → force that upstream for this request |
| **Live connections** | Every active tunnel: id, host, target, proxy, source, upload/download bytes, start time, age |
| **Web dashboard** | Auto-refreshes every 1 s, filter by host/target/proxy, close one or all, dark theme |
| **REST API** | `/api/connections`, `/api/connections/:id`, `/api/proxies`, `/api/stats`, `/api/healthz` |
| **Cross-platform** | Linux (amd64 / arm64 / armv7), macOS (amd64 / arm64), Windows (amd64 / arm64) — single static binary, zero runtime deps |
| **Embeddable** | UI assets compiled into the binary via `go:embed` — no filesystem dependencies |
| **Configurable** | YAML config at a per-user path; CLI flag to override |
| **Health checks** | Optional background TCP-dial health checks that mark upstreams down |
| **Low footprint** | ~6 MB binary, ~12 MB RSS for 100 active connections, no GC pressure |

---

## 2. Quick start (60 seconds)

If you have **Go 1.22+** installed and you're on a Unix machine:

```bash
# Clone or cd into the project
cd proxypool

# Build for the current host
./scripts/build.sh

# Launch — first run auto-creates a config file you can edit
./scripts/start.sh
```

Then in another terminal:

```bash
# Random upstream
curl -x http://127.0.0.1:7890 https://api.ipify.org

# Open the dashboard
open http://127.0.0.1:9090         # macOS
# xdg-open http://127.0.0.1:9090   # Linux
# start http://127.0.0.1:9090      # Windows
```

That's it. Skip to [§3.5](#35-use-the-proxy) for more usage patterns.

---

## 3. Detailed walkthrough

### 3.1 Prerequisites

| Requirement | Why | Min version |
|---|---|---|
| **OS** | Tested on macOS 14+, Ubuntu 22.04+, Windows 10+ | — |
| **Go** (only for building from source) | Compiler | **1.22+** |
| **A C toolchain** | Not needed — we build with `CGO_ENABLED=0` | — |
| **Open TCP ports** 7890, 7891, 9090 | Local proxy + dashboard | 1 free port each |
| **~10 MB disk** | Static binary | — |

Check Go:

```bash
go version    # should print go1.22 or newer
```

If you don't have Go, you can still use pre-built binaries from `bin/` (after
running `./scripts/build.sh` once on a machine that does have Go), or copy
the binary to a machine that doesn't.

### 3.2 Build

From the project root:

```bash
# Build only for the current host (fastest)
./scripts/build.sh

# Build for all common platforms (Linux x3, macOS x2, Windows x2)
./scripts/build.sh all

# Build for one specific target
./scripts/build.sh linux arm64
./scripts/build.sh windows amd64
```

Output goes to `bin/`:

```
bin/
├── proxypool-darwin-arm64              # 6.0 MB  ← you
├── proxypool-darwin-amd64              # 6.4 MB
├── proxypool-linux-amd64               # 6.2 MB
├── proxypool-linux-arm64               # 5.9 MB
├── proxypool-linux-arm                 # 6.1 MB  ← Raspberry Pi / OpenWrt
├── proxypool-windows-amd64.exe         # 6.4 MB
└── proxypool-windows-arm64.exe         # 5.9 MB
```

The script uses `CGO_ENABLED=0`, so binaries are **fully static** and run on
any glibc/musl libc, no `libc.so` or system DLLs required.

Verify what you got:

```bash
file bin/proxypool-darwin-arm64
# → Mach-O 64-bit executable arm64

file bin/proxypool-linux-amd64
# → ELF 64-bit LSB executable, x86-64, statically linked, stripped

file bin/proxypool-windows-amd64.exe
# → PE32+ executable (console) x86-64, for MS Windows
```

### 3.3 Configure

#### First run — auto-bootstrap

```bash
./scripts/start.sh
```

On the first run, `start.sh` will:

1. Detect your OS.
2. Pick the per-user config path:
   - macOS: `~/Library/Application Support/proxypool/config.yaml`
   - Linux: `~/.config/proxypool/config.yaml` (or `$XDG_CONFIG_HOME/proxypool/`)
   - Windows: `%APPDATA%\proxypool\config.yaml`
3. Copy `configs/config.example.yaml` there.
4. Print the path and exit so you can edit it.

After editing, run `./scripts/start.sh` again to actually launch.

#### Manual config

The config file is plain YAML. Minimal working example:

```yaml
server:
  http_listen:   "127.0.0.1:7890"
  socks5_listen: "127.0.0.1:7891"
  api_listen:    "127.0.0.1:9090"
  api_enable:    true

pool:
  strategy: random
  health_check: false
  proxies:
    - { name: direct, type: direct }     # always include at least one
    - name: us-1
      type: http
      server: 1.2.3.4
      port: 8080
      username: alice                   # optional
      password: s3cr3t                  # optional
    - name: jp-1
      type: socks5
      server: 5.6.7.8
      port: 1080
```

Full annotated example: see [`configs/config.example.yaml`](configs/config.example.yaml).

#### Config field reference

| Field | Type | Default | Notes |
|---|---|---|---|
| `server.http_listen` | `host:port` | `127.0.0.1:7890` | Set to `""` to disable HTTP entry |
| `server.socks5_listen` | `host:port` | `127.0.0.1:7891` | Set to `""` to disable SOCKS5 entry |
| `server.api_listen` | `host:port` | `127.0.0.1:9090` | Set to `""` to disable dashboard |
| `server.api_enable` | bool | `true` | Master switch for the dashboard / API |
| `pool.strategy` | enum | `random` | `random` \| `round-robin` \| `hash` \| `name` |
| `pool.health_check` | bool | `false` | Enable background TCP-dial checks every 30 s |
| `pool.proxies[].name` | string | **required** | Unique within the pool |
| `pool.proxies[].type` | enum | **required** | `direct` \| `http` \| `socks5` |
| `pool.proxies[].server` | string | required for `http`/`socks5` | Hostname or IP |
| `pool.proxies[].port` | int | required for `http`/`socks5` | 1–65535 |
| `pool.proxies[].username` | string | optional | Forwarded to upstream |
| `pool.proxies[].password` | string | optional | Forwarded to upstream |

### 3.4 Run

```bash
# Easiest — uses the per-user config and prebuilt binary
./scripts/start.sh

# Or run the binary directly
./bin/proxypool-darwin-arm64 -config configs/config.example.yaml

# Or via go run (no build step)
go run ./cmd/proxypool -config configs/config.example.yaml
```

Expected startup output:

```
2026/06/15 10:23:45 proxypool started · strategy=random · 3 upstreams
2026/06/15 10:23:45 [http]   listening on 127.0.0.1:7890
2026/06/15 10:23:45 [socks5] listening on 127.0.0.1:7891
2026/06/15 10:23:45 [api]    dashboard on http://127.0.0.1:9090
```

Stop with **Ctrl-C** (or `kill -TERM <pid>`). On shutdown:

```
2026/06/15 10:24:01 shutdown signal received, closing connections…
```

#### Run as a background service

```bash
# Foreground first, just to see it works
./bin/proxypool-linux-amd64 -config /etc/proxypool/config.yaml

# Detach with nohup (Linux/macOS)
nohup ./bin/proxypool-linux-amd64 -config /etc/proxypool/config.yaml \
  > /var/log/proxypool.log 2>&1 &
echo $! > /var/run/proxypool.pid

# Stop later
kill $(cat /var/run/proxypool.pid)
```

For a proper service definition, see the
[Cross-platform guide §8](#8-cross-platform-guide) for systemd / launchd /
Windows Service examples.

### 3.5 Use the proxy

#### Pattern 1 — Random upstream (default)

```bash
# HTTP
curl -x http://127.0.0.1:7890 https://api.ipify.org

# HTTPS via CONNECT (curl automatically uses CONNECT for https://)
curl -x http://127.0.0.1:7890 https://www.google.com

# SOCKS5
curl --proxy socks5h://127.0.0.1:7891 https://api.ipify.org
```

Each request independently picks an upstream according to the configured
strategy.

#### Pattern 2 — Pin a specific upstream (per-request)

The proxy-auth **username** is interpreted as the upstream name. This works
identically for both HTTP and SOCKS5 entries.

```bash
# Force the request to use upstream "us-1" (password is ignored)
curl -x http://us-1:any@127.0.0.1:7890 https://api.ipify.org

# Force "direct" — bypass all upstreams, just dial the target
curl -x http://direct:any@127.0.0.1:7890 https://api.ipify.org

# SOCKS5 variant — proxy-auth username is the upstream name
curl --proxy-user 'jp-1:any' --proxy socks5h://127.0.0.1:7891 \
  https://api.ipify.org
```

If the username doesn't match a configured upstream, the request is rejected
with `502 Bad Gateway`. You'll see the attempt (and rejection) in the
dashboard.

#### Pattern 3 — Set as system proxy (macOS / Windows / Linux)

| OS | Where to set it |
|---|---|
| **macOS** | System Settings → Network → your interface → Details → Proxies. Enable "Web Proxy (HTTP)" → `127.0.0.1:7890`, "Secure Web Proxy (HTTPS)" → `127.0.0.1:7890`, "SOCKS Proxy" → `127.0.0.1:7891` |
| **Windows** | Settings → Network & Internet → Proxy → Manual proxy setup. HTTP `127.0.0.1:7890`, HTTPS `127.0.0.1:7890`, SOCKS `127.0.0.1:7891` |
| **Ubuntu/GNOME** | Settings → Network → Network Proxy → Manual. HTTP `127.0.0.1:7890`, HTTPS `127.0.0.1:7890`, SOCKS `127.0.0.1:7891` |

> ⚠️ System-level proxies only work for apps that honor them. Many desktop
> apps (Electron, Discord, Steam, most games) ignore the system proxy. For
> those, use per-app proxy configuration (e.g. `HTTPS_PROXY=...` env var, or
> a tool like ProxyChains-NG / tun2socks).

#### Pattern 4 — Programmatic use

```python
import requests
proxies = {
    "http":  "http://127.0.0.1:7890",
    "https": "http://127.0.0.1:7890",
}
r = requests.get("https://api.ipify.org", proxies=proxies, timeout=10)
print(r.text)
```

```javascript
// Node 18+ with undici
import { fetch, ProxyAgent } from "undici";
const proxyAgent = new ProxyAgent("http://127.0.0.1:7890");
const r = await fetch("https://api.ipify.org", { dispatcher: proxyAgent });
console.log(await r.text());
```

```go
httpClient := &http.Client{
    Transport: &http.Transport{Proxy: http.ProxyURL(mustParse("http://127.0.0.1:7890"))},
}
```

### 3.6 Open the dashboard

Point any browser at **`http://127.0.0.1:9090`**. You'll see two tabs:

- **Connections** — every live tunnel, refreshing once per second.
- **Proxies** — your pool with health status.

The header bar shows aggregate counters: active connections, total upload,
total download, current strategy, pool size.

Click the **✕** on any row to kill that connection, or **Close All** to
drain everything (useful when switching networks or rotating an upstream).

---

## 4. Selection strategies

| Strategy | Behavior | Best for |
|---|---|---|
| `random` | Uniform random among healthy upstreams | Spreading load, evading per-IP rate limits |
| `round-robin` | Cycle through upstreams in order | Even distribution, simple fairness |
| `hash` | `FNV32a(host) % len(pool)` — sticky per destination host | Keeping a session on the same egress IP (e.g. sticky geo-binding) |
| `name` | Always the first healthy upstream | Debugging, "always use my private proxy" |

If the strategy selects an upstream that has been marked unhealthy, the
selection is retried against the remaining healthy upstreams. If all are down,
the **last-selected** one is used anyway (fail-open) and the connection
attempt will fail at dial time — this prevents a complete outage from a
single bad health check.

Set the strategy in the config:

```yaml
pool:
  strategy: hash
```

---

## 5. Pinning a specific upstream (per-request)

This is the killer feature. When you want one specific request to go through
a specific upstream, encode the upstream's name as the proxy-auth username:

| Protocol | How to pin |
|---|---|
| HTTP | `curl -x http://<upstream-name>:any@127.0.0.1:7890 <url>` |
| SOCKS5 | `curl --proxy-user '<upstream-name>:any' --proxy socks5h://127.0.0.1:7891 <url>` |
| Apps | Set the proxy URL with embedded userinfo, e.g. `http://us-1:x@127.0.0.1:7890` |

The password is ignored by ProxyPool — your upstream's own credentials (if
any) live in the `username` / `password` fields of the config, and are
forwarded automatically when dialing.

If you pin to an unknown upstream name, the request gets `502 Bad Gateway`
and the dashboard logs the rejection.

**Use case examples:**

```bash
# Test reachability of each upstream individually
for p in direct us-1 jp-1; do
  echo "=== $p ==="
  curl -s -x "http://$p:x@127.0.0.1:7890" https://api.ipify.org
  echo
done

# Force a sensitive request onto a specific trusted proxy
curl -x "http://trusted-only:x@127.0.0.1:7890" https://my-bank.com/login
```

---

## 6. Web dashboard reference

URL: **`http://127.0.0.1:9090`** (configurable via `server.api_listen`)

### Header

| Field | Meaning |
|---|---|
| **N active** | Number of currently open tunnels |
| **▲ Up** | Total bytes uploaded (client → remote) since process start |
| **▼ Down** | Total bytes downloaded (remote → client) since process start |
| **strategy: X (N)** | Active strategy and pool size |

### Connections tab

| Column | Description |
|---|---|
| **Start** | Wall-clock time the connection was opened |
| **Type** | Inbound protocol: `HTTP` or `SOCKS5` (color-coded pill) |
| **Host** | Destination hostname (CONNECT target, SNI, or SOCKS5 domain) |
| **Target** | Resolved `ip:port` actually dialled |
| **Proxy** | Name of the upstream that served this connection (blue) |
| **Source** | Client `ip:port` (the one speaking to ProxyPool) |
| **▲ Up / ▼ Down** | Bytes transferred in each direction |
| **Age** | Time elapsed since `Start`, formatted `1.2s` / `1m 23s` |
| **✕** | Click to close this specific connection |

The table refreshes **every 1 second** (configurable in `app.js`).

The **filter** box does a case-insensitive substring match on host / target
/ proxy.

**Close All** sends `DELETE /api/connections` and drops every active tunnel.

### Proxies tab

A flat list of every configured upstream with its type, address, and a
green/red health indicator.

---

## 7. REST API reference

All endpoints return JSON. Authentication is **not** included — the API
binds to `127.0.0.1` by default (see [§11 Security](#11-security-checklist)
if you need to expose it).

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/healthz` | Liveness probe — `{"status":"ok"}` |
| `GET` | `/api/connections` | Snapshot of every active connection |
| `DELETE` | `/api/connections` | Close all active connections |
| `DELETE` | `/api/connections/:id` | Close one connection by id |
| `GET` | `/api/proxies` | Upstream list with health |
| `GET` | `/api/stats` | Aggregate counters |
| `GET` | `/` | The dashboard SPA |
| `GET` | `/static/*` | Dashboard assets |

### `GET /api/connections` — response shape

```json
[
  {
    "id":         "7f3a1b2c4d5e6f70",
    "start_time": "2026-06-15T10:23:45.123456+08:00",
    "inbound":    "HTTP",
    "network":    "TCP",
    "source":     "127.0.0.1:51234",
    "host":       "api.ipify.org",
    "target":     "104.21.54.12:443",
    "proxy":      "us-http-1",
    "upload":     412,
    "download":   318,
    "closed":     false
  }
]
```

### `GET /api/stats` — response shape

```json
{
  "active":      3,
  "upload":      1234,
  "download":    5678,
  "strategy":    "random",
  "proxy_count": 3
}
```

### cURL examples

```bash
# All active connections
curl -s http://127.0.0.1:9090/api/connections | jq

# Aggregate counters
curl -s http://127.0.0.1:9090/api/stats

# Pool + health
curl -s http://127.0.0.1:9090/api/proxies | jq

# Close one
curl -X DELETE http://127.0.0.1:9090/api/connections/7f3a1b2c4d5e6f70

# Close all
curl -X DELETE http://127.0.0.1:9090/api/connections
```

---

## 8. Cross-platform guide

### 8.1 Build matrix

```bash
./scripts/build.sh all
```

Produces:

| Binary | Target |
|---|---|
| `proxypool-linux-amd64` | Linux x86-64 (Intel/AMD servers, most desktops) |
| `proxypool-linux-arm64` | Linux arm64 (AWS Graviton, Raspberry Pi 4/5, M-series Macs in Linux VMs) |
| `proxypool-linux-arm` | Linux armv7 (Raspberry Pi 2/3, OpenWrt) |
| `proxypool-darwin-amd64` | macOS Intel |
| `proxypool-darwin-arm64` | macOS Apple Silicon |
| `proxypool-windows-amd64.exe` | Windows x86-64 |
| `proxypool-windows-arm64.exe` | Windows on ARM |

All built with `CGO_ENABLED=0` → no libc dependency.

### 8.2 One-off target

```bash
# FreeBSD
GOOS=freebsd GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags='-s -w' -o bin/proxypool-freebsd-amd64 ./cmd/proxypool

# OpenWrt (mipsel — typical router)
GOOS=linux GOARCH=mipsle CGO_ENABLED=0 go build -trimpath \
  -ldflags='-s -w' -o bin/proxypool-openwrt-mipsel ./cmd/proxypool

# RISC-V
GOOS=linux GOARCH=riscv64 CGO_ENABLED=0 go build -trimpath \
  -ldflags='-s -w' -o bin/proxypool-linux-riscv64 ./cmd/proxypool
```

### 8.3 Per-user config path by OS

| OS | Path |
|---|---|
| Linux / BSD | `$XDG_CONFIG_HOME/proxypool/config.yaml` (default `~/.config/proxypool/`) |
| macOS | `~/Library/Application Support/proxypool/config.yaml` |
| Windows | `%APPDATA%\proxypool\config.yaml` |

Override with `-config /path/to/config.yaml`.

### 8.4 Run as a service

#### Linux (systemd)

```ini
# /etc/systemd/system/proxypool.service
[Unit]
Description=ProxyPool local proxy
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=nobody
ExecStart=/usr/local/bin/proxypool -config /etc/proxypool/config.yaml
Restart=on-failure
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

```bash
sudo install -m 0755 bin/proxypool-linux-amd64 /usr/local/bin/proxypool
sudo install -d -m 0755 /etc/proxypool
sudo cp configs/config.example.yaml /etc/proxypool/config.yaml
sudo systemctl daemon-reload
sudo systemctl enable --now proxypool
sudo systemctl status proxypool
journalctl -u proxypool -f
```

#### macOS (launchd)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>com.user.proxypool</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/proxypool</string>
        <string>-config</string>
        <string>/Users/<your-user>/Library/Application Support/proxypool/config.yaml</string>
    </array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardOutPath</key><string>/tmp/proxypool.out.log</string>
    <key>StandardErrorPath</key><string>/tmp/proxypool.err.log</string>
</dict>
</plist>
```

```bash
# Save as ~/Library/LaunchAgents/com.user.proxypool.plist
launchctl load -w ~/Library/LaunchAgents/com.user.proxypool.plist
launchctl list | grep proxypool
# To stop:
launchctl unload ~/Library/LaunchAgents/com.user.proxypool.plist
```

#### Windows

```powershell
# Install via NSSM (preferred) — https://nssm.cc
nssm install proxypool "C:\Program Files\proxypool\proxypool-windows-amd64.exe" "-config C:\proxypool\config.yaml"
nssm set proxypool AppStdout "C:\proxypool\proxypool.log"
nssm set proxypool AppStderr "C:\proxypool\proxypool.log"
nssm set proxypool AppRotateFiles 1
nssm set proxypool AppRotateBytes 10485760
nssm start proxypool
```

#### Routers / OpenWrt

```bash
# After building for mipsel (see §8.2)
scp bin/proxypool-openwrt-mipsel root@router:/usr/bin/proxypool
ssh root@router 'chmod +x /usr/bin/proxypool && mkdir -p /etc/proxypool'
scp configs/config.example.yaml root@router:/etc/proxypool/config.yaml
ssh root@router '/usr/bin/proxypool -config /etc/proxypool/config.yaml &'
# To survive reboot, add the last line to /etc/rc.local
```

---

## 9. Health checking

By default, health checks are **off** (`pool.health_check: false`). Enable
them to mark upstreams as down when they're unreachable:

```yaml
pool:
  health_check: true
```

When enabled, a background goroutine TCP-dials every non-`direct` upstream
every 30 s. If the dial fails, the upstream's `healthy` flag is set to
`false` and selection will skip it (the strategy will fall back to the next
healthy upstream; if all are down, the last-selected is still used so a
single bad check can't take you offline).

You'll see health in two places:

- `GET /api/proxies` — `healthy: true/false` per upstream
- Dashboard **Proxies** tab — green ● / red ○ indicator

> ⚠️ Health checks use a plain TCP dial, not an application-level probe. For
> a real HTTP upstream this verifies the port is open but not that the proxy
> protocol is functional. For higher fidelity, run a tiny external probe (a
> periodic `curl` to a known URL) and update the config / restart the
> service.

---

## 10. Logging & troubleshooting

### 10.1 Default log output

ProxyPool writes to **stdout** in the standard Go `log` format:

```
2026/06/15 10:23:45 proxypool started · strategy=random · 3 upstreams
2026/06/15 10:23:45 [http]   listening on 127.0.0.1:7890
2026/06/15 10:23:45 [socks5] listening on 127.0.0.1:7891
2026/06/15 10:23:45 [api]    dashboard on http://127.0.0.1:9090
2026/06/15 10:24:12 [http]   pool select: proxy "missing" not found in pool
2026/06/15 10:24:12 [http]   dial upstream us-1: dial tcp 1.2.3.4:8080: i/o timeout
2026/06/15 10:25:01 shutdown signal received, closing connections…
```

### 10.2 Common errors

| Symptom | Likely cause | Fix |
|---|---|---|
| `bind: address already in use` | Another process on 7890/7891/9090 | `lsof -nP -iTCP:7890` to find, then kill or change `http_listen` |
| `connect: connection refused` when curling through the proxy | ProxyPool isn't running | `./scripts/start.sh` again |
| `502 Bad Gateway` for every request | All upstreams are unreachable | Check config, try `health_check: true` and look at the Proxies tab |
| `502 Bad Gateway` with `proxy "x" not found in pool` | You used a non-existent upstream name as the username | Match the `name` in your config exactly |
| Dashboard is blank | Bind address wrong or port blocked | Check `server.api_listen` and `curl -v http://127.0.0.1:9090/api/healthz` |
| `permission denied` on `:80` etc. | Binding to a privileged port | Use `>1024` ports, or `setcap 'cap_net_bind_service=+ep' /path/to/proxypool` |
| `tls: handshake failure` | SOCKS5 user/pass auth issue (old client) | Make sure the client sends the `ver=0x01` byte as RFC 1929 specifies |
| `EOF` mid-tunnel | Upstream crashed or auth rejected | Check upstream logs |

### 10.3 Verbose debugging

For a quick connection log:

```bash
# macOS
sudo tcpdump -i lo0 -A -n port 7890 or port 7891

# Linux
sudo tcpdump -i lo -A -n port 7890 or port 7891
```

To follow what's happening at the API level:

```bash
watch -n1 'curl -s http://127.0.0.1:9090/api/connections | jq'
```

To trace a specific request through the pool:

```bash
# Pin upstream explicitly + watch the row appear in the dashboard
curl -x http://us-1:x@127.0.0.1:7890 https://httpbin.org/ip
```

---

## 11. Security checklist

ProxyPool is a tool — by default it has **no authentication** and binds to
`127.0.0.1`. If you need to expose it on a LAN or WAN, take these steps:

| Surface | Default | What to do if exposing |
|---|---|---|
| **HTTP proxy** `:7890` | Open relay, no auth | Bind to `127.0.0.1` only, or put behind a reverse proxy with auth |
| **SOCKS5 proxy** `:7891` | Username-pin mode (no password) | Same as above |
| **Dashboard / API** `:9090` | No auth, JSON only | Bind to `127.0.0.1`, or add a reverse proxy with Basic Auth / mTLS |
| **Config file** | World-readable | `chmod 600` after editing |
| **Logs** | Stdout | Redact sensitive headers if piping to a centralized log |
| **Auth credentials** in `config.yaml` | Plaintext | Use a secrets manager or env-var substitution (TODO: see [Roadmap](#14-roadmap)) |

Recommended posture for an exposed deployment:

```yaml
server:
  http_listen:   "127.0.0.1:7890"   # local only
  socks5_listen: "127.0.0.1:7891"
  api_listen:    "127.0.0.1:9090"    # front with nginx/caddy if you need remote
```

Forbidding direct internet egress from the binary (a defense-in-depth trick
on Linux):

```bash
sudo iptables -A OUTPUT -m owner --uid-owner proxypool -d 1.2.3.4 -j ACCEPT
sudo iptables -A OUTPUT -m owner --uid-owner proxypool -d 5.6.7.8 -j ACCEPT
sudo iptables -A OUTPUT -m owner --uid-owner proxypool -j REJECT
```

This makes it impossible for ProxyPool to accidentally bypass the upstreams
in its pool.

---

## 12. Project layout

```
proxypool/
├── cmd/
│   └── proxypool/
│       └── main.go                  # entry point — wires everything
├── internal/
│   ├── config/
│   │   └── config.go                # YAML loader, validation, per-OS paths
│   ├── pool/
│   │   └── pool.go                  # upstream pool + selection strategies
│   ├── proxy/
│   │   ├── util.go                  # helpers (id, joinedHost, safeClose)
│   │   ├── dialer.go                # dial through http / socks5 / direct upstream
│   │   ├── http.go                  # local HTTP proxy (CONNECT + forward)
│   │   └── socks5.go                # local SOCKS5 (no-auth + user/pass)
│   ├── conntrack/
│   │   └── conn.go                  # live connection registry
│   ├── api/
│   │   └── api.go                   # REST endpoints
│   └── web/
│       ├── web.go                   # embed.FS handler
│       └── static/
│           ├── index.html           # dashboard markup
│           ├── style.css            # dark theme
│           └── app.js               # polling logic, table rendering
├── configs/
│   ├── config.example.yaml          # annotated starter config
│   └── test.local.yaml              # e2e test config (uses echo servers)
├── scripts/
│   ├── build.sh                     # cross-compile matrix
│   ├── start.sh                     # macOS/Linux launcher
│   ├── start.bat                    # Windows launcher
│   └── test_echo_servers.py         # e2e harness helpers
├── docs/
│   ├── USAGE.md                     # proxy semantics, strategies, auth
│   └── PORTING.md                   # exotic platforms (FreeBSD, OpenWrt, RISC-V)
├── bin/                             # prebuilt cross-platform binaries
├── go.mod
├── go.sum
├── README.md                        # ← you are here
└── LICENSE
```

Total Go code: **~1,600 LOC**, 1 external dep (`gopkg.in/yaml.v3`).

---

## 13. Development

### 13.1 Tweak the dashboard

The UI is plain HTML/CSS/JS in `internal/web/static/`. Edit, then rebuild:

```bash
go build -o bin/proxypool-darwin-arm64 ./cmd/proxypool
```

The `//go:embed static` directive picks up your changes at compile time.

### 13.2 Run from source (no build step)

```bash
go run ./cmd/proxypool -config configs/config.example.yaml
```

### 13.3 Tests

There is an end-to-end harness using two tiny Python echo servers (one
SOCKS5, one HTTP-CONNECT) and `config.test.local.yaml`. The harness is what
was used to validate the 11/11 tests in the project history.

To re-run manually:

```bash
# Terminal 1: echo servers
python3 scripts/test_echo_servers.py

# Terminal 2: proxypool against them
./bin/proxypool-darwin-arm64 -config configs/test.local.yaml

# Terminal 3: poke at it
curl -x http://127.0.0.1:7890 -x ...  # see e2e walkthrough
```

### 13.4 Code style

- Standard `gofmt` / `go vet` — no exceptions.
- No external deps beyond `yaml.v3`. Add a new dep only with a strong reason.
- No cgo. We must stay 100% static.
- Comment every exported identifier.

---

## 14. Roadmap

| Feature | Status | Notes |
|---|---|---|
| UDP / SOCKS5 UDP ASSOCIATE | planned | Needed for QUIC, WebRTC, DNS-over-UDP |
| Config hot-reload on SIGHUP | planned | Watch file, re-parse, swap pool atomically |
| Rule-based routing (domain → upstream) | planned | Similar to Clash `Rule` — `DOMAIN-SUFFIX,google.com,us-1` |
| Per-upstream traffic totals (persistent) | planned | SQLite in `~/.local/share/proxypool/` |
| TUN device mode (Linux/macOS) | research | Whole-system VPN-style capture |
| DNS resolution strategy | planned | `resolve-via-upstream` vs `resolve-locally` |
| Configurable dashboard auth | planned | Basic Auth token / mTLS |
| Env-var substitution in config | planned | `${PROXY_USER}` → from env |
| TPROXY / transparent proxy on Linux | planned | Requires iptables + kernel ≥ 4.19 |
| Windows service install script (`install.ps1`) | planned | Use NSSM under the hood |

---

## 15. FAQ

**Q: Why not just use Clash / sing-box / mihomo?**

A: They're great — and a superset of what this does. ProxyPool is for the
case where you (a) want to understand every line of the code that's
forwarding your bytes, (b) want a tiny embeddable component (6 MB, ~1.6k
LOC, 1 dep) rather than a 50 MB binary with TUN, DNS, rules, etc., or
(c) need a per-request "force this upstream" primitive that's awkward to
express in Clash's rule system.

**Q: Can I use this to scrape / bypass geo-restrictions?**

A: It's a generic pool proxy. Use it for whatever your upstream licenses
permit. Be a good citizen: respect `robots.txt`, the upstream's ToS, and
the target site's ToS.

**Q: Why doesn't my proxy-auth password get checked?**

A: Because the password is irrelevant to ProxyPool — it only uses the
username to select an upstream. The actual credentials to the upstream are
in the config (`username`/`password` per upstream entry). This design lets
you embed upstream selection in apps that only support a basic username:
password form.

**Q: Does it support HTTPS interception / MITM?**

A: No, by design. ProxyPool is a forward proxy, not a transparent MITM.
Browsers and apps trust the upstream CAs; we don't add ours.

**Q: Why no `CONNECT` keep-alive for HTTP/2?**

A: The upstream picks. ProxyPool just bridges bytes — if the upstream and
client negotiate h2, the tunnel carries h2 frames. No special handling
needed.

**Q: My upstream is HTTPS itself (e.g. `https://proxy.example.com:443`). Can I connect to it?**

A: Not yet. Out of scope for v0.1; tracked in the Roadmap. PRs welcome.

**Q: How do I rotate to a different pool config without dropping connections?**

A: Today: SIGTERM, edit config, restart. The Roadmap has SIGHUP-based hot
reload.

**Q: Can I embed ProxyPool in my Go app as a library?**

A: Yes — all the interesting bits are in `internal/`, exported via the
package APIs. Just be aware that "internal" in Go means external importers
can't reach in, so you'd either fork or rename the dirs.

---

## License

MIT — see `LICENSE`.

## Acknowledgments

- Clash / Mihomo for the inspiration and the model of how a pool-proxy UI should look.
- The Go stdlib — `net`, `net/http`, `crypto/rand`, `embed` — for making this possible in 1.6k LOC.
- Everyone who's ever hand-rolled a SOCKS5 handshake.
