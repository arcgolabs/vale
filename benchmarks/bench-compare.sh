#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.yml"
LOCAL_COMPOSE_FILE="$SCRIPT_DIR/docker-compose.local.yml"
DURATION="${DURATION:-15s}"
WARMUP="${WARMUP:-3s}"
CONCURRENCY="${CONCURRENCY:-32}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-60}"
LOG_LEVEL="${LOG_LEVEL:-info}"
STAMP=$(date +%Y%m%d-%H%M%S)
OUTPUT_DIR="${OUTPUT_DIR:-$SCRIPT_DIR/results/$STAMP}"
LOCAL_BUILD="${LOCAL_BUILD:-0}"
SKIP_PULL="${SKIP_PULL:-0}"
KEEP="${KEEP:-0}"

if [ "$LOCAL_BUILD" = "1" ] && [ -z "${VALE_IMAGE+x}" ]; then
  VALE_IMAGE="vale-bench-vale:latest"
else
  VALE_IMAGE="${VALE_IMAGE:-ghcr.io/arcgolabs/vale:latest}"
fi
export VALE_IMAGE

mkdir -p "$OUTPUT_DIR"

bench_log() {
  echo "bench: $*" >&2
}

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
    bench_log "stopping compose stack"
    docker compose "$@" down -v
  fi
}

cd "$ROOT_DIR"
set -- -f "$COMPOSE_FILE"
if [ "$LOCAL_BUILD" = "1" ]; then
  set -- "$@" -f "$LOCAL_COMPOSE_FILE"
fi
trap 'cleanup "$@"' EXIT

bench_log "starting compose stack"
if [ "$LOCAL_BUILD" = "1" ]; then
  docker compose "$@" up -d --build
else
  if [ "$SKIP_PULL" != "1" ]; then
    bench_log "pulling vale image $VALE_IMAGE"
    docker compose "$@" pull vale
  fi
  docker compose "$@" up -d
fi

bench_log "waiting for vale"
wait_endpoint vale http://127.0.0.1:18080/
bench_log "waiting for traefik"
wait_endpoint traefik http://127.0.0.1:18081/
bench_log "waiting for caddy"
wait_endpoint caddy http://127.0.0.1:18082/

bench_log "recording image metadata in $OUTPUT_DIR"
docker compose "$@" images > "$OUTPUT_DIR/images.txt"

bench_log "running proxybench"
go run ./benchmarks/cmd/proxybench \
  -duration "$DURATION" \
  -warmup "$WARMUP" \
  -concurrency "$CONCURRENCY" \
  -log-level "$LOG_LEVEL" \
  -target "vale=http://127.0.0.1:18080,traefik=http://127.0.0.1:18081,caddy=http://127.0.0.1:18082" \
  -json "$OUTPUT_DIR/proxybench.json" \
  -markdown "$OUTPUT_DIR/proxybench.md"
