#!/usr/bin/env bash
# Builds the server's image (the web UI embedded in a static binary) and its Caddy's.
#   scripts/build-server.sh            # linux/amd64 (mini-PC)
#   GOARCH=arm64 scripts/build-server.sh
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
arch="${GOARCH:-amd64}"
version="$(git -C "$root" describe --always --dirty 2>/dev/null || echo dev)"

GOOS=linux GOARCH="$arch" "$root/scripts/build.sh"
cp "$root/bin/hub-server" "$root/deploy/server/hub-server"
docker build --platform "linux/$arch" -t "hub-server:$version" -t hub-server:local "$root/deploy/server"
docker build --platform "linux/$arch" -t hub-caddy:local "$root/deploy/server/caddy"
echo "built hub-server:$version (linux/$arch)"
