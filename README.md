# Zephyr

Zephyr is a lightweight operations cockpit for small teams. It gives one clean entry point for deployment, pipeline diagnosis, resource monitoring, logs, and infrastructure links while keeping the underlying tools replaceable.

The default stack is:

- Zephyr: operations cockpit and task registry
- Woodpecker: CI/CD runner and manual deployment executor
- Beszel: host and container resource visibility
- Grafana + Prometheus + Loki + Tempo: metrics, logs, and traces

Zephyr does not replace these systems. It hides the daily complexity so operators can work from one product surface.

## Quick Start

```bash
git clone <your-zephyr-repo> zephyr
cd zephyr
scripts/bootstrap.sh
vi .env
docker compose up -d --build
```

Open:

- Zephyr: `http://127.0.0.1:8095`
- Woodpecker: `http://127.0.0.1:8000`
- Beszel: `http://127.0.0.1:8090`
- Grafana: `http://127.0.0.1:3000`

Run checks:

```bash
scripts/doctor.sh
go test ./...
npm --prefix frontend run build
```

## Configuration Model

Zephyr is intentionally configuration-driven.

- Repositories and deployment tasks live in `data/zephyr/tasks.json`.
- Monitored hosts live in `ZEPHYR_MONITOR_HOSTS_JSON`.
- External links live in `ZEPHYR_LINKS_JSON`.
- User accounts can use MySQL through `ZEPHYR_DB_DSN`; otherwise Zephyr falls back to a single emergency password.

The bundled `examples/` folder contains:

- `tasks.generic.json`: neutral example for a normal service
- `monitor-hosts.generic.json`: local + remote host monitoring example
- `tasks.novelcat.json`: our studio-specific migration example, intentionally outside the app defaults

## Required Secrets

At minimum, set these in `.env`:

```env
ZEPHYR_SESSION_SECRET=...
ZEPHYR_PASSWORD=...
WOODPECKER_TOKEN=...
WOODPECKER_AGENT_SECRET=...
```

For real team usage, prefer database auth:

```env
ZEPHYR_DB_DSN=zephyr:password@tcp(mysql-host:3306)/zephyr?parseTime=true&charset=utf8mb4&loc=Local
ZEPHYR_BOOTSTRAP_USERNAME=admin
ZEPHYR_BOOTSTRAP_PASSWORD=change-this-on-first-login
```

Zephyr creates these tables automatically:

- `zephyr_users`
- `zephyr_deploy_audit_logs`

## Deployment Tasks

Task example:

```json
{
  "repos": {
    "1": "your-service-repo"
  },
  "tasks": [
    {
      "id": "app-deploy",
      "group": "业务服务",
      "title": "部署业务服务",
      "repo_id": 1,
      "repo_name": "your-service-repo",
      "branch": "main",
      "risk": "normal",
      "variables": {
        "DEPLOY_ACTION": "deploy",
        "ZEPHYR_PROJECT_ID": "app",
        "ZEPHYR_PROJECT_NAME": "业务服务"
      }
    }
  ]
}
```

Use the same `ZEPHYR_PROJECT_ID` for deploy and rollback tasks so the project status table can merge them into one service row.

## Monitoring Hosts

Minimal example:

```env
ZEPHYR_MONITOR_HOSTS_JSON=[{"id":"prod","name":"生产机","role":"production","ssh_host":"example.com:22","ssh_user":"ops","containers":["api","worker","mysql"]}]
```

Beszel is preferred. SSH is only a read-only fallback and uses:

```env
ZEPHYR_MONITOR_SSH_KEY_PATH=/data/ssh/monitor_ed25519
```

Put the private key at `data/zephyr/ssh/monitor_ed25519` when using the default compose volume path.

## Migrating To A New Machine

1. Install Docker and the Compose plugin.
2. Clone this repository.
3. Run `scripts/bootstrap.sh`.
4. Edit `.env` public URLs, secrets, database DSN, and OAuth settings.
5. Put deploy tasks into `data/zephyr/tasks.json`.
6. Configure Beszel systems or SSH fallback hosts.
7. Run `docker compose up -d --build`.

After that, daily operations should happen inside Zephyr:

- trigger deployments and rollbacks
- inspect running and failed pipelines
- read failure summaries and tail logs
- check host CPU, memory, disk, containers
- open Grafana/Beszel/Woodpecker only for deeper details

## Boundary

Zephyr should stay generic. Product-specific scripts, dashboards, repositories, and task variables belong in `examples/`, `data/zephyr/tasks.json`, or the target project repositories. Do not hard-code business project names into Zephyr itself.
