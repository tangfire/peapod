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
test -f data/zephyr/tasks.json || echo "WARN: data/zephyr/tasks.json missing; Zephyr will start with only infrastructure links"

docker compose config >/dev/null

echo "Docker: $(docker --version)"
echo "Compose: $(docker compose version)"
echo "Config: ok"
