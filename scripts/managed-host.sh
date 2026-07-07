#!/usr/bin/env sh
set -eu

usage() {
  cat <<'EOF'
Usage:
  PEAPOD_MONITOR_PUBLIC_KEY='ssh-ed25519 ... peapod-monitor' scripts/managed-host.sh

Optional:
  PEAPOD_MANAGED_USER=peapod-monitor
  INSTALL_DOCKER=1

Run this on a server that Peapod should monitor or deploy to.
EOF
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

managed_user="${PEAPOD_MANAGED_USER:-peapod-monitor}"
public_key="${PEAPOD_MONITOR_PUBLIC_KEY:-}"

echo "[1/4] host facts"
echo "hostname=$(hostname)"
echo "ip=$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
df -h / || true
free -h || true

echo "[2/4] optional Docker install/check"
if command -v docker >/dev/null 2>&1; then
  docker version --format 'docker={{.Server.Version}}' 2>/dev/null || docker --version
elif [ "${INSTALL_DOCKER:-}" = "1" ]; then
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL https://get.docker.com | sh
  else
    echo "WARN: curl is not installed; install Docker manually"
  fi
else
  echo "WARN: Docker is not installed; set INSTALL_DOCKER=1 to install with get.docker.com"
fi

echo "[3/4] create managed user"
if id "$managed_user" >/dev/null 2>&1; then
  echo "user $managed_user already exists"
else
  if [ "$(id -u)" = "0" ]; then
    useradd -m -s /bin/bash "$managed_user"
  elif command -v sudo >/dev/null 2>&1; then
    sudo useradd -m -s /bin/bash "$managed_user"
  else
    echo "WARN: cannot create $managed_user without root or sudo"
  fi
fi

echo "[4/4] install monitor public key"
if [ -n "$public_key" ]; then
  home_dir="$(getent passwd "$managed_user" 2>/dev/null | cut -d: -f6 || echo "/home/$managed_user")"
  if [ "$(id -u)" = "0" ]; then
    mkdir -p "$home_dir/.ssh"
    grep -qxF "$public_key" "$home_dir/.ssh/authorized_keys" 2>/dev/null || echo "$public_key" >> "$home_dir/.ssh/authorized_keys"
    chown -R "$managed_user:$managed_user" "$home_dir/.ssh"
    chmod 700 "$home_dir/.ssh"
    chmod 600 "$home_dir/.ssh/authorized_keys"
  elif command -v sudo >/dev/null 2>&1; then
    sudo mkdir -p "$home_dir/.ssh"
    sudo sh -c "grep -qxF '$public_key' '$home_dir/.ssh/authorized_keys' 2>/dev/null || echo '$public_key' >> '$home_dir/.ssh/authorized_keys'"
    sudo chown -R "$managed_user:$managed_user" "$home_dir/.ssh"
    sudo chmod 700 "$home_dir/.ssh"
    sudo chmod 600 "$home_dir/.ssh/authorized_keys"
  else
    echo "WARN: cannot install key without root or sudo"
  fi
else
  echo "WARN: PEAPOD_MONITOR_PUBLIC_KEY is empty; skip SSH key install"
fi

echo "DONE managed_user=$managed_user"
