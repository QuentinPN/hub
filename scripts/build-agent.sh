#!/usr/bin/env bash
# Builds the agent's image: a static binary in a scratch image, nothing else. Same Dockerfile as
# the CI (.gitea/workflows/release.yml, job images).
#   scripts/build-agent.sh            # linux/amd64 (mini-PC, QNAP)
#   GOARCH=arm64 scripts/build-agent.sh   # Raspberry Pi
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
arch="${GOARCH:-amd64}"
version="$(git -C "$root" describe --always --dirty 2>/dev/null || echo dev)"

docker build --platform "linux/$arch" --build-arg "VERSION=$version" \
	-f "$root/deploy/agent/Dockerfile" -t "hub-agent:$version" -t hub-agent:local "$root"
echo "built hub-agent:$version (linux/$arch)"
