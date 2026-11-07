#!/usr/bin/env bash
# Builds the web UI, embeds it in the Go server and produces bin/hub-server.
# GOOS/GOARCH are honoured, e.g. GOOS=linux GOARCH=arm64 scripts/build.sh for the Pi.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"

(cd "$root/web" && npm ci && npm run build)

dist="$root/server/internal/webui/dist"
find "$dist" -mindepth 1 ! -name .gitkeep -exec rm -rf {} +
cp -r "$root/web/build/." "$dist/"

ext=""
[ "$(go env GOOS)" = "windows" ] && ext=".exe"
version="$(git -C "$root" describe --always --dirty 2>/dev/null || echo dev)"
(cd "$root/server" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$version" -o "$root/bin/hub-server$ext" ./cmd/hub-server)
echo "built $root/bin/hub-server$ext"
