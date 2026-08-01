#!/bin/sh
set -eu

DEPLOY_ACTION="${DEPLOY_ACTION:-deploy}"
DEPLOY_DIR="${PEAPOD_DEPLOY_DIR:-/opt/peapod}"
COMPOSE_SERVICE="${PEAPOD_COMPOSE_SERVICE:-peapod}"
HEALTH_URL="${PEAPOD_HEALTH_URL:-http://127.0.0.1:8095/healthz}"
RUNTIME_BASE_DOCKERFILE="${PEAPOD_RUNTIME_BASE_DOCKERFILE:-Dockerfile.runtime-base}"
RUNTIME_BASE_IMAGE_PREFIX="${PEAPOD_RUNTIME_BASE_IMAGE_PREFIX:-peapod-runtime-base}"
PEAPODCTL_INSTALL_DIR="${PEAPODCTL_INSTALL_DIR:-$DEPLOY_DIR/bin}"

export DOCKER_BUILDKIT="${DOCKER_BUILDKIT:-1}"
export COMPOSE_DOCKER_CLI_BUILD="${COMPOSE_DOCKER_CLI_BUILD:-1}"

host_healthcheck() {
  attempts="${1:-30}"
  i=1
  while [ "$i" -le "$attempts" ]; do
    container_id="$(compose ps -q "$COMPOSE_SERVICE" 2>/dev/null || true)"
    if [ -n "$container_id" ] && docker exec "$container_id" \
      sh -lc "wget -qO- --timeout=5 '$HEALTH_URL'" >/tmp/peapod-health.out 2>/tmp/peapod-health.err; then
      cat /tmp/peapod-health.out
      rm -f /tmp/peapod-health.out /tmp/peapod-health.err
      return 0
    fi
    sleep 2
    i=$((i + 1))
  done
  echo "Peapod health check failed: $HEALTH_URL" >&2
  cat /tmp/peapod-health.err >&2 2>/dev/null || true
  rm -f /tmp/peapod-health.out /tmp/peapod-health.err
  return 1
}

compose() {
  cd "$DEPLOY_DIR"
  docker compose "$@"
}

ensure_runtime_base_image() {
  cd "$DEPLOY_DIR"
  if [ ! -f "$RUNTIME_BASE_DOCKERFILE" ]; then
    echo "runtime base Dockerfile not found: $DEPLOY_DIR/$RUNTIME_BASE_DOCKERFILE" >&2
    return 1
  fi

  runtime_hash="$(sha256sum "$RUNTIME_BASE_DOCKERFILE" | awk '{print $1}' | cut -c1-16)"
  runtime_image="${RUNTIME_BASE_IMAGE_PREFIX}:${runtime_hash}"
  if [ "${PEAPOD_FORCE_RUNTIME_BASE_BUILD:-0}" != "1" ] && docker image inspect "$runtime_image" >/dev/null 2>&1; then
    echo "Peapod runtime base already exists: $runtime_image"
  else
    echo "Building Peapod runtime base: $runtime_image"
    docker build \
      --label "peapod.runtime-base-sha=$runtime_hash" \
      -f "$RUNTIME_BASE_DOCKERFILE" \
      -t "$runtime_image" \
      .
  fi
  export PEAPOD_RUNTIME_BASE_IMAGE="$runtime_image"
}

install_peapodctl() {
  container_id="$(compose ps -q "$COMPOSE_SERVICE" 2>/dev/null || true)"
  if [ -z "$container_id" ]; then
    echo "cannot install peapodctl: compose service is not running" >&2
    return 1
  fi
  mkdir -p "$PEAPODCTL_INSTALL_DIR"
  temp_path="$PEAPODCTL_INSTALL_DIR/.peapodctl.tmp"
  rm -f "$temp_path"
  docker cp "$container_id:/app/peapodctl" "$temp_path"
  chmod 0755 "$temp_path"
  mv -f "$temp_path" "$PEAPODCTL_INSTALL_DIR/peapodctl"
  echo "installed peapodctl at $PEAPODCTL_INSTALL_DIR/peapodctl"
}

