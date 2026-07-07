# 迁移到专用运维/构建机 Runbook

这份文档用于把 Peapod、Woodpecker、Beszel、Dozzle、Grafana/Loki 等运维能力从临时机器迁移到一台新的专用运维/构建机。目标是：迁完以后，业务机器只作为被管理节点，不再承担运维驾驶舱职责。

## 迁移前确认

准备新机器：

- 2 核 4G 起步，推荐 4 核 8G 或更高。
- 磁盘 80GB 起步，推荐 100GB 以上。
- Docker 和 Docker Compose plugin。
- 可访问 GitHub、镜像源、业务机器 SSH 端口。
- 域名已准备好：Peapod、Woodpecker、Beszel、Dozzle；如果开启完整观测栈，再准备 Grafana。
- 云厂商安全组已放行临时验证端口，或已准备 80/443 反代：
  - Peapod: `8095`
  - Woodpecker: `8000`
  - Beszel hub: `8090`
  - Dozzle: `8081`
  - Grafana: `3000`

准备迁移资料：

- Peapod `.env`，但不要把旧 secret 发到聊天或提交 git。
- Peapod `data/peapod/tasks.json`。
- Woodpecker repo 配置、agent secret、OAuth 配置和 deploy secrets。
- Beszel hub 数据或重新接入计划。
- Dozzle 入口配置。
- Grafana dashboard、datasource、alert 配置（仅完整观测栈）。
- Loki/Prometheus 数据是否需要保留。v1 可以不迁历史数据，只迁配置。

默认建议：新机器先用轻量方案，不迁旧 Loki/Tempo/Prometheus 历史数据。确实需要完整观测时，再迁 Grafana dashboard、datasource、Alloy/Prometheus/Loki/Tempo 配置。历史日志通常体积大、价值衰减快，新运维机上线后重新积累更干净。

