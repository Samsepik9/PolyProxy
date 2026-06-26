# PolyProxy

> A local HTTP + SOCKS5 proxy with **upstream proxy pool**, **free proxy crawling**, **dynamic rotation**, **auto-pool management**, and a **web dashboard** — written in Go, single static binary, runs on Linux / macOS / Windows.

Inspired by Clash / Clash Verge's *Connections* panel. PolyProxy gives you a single local port that fans out across N upstream proxies, with a live view of every byte that flows through it. Plus: automatically crawl, validate, and rotate free proxies from public sources.

[**中文文档**](README.md)

---

```
┌────────────────────────┐      ┌─────────────────────────────┐
│  Your apps / curl /    │      │     PolyProxy               │
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

## Features

| Capability | Detail |
|---|---|
| **Local proxy ports** | HTTP (`7890`, both `CONNECT` and plain `GET` forward) + SOCKS5 (`7891`) |
| **Proxy pool** | Mix `direct`, `http`, `socks5` upstreams freely |
| **Selection strategies** | `random` (default), `round-robin`, `hash` (sticky per host), `name` (first) |
| **Per-request pinning** | Proxy-auth username = upstream name → force that upstream for this request |
| **Connection failover** | Unhealthy upstreams auto-skipped; falls back to next healthy proxy |
| **Live connections** | Every active tunnel: id, host, target, proxy, source, upload/download bytes, start time, age |
| **Web dashboard** | Auto-refreshes every 1s, filter by host/target/proxy, close one or all, dark theme, i18n (中/EN) |
| **Free proxy crawling** | Built-in sources — one-click fetch from public proxy lists |
| **Async validation** | Concurrent TCP + HTTP validation with progress tracking |
| **Dynamic proxy rotation** | Auto crawl → validate on a configurable interval (min 10s) |
| **Auto-pool** | Automatically add validated proxies to the pool on each cycle |
| **REST API** | Full API for connections, proxies, pool management, dynamic control |
| **Cross-platform** | Linux (amd64 / arm64 / armv7), macOS (amd64 / arm64), Windows (amd64 / arm64) — single static binary, zero runtime deps |
| **Embeddable** | UI assets compiled into the binary via `go:embed` — no filesystem dependencies |
| **Configurable** | YAML config at a per-user path; CLI flag to override |
| **Health checks** | Optional background TCP-dial health checks that mark upstreams down |
| **Low footprint** | ~6 MB binary, ~12 MB RSS for 100 active connections |

---

## Quick Start (60 seconds)

### Download pre-built binary

Go to [Releases](https://github.com/Samsepik9/PolyProxy/releases) and download the binary for your platform:

| Platform | Binary |
|---|---|
| macOS (Apple Silicon) | `polyproxy-darwin-arm64` |
| macOS (Intel) | `polyproxy-darwin-amd64` |
| Linux (x86_64) | `polyproxy-linux-amd64` |
| Linux (ARM64) | `polyproxy-linux-arm64` |
| Linux (ARMv7) | `polyproxy-linux-arm` |
| Windows (x86_64) | `polyproxy-windows-amd64.exe` |
| Windows (ARM64) | `polyproxy-windows-arm64.exe` |

### Or build from source

```bash
git clone https://github.com/Samsepik9/PolyProxy.git
cd PolyProxy
./scripts/build.sh
```

### Run

```bash
# Start with example config
./bin/polyproxy-darwin-arm64 -config configs/config.example.yaml

# Open dashboard
open http://127.0.0.1:9090
```

Then in another terminal:

```bash
# Use as HTTP proxy
curl -x http://127.0.0.1:7890 https://api.ipify.org

# Use as SOCKS5 proxy
curl --proxy socks5h://127.0.0.1:7891 https://api.ipify.org
```

---

## Configuration

The config file is plain YAML. Full example: [`configs/config.example.yaml`](configs/config.example.yaml).

```yaml
server:
  http_listen:   "127.0.0.1:7890"
  socks5_listen: "127.0.0.1:7891"
  api_listen:    "127.0.0.1:9090"
  api_enable:    true

pool:
  strategy: random
  health_check: true
  proxies:
    - { name: direct, type: direct }
    - name: us-1
      type: http
      server: 1.2.3.4
      port: 8080

# Free proxy crawling (optional)
freeproxy:
  enabled: true
  test_urls:
    - "http://myip.ipip.net"
    - "http://www.baidu.com"
  crawl_timeout: 30
  timeout: 8
  concurrency: 50
