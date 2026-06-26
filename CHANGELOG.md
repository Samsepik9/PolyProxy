# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-06-26

### Changed
- Project renamed to **PolyProxy** — binary, module path, and config paths all updated
- README rewritten with bilingual (zh/en) documentation covering all features

### Added
- **Free proxy crawling** from built-in public sources (稻壳代理, 谷德代理, etc.)
- **Async validation** with progress tracking (concurrent TCP + HTTP probes)
- **Dynamic proxy rotation** — periodic crawl→validate loop (configurable interval, min 10s)
- **Auto-pool** — automatically add validated proxies to the pool each cycle
- **Connection failover** — unhealthy upstreams auto-skipped, falls back to next healthy proxy
- **i18n (zh/en)** — web dashboard language toggle, `applyI18n()` on page load
- **Proxy Crawl tab** in web dashboard — fetch, validate, dynamic, auto-pool controls
- **50-per-page pagination** with smart page navigation on crawl results
- **Priority display** — 稻壳代理 (docip) and 谷德代理 (goodips) sources shown first
- REST API endpoints: `/api/proxies/fetch`, `/api/proxies/validate`, `/api/proxies/dynamic`, `/api/proxies/auto-pool`, `/api/pool/add`, `/api/logs`
- GitHub Actions CI (3 OS: ubuntu, macOS, windows) + Release (7 platforms)

### Fixed
- Thread-safe validation progress reading (`ValidateTask.Snapshot()` method with proper locking)
- Race condition in `handleValidateProgress` that caused partial result display

## [0.1.0] - 2026-06-15

### Added
- HTTP proxy server (CONNECT + plain forward) on configurable port
- SOCKS5 proxy server (no-auth + user/pass) on configurable port
- Proxy pool with 4 selection strategies: `random`, `round-robin`, `hash`, `name`
- Per-request upstream pinning via proxy-auth username
- Web dashboard with live connection monitoring (auto-refresh every 1s)
- REST API: `/api/connections`, `/api/proxies`, `/api/stats`, `/api/healthz`
- Background TCP health checking for upstreams
- Cross-platform builds: Linux (amd64/arm64/arm), macOS (amd64/arm64), Windows (amd64/arm64)
- YAML configuration with per-user OS-specific paths
- Single static binary with embedded web assets (`go:embed`)
- Startup scripts for Linux/macOS (`start.sh`) and Windows (`start.bat`)
