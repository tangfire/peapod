#!/usr/bin/env sh
set -eu

cat <<'EOF'
Peapod quick start

1. Initialize:
   scripts/bootstrap.sh

2. Configure:
   .env
   data/peapod/tasks.json

3. Start:
   docker compose up -d --build

4. Open:
   Peapod      http://127.0.0.1:8095
   Woodpecker  http://127.0.0.1:8000
   Beszel      http://127.0.0.1:8090
   Grafana     http://127.0.0.1:3000

5. For another server:
   - copy this repository
   - run scripts/bootstrap.sh
   - edit .env public URLs and secrets
   - add host config to PEAPOD_MONITOR_HOSTS_JSON
   - add deploy tasks in Peapod Settings
EOF
