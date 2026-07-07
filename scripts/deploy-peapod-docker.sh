#!/bin/sh
set -eu

DEPLOY_ACTION="${DEPLOY_ACTION:-deploy}"
DEPLOY_DIR="${PEAPOD_DEPLOY_DIR:-/opt/peapod}"
COMPOSE_SERVICE="${PEAPOD_COMPOSE_SERVICE:-peapod}"
HEALTH_URL="${PEAPOD_HEALTH_URL:-http://127.0.0.1:8095/healthz}"

host_healthcheck() {
  attempts="${1:-30}"
  i=1
  while [ "$i" -le "$attempts" ]; do
    if docker run --rm --network host docker:28-cli \
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

case "$DEPLOY_ACTION" in
  deploy|restart|status) ;;
  *)
    echo "unsupported DEPLOY_ACTION=$DEPLOY_ACTION (expected deploy, restart or status)" >&2
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

compose build "$COMPOSE_SERVICE"
compose up -d --no-deps "$COMPOSE_SERVICE"
host_healthcheck 30

printf '%s\n' "${CI_COMMIT_SHA:-unknown}" > "$DEPLOY_DIR/.deploy/current-source-sha"
printf '%s deploy %s pipeline=%s\n' "$(date -Is)" "${CI_COMMIT_SHA:-unknown}" "${CI_PIPELINE_NUMBER:-manual}" >> "$DEPLOY_DIR/.deploy/deploy-history.log"
