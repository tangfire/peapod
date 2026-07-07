# Peapod

Peapod is a lightweight operations cockpit for small teams. It gives one clean entry point for deployment, pipeline diagnosis, resource monitoring, logs, and infrastructure links while keeping the underlying tools replaceable.

The default lightweight stack is:

- Peapod: operations cockpit and task registry
- Woodpecker: CI/CD runner and manual deployment executor
- Beszel: host and container resource visibility
- Dozzle: Docker-retained logs and live tailing without a central log database

The optional full observability stack is:

- Grafana + Prometheus + Loki + Tempo: historical metrics, logs, traces, dashboards, and alerts

Peapod does not replace these systems. It hides the daily complexity so operators can work from one product surface.

## Quick Start

```bash
git clone https://github.com/tangfire/peapod.git peapod
cd peapod
scripts/install.sh
```

This creates `.env`, generates local secrets, creates a monitor SSH key, starts the lightweight stack, and prints the first login steps. Open Peapod, then finish setup from `Settings -> 接入向导`.

To also start Grafana, Prometheus, Loki, and Tempo:

```bash
scripts/install.sh --observability
```

Open:

- Peapod: `http://127.0.0.1:8095`
- Woodpecker: `http://127.0.0.1:8000`
- Beszel: `http://127.0.0.1:8090`
- Dozzle: `http://127.0.0.1:8081`
- Grafana, when `observability` is enabled: `http://127.0.0.1:3000`

Run checks:

```bash
scripts/doctor.sh
go test ./...
npm --prefix frontend run build
```

The intended daily flow is UI-first:

1. `Settings -> 接入向导`: finish public URLs, Woodpecker token, Beszel, Dozzle/Grafana, SSH key and monitored hosts.
2. `Settings -> 仓库与任务`: lookup/save Woodpecker repos, then create deployment tasks from templates.
3. `Deploy`: run deploy/rollback from project status, not from raw pipeline variables.
4. `Monitoring`: check host pressure and core containers.
5. `Logs`: search recent Docker-retained logs from multiple containers through Dozzle MCP.
6. `Pipelines`: diagnose failures and jump to Woodpecker only when needed.

## Lowest-Cost Setup Path

For a small team or a fresh server, start with the lightweight mode:

- One operations machine runs Peapod, MySQL, Woodpecker, Beszel, and Dozzle.
- Business machines do not run Peapod. They only need Docker, a monitor SSH key, and optionally a Beszel agent.
- Deployment tasks are created from Peapod templates; users should not edit `tasks.json` for normal onboarding.
- Logs start with Dozzle MCP and Docker log rotation. Peapod can aggregate recent logs from selected containers, while Grafana/Loki remains the option for searchable cross-machine history.

To prepare a business machine, copy the command from `Settings -> 接入向导 -> 业务机接入命令`. It uses `scripts/managed-host.sh` to create a low-privilege monitor user, install the Peapod monitor public key, and optionally install Docker.

Back up or upgrade a self-hosted install:

```bash
scripts/backup.sh
scripts/upgrade.sh
```

## Configuration Model

Peapod is intentionally configuration-driven.

Peapod is configured through `PEAPOD_*` environment variables plus the runtime configuration saved from the Settings page.

- Repositories and deployment tasks live in `data/peapod/tasks.json`.
- Monitored hosts live in `PEAPOD_MONITOR_HOSTS_JSON`.
- External links live in `PEAPOD_LINKS_JSON`.
- User accounts can use MySQL through `PEAPOD_DB_DSN`; otherwise Peapod falls back to a single emergency password.
- The default compose stack includes a local `peapod-mysql` service for team accounts, audit logs, and setup state. You can later point `PEAPOD_DB_DSN` to a managed MySQL instance without changing Peapod code.

Compatibility note: upgrades from earlier internal builds can still read legacy environment aliases and copy old account data into the new `peapod_*` tables at startup. New installs and docs should use Peapod names only.

The bundled `examples/` folder contains:

- `tasks.generic.json`: neutral example for a normal service
- `tasks.peapod-self.json`: Peapod self-deploy tasks for Woodpecker
- `monitor-hosts.generic.json`: local + remote host monitoring example
- `tasks.novelcat.json`: studio-specific migration example, intentionally outside the app defaults

For daily use, prefer the Settings page:

- Use the onboarding guide to see which integrations are ready.
- Use repository lookup to save Woodpecker repos.
- Use task templates to create deployment, rollback, cleanup, static site, Go backend, blue/green, and Peapod self-deploy tasks.
- Use doctor checks before upgrades or when a new machine is being connected.

## Operations Docs

- [Architecture](docs/ops-architecture.md): how Peapod, Woodpecker, Beszel, Grafana, Loki, and managed machines fit together.
- [Component Profiles](docs/component-profiles.md): choose the lightweight or full observability stack.
- [Logging Standard](docs/logging-standard.md): structured JSON fields for services that Peapod can search and filter reliably.
- [Migration Runbook](docs/migration-runbook.md): how to move Peapod to a dedicated operations/build machine and connect production or test hosts.

## Required Secrets

At minimum, set these in `.env`:

```env
PEAPOD_SESSION_SECRET=...
PEAPOD_PASSWORD=...
WOODPECKER_TOKEN=...
WOODPECKER_AGENT_SECRET=...
```

For real team usage, keep database auth enabled. The default local Docker MySQL DSN looks like:

