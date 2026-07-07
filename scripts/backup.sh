#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

stamp="$(date +%Y%m%d%H%M%S)"
backup_root="${PEAPOD_BACKUP_DIR:-$ROOT_DIR/backups}"
backup_dir="$backup_root/peapod-$stamp"
mkdir -p "$backup_dir"
chmod 700 "$backup_dir"

echo "[1/4] backup env and compose files"
for file in .env docker-compose.yml docker-compose.override.yml; do
  if [ -f "$file" ]; then
    cp "$file" "$backup_dir/$file"
    chmod 600 "$backup_dir/$file" 2>/dev/null || true
  fi
done

echo "[2/4] backup Peapod data without SSH private keys"
if [ -d data/peapod ]; then
  tar \
    --exclude 'ssh/*' \
    --exclude '*.sock' \
    -czf "$backup_dir/peapod-data.tgz" \
    -C data peapod
else
  echo "WARN: data/peapod does not exist"
fi

echo "[3/4] backup MySQL dump when local compose mysql is running"
if docker compose ps --status running mysql >/dev/null 2>&1 && [ "$(docker compose ps --status running -q mysql 2>/dev/null | wc -l | tr -d ' ')" != "0" ]; then
  docker compose exec -T mysql sh -lc 'mysqldump -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE"' > "$backup_dir/mysql.sql"
  chmod 600 "$backup_dir/mysql.sql"
else
  echo "WARN: local mysql service is not running; skip mysqldump"
fi

echo "[4/4] write manifest"
{
  echo "created_at=$(date -Is)"
  echo "root=$ROOT_DIR"
  echo "git_commit=$(git rev-parse --short HEAD 2>/dev/null || true)"
  echo "compose_project=$(docker compose ls --format json 2>/dev/null | head -c 2000 || true)"
} > "$backup_dir/MANIFEST"
chmod 600 "$backup_dir/MANIFEST"

echo "Backup created: $backup_dir"