## 新机器安装

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl git
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker "$USER"
newgrp docker
docker version
docker compose version
```

克隆 Peapod：

```bash
sudo mkdir -p /opt
sudo chown "$USER":"$USER" /opt
git clone https://github.com/tangfire/peapod.git /opt/peapod
cd /opt/peapod
scripts/bootstrap.sh
```

编辑 `.env`：

```bash
cp .env.example .env
vi .env
```

必须配置：

```env
PEAPOD_PUBLIC_URL=https://deploy.example.com
WOODPECKER_PUBLIC_URL=https://ci.example.com
DOZZLE_PUBLIC_URL=https://logs.example.com
PEAPOD_DOZZLE_PUBLIC_URL=https://logs.example.com
PEAPOD_SESSION_SECRET=change-me
PEAPOD_PASSWORD=change-me
WOODPECKER_TOKEN=change-me
WOODPECKER_AGENT_SECRET=change-me
```

如果暂时不用云 MySQL，保留默认本地 Docker MySQL：

```env
PEAPOD_DB_DSN=peapod:password@tcp(mysql:3306)/peapod?parseTime=true&charset=utf8mb4&loc=Local
PEAPOD_MYSQL_BIND=127.0.0.1
PEAPOD_MYSQL_PORT=13307
```

以后切换云 MySQL 时，只需要把 `PEAPOD_DB_DSN` 指到云 MySQL，并确认安全组只允许运维机访问。

如果使用 MySQL 账号体系：

```env
PEAPOD_DB_DSN=peapod:password@tcp(mysql-host:3306)/peapod?parseTime=true&charset=utf8mb4&loc=Local
PEAPOD_BOOTSTRAP_USERNAME=admin
PEAPOD_BOOTSTRAP_PASSWORD=change-this-on-first-login
```

启动：

```bash
docker compose up -d --build
scripts/doctor.sh
```

默认命令只启动轻量方案。需要 Grafana/Loki/Prometheus/Tempo 时再执行：

```bash
docker compose --profile observability up -d --build
```

如果这是构建机，建议准备 Woodpecker 常用宿主目录：

```bash
sudo mkdir -p /opt/woodpecker-cache /opt/buildkit-cache /opt/deploy-cache
sudo chmod 755 /opt/woodpecker-cache /opt/buildkit-cache /opt/deploy-cache
```

默认 Compose 会把 `/opt/woodpecker-cache` 挂进 agent，业务流水线仍可以在各自 `.woodpecker` 中显式挂载 `/opt/woodpecker-cache`、`/opt/buildkit-cache`、`/root/.ssh`、业务部署目录等路径。

## 迁移 Peapod 任务配置

从旧机器复制任务配置：

```bash
scp old-host:/opt/old-peapod/data/tasks.json /opt/peapod/data/peapod/tasks.json
```

检查任务：

- `repo_id` 是否对应新 Woodpecker 里的 repo。
- `branch` 是否是默认部署分支。
- `variables` 是否仍匹配业务仓库的 `.woodpecker` 文件。
- 业务服务、Peapod、网关或中转站是否分别有部署和回退任务。
- 清理磁盘任务是否只清 Docker cache、构建缓存和明确允许的目录。

如果 repo id 改了，优先在 Peapod 设置页修改任务配置，不要改源码。

### Peapod 自部署流水线

Peapod 仓库自带 `.woodpecker/deploy.yml`，迁移后建议把 Peapod 自己也接进 Woodpecker，这样以后更新运维驾驶舱不需要 SSH 上机器手动替换。

接入顺序：

1. 在 Woodpecker 启用 Peapod 仓库。
2. 记录 Peapod repo id。
3. 给该 repo 开启 trusted volumes。Peapod 自部署需要挂载 `/var/run/docker.sock` 和 `/opt/peapod`。
4. 把 `examples/tasks.peapod-self.json` 合并到 `/opt/peapod/data/peapod/tasks.json`。
5. 把示例里的 `repo_id` 和 `repo_name` 改成真实值。
6. 先在 Peapod 里执行 `检查 Peapod 状态`，确认 Woodpecker 能触发到仓库。
7. 再执行 `部署 Peapod`。

Peapod 自部署目前支持：

- `DEPLOY_ACTION=status`：检查 Peapod Compose 服务和 health endpoint。
- `DEPLOY_ACTION=restart`：重启 Peapod Compose 服务，不改代码和配置。
- `DEPLOY_ACTION=deploy`：前端构建、Go 测试、Docker 镜像构建，然后 `docker compose up -d peapod`。

当前 `scripts/deploy-peapod-docker.sh` 默认 Peapod 以 Docker Compose service `peapod` 运行，部署目录为 `/opt/peapod`。

如果旧流水线使用 `skip_clone: true` 并依赖宿主缓存目录，迁移后要先预热源码缓存，或确认流水线会在缓存缺失时自行 clone：

```bash
sudo mkdir -p /opt/woodpecker-cache
sudo git clone --branch main git@github.com:owner/repo.git /opt/woodpecker-cache/repo
```

需要本地部署 runner 镜像的项目，也要在新构建机提前准备镜像，例如 `app-deploy-runner:latest`、`gateway-deploy-runner:latest`，否则 deploy step 会在第一轮找不到本地镜像。

## 迁移 Woodpecker

推荐新机器重新配置 Woodpecker，而不是直接复制数据库。这样更干净。

步骤：

1. 配好 OAuth 或 GitHub App。
2. 在 Woodpecker 里启用需要的仓库。
3. 为每个仓库补齐 secrets。
4. 记录 repo id。
5. 回到 Peapod 的 `tasks.json` 更新 repo id。
6. 运行一次低风险任务，比如查看状态或构建测试。

如果必须保留历史流水线，可迁移 Woodpecker 数据库和 volume，但要确保版本一致。

如果旧 Woodpecker 正在运行，不要直接 `tar` 活跃 SQLite 文件做最终迁移。最终切换前建议短暂停旧 Woodpecker：

```bash
cd /opt/woodpecker
docker compose stop woodpecker-server woodpecker-agent
# 复制 woodpecker volume 到新机器
docker compose start woodpecker-server woodpecker-agent
```

这样可以避免 `woodpecker.sqlite: file changed as we read it` 导致新库半截。

## 接入生产机和业务机

在每台被管理机器上执行：

1. 添加 monitor SSH public key。
2. 添加 deploy SSH public key。
3. 启动 Beszel agent。
4. 轻量方案可以先不启动日志采集 agent；完整观测方案再启动日志采集 agent。
5. 检查防火墙，只开放必要端口。

### Beszel agent

按 Beszel 页面生成 agent 命令。接入成功后，Peapod 监控页应该能看到对应机器。

如果 Beszel 暂时不可用，Peapod 可以通过 SSH fallback 展示基本资源，但这不是长期方案。

从旧 hub 迁到新 hub 时，旧 agent 往往仍指向旧地址。处理顺序：

1. 先确认新 hub 从业务机可达：`curl http://NEW_OPS_IP:8090/api/health`。
2. 在新 Beszel 页面确认或生成 agent key/token。
3. 停旧 agent，用新 `HUB_URL`、`KEY`、`TOKEN`、`SYSTEM_NAME` 重新启动。
4. Peapod 监控页应从 `ssh_fallback` 变成 `beszel` 或至少不再提示 Beszel 记录不可用。

