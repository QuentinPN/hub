#!/usr/bin/env bash
# Checks the running Docker proxy of deploy/agent/compose.yml: the four reads the agent
# needs pass, everything else is refused (inspect, logs, archive, export, info, any write).
# Run on the host where the proxy runs, after `docker compose up -d`.
set -uo pipefail
export MSYS_NO_PATHCONV=1
project="${COMPOSE_PROJECT:-hub-agent}"
volume="${project}_proxy-socket"

# A container to point inspect, logs and archive at: the newest one of the proxy itself.
target="$(docker ps -q --filter "label=com.docker.compose.project=$project" --filter label=com.docker.compose.service=docker-proxy | head -1)"
[ -n "$target" ] || { echo "proxy not running ($project)"; exit 1; }

# Same uid as the agent, same volume: the probe sees exactly what the agent sees.
probe() {
	docker run --rm --user 65534:65534 --network none -v "$volume:/run/hub-proxy" curlimages/curl:8.17.0 \
		-s -o /dev/null -w '%{http_code}' --max-time 5 --unix-socket /run/hub-proxy/docker.sock -X "$1" "http://docker$2"
}

fail=0
check() {
	local want="$1" method="$2" path="$3" got
	got="$(probe "$method" "$path")"
	if [ "$got" = "$want" ]; then
		printf 'ok    %s %-45s %s\n' "$method" "$path" "$got"
	else
		printf 'FAIL  %s %-45s %s (expected %s)\n' "$method" "$path" "$got" "$want"
		fail=1
	fi
}

check 200 GET /_ping
check 200 GET /version
check 200 GET /v1.47/version
check 200 GET '/containers/json?all=1'
check 403 GET "/containers/$target/json"
check 403 GET "/containers/$target/logs?stdout=1"
check 403 GET "/containers/$target/archive?path=/"
check 403 GET "/containers/$target/export"
check 403 GET "/containers/$target/top"
check 403 GET /info
check 403 GET /images/json
check 403 GET /volumes
check 403 GET /secrets
check 403 GET /version/../containers/x/json
check 405 POST /containers/create
check 405 POST "/containers/$target/stop"
check 405 POST "/containers/$target/exec"
check 405 DELETE "/containers/$target"
check 405 PUT "/containers/$target/archive?path=/"

exit "$fail"
