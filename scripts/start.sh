#!/usr/bin/env bash
# start.sh — launch proxypool on Linux / macOS.
# Usage: ./scripts/start.sh [--config PATH]

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Pick a binary: pre-built in ./bin matching host, otherwise `go run`.
HOST_OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
HOST_ARCH="$(uname -m)"
case "$HOST_ARCH" in
  x86_64)  HOST_ARCH=amd64 ;;
  aarch64) HOST_ARCH=arm64 ;;
  armv7l)  HOST_ARCH=arm   ;;
esac
BIN="$ROOT/bin/proxypool-${HOST_OS}-${HOST_ARCH}"

# Parse args (forward everything to proxypool)
ARGS=()
while [ $# -gt 0 ]; do
  case "$1" in
    --help|-h)
      echo "Usage: $0 [--config PATH] [extra proxypool args...]"
      echo ""
      echo "Defaults config to the per-user path; creates it from"
      echo "  $ROOT/configs/config.example.yaml if missing."
      exit 0
      ;;
    *) ARGS+=("$1"); shift ;;
  esac
done

# Default config bootstrap (only when -config not provided and file missing)
NEEDS_CONFIG=1
for a in "${ARGS[@]:-}"; do [ "$a" = "-config" ] && NEEDS_CONFIG=0; done
if [ "$NEEDS_CONFIG" = "1" ]; then
  CFG_PATH="$HOME/.config/proxypool/config.yaml"
  [ "$(uname -s)" = "Darwin" ] && CFG_PATH="$HOME/Library/Application Support/proxypool/config.yaml"
  if [ ! -f "$CFG_PATH" ]; then
    mkdir -p "$(dirname "$CFG_PATH")"
    cp "$ROOT/configs/config.example.yaml" "$CFG_PATH"
    echo "▸ bootstrapped config at $CFG_PATH — edit and re-run."
    exit 0
  fi
  ARGS=("-config" "$CFG_PATH" "${ARGS[@]}")
fi

if [ -x "$BIN" ]; then
  echo "▸ launching $BIN ${ARGS[*]}"
  exec "$BIN" "${ARGS[@]}"
elif command -v go >/dev/null 2>&1; then
  echo "▸ no prebuilt binary, falling back to: go run ./cmd/proxypool ${ARGS[*]}"
  exec go run "$ROOT/cmd/proxypool" "${ARGS[@]}"
else
  echo "✗ no binary at $BIN and go not installed" >&2
  exit 1
fi