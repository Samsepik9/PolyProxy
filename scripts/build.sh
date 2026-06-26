#!/usr/bin/env bash
# build.sh — cross-compile PolyProxy for common OS / arch pairs.
# Usage:  ./scripts/build.sh            # builds for host
#         ./scripts/build.sh all        # builds for host + linux/amd64, linux/arm64, windows/amd64, darwin/arm64

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUT="$ROOT/bin"
mkdir -p "$OUT"

VERSION="${VERSION:-0.1.0}"
LDFLAGS="-s -w -X main.version=$VERSION"

build_one() {
  local goos="$1" goarch="$2"
  local ext=""
  [ "$goos" = "windows" ] && ext=".exe"
  local out="$OUT/polyproxy-${goos}-${goarch}${ext}"
  echo "→ building $goos/$goarch"
  ( cd "$ROOT" && GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
      go build -trimpath -ldflags "$LDFLAGS" -o "$out" ./cmd/proxypool )
  echo "   $out ($(du -h "$out" | cut -f1))"
}

case "${1:-host}" in
  host|"")
    build_one "$(go env GOOS)" "$(go env GOARCH)"
    ;;
  all)
    build_one linux   amd64
    build_one linux   arm64
    build_one linux   arm   2>/dev/null || true
    build_one darwin  amd64
    build_one darwin  arm64
    build_one windows amd64
    build_one windows arm64 2>/dev/null || true
    ;;
  *)
    os="$1"; arch="$2"
    [ -z "$arch" ] && { echo "usage: $0 [host|all|<os> <arch>]"; exit 2; }
    build_one "$os" "$arch"
    ;;
esac

echo "done. binaries in $OUT/"