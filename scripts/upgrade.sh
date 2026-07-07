#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

echo "[1/5] pre-upgrade doctor"
scripts/doctor.sh

echo "[2/5] backup current install"
scripts/backup.sh

echo "[3/5] update source when this is a git checkout"
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git pull --ff-only
else
  echo "WARN: not a git checkout; skip git pull"
fi

echo "[4/5] build or pull containers"
docker compose pull --ignore-pull-failures
docker compose build peapod
docker compose up -d

echo "[5/5] verify Peapod"
scripts/doctor.sh
health_url="${PEAPOD_HEALTH_URL:-http://127.0.0.1:${PEAPOD_PORT:-8095}/healthz}"
if command -v curl >/dev/null 2>&1; then
  curl -fsS "$health_url" >/dev/null
fi

echo "Upgrade finished"
