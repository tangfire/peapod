#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

profile_args=""
case "${1:-}" in
  "")
    ;;
  --observability)
    profile_args="--profile observability"
    ;;
  -h|--help)
    cat <<'EOF'
Usage:
  scripts/install.sh                 Start the lightweight Peapod stack
  scripts/install.sh --observability Start Peapod plus Grafana/Loki/Prometheus/Tempo

The lightweight stack is recommended first. You can enable observability later.
EOF
    exit 0
    ;;
  *)
    echo "Unknown option: $1" >&2
    exit 1
    ;;
esac

echo "[1/4] bootstrap config and local data"
scripts/bootstrap.sh

echo "[2/4] start Docker Compose stack"
# shellcheck disable=SC2086
docker compose $profile_args up -d --build

echo "[3/4] run doctor"
scripts/doctor.sh || true

echo "[4/4] next steps"
scripts/print-next-steps.sh
