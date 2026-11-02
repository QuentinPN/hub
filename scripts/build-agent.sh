#!/usr/bin/env bash
# Builds the agent's image: a static binary in a scratch image, nothing else.
#   scripts/build-agent.sh            # linux/amd64 (mini-PC, QNAP)
#   GOARCH=arm64 scripts/build-agent.sh   # Raspberry Pi
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
arch="${GOARCH:-amd64}"
version="$(git -C "$root" describe --always --dirty 2>/dev/null || echo dev)"

(cd "$root/server" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
	-ldflags="-s -w -X main.version=$version" -o "$root/deploy/agent/hub-agent" ./cmd/hub-agent)
docker build --platform "linux/$arch" -t "hub-agent:$version" -t hub-agent:local "$root/deploy/agent"
echo "built hub-agent:$version (linux/$arch)"
