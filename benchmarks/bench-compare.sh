#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.yml"
DURATION="${DURATION:-15s}"
WARMUP="${WARMUP:-3s}"
CONCURRENCY="${CONCURRENCY:-32}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-60}"
STAMP=$(date +%Y%m%d-%H%M%S)
OUTPUT_DIR="${OUTPUT_DIR:-$SCRIPT_DIR/results/$STAMP}"
KEEP="${KEEP:-0}"

mkdir -p "$OUTPUT_DIR"

wait_endpoint() {
  name="$1"
  url="$2"
  end=$(( $(date +%s) + TIMEOUT_SECONDS ))
  while [ "$(date +%s)" -lt "$end" ]; do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "$name did not become ready at $url" >&2
  return 1
}

cleanup() {
  if [ "$KEEP" != "1" ]; then
    docker compose -f "$COMPOSE_FILE" down -v
  fi
}
trap cleanup EXIT

cd "$ROOT_DIR"
docker compose -f "$COMPOSE_FILE" up -d --build

wait_endpoint vale http://127.0.0.1:18080/
wait_endpoint traefik http://127.0.0.1:18081/
wait_endpoint caddy http://127.0.0.1:18082/

docker compose -f "$COMPOSE_FILE" images > "$OUTPUT_DIR/images.txt"

go run ./benchmarks/cmd/proxybench \
  -duration "$DURATION" \
  -warmup "$WARMUP" \
  -concurrency "$CONCURRENCY" \
  -target "vale=http://127.0.0.1:18080,traefik=http://127.0.0.1:18081,caddy=http://127.0.0.1:18082" \
  -json "$OUTPUT_DIR/proxybench.json" \
  -markdown "$OUTPUT_DIR/proxybench.md"
