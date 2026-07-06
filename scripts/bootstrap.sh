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
    -e "s/ZEPHYR_SESSION_SECRET=replace-with-random-secret/ZEPHYR_SESSION_SECRET=${session_secret}/" \
    -e "s/WOODPECKER_AGENT_SECRET=replace-with-agent-secret/WOODPECKER_AGENT_SECRET=${agent_secret}/" \
    -e "s/ZEPHYR_DB_PASSWORD=replace-with-db-password/ZEPHYR_DB_PASSWORD=${db_password}/" \
    -e "s/ZEPHYR_DB_ROOT_PASSWORD=replace-with-db-root-password/ZEPHYR_DB_ROOT_PASSWORD=${db_root_password}/" \
    -e "s#ZEPHYR_DB_DSN=zephyr:replace-with-db-password@tcp(zephyr-mysql:3306)/zephyr#ZEPHYR_DB_DSN=zephyr:${db_password}@tcp(zephyr-mysql:3306)/zephyr#" \
    -e "s/ZEPHYR_BOOTSTRAP_PASSWORD=change-me-at-first-login/ZEPHYR_BOOTSTRAP_PASSWORD=${bootstrap_password}/" \
    .env > "$tmp_file"
  mv "$tmp_file" .env
  echo "created .env"
  echo "created bootstrap admin password in .env (ZEPHYR_BOOTSTRAP_PASSWORD)"
else
  echo ".env already exists; keep it"
fi

mkdir -p data/zephyr/ssh
cache_dir="${WOODPECKER_CACHE_DIR:-/opt/woodpecker-cache}"
if ! mkdir -p "$cache_dir" 2>/dev/null; then
  cache_dir="./data/woodpecker-cache"
  mkdir -p "$cache_dir"
  if ! grep -q '^WOODPECKER_CACHE_DIR=' .env 2>/dev/null; then
    printf '\nWOODPECKER_CACHE_DIR=%s\n' "$cache_dir" >> .env
  fi
fi
chmod 700 data/zephyr/ssh

if [ ! -f data/zephyr/tasks.json ]; then
  cp examples/tasks.generic.json data/zephyr/tasks.json
  echo "created data/zephyr/tasks.json from generic example"
fi

echo
echo "Next:"
echo "  1. Edit .env: WOODPECKER_TOKEN, public URLs, Beszel account, optional DB DSN."
echo "  2. Edit data/zephyr/tasks.json or use Zephyr Settings after login."
echo "  3. Run: docker compose up -d --build"
