# Usage guide

## 1. Selection strategies

| Strategy      | Behaviour                                                     |
|---------------|---------------------------------------------------------------|
| `random`      | Uniform random among healthy upstreams (default)              |
| `round-robin` | Cyclical rotation                                             |
| `hash`        | FNV-hash of destination host → sticky per-host (good for sessions) |
| `name`        | Always the first healthy upstream                             |

If the pool's only entry is `direct`, every connection is just a regular dial.

## 2. Picking a specific upstream (per-request)

The proxy-auth **username** is interpreted as the upstream name. The password is
ignored (the upstream itself handles its own credentials).

```bash
# Force "us-http-1"
curl -x http://us-http-1:any@127.0.0.1:7890 https://example.com

# Force "direct"
curl -x http://direct:any@127.0.0.1:7890 https://example.com

# SOCKS5 variant
curl --proxy-user 'jp-socks-1:any' --proxy socks5h://127.0.0.1:7891 https://example.com
```

If the username doesn't match any upstream, the request is rejected with `502`.

## 3. Health checking

Set `pool.health_check: true` to enable a background goroutine that TCP-dials
every non-direct upstream every 30 s. Unhealthy upstreams are skipped by
strategy selection; they are still listed in the dashboard with status `down`.

## 4. Live connections

Every active tunnel is recorded with:

- inbound protocol (`HTTP` or `SOCKS5`)
- source (`client-ip:port`)
- destination host (`api.ipify.org`)
- target (`resolved ip:port`)
- chosen upstream name
- upload / download byte counters
- start time / age
- a `Close` button in the UI

The table refreshes every second via `setInterval`. Use `Close All` to drop
every active connection (e.g. when switching networks).

## 5. REST API

| Method | Path                       | Description                       |
|--------|----------------------------|-----------------------------------|
| GET    | `/api/healthz`             | `{"status":"ok"}`                 |
| GET    | `/api/connections`         | JSON snapshot of active conns     |
| DELETE | `/api/connections`         | Close all                         |
| DELETE | `/api/connections/:id`     | Close one                         |
| GET    | `/api/proxies`             | Upstream list + health            |
| GET    | `/api/stats`               | Aggregate counters                |

Sample `GET /api/connections`:

```json
[
  {
    "id": "7f3a1b2c4d5e6f70",
    "start_time": "2026-06-15T10:23:45Z",
    "inbound": "HTTP",
    "network": "TCP",
    "source": "127.0.0.1:51234",
    "host": "api.ipify.org",
    "target": "104.21.54.12:443",
    "proxy": "us-http-1",
    "upload": 412,
    "download": 318,
    "closed": false
  }
]
```

## 6. As a system proxy (macOS / Windows GUI)

Point your OS or browser proxy settings at `127.0.0.1:7890` (HTTP) and
`127.0.0.1:7891` (SOCKS5). Browsers that support per-request upstream
selection: Firefox with FoxyProxy, SwitchyOmega, etc.

## 7. Logging

Logs go to stdout. Adjust verbosity by replacing `log.Printf` calls with a
real logger if needed (e.g. zap / zerolog).