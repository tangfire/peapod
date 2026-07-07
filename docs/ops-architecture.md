# Peapod 运维驾驶舱架构

Peapod 的目标是成为工作室统一的运维入口：日常部署、回退、流水线排障、资源监控、日志查看和外部系统跳转，都从 Peapod 进入。底层仍然使用 Woodpecker、Beszel、Dozzle、Grafana、Loki、Prometheus 等成熟组件，但业务团队不需要每天直接操作这些系统。

## 部署形态

推荐只在一台专门的运维/构建机上运行 Peapod 全家桶。

```text
运维/构建机
- Peapod
- Woodpecker server / agent
- Beszel hub
- Dozzle
- Grafana / Loki / Prometheus / Tempo（可选完整观测栈）
- 构建缓存 / 可选镜像仓库

生产机 / 测试机 / 业务机
- 业务服务容器
- Beszel agent
- 日志采集 agent（仅完整观测栈需要）
- SSH deploy key / monitor key
```

业务机不需要运行 Peapod。它们只需要接入 Peapod 管理体系。

## 每类数据怎么来

### 部署与回退

Peapod 负责展示任务、确认参数、记录审计和调用 Woodpecker。

Woodpecker 负责真正执行 CI/CD。部署脚本通过 SSH 进入目标机器，完成拉镜像、切换蓝绿槽位、健康检查、回退等动作。

```text
Peapod -> Woodpecker API -> Woodpecker agent -> SSH -> 目标机器
```

### 资源监控

资源数据优先来自 Beszel。Beszel hub 放在运维/构建机，业务机上只跑 Beszel agent。

Peapod 展示摘要：CPU、内存、磁盘、核心容器、最近检查时间和异常原因。需要完整曲线时跳转 Beszel。

```text
业务机 Beszel agent -> 运维机 Beszel hub -> Peapod 摘要 / Beszel 详情
```

Peapod 也支持 SSH 只读兜底，用于 Beszel 不可用时读取 `df`、`free`、`docker ps`、`docker stats` 等基本状态。

### 日志

轻量方案使用 Dozzle 直接查看 Docker 当前仍保留的容器日志，并实时跟随新日志。它不建立集中日志库，也不要求业务机安装日志采集 agent，适合小机器、刚上线阶段和临时排障。

```text
运维机 Docker socket -> Dozzle -> Peapod 入口 / Dozzle 详情
```

完整观测方案使用集中 Loki。业务机上运行日志采集 agent，例如 Grafana Alloy、Promtail 或 Vector，把 Docker 日志、系统日志、Caddy/Nginx 日志和应用日志推到运维机 Loki。

Peapod 后续只做关键摘要和跳转入口；完整检索、图表和告警在 Grafana。

```text
业务机 Docker logs / app logs
  -> 日志采集 agent
  -> 运维机 Loki
  -> Grafana / Peapod 关键摘要
```

临时排障时，Peapod 可以通过 SSH 拉取 `docker logs --tail`，但这只是兜底，不替代 Loki。

### 指标与告警

Prometheus 放在运维/构建机。业务服务可以通过 node exporter、cadvisor、应用 metrics endpoint 或现有 observability agent 上报指标。

Grafana 负责完整看板和告警。Peapod 首页只显示上线前最关心的状态：机器资源、核心容器、运行中流水线、最近成功版本和异常列表。

## 新增一台机器的接入步骤

1. 在新机器安装 Docker 和 Compose 插件。
2. 创建只读监控用户或复用受控 SSH 用户。
3. 把 Peapod 的 monitor SSH public key 加到新机器。
4. 把 Woodpecker deploy SSH public key 加到新机器。
5. 启动 Beszel agent，接入运维机 Beszel hub。
6. 轻量方案可先跳过日志采集 agent；完整观测方案再启动日志采集 agent，把日志推到运维机 Loki。
7. 在 Peapod 配置里新增 host、核心容器和外部链接。
8. 如果这台机器承载部署目标，在 Peapod 任务配置里新增对应部署任务。

## 安全边界

- Peapod 不保存业务服务密钥明文到前端。
- SSH 私钥只放在服务器文件系统或 Woodpecker secret，不进 git。
- Beszel、Loki、Grafana 的账号密码只放 `.env` 或对应系统配置。
- Peapod 前端只展示摘要和安全日志尾部，不返回完整环境变量。
- 生产机不跑 Peapod，减少暴露面。

## 推荐域名

域名可以按团队习惯配置，建议至少拆开：

- `deploy.example.com`: Peapod
- `ci.example.com`: Woodpecker
- `monitor.example.com`: Beszel
- `logs.example.com`: Dozzle
- `grafana.example.com`: Grafana

在 NovelCat 当前环境里可以对应：

- `deploy.novelcat.cloud`: Peapod
- `ci.novelcat.cloud`: Woodpecker
- `beszel.novelcat.cloud`: Beszel
- `logs.novelcat.cloud`: Dozzle
- `grafana.novelcat.cloud`: Grafana

## 设计原则

- Peapod 只跑一套，业务机只接入 agent。
- 底层工具可替换，Peapod 的任务、主机、链接都配置化。
- 日常操作不要求团队理解 Woodpecker 参数、Loki 查询或 Beszel API。
- 失败时必须能回到原始系统排障：Woodpecker 日志、Grafana 日志、Beszel 曲线都保留入口。
- 写书猫、e站、9router 等业务项目不要硬编码进 Peapod 源码，只放在配置和示例里。
