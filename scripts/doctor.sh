#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

command -v docker >/dev/null 2>&1 || fail "docker is not installed"
docker compose version >/dev/null 2>&1 || fail "docker compose plugin is not available"
test -f .env || fail ".env missing; run scripts/bootstrap.sh first"
test -f data/peapod/tasks.json || echo "WARN: data/peapod/tasks.json missing; Peapod will start with only infrastructure links"

docker compose config >/dev/null

echo "Docker: $(docker --version)"
echo "Compose: $(docker compose version)"
echo "Config: ok"

check_env() {
  key="$1"
  if grep -q "^${key}=" .env 2>/dev/null && ! grep -q "^${key}=replace-with" .env 2>/dev/null; then
    echo "$key: configured"
  else
    echo "WARN: $key is not configured"
  fi
}

check_env PEAPOD_SESSION_SECRET
check_env PEAPOD_DB_DSN
check_env WOODPECKER_AGENT_SECRET
check_env WOODPECKER_TOKEN

if grep -q '^WOODPECKER_MAX_WORKFLOWS=1' .env 2>/dev/null; then
  echo "Woodpecker queue: single workflow"
else
  echo "WARN: set WOODPECKER_MAX_WORKFLOWS=1 on small operations machines"
fi

if [ -f data/peapod/tasks.json ]; then
  if grep -q 'PEAPOD_DEPLOY_MARKER_PATH\|PEAPOD_DEPLOY_VERIFY_URL' data/peapod/tasks.json; then
    echo "Deploy verification: configured in tasks.json"
  else
    echo "WARN: no deploy marker/healthz found in tasks.json"
  fi
fi

health_url="${PEAPOD_HEALTH_URL:-http://127.0.0.1:${PEAPOD_PORT:-8095}/healthz}"
if command -v curl >/dev/null 2>&1; then
  if curl -fsS "$health_url" >/dev/null 2>&1; then
    echo "Peapod health: ok ($health_url)"
  else
    echo "WARN: Peapod health is not reachable yet ($health_url)"
  fi
fi
