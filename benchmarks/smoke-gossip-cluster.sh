#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.gossip.yml"
LOCAL_COMPOSE_FILE="$SCRIPT_DIR/docker-compose.gossip.local.yml"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-120}"
TARGETARCH="${TARGETARCH:-amd64}"
LOCAL_BUILD="${LOCAL_BUILD:-0}"
SKIP_PULL="${SKIP_PULL:-0}"
KEEP="${KEEP:-0}"
STAMP=$(date +%Y%m%d-%H%M%S)
OUTPUT_DIR="${OUTPUT_DIR:-$SCRIPT_DIR/results/gossip-smoke-$STAMP}"

if [ "$LOCAL_BUILD" = "1" ] && [ -z "${VALE_IMAGE+x}" ]; then
  VALE_IMAGE="vale-gossip-smoke:latest"
else
  VALE_IMAGE="${VALE_IMAGE:-ghcr.io/arcgolabs/vale:latest}"
fi
export VALE_IMAGE

mkdir -p "$OUTPUT_DIR"

smoke_log() {
  echo "smoke: $*" >&2
}

build_smoke_image_context() {
  if [ "$LOCAL_BUILD" != "1" ]; then
    return 0
  fi
  image_context="$SCRIPT_DIR/.tmp/gossip-smoke-image"
  smoke_log "building linux/$TARGETARCH valed smoke binary"
  mkdir -p "$image_context"
  cp "$SCRIPT_DIR/Dockerfile.smoke" "$image_context/Dockerfile"
  CGO_ENABLED=0 GOOS=linux GOARCH="$TARGETARCH" \
    go build -C cmd -trimpath -ldflags="-s -w" -o "$image_context/valed" .
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

wait_raft_group_voters() {
  group="$1"
  expected="${2:-3}"
  url="http://127.0.0.1:28090/admin/cluster/peers?group=$group"
  out="$OUTPUT_DIR/peers-$group.json"
  end=$(( $(date +%s) + TIMEOUT_SECONDS ))
  while [ "$(date +%s)" -lt "$end" ]; do
    if curl -fsS --max-time 2 "$url" > "$out" 2>/dev/null; then
      voters=$(grep -o '"suffrage"[[:space:]]*:[[:space:]]*"Voter"' "$out" | wc -l | tr -d ' ')
      if [ "$voters" = "$expected" ] &&
        grep -q '"id"[[:space:]]*:[[:space:]]*"node-1"' "$out" &&
        grep -q '"id"[[:space:]]*:[[:space:]]*"node-2"' "$out" &&
        grep -q '"id"[[:space:]]*:[[:space:]]*"node-3"' "$out"; then
        return 0
      fi
    fi
    sleep 0.5
  done
  echo "raft group $group did not converge to $expected voters" >&2
  return 1
}

cleanup() {
  set +e
  docker compose "$@" ps > "$OUTPUT_DIR/containers.txt" 2>/dev/null
  docker compose "$@" logs --no-color > "$OUTPUT_DIR/logs.txt" 2>/dev/null
  if [ "$KEEP" != "1" ]; then
    smoke_log "stopping gossip cluster compose stack"
    docker compose "$@" down -v
  fi
}

cd "$ROOT_DIR"
set -- -f "$COMPOSE_FILE"
if [ "$LOCAL_BUILD" = "1" ]; then
  set -- "$@" -f "$LOCAL_COMPOSE_FILE"
fi
trap 'cleanup "$@"' EXIT

build_smoke_image_context

smoke_log "starting gossip cluster compose stack"
if [ "$LOCAL_BUILD" = "1" ]; then
  docker compose "$@" up -d --build
else
  if [ "$SKIP_PULL" != "1" ]; then
    smoke_log "pulling vale image $VALE_IMAGE"
    docker compose "$@" pull vale-1 vale-2 vale-3
  fi
  docker compose "$@" up -d
fi

smoke_log "waiting for proxy endpoint"
wait_endpoint vale-gossip http://127.0.0.1:18080/

for group in metadata data certificates; do
  smoke_log "waiting for $group raft voters"
  wait_raft_group_voters "$group" 3
done

smoke_log "gossip cluster converged"