不要在安全组未放通 `8090` 前切 agent，否则 agent 会反复重连失败，Peapod 只能继续走 SSH fallback。

### 轻量日志：Dozzle

默认 compose 会启动 Dozzle，它通过 Docker socket 查看运维机本机 Docker 当前仍保留的容器日志，并实时跟随新日志。它不迁历史数据、不建立集中日志库。建议只通过内网、反代和登录保护暴露：

```env
DOZZLE_BIND=127.0.0.1
DOZZLE_PORT=8081
DOZZLE_PUBLIC_URL=https://logs.example.com
PEAPOD_DOZZLE_PUBLIC_URL=https://logs.example.com
DOZZLE_ENABLE_ACTIONS=false
DOZZLE_ENABLE_SHELL=false
```

### 完整观测：日志采集 agent

推荐 Grafana Alloy。业务机只负责采集并推送，Loki 放在运维/构建机。

需要采集：

- Docker container logs
- Caddy/Nginx access logs
- Caddy/Nginx error logs
- 应用结构化日志
- 系统关键日志

接入后，在 Grafana 里用机器名、项目名、容器名筛选日志。

## 切换域名

把域名解析到新运维/构建机：

- `deploy.example.com`
- `ci.example.com`
- `beszel.example.com`
- `logs.example.com`
- `grafana.example.com`

如果新机器一开始没有域名，可以先用 IP + 端口验证：

- Peapod: `http://NEW_IP:8095`
- Woodpecker: `http://NEW_IP:8000`
- Beszel: `http://NEW_IP:8090`
- Dozzle: `http://NEW_IP:8081`
- Grafana: `http://NEW_IP:3000`

但 GitHub OAuth、Webhook 回调、HTTPS 登录 Cookie 最终仍建议使用正式域名。

切换后检查：

```bash
curl -I https://deploy.example.com
curl -I https://ci.example.com
curl -I https://beszel.example.com
curl -I https://logs.example.com
curl -I https://grafana.example.com
```

如果使用 Caddy 或 Nginx 统一入口，先让新机器签好证书，再切流量。

## 上线验收

必须通过：

- Peapod 可以登录。
- Peapod 首页显示两台业务机资源。
- Peapod 可以打开 Woodpecker、Beszel、Dozzle；完整观测方案下也可以打开 Grafana。
- 核心业务部署任务可以触发，并能跳转到 Woodpecker 对应流水线。
- 失败流水线能在 Peapod 看到关键错误摘要。
- 轻量方案下，Dozzle 能查看运维机本机 Docker 已保留日志并实时跟随新日志。
- 完整观测方案下，生产机和业务机日志能在 Grafana/Loki 搜索。
- 清理磁盘任务有确认框、审计记录，并且不会清理业务数据目录。
- 回退任务可以看到上一成功版本。

## 回滚方案

迁移期间不要立刻停旧构建机。

保留旧环境至少 24 到 72 小时。新环境异常时：

1. 暂停新 Peapod 的部署入口。
2. 域名切回旧构建机。
3. Woodpecker 任务回到旧 repo id 和旧 secret。
4. 业务机上的 agent 可以继续保留，不影响业务服务。

## 后续优化

- 把业务机 agent 安装做成一条 bootstrap 命令。
- 给 Peapod 增加主机接入向导。
- 给 Loki 增加项目维度的默认查询模板。
- 给 Dozzle 增加按服务分组的快捷入口。
- 给 Woodpecker secrets 增加迁移清单。
- 给 Peapod 增加“新机器接入检查”页面。
