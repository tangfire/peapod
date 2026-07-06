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
- The default compose stack includes a local `zephyr-mysql` service for team accounts, audit logs, and setup state. You can later point `ZEPHYR_DB_DSN` to a managed MySQL instance without changing Zephyr code.

The bundled `examples/` folder contains:

- `tasks.generic.json`: neutral example for a normal service
- `tasks.zephyr-self.json`: Zephyr self-deploy tasks for Woodpecker
- `monitor-hosts.generic.json`: local + remote host monitoring example
- `tasks.novelcat.json`: our studio-specific migration example, intentionally outside the app defaults

## Operations Docs

- [Architecture](docs/ops-architecture.md): how Zephyr, Woodpecker, Beszel, Grafana, Loki, and managed machines fit together.
- [Migration Runbook](docs/migration-runbook.md): how to move Zephyr to a dedicated operations/build machine and connect production or test hosts.

## Required Secrets

At minimum, set these in `.env`:

```env
ZEPHYR_SESSION_SECRET=...
ZEPHYR_PASSWORD=...
WOODPECKER_TOKEN=...
WOODPECKER_AGENT_SECRET=...
```

For real team usage, keep database auth enabled. The default local Docker MySQL DSN looks like:

```env
ZEPHYR_DB_DSN=zephyr:password@tcp(zephyr-mysql:3306)/zephyr?parseTime=true&charset=utf8mb4&loc=Local
ZEPHYR_BOOTSTRAP_USERNAME=admin
ZEPHYR_BOOTSTRAP_PASSWORD=change-this-on-first-login
```

To move to cloud MySQL later, create the same database and change only `ZEPHYR_DB_DSN`.

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

## Zephyr Self Deploy

This repository includes `.woodpecker/deploy.yml` for Zephyr itself. It is a manual pipeline with three supported actions:

- `DEPLOY_ACTION=status`: check the host service and health endpoint.
- `DEPLOY_ACTION=restart`: restart the host Zephyr service.
- `DEPLOY_ACTION=deploy`: run frontend build, Go tests, Go build, copy the new release into `/opt/zephyr`, and restart.

To enable it on a new operations machine:

1. Enable the Zephyr repository in Woodpecker.
2. Mark the repo as trusted for volumes, because the deploy step mounts `/var/run/docker.sock` and `/opt/zephyr`.
3. Add the task snippet from `examples/tasks.zephyr-self.json` into `data/zephyr/tasks.json`.
4. Replace `repo_id` and `repo_name` with the real Woodpecker repo id and Git repo name.
5. Run `检查 Zephyr 状态` first, then run `部署 Zephyr`.

The current deploy script expects Zephyr to run as a host systemd service named `zephyr`. If you run Zephyr only as a Compose container, keep using `docker compose up -d --build` or adapt `scripts/deploy-zephyr-local.sh` for that deployment shape.

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
