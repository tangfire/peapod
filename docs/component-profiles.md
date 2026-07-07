# Peapod 组件方案

Peapod 的底层组件可以按机器规格和团队阶段选择。默认推荐先跑轻量方案，等确实需要历史日志、指标保留、链路追踪和告警时，再开启完整观测方案。

## 轻量方案

适合 2-4 核、4GB 内存左右的小机器，也适合刚上线、日志量不大的团队。

```bash
docker compose up -d --build
```

包含：

- Peapod：统一入口、任务配置、审计和资源摘要。
- MySQL：成员账号、审计记录和接入配置。
- Woodpecker：CI/CD 和部署执行。
- Beszel：机器资源、磁盘、容器状态和趋势曲线。
- Dozzle：查看 Docker 当前仍保留的容器日志，并通过只读 MCP 给 Peapod 提供多容器日志汇总。

特点：

- 常驻资源占用低。
- 不需要业务机运行日志采集 agent。
- 适合临时排障、上线前检查、部署失败后在 Peapod 里看近期容器日志。
- 缺点是没有独立长期日志库；可看多久取决于 Docker 日志驱动、轮转配置和容器是否还在，也没有完整指标/链路告警。

默认入口：

- Peapod: `http://127.0.0.1:8095`
- Woodpecker: `http://127.0.0.1:8000`
- Beszel: `http://127.0.0.1:8090`
- Dozzle: `http://127.0.0.1:8081`

## 完整观测方案

适合日志量变大、需要跨机器检索、需要告警、需要保留指标和链路追踪时启用。

```bash
docker compose --profile observability up -d --build
```

在轻量方案之外增加：

- Grafana：统一仪表盘、日志检索、告警入口。
- Loki：集中日志存储和查询。
- Prometheus：指标抓取和保留。
- Tempo：链路追踪。

特点：

- 可以保留和检索历史日志。
- 可以做 Grafana dashboard 和告警。
- 可以接入业务机日志采集 agent，例如 Alloy、Promtail 或 Vector。
- 成本是更高的内存、磁盘和 IO 占用，尤其是 Loki 和 Prometheus。

建议保留周期：

- Loki: 3-7 天起步。
- Prometheus: 7-15 天起步。
- Tempo: 24-72 小时起步。

## 选择建议

| 场景 | 推荐方案 |
| --- | --- |
| 新机器、内部测试、刚上线 | 轻量方案 |
| 小团队日常部署和看资源 | 轻量方案 |
| 需要查 1-7 天前的日志 | 完整观测方案 |
| 需要统一告警和 dashboard | 完整观测方案 |
| 需要链路追踪 | 完整观测方案 |

## 安全和资源注意

- Dozzle 挂载 Docker socket，只绑定 `127.0.0.1` 端口，并通过外层反代/登录保护暴露；`DOZZLE_ENABLE_MCP=true` 仅开放只读工具，`DOZZLE_ENABLE_ACTIONS=false` 和 `DOZZLE_ENABLE_SHELL=false` 保持关闭。
- Woodpecker agent 也会使用 Docker socket，继续保持 `WOODPECKER_MAX_WORKFLOWS=1`，避免小机器多个项目同时构建。
- 无论轻量还是完整方案，都要限制 Docker json-file 日志大小；默认 compose 已设置 `DOCKER_LOG_MAX_SIZE` 和 `DOCKER_LOG_MAX_FILE`。
- 完整观测方案的历史数据不建议无限保留，迁移机器时优先迁配置，不迁旧日志。
