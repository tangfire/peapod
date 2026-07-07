#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

rand_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return
  fi
  date +%s | sha256sum | awk '{print $1}'
}

if [ ! -f .env ]; then
  cp .env.example .env
  session_secret="$(rand_secret)"
  agent_secret="$(rand_secret)"
  db_password="$(rand_secret)"
  db_root_password="$(rand_secret)"
  bootstrap_password="$(rand_secret)"
  tmp_file=".env.tmp"
  sed \
    -e "s/PEAPOD_SESSION_SECRET=replace-with-random-secret/PEAPOD_SESSION_SECRET=${session_secret}/" \
    -e "s/WOODPECKER_AGENT_SECRET=replace-with-agent-secret/WOODPECKER_AGENT_SECRET=${agent_secret}/" \
    -e "s/PEAPOD_DB_PASSWORD=replace-with-db-password/PEAPOD_DB_PASSWORD=${db_password}/" \
    -e "s/PEAPOD_DB_ROOT_PASSWORD=replace-with-db-root-password/PEAPOD_DB_ROOT_PASSWORD=${db_root_password}/" \
    -e "s#PEAPOD_DB_DSN=peapod:replace-with-db-password@tcp(mysql:3306)/peapod#PEAPOD_DB_DSN=peapod:${db_password}@tcp(mysql:3306)/peapod#" \
    -e "s/PEAPOD_BOOTSTRAP_PASSWORD=change-me-at-first-login/PEAPOD_BOOTSTRAP_PASSWORD=${bootstrap_password}/" \
    .env > "$tmp_file"
  mv "$tmp_file" .env
  echo "created .env"
  echo "created bootstrap admin password in .env (PEAPOD_BOOTSTRAP_PASSWORD)"
else
  echo ".env already exists; keep it"
fi

mkdir -p data/peapod/ssh
if [ ! -f data/peapod/ssh/monitor_ed25519 ]; then
  if command -v ssh-keygen >/dev/null 2>&1; then
    ssh-keygen -t ed25519 -N "" -C "peapod-monitor" -f data/peapod/ssh/monitor_ed25519 >/dev/null
    echo "created monitor SSH key at data/peapod/ssh/monitor_ed25519"
  else
    echo "WARN: ssh-keygen is not installed; create data/peapod/ssh/monitor_ed25519 manually if SSH fallback monitoring is needed"
  fi
else
  echo "monitor SSH key already exists; keep it"
fi
cache_dir="${WOODPECKER_CACHE_DIR:-/opt/woodpecker-cache}"
if ! mkdir -p "$cache_dir" 2>/dev/null; then
  cache_dir="./data/woodpecker-cache"
  mkdir -p "$cache_dir"
  if ! grep -q '^WOODPECKER_CACHE_DIR=' .env 2>/dev/null; then
    printf '\nWOODPECKER_CACHE_DIR=%s\n' "$cache_dir" >> .env
  fi
fi
chmod 700 data/peapod/ssh
chmod 600 data/peapod/ssh/monitor_ed25519 2>/dev/null || true
chmod 644 data/peapod/ssh/monitor_ed25519.pub 2>/dev/null || true

if [ ! -f data/peapod/tasks.json ]; then
  cp examples/tasks.generic.json data/peapod/tasks.json
  echo "created data/peapod/tasks.json from generic example"
fi

echo
echo "Next:"
echo "  1. Run: scripts/install.sh"
echo "  2. Open Peapod and follow Settings -> 接入向导."
echo "  3. Use Settings -> 仓库与任务 -> 从模板创建任务."
echo
echo "Login:"
echo "  username: ${PEAPOD_BOOTSTRAP_USERNAME:-admin}"
echo "  password: grep '^PEAPOD_BOOTSTRAP_PASSWORD=' .env | cut -d= -f2-"
