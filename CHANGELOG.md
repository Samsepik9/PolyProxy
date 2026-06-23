# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