```

### Config paths

| OS | Path |
|---|---|
| Linux | `~/.config/PolyProxy/config.yaml` |
| macOS | `~/Library/Application Support/PolyProxy/config.yaml` |
| Windows | `%APPDATA%\PolyProxy\config.yaml` |

---

## Web Dashboard

Open **`http://127.0.0.1:9090`** in your browser.

### Tabs

- **连接 (Connections)** — live tunnels, filterable, closable one-by-one or all-at-once
- **代理采集 (Proxy Crawl)** — fetch free proxies from public sources, validate them, add to pool
- **代理池 (Proxy Pool)** — view/manage upstreams, switch strategies, save config
- **运行日志 (Logs)** — real-time operation logs

### Proxy Crawl tab features

| Button | What it does |
|---|---|
| 🔍 获取代理 | Fetch proxies from all built-in sources |
| ✅ 验证代理 | Validate fetched proxies (concurrent, with progress) |
| 🔄 动态代理 | Start/stop periodic crawl→validate cycle (configurable interval) |
| 🤖 自动入池 | Toggle auto-add: when ON, each dynamic cycle auto-adds valid proxies to pool |
| 📥 移入代理池 | Manually add selected proxies to the pool |

### Priority display

Proxies from **稻壳代理 (docip)** and **谷德代理 (goodips)** are displayed first. Results are paginated at 50 per page with smart page navigation.

---

## Selection Strategies

| Strategy | Behavior | Best for |
|---|---|---|
| `random` | Uniform random among healthy upstreams | Spreading load, evading per-IP rate limits |
| `round-robin` | Cycle through upstreams in order | Even distribution |
| `hash` | `FNV32a(host) % len(pool)` — sticky per destination host | Keeping sessions on same egress IP |
| `name` | Always the first healthy upstream | Debugging, "always use my private proxy" |

---

## Per-Request Pinning

Encode the upstream name as the proxy-auth username:

```bash
# Force "us-1"
curl -x http://us-1:any@127.0.0.1:7890 https://api.ipify.org

# Force "direct" (bypass all upstreams)
curl -x http://direct:any@127.0.0.1:7890 https://api.ipify.org

# SOCKS5 variant
curl --proxy-user 'jp-1:any' --proxy socks5h://127.0.0.1:7891 https://api.ipify.org
```

---

## REST API

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/healthz` | Liveness probe |
| `GET` | `/api/connections` | Active connections snapshot |
| `DELETE` | `/api/connections` | Close all connections |
| `DELETE` | `/api/connections/:id` | Close one connection |
| `GET` | `/api/proxies` | Upstream list with health |
| `DELETE` | `/api/proxies/:name` | Remove proxy from pool |
| `GET` | `/api/stats` | Aggregate counters |
| `POST` | `/api/proxies/fetch` | Crawl free proxies |
| `POST` | `/api/proxies/validate` | Validate cached proxies |
| `GET` | `/api/proxies/validate/:id` | Validation progress |
| `POST` | `/api/proxies/dynamic` | Start/stop dynamic cycle |
| `GET` | `/api/proxies/dynamic` | Dynamic cycle status |
| `POST` | `/api/proxies/auto-pool` | Toggle auto-pool |
| `POST` | `/api/pool/add` | Add proxies to pool |
| `PUT` | `/api/pool/strategy` | Change selection strategy |
| `POST` | `/api/pool/save` | Save pool to config file |
| `GET` | `/api/logs` | Operation logs |

---

## Project Layout

```
.
├── cmd/proxypool/          # Entry point
├── internal/
│   ├── api/                # REST API + dynamic runner
│   ├── config/             # YAML config loading
│   ├── conntrack/          # Connection tracking
│   ├── freeproxy/          # Free proxy crawling & validation
│   ├── pool/               # Upstream pool + health check
│   ├── proxy/              # HTTP/SOCKS5 proxy servers + failover
│   └── web/                # Embedded web dashboard
├── configs/                # Example configs
├── scripts/                # Build & launch scripts
└── .github/workflows/      # CI (3 OS) + Release (7 platforms)
```

---

## Development

```bash
# Prerequisites: Go 1.24+

# Build
go build -o bin/polyproxy ./cmd/proxypool

# Run tests
go test ./... -v

# Cross-compile all platforms
./scripts/build.sh all
```

---

## License

MIT — see [LICENSE](LICENSE).

---

[中文文档](README.md)
