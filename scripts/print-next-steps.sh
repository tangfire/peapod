#!/usr/bin/env sh
set -eu

cat <<'EOF'
Peapod quick start

1. Start the lightweight stack:
   scripts/install.sh

2. Open:
   Peapod      http://127.0.0.1:8095
   Woodpecker  http://127.0.0.1:8000
   Beszel      http://127.0.0.1:8090
   Dozzle      http://127.0.0.1:8081

3. Login:
   username admin
   password is PEAPOD_BOOTSTRAP_PASSWORD in .env

4. In Peapod:
   Settings -> 接入向导: finish public URLs, Woodpecker token, Beszel/Dozzle, SSH key checks
   Settings -> 仓库与任务: save Woodpecker repos and create deploy tasks from templates
   Monitoring: confirm host resources and core containers

5. Optional full observability:
   scripts/install.sh --observability
EOF
