#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.cluster.yml"
LOCAL_COMPOSE_FILE="$SCRIPT_DIR/docker-compose.cluster.local.yml"
DURATION="${DURATION:-15s}"
WARMUP="${WARMUP:-3s}"
CONCURRENCY="${CONCURRENCY:-32}"
REPEAT="${REPEAT:-1}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-90}"
LOG_LEVEL="${LOG_LEVEL:-info}"
STAMP=$(date +%Y%m%d-%H%M%S)
OUTPUT_DIR="${OUTPUT_DIR:-$SCRIPT_DIR/results/cluster-$STAMP}"
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

if [ "$REPEAT" -le 0 ]; then
  echo "REPEAT must be positive" >&2
  exit 1
fi

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

wait_vale_cluster_leader() {
  url="$1"
  end=$(( $(date +%s) + TIMEOUT_SECONDS ))
  while [ "$(date +%s)" -lt "$end" ]; do
    if curl -fsS --max-time 2 "$url" 2>/dev/null | grep -q '"leader_ready"[[:space:]]*:[[:space:]]*true'; then
      return 0
    fi
    sleep 0.5
  done
  echo "vale cluster did not elect a ready leader at $url" >&2
  return 1
}

cleanup() {
  if [ "$KEEP" != "1" ]; then
    bench_log "stopping cluster compose stack"
    docker compose "$@" down -v
  fi
}

cd "$ROOT_DIR"
set -- -f "$COMPOSE_FILE"
if [ "$LOCAL_BUILD" = "1" ]; then
  set -- "$@" -f "$LOCAL_COMPOSE_FILE"
fi
trap 'cleanup "$@"' EXIT

bench_log "starting cluster compose stack"
if [ "$LOCAL_BUILD" = "1" ]; then
  docker compose "$@" up -d --build
else
  if [ "$SKIP_PULL" != "1" ]; then
    bench_log "pulling vale image $VALE_IMAGE"
    docker compose "$@" pull vale-1 vale-2 vale-3
  fi
  docker compose "$@" up -d
fi

bench_log "waiting for vale cluster node"
wait_endpoint vale-cluster http://127.0.0.1:18080/
bench_log "waiting for traefik single node"
wait_endpoint traefik-single http://127.0.0.1:18081/
bench_log "waiting for vale raft leader"
wait_vale_cluster_leader http://127.0.0.1:28090/admin/cluster/status

bench_log "recording metadata in $OUTPUT_DIR"
docker compose "$@" images > "$OUTPUT_DIR/images.txt"
docker compose "$@" ps > "$OUTPUT_DIR/containers.txt"
curl -fsS http://127.0.0.1:28090/admin/cluster/status > "$OUTPUT_DIR/vale-cluster-status.json"

run=1
while [ "$run" -le "$REPEAT" ]; do
  run_name=$(printf 'run-%02d' "$run")
  json_path="$OUTPUT_DIR/$run_name-proxybench.json"
  markdown_path="$OUTPUT_DIR/$run_name-proxybench.md"
  if [ "$REPEAT" = "1" ]; then
    json_path="$OUTPUT_DIR/proxybench.json"
    markdown_path="$OUTPUT_DIR/proxybench.md"
  fi
  bench_log "running proxybench $run_name of $REPEAT"
  go run ./benchmarks/cmd/proxybench \
    -duration "$DURATION" \
    -warmup "$WARMUP" \
    -concurrency "$CONCURRENCY" \
    -log-level "$LOG_LEVEL" \
    -target "vale-cluster=http://127.0.0.1:18080,traefik-single=http://127.0.0.1:18081" \
    -json "$json_path" \
    -markdown "$markdown_path"
  run=$((run + 1))
done
