#!/usr/bin/env bash
# Cross-compile rulemux binaries into dist/ for npm publishing.
#
# Usage: scripts/build-dist.sh
#   Produces dist/rulemux-<os>-<arch>[.exe] for the platforms we ship.
#   rulemux is pure Go (stdlib only), so cross-compiling needs no cgo and no network.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# Version may be passed in (the release pipeline passes the git tag); otherwise
# it is read from main.go so local builds stay consistent.
VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  VERSION="$(grep -m1 -E '^(const|var) version' main.go | sed -E 's/.*"([^"]+)".*/\1/')"
fi
OUT="dist"
mkdir -p "$OUT"

echo "building rulemux $VERSION into $OUT/"

build() {
  local goos="$1" goarch="$2" name="rulemux-$1-$2"
  if [ "$goos" = "windows" ]; then
    name="$name.exe"
  fi
  echo "  -> $name"
  # -ldflags="-s -w" strips the symbol table and DWARF debug info: ~33% smaller
  # (4.2MB -> 2.8MB per binary). Only cost is that panic stack traces lose
  # file/line detail, which is acceptable for a CLI.
  # -X main.version= stamps the version so `rulemux --version` reports the release tag.
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$OUT/$name" .
}

build linux   amd64
build linux   arm64
build darwin  amd64
build darwin  arm64
build windows amd64

echo
echo "built:"
ls -lh "$OUT" | tail -n +2
