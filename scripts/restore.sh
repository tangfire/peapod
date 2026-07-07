#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

backup_dir="${1:-}"
if [ -z "$backup_dir" ]; then
  echo "Usage: CONFIRM_RESTORE=YES scripts/restore.sh /path/to/backup-dir" >&2
  exit 1
fi
if [ "${CONFIRM_RESTORE:-}" != "YES" ]; then
  echo "Refusing to restore without CONFIRM_RESTORE=YES" >&2
  exit 1
fi
if [ ! -d "$backup_dir" ]; then
  echo "Backup directory not found: $backup_dir" >&2
  exit 1
fi

echo "[1/4] stop Peapod service when compose is available"
docker compose stop peapod >/dev/null 2>&1 || true

echo "[2/4] restore env and Peapod data"
for file in .env docker-compose.override.yml; do
  if [ -f "$backup_dir/$file" ]; then
    cp "$backup_dir/$file" "$file"
    chmod 600 "$file" 2>/dev/null || true
  fi
done
if [ -f "$backup_dir/peapod-data.tgz" ]; then
  mkdir -p data
  tar -xzf "$backup_dir/peapod-data.tgz" -C data
fi

echo "[3/4] start database and restore MySQL dump when present"
if [ -f "$backup_dir/mysql.sql" ]; then
  docker compose up -d mysql
  i=1
  while [ "$i" -le 30 ]; do
    if docker compose exec -T mysql sh -lc 'mysqladmin ping -h 127.0.0.1 -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" --silent' >/dev/null 2>&1; then
      break
    fi
    sleep 2
    i=$((i + 1))
  done
  docker compose exec -T mysql sh -lc 'mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE"' < "$backup_dir/mysql.sql"
fi

echo "[4/4] start Peapod"
docker compose up -d peapod

echo "Restore finished from: $backup_dir"