```env
PEAPOD_DB_DSN=peapod:password@tcp(mysql:3306)/peapod?parseTime=true&charset=utf8mb4&loc=Local
PEAPOD_BOOTSTRAP_USERNAME=admin
PEAPOD_BOOTSTRAP_PASSWORD=change-this-on-first-login
```

To move to cloud MySQL later, create the same database and change only `PEAPOD_DB_DSN`.

Peapod creates these tables automatically:

- `peapod_users`
- `peapod_deploy_audit_logs`

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
        "PEAPOD_PROJECT_ID": "app",
        "PEAPOD_PROJECT_NAME": "业务服务",
        "PEAPOD_DEPLOY_MARKER_PATH": "/opt/your-service/.deploy/current-source-sha",
        "PEAPOD_DEPLOY_VERIFY_URL": "http://127.0.0.1:8080/healthz"
      }
    }
  ]
}
```

Use the same `PEAPOD_PROJECT_ID` for deploy and rollback tasks so the project status table can merge them into one service row.
Deploy and rollback tasks must set `PEAPOD_DEPLOY_MARKER_PATH` or `PEAPOD_DEPLOY_VERIFY_URL`. Peapod treats a Woodpecker success as only a build result; a deployment becomes the current verified online version only after the marker commit matches and/or the health endpoint returns 2xx/3xx. Without either check, Peapod shows "build succeeded, deployment unverified" and disables that task as a trusted deploy entry.

By default the Peapod service mounts `${PEAPOD_DEPLOY_MARKER_ROOT:-/opt}` read-only into the container so it can read host-side deployment markers such as `/opt/woodpecker-cache/<repo>-meta/current-source-sha` or `/opt/<service>/.deploy/current-source-sha`. If a service is remote or the marker path is not mounted, keep a health URL configured; Peapod will treat a passing health check as usable and show the missing marker as a verification wiring warning instead of a failed deployment.

## Peapod Self Deploy

This repository includes `.woodpecker/deploy.yml` for Peapod itself. It is a manual pipeline with three supported actions:

- `DEPLOY_ACTION=status`: check the Peapod compose service and health endpoint.
- `DEPLOY_ACTION=restart`: restart the Peapod compose service.
- `DEPLOY_ACTION=deploy`: run frontend build, Go tests, build the Peapod image, run `docker compose up -d peapod`, and verify health.

To enable it on a new operations machine:

1. Enable the Peapod repository in Woodpecker.
2. Mark the repo as trusted for volumes, because the deploy step mounts `/var/run/docker.sock` and `/opt/peapod`.
3. Add the task snippet from `examples/tasks.peapod-self.json` into `data/peapod/tasks.json`.
4. Replace `repo_id` and `repo_name` with the real Woodpecker repo id and Git repo name.
5. Run `检查 Peapod 状态` first, then run `部署 Peapod`.

The bundled deploy script expects Peapod to live at `/opt/peapod` and run as the `peapod` Docker Compose service.

## Build Queue

Keep `WOODPECKER_MAX_WORKFLOWS=1` on small operations machines. Peapod can trigger multiple deployments, but Woodpecker will run only one workflow at a time and keep the rest pending, which prevents two large Docker builds from exhausting CPU, memory, or disk IO at the same time.

## Backup, Restore, Upgrade

Peapod includes conservative operation scripts:

- `scripts/backup.sh`: stores `.env`, compose files, Peapod data, and a local MySQL dump when compose MySQL is running. It excludes SSH private keys by default.
- `scripts/restore.sh`: restores from a backup directory and requires `CONFIRM_RESTORE=YES`.
- `scripts/upgrade.sh`: runs doctor checks, backs up, pulls source/images, rebuilds Peapod, starts compose, and verifies health.

These scripts are also listed in the onboarding guide so a new team can operate Peapod without memorizing the repository layout.

## Monitoring Hosts

Minimal example:

```env
PEAPOD_MONITOR_HOSTS_JSON=[{"id":"prod","name":"生产机","role":"production","ssh_host":"example.com:22","ssh_user":"ops","containers":["api","worker","mysql"]}]
```

Beszel is preferred. SSH is only a read-only fallback and uses:

```env
PEAPOD_MONITOR_SSH_KEY_PATH=/data/ssh/monitor_ed25519
```

Put the private key at `data/peapod/ssh/monitor_ed25519` when using the default compose volume path.

## Migrating To A New Machine

1. Install Docker and the Compose plugin.
2. Clone this repository.
3. Run `scripts/bootstrap.sh`.
4. Edit `.env` public URLs, secrets, database DSN, and OAuth settings.
5. Put deploy tasks into `data/peapod/tasks.json`.
6. Configure Beszel systems or SSH fallback hosts.
7. Run `docker compose up -d --build`.

After that, daily operations should happen inside Peapod:

- trigger deployments and rollbacks
- inspect running and failed pipelines
- read failure summaries and recent container logs
- check host CPU, memory, disk, containers
- open Woodpecker/Beszel/Dozzle/Grafana only for deeper details

For small machines, keep the default lightweight profile first. It uses Beszel for resource curves and Dozzle MCP for Docker-retained container logs plus live tailing. Enable the `observability` profile only when you need searchable log history across hosts, metrics retention, traces, or Grafana alerts. Peapod's setup page shows the active log strategy, Dozzle MCP status, and Docker log rotation values, defaulting to `DOCKER_LOG_MAX_SIZE=20m` and `DOCKER_LOG_MAX_FILE=3`.

## Boundary

Peapod should stay generic. Product-specific scripts, dashboards, repositories, and task variables belong in `examples/`, `data/peapod/tasks.json`, or the target project repositories. Do not hard-code business project names into Peapod itself.