case "$DEPLOY_ACTION" in
  deploy|rollback|restart|status) ;;
  *)
    echo "unsupported DEPLOY_ACTION=$DEPLOY_ACTION (expected deploy, rollback, restart or status)" >&2
    exit 1
    ;;
esac

if [ "$DEPLOY_ACTION" = "status" ]; then
  test -d "$DEPLOY_DIR"
  compose ps
  host_healthcheck 1
  exit 0
fi

if [ "$DEPLOY_ACTION" = "restart" ]; then
  test -d "$DEPLOY_DIR"
  compose up -d --no-deps "$COMPOSE_SERVICE"
  compose restart "$COMPOSE_SERVICE"
  host_healthcheck 30
  exit 0
fi

if [ "$DEPLOY_ACTION" = "rollback" ]; then
  rollback_target="${ROLLBACK_COMMIT:-${ROLLBACK_VERSION:-}}"
  if [ -z "$rollback_target" ]; then
    echo "ROLLBACK_COMMIT or ROLLBACK_VERSION is required for rollback" >&2
    exit 1
  fi
  git rev-parse --verify "$rollback_target^{commit}" >/dev/null
  git checkout --detach "$rollback_target"
fi

deployed_sha="$(git rev-parse HEAD 2>/dev/null || printf '%s' "${CI_COMMIT_SHA:-unknown}")"

docker compose version >/dev/null
mkdir -p "$DEPLOY_DIR"

owner_group="$(stat -c '%u:%g' "$DEPLOY_DIR" 2>/dev/null || echo '1000:1000')"
stamp="$(date +%Y%m%d%H%M%S)"
backup_dir="$DEPLOY_DIR/.deploy/backups/$stamp"
mkdir -p "$backup_dir"

if [ -f "$DEPLOY_DIR/docker-compose.yml" ]; then
  cp "$DEPLOY_DIR/docker-compose.yml" "$backup_dir/docker-compose.yml"
fi
if [ -f "$DEPLOY_DIR/.env" ]; then
  cp "$DEPLOY_DIR/.env" "$backup_dir/env"
fi

tar \
  --exclude '.env' \
  --exclude '.env.host' \
  --exclude 'docker-compose.override.yml' \
  --exclude 'data' \
  --exclude '.deploy' \
  --exclude 'frontend/node_modules' \
  --exclude 'frontend/dist' \
  --exclude 'frontend/tsconfig.tsbuildinfo' \
  --exclude '.git' \
  --exclude '.woodpecker-build' \
  --exclude '*.bak*' \
  -cf - . | tar -xf - -C "$DEPLOY_DIR"

if [ ! -f "$DEPLOY_DIR/.env" ]; then
  cp "$DEPLOY_DIR/.env.example" "$DEPLOY_DIR/.env"
  echo "created $DEPLOY_DIR/.env from .env.example; update secrets before production use" >&2
fi

mkdir -p "$DEPLOY_DIR/data/peapod/ssh" "$DEPLOY_DIR/.deploy" "${PEAPOD_DEPLOY_MARKER_ROOT:-/opt}"
chown -R "$owner_group" "$DEPLOY_DIR" 2>/dev/null || true

ensure_runtime_base_image
compose build "$COMPOSE_SERVICE"
compose up -d --no-deps "$COMPOSE_SERVICE"
install_peapodctl
host_healthcheck 30

printf '%s\n' "$deployed_sha" > "$DEPLOY_DIR/.deploy/current-source-sha"
printf '%s %s %s pipeline=%s rollback_target=%s\n' "$(date -Is)" "$DEPLOY_ACTION" "$deployed_sha" "${CI_PIPELINE_NUMBER:-manual}" "${rollback_target:-}" >> "$DEPLOY_DIR/.deploy/deploy-history.log"
