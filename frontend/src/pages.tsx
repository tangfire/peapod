import {
  Alert,
  App as AntApp,
  Button,
  Card,
  Col,
  Descriptions,
  Drawer,
  Form,
  Input,
  InputNumber,
  List,
  Popconfirm,
  Progress,
  Row,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Tooltip,
  Typography
} from "antd";
import type { ColumnsType } from "antd/es/table";
import { ProCard, ProTable, StatisticCard } from "@ant-design/pro-components";
import type { ProColumns } from "@ant-design/pro-components";
import {
  Activity,
  ExternalLink,
  FileText,
  GitBranch,
  Home,
  Link2,
  Play,
  Plus,
  RefreshCw,
  Rocket,
  Server,
  Settings,
  Trash2,
  XCircle
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { ApiError, api, errorText } from "./api";
import type {
  AuditRecord,
  DeploymentStatus,
  MonitoringAlert,
  MonitoringContainer,
  MonitoringHost,
  MonitoringSummary,
  Pipeline,
  PipelineStep,
  PipelineSummary,
  RuntimeConfigInput,
  Risk,
  SetupConfigResponse,
  StateResponse,
  Task,
  TaskConfig,
  User,
  WoodpeckerRepo,
  WoodpeckerReposResponse
} from "./types";

const { Text, Title } = Typography;

const statusColors: Record<string, string> = {
  success: "success",
  running: "processing",
  pending: "warning",
  failure: "error",
  error: "error",
  killed: "default",
  skipped: "default"
};

const riskColors: Record<Risk, string> = {
  normal: "green",
  warning: "gold",
  danger: "red",
  link: "blue"
};

const OVERVIEW_PIPELINE_LOOKBACK_SECONDS = 24 * 60 * 60;

export function zephyrNavItems() {
  return [
    { key: "overview", icon: <Home size={16} />, label: "总览" },
    { key: "deploy", icon: <Rocket size={16} />, label: "部署" },
    { key: "pipelines", icon: <Activity size={16} />, label: "流水线" },
    { key: "monitoring", icon: <Server size={16} />, label: "监控" },
    { key: "links", icon: <Link2 size={16} />, label: "入口" },
    { key: "docs", icon: <FileText size={16} />, label: "文档" },
    { key: "settings", icon: <Settings size={16} />, label: "设置" }
  ];
}

export function OverviewPage({
  state,
  monitoring,
  monitoringLoading,
  pipelines,
  deploymentStatuses,
  runningCount,
  failedCount,
  nowMs,
  triggeringTaskIds,
  onRun,
  onNavigate,
  onRefresh,
  onInspectPipeline
}: {
  state: StateResponse;
  monitoring: MonitoringSummary | null;
  monitoringLoading: boolean;
  pipelines: Pipeline[];
  deploymentStatuses: DeploymentStatus[];
  runningCount: number;
  failedCount: number;
  nowMs: number;
  triggeringTaskIds: string[];
  onRun: (task: Task) => void;
  onNavigate: (key: string) => void;
  onRefresh: () => void;
  onInspectPipeline: (row: Pipeline) => void;
}) {
  const alerts = monitoring?.alerts || [];
  const alertLevel = highestMonitoringAlertLevel(alerts);
  const attentionPipelines = overviewPipelineRows(pipelines, nowMs, 4);
  const productDeploy = state.tasks.find((task) => task.id === "xzm-product-deploy");
  const productRollback = state.tasks.find((task) => task.id === "xzm-product-rollback");
  const productionCleanup = state.tasks.find((task) => task.id === "production-cleanup");
  const triggeringTaskIDSet = new Set(triggeringTaskIds);
  const highestDisk = highestHostMetric(monitoring?.hosts || [], "disk_percent");
  const highestMemory = highestHostMetric(monitoring?.hosts || [], "memory_percent");
  const healthyText = alertLevel === "critical" ? "需要处理" : alertLevel === "warning" ? "有提醒" : "可以上线";
  return (
    <Space direction="vertical" size={16} className="side-stack">
      <ProCard className="overview-hero-card">
        <div className="overview-hero">
          <div>
            <Space size={8} wrap>
              <Tag color={monitoringAlertColor(alertLevel)}>{healthyText}</Tag>
              {monitoring?.source && <Tag color={monitoringSourceColor(monitoring.source)}>{monitoringSourceText(monitoring.source)}</Tag>}
              {monitoring?.checked_at && <Text type="secondary">{checkedAtText(monitoring.checked_at, nowMs)}</Text>}
            </Space>
            <Title level={3}>运行总览</Title>
            <Text type="secondary">线上版本、队列和资源水位集中在这里。</Text>
          </div>
          <Space wrap>
            {productDeploy && (
              <Button type="primary" icon={<Rocket size={16} />} loading={triggeringTaskIDSet.has(productDeploy.id)} onClick={() => onRun(productDeploy)}>
                部署产品
              </Button>
            )}
            {productRollback && (
              <Button danger loading={triggeringTaskIDSet.has(productRollback.id)} onClick={() => onRun(productRollback)}>
                回退产品
              </Button>
            )}
            {productionCleanup && (
              <Button danger onClick={() => onRun(productionCleanup)}>
                清理生产机
              </Button>
            )}
            <Button icon={<RefreshCw size={16} />} onClick={onRefresh}>
              刷新
            </Button>
          </Space>
        </div>
      </ProCard>

      <StatisticCard.Group className="overview-stat-group" direction="row">
        <StatisticCard statistic={{ title: "运行中 / 排队", value: runningCount, prefix: <Activity size={18} /> }} />
        <StatisticCard statistic={{ title: "24h 失败", value: failedCount, valueStyle: { color: failedCount ? "#bd2c2c" : undefined } }} />
        <StatisticCard statistic={{ title: "磁盘最高", value: `${formatPercent(highestDisk.value)}%`, description: highestDisk.host?.name || "-" }} />
        <StatisticCard statistic={{ title: "内存最高", value: `${formatPercent(highestMemory.value)}%`, description: highestMemory.host?.name || "-" }} />
      </StatisticCard.Group>

      {alerts.length > 0 && (
        <Alert
          type={alertLevel === "critical" ? "error" : "warning"}
          showIcon
          message={alerts[0].title}
          description={alerts.slice(0, 3).map((item) => item.message).join("；")}
          action={<Button size="small" onClick={() => onNavigate("monitoring")}>查看监控</Button>}
        />
      )}

      <ProCard split="vertical" gutter={16} className="overview-split-card">
        <ProCard title="线上版本" extra={<Button type="link" onClick={() => onNavigate("deploy")}>进入部署</Button>}>
          <DeploymentVersionList rows={deploymentStatuses.slice(0, 6)} nowMs={nowMs} />
        </ProCard>
        <ProCard title="近期流水线" extra={<Button type="link" onClick={() => onNavigate("pipelines")}>查看全部</Button>}>
          <PipelineActivityList rows={attentionPipelines} nowMs={nowMs} onInspect={onInspectPipeline} />
        </ProCard>
      </ProCard>

      <HomeResourceStrip summary={monitoring} loading={monitoringLoading} onOpenMonitoring={() => onNavigate("monitoring")} />
    </Space>
  );
}

function DeploymentVersionList({ rows, nowMs }: { rows: DeploymentStatus[]; nowMs: number }) {
  if (!rows.length) return <Alert type="info" showIcon message="暂无部署状态" />;
  return (
    <List
      className="overview-version-list"
      dataSource={rows}
      renderItem={(row) => (
        <List.Item>
          <List.Item.Meta
            title={
              <Space wrap>
                <Text strong>{row.name}</Text>
                <Tag color={statusColors[row.latest_status || row.last_status] || "default"}>{statusText(row.latest_status || row.last_status)}</Tag>
              </Space>
            }
            description={
              <Text type="secondary">
                {row.current_branch ? `${row.current_branch} · ${(row.current_commit || "").slice(0, 8) || "-"} · ${deployedAgeText(row.last_deployed_at, nowMs)}` : "暂无成功部署"}
              </Text>
            }
          />
        </List.Item>
      )}
    />
  );
}

function PipelineActivityList({ rows, nowMs, onInspect }: { rows: Pipeline[]; nowMs: number; onInspect: (row: Pipeline) => void }) {
  if (!rows.length) return <Alert type="success" showIcon message="24 小时内没有需要关注的流水线" />;
  return (
    <List
      className="overview-pipeline-list"
      dataSource={rows}
      renderItem={(row) => (
        <List.Item actions={[<Button key="detail" size="small" onClick={() => onInspect(row)}>详情</Button>]}>
          <List.Item.Meta
            title={
              <Space wrap>
                <Text strong>{row.repo_name} #{row.number}</Text>
                <Tag color={statusColors[row.status] || "default"}>{statusText(row.status)}</Tag>
              </Space>
            }
            description={<Text type="secondary">{pipelineTaskText(row)} · {pipelineActivityMetaText(row, nowMs)}</Text>}
          />
        </List.Item>
      )}
    />
  );
}

export function DeployPage({
  state,
  rows,
  woodpecker,
  nowMs,
  tasks,
  currentUser,
  triggeringTaskIds,
  refreshing,
  onRun,
  onRefresh
}: {
  state: StateResponse;
  rows: DeploymentStatus[];
  woodpecker: string;
  nowMs: number;
  tasks: Task[];
  currentUser: User;
  triggeringTaskIds: string[];
  refreshing: boolean;
  onRun: (task: Task) => void;
  onRefresh: () => void;
}) {
  const [filters, setFilters] = useState({ group: "", repo: "", risk: "", q: "" });
  return (
    <Space direction="vertical" size={16} className="side-stack">
      <ProCard
        title={
          <Space size={8}>
            <GitBranch size={16} />
            <span>部署矩阵</span>
          </Space>
        }
        extra={<Button icon={<RefreshCw size={16} />} loading={refreshing} onClick={onRefresh}>刷新</Button>}
      >
        <DeploymentStatusTable
          rows={rows}
          woodpecker={woodpecker}
          nowMs={nowMs}
          tasks={tasks}
          currentUser={currentUser}
          triggeringTaskIds={triggeringTaskIds}
          onRun={onRun}
        />
      </ProCard>
      <ProCard
        title={
          <Space size={8}>
            <Play size={16} />
            <span>动作目录</span>
          </Space>
        }
      >
        <TaskFilters state={state} value={filters} onChange={setFilters} />
        <TaskTable state={state} filters={filters} triggeringTaskIds={triggeringTaskIds} onRun={onRun} />
      </ProCard>
    </Space>
  );
}

export function PipelinePage({
  rows,
  woodpecker,
  nowMs,
  refreshing,
  onRefresh,
  onCancel,
  onInspect
}: {
  rows: Pipeline[];
  woodpecker: string;
  nowMs: number;
  refreshing: boolean;
  onRefresh: () => void;
  onCancel: (row: Pipeline) => void;
  onInspect: (row: Pipeline) => void;
}) {
  const runningRows = rows.filter((row) => ["running", "pending"].includes(row.status));
  return (
    <Space direction="vertical" size={16} className="side-stack">
      {runningRows.length > 0 && (
        <ProCard title="运行中队列">
          <PipelineActivityList rows={runningRows} nowMs={nowMs} onInspect={onInspect} />
        </ProCard>
      )}
      <ProCard title="流水线进度" extra={<Button icon={<RefreshCw size={16} />} loading={refreshing} onClick={onRefresh}>刷新</Button>}>
        <PipelineTable rows={rows} woodpecker={woodpecker} nowMs={nowMs} onCancel={onCancel} onInspect={onInspect} />
      </ProCard>
    </Space>
  );
}

export function SettingsPage({
  state,
  customConfig,
  auditRecords,
  auditLoading,
  onReload,
  onAuditRefresh,
  onAddTask,
  onEditTask,
  onDeleteTask
}: {
  state: StateResponse;
  customConfig: TaskConfig | null;
  auditRecords: AuditRecord[];
  auditLoading: boolean;
  onReload: () => Promise<void>;
  onAuditRefresh: () => void;
  onAddTask: () => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
}) {
  return (
    <Tabs
      className="settings-tabs"
      items={[
        {
          key: "account",
          label: "账号",
          children: (
            <Space direction="vertical" size={16} className="side-stack">
              <Profile state={state} onReload={onReload} />
              {state.current_user.role === "admin" && state.auth_mode === "db" && <Users />}
            </Space>
          )
        },
        {
          key: "repos",
          label: "仓库",
          children: state.current_user.role === "admin" ? <RepositoryConfigPanel state={state} onReload={onReload} /> : <Alert type="info" showIcon message="仓库配置只允许管理员查看和修改" />
        },
        {
          key: "tasks",
          label: "部署任务",
          children: state.configurable ? (
            <TaskConfigView config={customConfig} tasks={state.tasks || []} onAdd={onAddTask} onEdit={onEditTask} onDelete={onDeleteTask} />
          ) : (
            <Alert type="info" showIcon message="当前环境未开启任务配置文件" />
          )
        },
        {
          key: "setup",
          label: "接入配置",
          children: state.current_user.role === "admin" ? <SetupConfigPanel onReload={onReload} /> : <Alert type="info" showIcon message="接入配置只允许管理员查看和修改" />
        },
        { key: "audit", label: "操作历史", children: <AuditLogView records={auditRecords} loading={auditLoading} state={state} onRefresh={onAuditRefresh} /> }
      ]}
    />
  );
}

export function TaskRunContext({ task, statuses, nowMs }: { task: Task; statuses: DeploymentStatus[]; nowMs: number }) {
  const status = deploymentStatusForTask(task, statuses);
  if (!status) return null;
  const rollback = isRollbackTask(task);
  return (
    <div className="run-context">
      <Descriptions size="small" column={1} bordered>
        <Descriptions.Item label="线上版本">
          {status.current_branch ? `${status.current_branch} · ${(status.current_commit || "").slice(0, 8) || "-"}` : "暂无成功部署"}
          {status.last_deployed_at ? ` · ${deployedAgeText(status.last_deployed_at, nowMs)}` : ""}
        </Descriptions.Item>
        <Descriptions.Item label="最近执行">
          {status.latest_action || "-"} · {statusText(status.latest_status)}
          {status.latest_at ? ` · ${deployedAgeText(status.latest_at, nowMs)}` : ""}
        </Descriptions.Item>
        {rollback && (
          <Descriptions.Item label="上一成功版本">
            {status.previous_branch ? `${status.previous_branch} · ${(status.previous_commit || "").slice(0, 8) || "-"}` : "当前列表里没有上一成功版本；实际回退目标由部署脚本按服务器记录决定"}
          </Descriptions.Item>
        )}
      </Descriptions>
    </div>
  );
}

function TaskTable({
  state,
  filters,
  triggeringTaskIds,
  onRun,
  onEdit,
  onDelete
}: {
  state: StateResponse;
  filters: { group: string; repo: string; risk: string; q: string };
  triggeringTaskIds: string[];
  onRun: (task: Task) => void;
  onEdit?: (task: Task) => void;
  onDelete?: (task: Task) => void;
}) {
  const triggeringTaskIDSet = useMemo(() => new Set(triggeringTaskIds), [triggeringTaskIds]);
  const data = (state.tasks || []).filter((item) => {
    if (item.external_url) return false;
    if (filters.group && item.group !== filters.group) return false;
    if (filters.repo && String(item.repo_id) !== filters.repo) return false;
    if (filters.risk && item.risk !== filters.risk) return false;
    const q = filters.q.trim().toLowerCase();
    if (q) {
      const haystack = [item.title, item.description, item.group, item.branch, repoName(state, item), variablesText(item.variables)]
        .join("\n")
        .toLowerCase();
      if (!haystack.includes(q)) return false;
    }
    return true;
  });
  const columns: ProColumns<Task>[] = [
    {
      title: "动作",
      dataIndex: "title",
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          <Text strong>{row.title}</Text>
          <Text type="secondary">{row.description || row.group}</Text>
        </Space>
      )
    },
    {
      title: "归属",
      width: 220,
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          <Text>{row.group || "默认模块"}</Text>
          <Text type="secondary">{repoName(state, row)}</Text>
        </Space>
      )
    },
    {
      title: "默认",
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          <Text>{row.branch || "main"}</Text>
          {pipelineVariableHint(taskToPipelinePreview(state, row)) && <Text type="secondary">{pipelineVariableHint(taskToPipelinePreview(state, row))}</Text>}
          {row.confirm_text && <Text type="secondary">确认：{row.confirm_text}</Text>}
        </Space>
      )
    },
    {
      title: "风险",
      width: 120,
      render: (_, row) => (
        <Space direction="vertical" size={2}>
          <Tag color={riskColors[row.risk] || "default"}>{riskLabel(row.risk)}</Tag>
          {!canRunTask(state.current_user, row) && <Text type="secondary">仅管理员</Text>}
        </Space>
      )
    },
    {
      title: "",
      width: 190,
      render: (_, row) => {
        const triggering = triggeringTaskIDSet.has(row.id);
        const allowed = canRunTask(state.current_user, row);
        return (
          <Space>
            <Tooltip title={allowed ? "" : "仅管理员可执行"}>
              <span>
                <Button type="primary" icon={<Play size={15} />} danger={row.risk === "danger"} loading={triggering} disabled={triggering || !allowed} onClick={() => onRun(row)}>
                  执行
                </Button>
              </span>
            </Tooltip>
            {state.configurable && onEdit && <Button icon={<Settings size={15} />} onClick={() => onEdit(row)} />}
            {state.configurable && onDelete && (row.custom || row.overridden) && (
              <Popconfirm title={row.overridden ? "恢复这个内置任务的默认配置？" : "删除这个自定义任务？"} onConfirm={() => onDelete(row)}>
                <Button icon={<Trash2 size={15} />} />
              </Popconfirm>
            )}
          </Space>
        );
      }
    }
  ];
  return (
    <>
      <ProTable<Task>
        className="desktop-task-table"
        rowKey="id"
        size="middle"
        columns={columns}
        dataSource={data}
        search={false}
        options={false}
        tableAlertRender={false}
        pagination={false}
        scroll={{ x: 860 }}
      />
      <List
        className="mobile-task-list"
        dataSource={data}
        renderItem={(row) => (
          <List.Item
            actions={[
              <Button key="run" type="primary" danger={row.risk === "danger"} loading={triggeringTaskIDSet.has(row.id)} disabled={triggeringTaskIDSet.has(row.id) || !canRunTask(state.current_user, row)} onClick={() => onRun(row)}>
                执行
              </Button>
            ]}
          >
            <List.Item.Meta
              title={
                <Space wrap>
                  <Text strong>{row.title}</Text>
                  <Tag color={riskColors[row.risk] || "default"}>{riskLabel(row.risk)}</Tag>
                  {!canRunTask(state.current_user, row) && <Tag>仅管理员</Tag>}
                </Space>
              }
              description={
                <Space direction="vertical" size={4}>
                  <Text type="secondary">{row.description}</Text>
                  <Text>{row.group || "默认模块"} · {repoName(state, row)} · {row.branch || "main"}</Text>
                  <Text type="secondary">{pipelineTaskText(taskToPipelinePreview(state, row))}</Text>
                  {state.configurable && onEdit && (
                    <Space>
                      <Button size="small" icon={<Settings size={14} />} onClick={() => onEdit(row)}>设置</Button>
                      {onDelete && (row.custom || row.overridden) && (
                        <Popconfirm title={row.overridden ? "恢复这个内置任务的默认配置？" : "删除这个自定义任务？"} onConfirm={() => onDelete(row)}>
                          <Button size="small" icon={<Trash2 size={14} />}>{row.overridden ? "恢复默认" : "删除"}</Button>
                        </Popconfirm>
                      )}
                    </Space>
                  )}
                </Space>
              }
            />
          </List.Item>
        )}
      />
    </>
  );
}

function TaskFilters({
  state,
  value,
  onChange
}: {
  state: StateResponse;
  value: { group: string; repo: string; risk: string; q: string };
  onChange: (value: { group: string; repo: string; risk: string; q: string }) => void;
}) {
  const tasks = (state.tasks || []).filter((item) => !item.external_url);
  const groups = Array.from(new Set(tasks.map((item) => item.group).filter(Boolean))).sort();
  const repoIDs = Array.from(new Set(tasks.map((item) => String(item.repo_id)).filter(Boolean))).sort((a, b) => Number(a) - Number(b));
  const update = (patch: Partial<typeof value>) => onChange({ ...value, ...patch });
  return (
    <Row gutter={[10, 10]} className="task-filter-row">
      <Col xs={24} md={6}>
        <Input
          allowClear
          placeholder="搜索动作、变量、模块、仓库"
          value={value.q}
          onChange={(event) => update({ q: event.target.value })}
        />
      </Col>
      <Col xs={12} md={5}>
        <Select
          allowClear
          placeholder="模块"
          value={value.group || undefined}
          onChange={(next) => update({ group: next || "" })}
          options={groups.map((group) => ({ value: group, label: group }))}
          className="full-width"
        />
      </Col>
      <Col xs={12} md={5}>
        <Select
          allowClear
          placeholder="执行仓库"
          value={value.repo || undefined}
          onChange={(next) => update({ repo: next || "" })}
          options={repoIDs.map((id) => ({ value: id, label: state.repos[id] || `Repo ${id}` }))}
          className="full-width"
        />
      </Col>
      <Col xs={12} md={4}>
        <Select
          allowClear
          placeholder="风险"
          value={value.risk || undefined}
          onChange={(next) => update({ risk: next || "" })}
          options={[
            { value: "normal", label: "普通" },
            { value: "warning", label: "注意" },
            { value: "danger", label: "高危" }
          ]}
          className="full-width"
        />
      </Col>
      <Col xs={12} md={4}>
        <Button block onClick={() => onChange({ group: "", repo: "", risk: "", q: "" })}>
          重置
        </Button>
      </Col>
    </Row>
  );
}

export function DeployErrorContent({ error, task }: { error: unknown; task: Task }) {
  const details = error instanceof ApiError ? error.details : [];
  const variables = safeVariablesTextForDisplay(task.variables || {});
  return (
    <Space direction="vertical" size={10} style={{ width: "100%" }}>
      <Alert
        type="error"
        showIcon
        message={errorText(error) || "Woodpecker 没有返回可读错误"}
        description="Zephyr 已经记录这次失败。优先检查 Woodpecker 仓库 ID、分支、token 权限、仓库 Trusted/Secrets 配置，以及 Woodpecker Server 日志。"
      />
      <Descriptions size="small" column={1} bordered>
        <Descriptions.Item label="任务">{task.title}</Descriptions.Item>
        <Descriptions.Item label="仓库">Repo {task.repo_id}</Descriptions.Item>
        <Descriptions.Item label="分支">{task.branch || "main"}</Descriptions.Item>
        <Descriptions.Item label="变量">{variables || "-"}</Descriptions.Item>
      </Descriptions>
      {details.length > 0 && (
        <div>
          <Text strong>后端诊断</Text>
          <ul className="deploy-error-details">
            {details.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </div>
      )}
    </Space>
  );
}

function DeploymentStatusTable({
  rows,
  woodpecker,
  nowMs,
  tasks,
  currentUser,
  triggeringTaskIds,
  onRun
}: {
  rows: DeploymentStatus[];
  woodpecker: string;
  nowMs: number;
  tasks: Task[];
  currentUser: User;
  triggeringTaskIds: string[];
  onRun: (task: Task) => void;
}) {
  const triggeringTaskIDSet = useMemo(() => new Set(triggeringTaskIds), [triggeringTaskIds]);
  const columns: ProColumns<DeploymentStatus>[] = [
    {
      title: "项目",
      width: 210,
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          <Text strong>{row.name}</Text>
          <Text type="secondary">{row.group} · {row.repo_name || `Repo ${row.repo_id}`}</Text>
        </Space>
      )
    },
    {
      title: "线上版本",
      width: 250,
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          {row.current_branch ? (
            <Space size={6}>
              <Tag color={row.current_branch === row.configured_branch ? "green" : "gold"}>{row.current_branch}</Tag>
              {row.current_branch !== row.configured_branch && <Text type="warning">与配置不同</Text>}
            </Space>
          ) : (
            <Tag>暂无成功部署</Tag>
          )}
          <Text code={Boolean(row.current_commit)}>{(row.current_commit || "").slice(0, 8) || "-"}</Text>
          <Text type="secondary">{row.last_deployed_at ? `${formatUnixTime(row.last_deployed_at)} · ${deployedAgeText(row.last_deployed_at, nowMs)}` : `配置：${row.configured_branch || "main"}`}</Text>
        </Space>
      )
    },
    {
      title: "最近执行",
      width: 230,
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          <Space size={6}>
            <Text>{row.latest_action || row.last_action || "-"}</Text>
            <Tag color={statusColors[row.latest_status || row.last_status] || "default"}>{statusText(row.latest_status || row.last_status)}</Tag>
          </Space>
          <Text type="secondary">
            {[row.latest_triggered_by || row.triggered_by, row.latest_at ? formatUnixTime(row.latest_at) : ""].filter(Boolean).join(" · ") || "-"}
          </Text>
        </Space>
      )
    },
    {
      title: "上一成功版本",
      width: 180,
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          <Text>{row.previous_branch || "-"}</Text>
          <Text code={Boolean(row.previous_commit)}>{(row.previous_commit || "").slice(0, 8) || "-"}</Text>
          {row.previous_deployed_at ? <Text type="secondary">{deployedAgeText(row.previous_deployed_at, nowMs)}</Text> : null}
        </Space>
      )
    },
    {
      title: "",
      width: 240,
      render: (_, row) => {
        const actions = deploymentActionsForStatus(row, tasks);
        return (
          <Space>
            {actions.deploy && (
              <Button size="small" type="primary" loading={triggeringTaskIDSet.has(actions.deploy.id)} disabled={!canRunTask(currentUser, actions.deploy)} onClick={() => onRun(actions.deploy!)}>
                部署
              </Button>
            )}
            {actions.rollback && (
              <Tooltip title={canRunTask(currentUser, actions.rollback) ? "" : "仅管理员可执行"}>
                <span>
                  <Button size="small" danger loading={triggeringTaskIDSet.has(actions.rollback.id)} disabled={!canRunTask(currentUser, actions.rollback)} onClick={() => onRun(actions.rollback!)}>
                    回退
                  </Button>
                </span>
              </Tooltip>
            )}
            {row.pipeline ? <Button size="small" href={deploymentPipelineURL(woodpecker, row)} target="_blank" icon={<ExternalLink size={14} />} /> : null}
          </Space>
        );
      }
    }
  ];

  return (
    <>
      <ProTable<DeploymentStatus>
        className="desktop-deployment-table"
        rowKey={(row) => row.id}
        size="small"
        columns={columns}
        dataSource={rows}
        search={false}
        options={false}
        tableAlertRender={false}
        pagination={false}
        scroll={{ x: 1040 }}
      />
      <List
        className="mobile-deployment-list"
        dataSource={rows}
        renderItem={(row) => (
          <List.Item
            actions={[
              ...mobileDeploymentActions(row, tasks, currentUser, triggeringTaskIDSet, onRun),
              row.pipeline ? <Button key="open" size="small" href={deploymentPipelineURL(woodpecker, row)} target="_blank" icon={<ExternalLink size={14} />} /> : null
            ].filter(Boolean)}
          >
            <List.Item.Meta
              title={<Space><Text strong>{row.name}</Text><Tag color={statusColors[row.latest_status || row.last_status] || "default"}>{statusText(row.latest_status || row.last_status)}</Tag></Space>}
              description={
                <Space direction="vertical" size={4} className="side-stack">
                  <Text type="secondary">{row.repo_name || `Repo ${row.repo_id}`} · 配置 {row.configured_branch || "main"}</Text>
                  <Text>{row.current_branch ? `已部署 ${row.current_branch} · ${(row.current_commit || "").slice(0, 8) || "-"}` : "暂无成功部署"}</Text>
                  <Text type="secondary">
                    最近执行：{row.latest_action || "-"} · {row.latest_at ? `${formatUnixTime(row.latest_at)} · ${deployedAgeText(row.latest_at, nowMs)}` : "-"}
                  </Text>
                </Space>
              }
            />
          </List.Item>
        )}
      />
    </>
  );
}

function PipelineTable({
  rows,
  woodpecker,
  nowMs,
  onCancel,
  onInspect
}: {
  rows: Pipeline[];
  woodpecker: string;
  nowMs: number;
  onCancel: (row: Pipeline) => void;
  onInspect: (row: Pipeline) => void;
}) {
  const columns: ProColumns<Pipeline>[] = [
    {
      title: "流水线",
      width: 210,
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          <Text strong>{row.repo_name} #{row.number}</Text>
          <Text type="secondary">{row.event || "manual"}</Text>
        </Space>
      )
    },
    {
      title: "代码版本",
      width: 180,
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          <Text>{row.branch || "-"}</Text>
          <Text type="secondary">{(row.commit || "").slice(0, 8) || "-"}</Text>
        </Space>
      )
    },
    {
      title: "动作",
      width: 190,
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          <Text>{pipelineTaskText(row)}</Text>
          {pipelineVariableHint(row) && <Text type="secondary">{pipelineVariableHint(row)}</Text>}
        </Space>
      )
    },
    {
      title: "触发",
      width: 170,
      render: (_, row) => <Text type="secondary">{pipelineTriggerText(row)}</Text>
    },
    {
      title: "时间",
      width: 150,
      render: (_, row) => <Text type="secondary">{pipelineTimeText(row)}</Text>
    },
    {
      title: "耗时",
      width: 110,
      render: (_, row) => <Text type="secondary">{pipelineDurationText(row, nowMs)}</Text>
    },
    {
      title: "状态",
      width: 92,
      render: (_, row) => <Tag color={statusColors[row.status] || "default"}>{row.status}</Tag>
    },
    {
      title: "进度",
      width: 110,
      render: (_, row) => <Progress percent={pipelinePercent(row, nowMs)} size="small" status={progressStatus(row)} />
    },
    {
      title: "",
      width: 128,
      render: (_, row) => (
        <Space>
          <Button size="small" onClick={() => onInspect(row)}>
            详情
          </Button>
          <Button size="small" href={pipelineURL(woodpecker, row)} target="_blank" icon={<ExternalLink size={14} />} />
          {["running", "pending"].includes(row.status) && (
            <Popconfirm title="取消这条流水线？" onConfirm={() => onCancel(row)}>
              <Button size="small" danger icon={<XCircle size={14} />} />
            </Popconfirm>
          )}
        </Space>
      )
    }
  ];
  return (
    <>
      <ProTable<Pipeline>
        className="desktop-pipeline-table"
        rowKey={(row) => `${row.repo_id}-${row.number}`}
        size="small"
        columns={columns}
        dataSource={rows}
        search={false}
        options={false}
        tableAlertRender={false}
        pagination={{ pageSize: 12, showSizeChanger: true, pageSizeOptions: [12, 30], showTotal: (total) => `共 ${total} 条` }}
        scroll={{ x: 1120 }}
      />
      <List
        className="mobile-pipeline-list"
        dataSource={rows}
        pagination={{ pageSize: 12, size: "small" }}
        renderItem={(row) => (
          <List.Item
            actions={[
              <Button key="detail" size="small" onClick={() => onInspect(row)}>详情</Button>,
              <Button key="open" size="small" href={pipelineURL(woodpecker, row)} target="_blank" icon={<ExternalLink size={14} />} />,
              ["running", "pending"].includes(row.status) ? (
                <Popconfirm key="cancel" title="取消这条流水线？" onConfirm={() => onCancel(row)}>
                  <Button size="small" danger icon={<XCircle size={14} />} />
                </Popconfirm>
              ) : null
            ].filter(Boolean)}
          >
            <List.Item.Meta
              title={<Space><Text strong>{row.repo_name} #{row.number}</Text><Tag color={statusColors[row.status] || "default"}>{row.status}</Tag></Space>}
              description={
                <Space direction="vertical" size={4} className="side-stack">
                  <Text type="secondary">{row.branch || "-"} · {(row.commit || "").slice(0, 8) || "-"}</Text>
                  <Text>{pipelineTaskText(row)}</Text>
                  <Text type="secondary">{pipelineTriggerText(row)}</Text>
                  <Text type="secondary">{pipelineTimeText(row)} · {pipelineDurationText(row, nowMs)}</Text>
                  <Progress percent={pipelinePercent(row, nowMs)} size="small" status={progressStatus(row)} />
                </Space>
              }
            />
          </List.Item>
        )}
      />
    </>
  );
}

export function PipelineSummaryDrawer({
  open,
  loading,
  summary,
  nowMs,
  onClose
}: {
  open: boolean;
  loading: boolean;
  summary: PipelineSummary | null;
  nowMs: number;
  onClose: () => void;
}) {
  const pipeline = summary?.pipeline;
  return (
    <Drawer
      title={pipeline ? `${pipeline.repo_name || "Repo"} #${pipeline.number}` : "流水线详情"}
      open={open}
      onClose={onClose}
      width={720}
      destroyOnClose
    >
      <Space direction="vertical" size={16} className="side-stack">
        <ProCard loading={loading}>
          {pipeline ? (
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label="状态">
                <Tag color={statusColors[pipeline.status] || "default"}>{statusText(pipeline.status)}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="动作">{pipelineTaskText(pipeline)}</Descriptions.Item>
              <Descriptions.Item label="分支 / 提交">{pipeline.branch || "-"} · {(pipeline.commit || "").slice(0, 10) || "-"}</Descriptions.Item>
              <Descriptions.Item label="触发">{pipelineTriggerText(pipeline)}</Descriptions.Item>
              <Descriptions.Item label="耗时">{pipelineDurationText(pipeline, nowMs)}</Descriptions.Item>
              {summary?.failure_summary && <Descriptions.Item label="失败摘要">{summary.failure_summary}</Descriptions.Item>}
            </Descriptions>
          ) : (
            <Card loading />
          )}
        </ProCard>

        <ProCard title="步骤">
          {summary?.steps?.length ? (
            <List
              dataSource={summary.steps}
              renderItem={(step) => (
                <List.Item>
                  <List.Item.Meta
                    title={
                      <Space wrap>
                        <Text strong>{step.name || `Step ${step.id}`}</Text>
                        <Tag color={statusColors[step.state] || (step.exit_code ? "error" : "default")}>{statusText(step.state) || step.state || "-"}</Tag>
                        {step.exit_code ? <Tag color="red">exit {step.exit_code}</Tag> : null}
                      </Space>
                    }
                    description={<Text type={step.error ? "danger" : "secondary"}>{step.error || pipelineStepTimeText(step)}</Text>}
                  />
                </List.Item>
              )}
            />
          ) : (
            <Text type="secondary">Woodpecker 没有返回步骤明细。</Text>
          )}
        </ProCard>

        <ProCard
          title="尾部日志"
          extra={summary?.woodpecker_url ? <Button href={summary.woodpecker_url} target="_blank" icon={<ExternalLink size={14} />}>Woodpecker</Button> : null}
        >
          {summary?.log_tail?.length ? (
            <pre className="pipeline-log-tail">{summary.log_tail.join("\n")}</pre>
          ) : (
            <Text type="secondary">暂无可展示日志。失败发生在容器启动前时，Woodpecker 可能不会生成步骤日志。</Text>
          )}
        </ProCard>
      </Space>
    </Drawer>
  );
}

function HomeResourceStrip({
  summary,
  loading,
  onOpenMonitoring
}: {
  summary: MonitoringSummary | null;
  loading: boolean;
  onOpenMonitoring: () => void;
}) {
  const alertLevel = highestMonitoringAlertLevel(summary?.alerts || []);
  const statusColor = monitoringAlertColor(alertLevel);
  return (
    <Card className="home-resource-card" loading={!summary && loading}>
      <div className="home-resource-head">
        <Space size={8}>
          <Server size={16} />
          <Text strong>资源状态</Text>
          {summary?.source && <Tag color={monitoringSourceColor(summary.source)}>{monitoringSourceText(summary.source)}</Tag>}
          {alertLevel !== "info" && <Tag color={statusColor}>{monitoringAlertText(alertLevel)}</Tag>}
        </Space>
        <Button size="small" type="link" onClick={onOpenMonitoring}>
          查看监控
        </Button>
      </div>
      <div className="home-resource-grid">
        {(summary?.hosts || []).map((host) => (
          <div className="home-resource-host" key={host.id}>
            <Space size={8} className="home-resource-host-title">
              <Text strong>{host.name}</Text>
              <Tag color={monitoringStatusColor(host.status)}>{monitoringHostStatusText(host.status)}</Tag>
            </Space>
            <div className="home-resource-metrics">
              <MetricPill label="CPU" value={host.cpu_percent || 0} />
              <MetricPill label="内存" value={host.memory_percent || 0} />
              <MetricPill label="磁盘" value={host.disk_percent || 0} />
            </div>
          </div>
        ))}
        {!summary?.hosts?.length && !loading && <Text type="secondary">监控数据暂不可用</Text>}
      </div>
    </Card>
  );
}

function MetricPill({ label, value }: { label: string; value: number }) {
  return (
    <span className="metric-pill">
      <span>{label}</span>
      <CompactMetric value={value} />
    </span>
  );
}

export function MonitoringView({
  state,
  summary,
  loading,
  nowMs,
  onRefresh,
  onRun
}: {
  state: StateResponse;
  summary: MonitoringSummary | null;
  loading: boolean;
  nowMs: number;
  onRefresh: () => void;
  onRun: (task: Task) => void;
}) {
  const cleanupTasks = new Map((state.tasks || []).map((task) => [task.id, task]));
  const links = summary?.links || state.links || {};
  const hosts = summary?.hosts || [];
  const alerts = summary?.alerts || [];
  return (
    <Space direction="vertical" size={16} className="side-stack">
      <Card
        title={
          <Space size={8}>
            <Server size={16} />
            <span>资源监控</span>
          </Space>
        }
        loading={!summary && loading}
        extra={
          <Space wrap>
            {summary?.source && <Tag color={monitoringSourceColor(summary.source)}>{monitoringSourceText(summary.source)}</Tag>}
            {summary?.checked_at && <Text type="secondary">{checkedAtText(summary.checked_at, nowMs)}</Text>}
            {links.beszel && <Button href={links.beszel} target="_blank" icon={<ExternalLink size={15} />}>Beszel</Button>}
            {links.grafana && <Button href={links.grafana} target="_blank" icon={<ExternalLink size={15} />}>Grafana</Button>}
            {links.woodpecker && <Button href={links.woodpecker} target="_blank" icon={<ExternalLink size={15} />}>流水线</Button>}
            <Button icon={<RefreshCw size={16} />} loading={loading} onClick={onRefresh}>
              刷新
            </Button>
          </Space>
        }
      >
        {summary?.degraded_reason && (
          <Alert className="monitoring-alert-inline" type="warning" showIcon message="监控已降级" description={summary.degraded_reason} />
        )}
        <MonitoringResourceOverview hosts={hosts} alerts={alerts} loading={loading} />
      </Card>

      <Card
        title="机器资源"
        extra={<Text type="secondary">{hosts.length ? `${hosts.length} 台被管机器` : "未配置被管机器"}</Text>}
      >
        <MonitoringSystemTable
          rows={hosts}
          cleanupTasks={cleanupTasks}
          currentUser={state.current_user}
          onRun={onRun}
        />
      </Card>

      <Card
        title="核心容器"
        extra={<Text type="secondary">{summary?.containers?.length || 0} 个检查项</Text>}
      >
        <MonitoringContainerTable rows={summary?.containers || []} />
      </Card>
    </Space>
  );
}

function MonitoringResourceOverview({
  hosts,
  alerts,
  loading
}: {
  hosts: MonitoringHost[];
  alerts: MonitoringAlert[];
  loading: boolean;
}) {
  const normalCount = hosts.filter((host) => monitoringStatusColor(host.status) === "success").length;
  const highestCPU = highestHostMetric(hosts, "cpu_percent");
  const highestMemory = highestHostMetric(hosts, "memory_percent");
  const highestDisk = highestHostMetric(hosts, "disk_percent");
  const criticalCount = alerts.filter((alert) => alert.level === "critical").length;
  const warningCount = alerts.filter((alert) => alert.level === "warning").length;
  if (!hosts.length && !loading) {
    return <Alert type="info" showIcon message="监控数据暂不可用" />;
  }
  const headline = criticalCount ? `${criticalCount} 个紧急异常` : warningCount ? `${warningCount} 个提醒` : "资源状态正常";
  const headlineTone = criticalCount ? "danger" : warningCount ? "warning" : "success";
  const topAlerts = alerts.filter((alert) => alert.level === "critical" || alert.level === "warning").slice(0, 3);
  return (
    <div className="monitor-overview-panel">
      <div className="monitor-overview-head">
        <div>
          <Tag color={monitoringAlertColor(headlineTone === "danger" ? "critical" : headlineTone === "warning" ? "warning" : "info")}>
            {headline}
          </Tag>
          <Text strong className="monitor-overview-title">{hosts.length ? `${normalCount}/${hosts.length} 台在线` : "等待监控数据"}</Text>
        </div>
        <Text type="secondary">只保留关键水位，细节在下方机器资源表查看。</Text>
      </div>
      <div className="monitor-pressure-grid">
        <MonitoringOverviewItem label="CPU 峰值" value={`${formatPercent(highestCPU.value)}%`} meta={highestCPU.host?.name || "-"} tone={metricTone(highestCPU.value)} />
        <MonitoringOverviewItem label="内存峰值" value={`${formatPercent(highestMemory.value)}%`} meta={highestMemory.host?.name || "-"} tone={metricTone(highestMemory.value)} />
        <MonitoringOverviewItem label="磁盘峰值" value={`${formatPercent(highestDisk.value)}%`} meta={highestDisk.host?.name || "-"} tone={metricTone(highestDisk.value)} />
      </div>
      {topAlerts.length ? (
        <div className="monitor-alert-strip">
          {topAlerts.map((alert, index) => (
            <div className="monitor-alert-chip" key={`${alert.host_id || alert.host_name || "alert"}-${alert.metric || index}`}>
              <Tag color={monitoringAlertColor(alert.level)}>{monitoringAlertText(alert.level)}</Tag>
              <Text type={alert.level === "critical" ? "danger" : "secondary"}>{alert.message}</Text>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function MonitoringOverviewItem({
  label,
  value,
  meta,
  tone
}: {
  label: string;
  value: string;
  meta?: string;
  tone: "normal" | "success" | "warning" | "danger";
}) {
  return (
    <div className={`monitor-overview-item monitor-overview-${tone}`}>
      <Text type="secondary" className="monitor-overview-label">{label}</Text>
      <Space size={8} align="baseline">
        <Text strong className="monitor-overview-value">{value}</Text>
        {meta && <Text type="secondary" className="monitor-overview-meta">{meta}</Text>}
      </Space>
    </div>
  );
}

function MonitoringSystemTable({
  rows,
  cleanupTasks,
  currentUser,
  onRun
}: {
  rows: MonitoringHost[];
  cleanupTasks: Map<string, Task>;
  currentUser: User;
  onRun: (task: Task) => void;
}) {
  const columns: ColumnsType<MonitoringHost> = [
    {
      title: "系统",
      width: 260,
      render: (_, row) => (
        <Space size={8}>
          <span className={`system-dot system-dot-${monitoringStatusColor(row.status)}`} />
          <Space direction="vertical" size={0}>
            <Space size={6} wrap>
              <Text strong>{row.name}</Text>
              <Tag color={monitoringStatusColor(row.status)}>{monitoringHostStatusText(row.status)}</Tag>
            </Space>
            <Text type="secondary">{monitoringRoleText(row.role)} · {row.message || "监控正常"}</Text>
          </Space>
        </Space>
      )
    },
    {
      title: "资源水位",
      width: 330,
      render: (_, row) => (
        <div className="monitor-resource-stack">
          <CompactMetric label="CPU" value={row.cpu_percent || 0} />
          <CompactMetric label="内存" value={row.memory_percent || 0} />
          <CompactMetric label="磁盘" value={row.disk_percent || 0} />
        </div>
      )
    },
    {
      title: "运行",
      width: 260,
      render: (_, row) => (
        <Space direction="vertical" size={2}>
          <Text>负载 {formatLoad(row.load_1)} / {formatLoad(row.load_5)} / {formatLoad(row.load_15)}</Text>
          <Text type="secondary">网络 {formatBytes(row.network_bytes_per_second || 0)}/s · {row.uptime || "-"}</Text>
        </Space>
      )
    },
    {
      title: "来源",
      width: 100,
      render: (_, row) => <Tag color={monitoringSourceColor(row.source)}>{monitoringSourceText(row.source)}</Tag>
    },
    {
      title: "操作",
      width: 120,
      fixed: "right",
      render: (_, row) => (
        <MonitoringHostActions
          cleanupTask={row.cleanup_task_id ? cleanupTasks.get(row.cleanup_task_id) : undefined}
          currentUser={currentUser}
          onRun={onRun}
        />
      )
    }
  ];
  return (
    <>
      <Table
        className="desktop-monitor-system-table"
        rowKey="id"
        size="middle"
        columns={columns}
        dataSource={rows}
        pagination={false}
        scroll={{ x: 1080 }}
      />
      <List
        className="mobile-monitor-system-list"
        dataSource={rows}
        renderItem={(row) => (
          <List.Item>
            <List.Item.Meta
              title={
                <Space wrap>
                  <Text strong>{row.name}</Text>
                  <Tag color={monitoringStatusColor(row.status)}>{monitoringHostStatusText(row.status)}</Tag>
                  <Tag color={monitoringSourceColor(row.source)}>{monitoringSourceText(row.source)}</Tag>
                </Space>
              }
              description={
                <Space direction="vertical" size={8} className="side-stack">
                  <Text type="secondary">{monitoringRoleText(row.role)} · {row.message || "监控正常"}</Text>
                  <CompactMetric label="CPU" value={row.cpu_percent || 0} />
                  <CompactMetric label="内存" value={row.memory_percent || 0} />
                  <CompactMetric label="磁盘" value={row.disk_percent || 0} />
                  <Text type="secondary">
                    负载 {formatLoad(row.load_1)} {formatLoad(row.load_5)} {formatLoad(row.load_15)} · 网络 {formatBytes(row.network_bytes_per_second || 0)}/s · {row.uptime || "-"}
                  </Text>
                  <MonitoringHostActions
                    cleanupTask={row.cleanup_task_id ? cleanupTasks.get(row.cleanup_task_id) : undefined}
                    currentUser={currentUser}
                    onRun={onRun}
                  />
                </Space>
              }
            />
          </List.Item>
        )}
      />
    </>
  );
}

function MonitoringHostActions({
  cleanupTask,
  currentUser,
  onRun
}: {
  cleanupTask?: Task;
  currentUser: User;
  onRun: (task: Task) => void;
}) {
  const canCleanup = cleanupTask ? canRunTask(currentUser, cleanupTask) : false;
  if (!cleanupTask) {
    return <Text type="secondary">-</Text>;
  }
  return (
    <Space wrap size={6} className="monitor-table-actions">
      <Tooltip title={canCleanup ? "" : "仅管理员可执行"}>
        <span>
          <Button size="small" danger disabled={!canCleanup} onClick={() => onRun(cleanupTask)}>
            清理磁盘
          </Button>
        </span>
      </Tooltip>
    </Space>
  );
}

function CompactMetric({ label, value }: { label?: string; value: number }) {
  return (
    <div className="compact-metric">
      {label && <Text type="secondary" className="compact-metric-label">{label}</Text>}
      <Text className="compact-metric-value">{formatPercent(value)}%</Text>
      <span className="compact-meter" aria-hidden="true">
        <span className={`compact-meter-fill compact-meter-${metricTone(value)}`} style={{ width: `${Math.min(100, Math.max(0, value))}%` }} />
      </span>
    </div>
  );
}

function MonitoringContainerTable({ rows }: { rows: MonitoringContainer[] }) {
  const columns: ColumnsType<MonitoringContainer> = [
    {
      title: "容器",
      width: 260,
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          <Text strong>{row.name}</Text>
          <Text type="secondary">{row.host_name}</Text>
        </Space>
      )
    },
    {
      title: "状态",
      width: 180,
      render: (_, row) => <Tag color={containerStatusColor(row.status)}>{row.status || "-"}</Tag>
    },
    {
      title: "CPU",
      width: 120,
      render: (_, row) => <Text>{formatPercent(row.cpu_percent || 0)}%</Text>
    },
    {
      title: "内存",
      width: 210,
      render: (_, row) => (
        <Space direction="vertical" size={0} className="side-stack">
          <Text>{row.memory_usage || "-"}</Text>
          {row.memory_percent ? <Progress percent={row.memory_percent} size="small" status={metricProgressStatus(row.memory_percent)} /> : null}
        </Space>
      )
    },
    {
      title: "说明",
      render: (_, row) => <Text type={row.message ? "danger" : "secondary"}>{row.message || "核心容器"}</Text>
    }
  ];
  return (
    <>
      <Table
        className="desktop-monitor-container-table"
        rowKey={(row) => `${row.host_id}-${row.name}`}
        size="small"
        columns={columns}
        dataSource={rows}
        pagination={{ pageSize: 12, showSizeChanger: true, pageSizeOptions: [12, 30], showTotal: (total) => `共 ${total} 个` }}
        scroll={{ x: 920 }}
      />
      <List
        className="mobile-monitor-container-list"
        dataSource={rows}
        renderItem={(row) => (
          <List.Item>
            <List.Item.Meta
              title={
                <Space wrap>
                  <Text strong>{row.name}</Text>
                  <Tag color={containerStatusColor(row.status)}>{row.status || "-"}</Tag>
                </Space>
              }
              description={
                <Space direction="vertical" size={4} className="side-stack">
                  <Text type="secondary">{row.host_name}</Text>
                  <Text>CPU {formatPercent(row.cpu_percent || 0)}% · 内存 {row.memory_usage || "-"}</Text>
                  {row.memory_percent ? <Progress percent={row.memory_percent} size="small" status={metricProgressStatus(row.memory_percent)} /> : null}
                  {row.message && <Text type="danger">{row.message}</Text>}
                </Space>
              }
            />
          </List.Item>
        )}
      />
    </>
  );
}

export function InfrastructureLinks({ tasks }: { tasks: Task[] }) {
  const links = tasks.filter((item) => item.external_url);
  const columns: ColumnsType<Task> = [
    {
      title: "服务",
      dataIndex: "title",
      width: 210,
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          <Text strong>{row.title.replace(/^打开\s*/, "")}</Text>
          <Tag color="blue">入口</Tag>
        </Space>
      )
    },
    {
      title: "用途",
      dataIndex: "description",
      render: (value) => <Text type="secondary">{value}</Text>
    },
    {
      title: "地址",
      dataIndex: "external_url",
      render: (value) => <Text copyable>{value}</Text>
    },
    {
      title: "",
      width: 110,
      render: (_, row) => (
        <Button href={row.external_url} target="_blank" icon={<ExternalLink size={15} />}>
          打开
        </Button>
      )
    }
  ];

  return (
    <Space direction="vertical" size={16} className="side-stack">
      <Card className="page-head-card">
        <Space direction="vertical" size={4}>
          <Title level={4}>基础设施入口</Title>
          <Text type="secondary">Woodpecker、监控、日志和外部控制台集中在这里。</Text>
        </Space>
      </Card>
      <Card>
        <Table
          className="desktop-infra-table"
          rowKey="id"
          size="middle"
          columns={columns}
          dataSource={links}
          pagination={false}
          scroll={{ x: 820 }}
        />
        <List
          className="mobile-infra-list"
          dataSource={links}
          renderItem={(row) => (
            <List.Item
              actions={[
                <Button key="open" href={row.external_url} target="_blank" icon={<ExternalLink size={14} />}>
                  打开
                </Button>
              ]}
            >
              <List.Item.Meta
                title={<Text strong>{row.title.replace(/^打开\s*/, "")}</Text>}
                description={
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">{row.description}</Text>
                    <Text copyable>{row.external_url}</Text>
                  </Space>
                }
              />
            </List.Item>
          )}
        />
      </Card>
    </Space>
  );
}

function AuditLogView({
  records,
  loading,
  state,
  onRefresh
}: {
  records: AuditRecord[];
  loading: boolean;
  state: StateResponse;
  onRefresh: () => void;
}) {
  const columns: ColumnsType<AuditRecord> = [
    {
      title: "时间",
      width: 150,
      render: (_, row) => <Text type="secondary">{formatShortTime(row.time) || "-"}</Text>
    },
    {
      title: "操作者",
      width: 140,
      render: (_, row) => <Text>{row.username || "-"}</Text>
    },
    {
      title: "动作",
      width: 220,
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          <Text strong>{row.task_title || row.task_id}</Text>
          <Text type="secondary">{repoNameByID(state, row.repo_id)} · {row.branch || "-"}</Text>
        </Space>
      )
    },
    {
      title: "变量",
      render: (_, row) => (
        <Space wrap size={[4, 4]}>
          {Object.entries(row.variables || {}).length ? Object.entries(row.variables || {}).map(([key, value]) => (
            <Tag key={key}>{key}={value || "-"}</Tag>
          )) : <Text type="secondary">-</Text>}
        </Space>
      )
    },
    {
      title: "结果",
      width: 120,
      render: (_, row) => <Tag color={row.status === "ok" ? "success" : "error"}>{row.status === "ok" ? "成功" : "失败"}</Tag>
    },
    {
      title: "说明",
      width: 210,
      render: (_, row) => <Text type={row.error ? "danger" : "secondary"}>{row.error || (row.pipeline ? `流水线 #${row.pipeline}` : "-")}</Text>
    },
    {
      title: "",
      width: 76,
      render: (_, row) => row.pipeline ? (
        <Button size="small" href={auditPipelineURL(state.links.woodpecker, row)} target="_blank" icon={<ExternalLink size={14} />} />
      ) : null
    }
  ];
  return (
    <Card title="操作历史" extra={<Button icon={<RefreshCw size={16} />} loading={loading} onClick={onRefresh}>刷新</Button>}>
      <Table
        rowKey={(row) => `${row.time}-${row.task_id}-${row.pipeline}-${row.status}`}
        size="small"
        loading={loading}
        columns={columns}
        dataSource={records}
        pagination={{ pageSize: 14, showSizeChanger: true, pageSizeOptions: [14, 30, 60], showTotal: (total) => `共 ${total} 条` }}
        scroll={{ x: 1050 }}
      />
    </Card>
  );
}

function Profile({ state, onReload }: { state: StateResponse; onReload: () => Promise<void> }) {
  const { message } = AntApp.useApp();
  const [profileForm] = Form.useForm();
  const [passwordForm] = Form.useForm();

  useEffect(() => {
    profileForm.setFieldsValue(state.current_user);
  }, [state.current_user.id]);

  async function saveProfile(values: Record<string, string>) {
    try {
      await api("/api/me", { method: "PATCH", body: JSON.stringify(values) });
      message.success("资料已保存");
      await onReload();
    } catch (error) {
      message.error(errorText(error) || "保存失败");
    }
  }

  async function changePassword(values: Record<string, string>) {
    try {
      await api("/api/me/password", { method: "POST", body: JSON.stringify(values) });
      passwordForm.resetFields();
      message.success("密码已修改");
    } catch (error) {
      message.error(errorText(error) || "修改失败");
    }
  }

  if (state.auth_mode !== "db") {
    return <Alert type="info" showIcon message="当前是共享密码模式；配置数据库后可使用账号、邮箱和成员管理。" />;
  }

  return (
    <Row gutter={[16, 16]}>
      <Col xs={24} lg={12}>
        <Card title="账号资料">
          <Form form={profileForm} layout="vertical" onFinish={saveProfile}>
            <Form.Item label="账号名" name="username" rules={[{ required: true, message: "请输入账号名" }]}>
              <Input />
            </Form.Item>
            <Form.Item label="姓名/昵称" name="display_name">
              <Input />
            </Form.Item>
            <Form.Item label="邮箱" name="email">
              <Input type="email" />
            </Form.Item>
            <Button type="primary" htmlType="submit">
              保存资料
            </Button>
          </Form>
        </Card>
      </Col>
      <Col xs={24} lg={12}>
        <Card title="修改密码">
          <Form form={passwordForm} layout="vertical" onFinish={changePassword}>
            <Form.Item label="旧密码" name="old_password" rules={[{ required: true }]}>
              <Input.Password />
            </Form.Item>
            <Form.Item label="新密码" name="new_password" rules={[{ required: true, min: 8 }]}>
              <Input.Password />
            </Form.Item>
            <Button htmlType="submit">修改密码</Button>
          </Form>
        </Card>
      </Col>
    </Row>
  );
}

function Users() {
  const { message } = AntApp.useApp();
  const [users, setUsers] = useState<User[]>([]);
  const [loadingUsers, setLoadingUsers] = useState(false);
  const [form] = Form.useForm();

  async function loadUsers(options: { notify?: boolean } = {}) {
    setLoadingUsers(true);
    if (options.notify) {
      message.open({ type: "loading", content: "正在刷新成员", key: "users-refresh", duration: 0 });
    }
    try {
      const data = await api<{ users: User[] }>("/api/users");
      setUsers(data.users || []);
      if (options.notify) {
        message.open({ type: "success", content: "成员已刷新", key: "users-refresh", duration: 1.8 });
      }
    } catch (error) {
      if (options.notify) {
        message.open({ type: "error", content: errorText(error) || "刷新失败", key: "users-refresh", duration: 4 });
      } else {
        message.error(errorText(error) || "成员加载失败");
      }
    } finally {
      setLoadingUsers(false);
    }
  }

  useEffect(() => {
    loadUsers();
  }, []);

  async function createUser(values: Record<string, string>) {
    try {
      await api("/api/users", { method: "POST", body: JSON.stringify(values) });
      form.resetFields();
      message.success("成员已创建");
      await loadUsers();
    } catch (error) {
      message.error(errorText(error) || "创建失败");
    }
  }

  return (
    <Card
      title="成员账号"
      extra={
        <Button icon={<RefreshCw size={16} />} loading={loadingUsers} onClick={() => loadUsers({ notify: true })}>
          刷新
        </Button>
      }
    >
      <Form form={form} layout="inline" onFinish={createUser} className="inline-create-form">
        <Form.Item name="username" rules={[{ required: true }]}>
          <Input placeholder="账号" />
        </Form.Item>
        <Form.Item name="display_name">
          <Input placeholder="姓名/昵称" />
        </Form.Item>
        <Form.Item name="email">
          <Input placeholder="邮箱" />
        </Form.Item>
        <Form.Item name="password" rules={[{ required: true, min: 8 }]}>
          <Input.Password placeholder="初始密码" />
        </Form.Item>
        <Form.Item name="role" initialValue="operator">
          <Select style={{ width: 110 }} options={[{ value: "operator", label: "成员" }, { value: "admin", label: "管理员" }]} />
        </Form.Item>
        <Button type="primary" htmlType="submit">
          创建
        </Button>
      </Form>
      <Table
        className="users-table"
        rowKey="id"
        dataSource={users}
        pagination={false}
        columns={[
          { title: "账号", dataIndex: "username" },
          { title: "姓名", dataIndex: "display_name" },
          { title: "邮箱", dataIndex: "email" },
          { title: "角色", dataIndex: "role", render: (value) => (value === "admin" ? "管理员" : "成员") },
          { title: "状态", dataIndex: "active", render: (value) => <Tag color={value ? "green" : "red"}>{value ? "启用" : "停用"}</Tag> }
        ]}
      />
    </Card>
  );
}

function RepositoryConfigPanel({ state, onReload }: { state: StateResponse; onReload: () => Promise<void> }) {
  const { message } = AntApp.useApp();
  const [repos, setRepos] = useState<WoodpeckerRepo[]>([]);
  const [configured, setConfigured] = useState<Record<string, string>>(state.repos || {});
  const [errors, setErrors] = useState<string[]>([]);
  const [loadingRepos, setLoadingRepos] = useState(false);
  const [lookupLoading, setLookupLoading] = useState(false);
  const [savingID, setSavingID] = useState<number | null>(null);
  const [lookupResult, setLookupResult] = useState<WoodpeckerRepo | null>(null);
  const [lookupForm] = Form.useForm();

  async function loadRepos(options: { notify?: boolean } = {}) {
    setLoadingRepos(true);
    try {
      const data = await api<WoodpeckerReposResponse>("/api/woodpecker/repos");
      setRepos(data.repos || []);
      setConfigured(data.configured || {});
      setErrors(data.errors || []);
      if (options.notify) message.success("仓库已刷新");
    } catch (error) {
      message.error(errorText(error) || "仓库加载失败");
    } finally {
      setLoadingRepos(false);
    }
  }

  useEffect(() => {
    lookupForm.setFieldsValue({ owner: "tangfire", name: "zephyr" });
    loadRepos();
  }, []);

  async function lookup(values: { owner: string; name: string }) {
    setLookupLoading(true);
    setLookupResult(null);
    try {
      const data = await api<{ repo: WoodpeckerRepo }>("/api/woodpecker/repos/lookup", {
        method: "POST",
        body: JSON.stringify(values)
      });
      setLookupResult(data.repo);
      message.success(`已找到 ${data.repo.full_name || values.owner + "/" + values.name}`);
    } catch (error) {
      message.error(errorText(error) || "Woodpecker 当前授权看不到这个仓库");
    } finally {
      setLookupLoading(false);
    }
  }

  async function saveRepo(repo: WoodpeckerRepo) {
    const repoID = repo.id;
    const repoName = repo.full_name || [repo.owner, repo.name].filter(Boolean).join("/") || `Repo ${repo.id}`;
    if (!repoID || !repoName) {
      message.error("仓库缺少 Repo ID 或名称");
      return;
    }
    setSavingID(repoID);
    try {
      await api("/api/woodpecker/repos/save", {
        method: "POST",
        body: JSON.stringify({ repo_id: repoID, repo_name: repoName })
      });
      message.success(`已保存 ${repoName}`);
      await loadRepos();
      await onReload();
    } catch (error) {
      message.error(errorText(error) || "保存仓库映射失败");
    } finally {
      setSavingID(null);
    }
  }

  async function activateAndSave(repo: WoodpeckerRepo) {
    if (!repo.forge_remote_id) {
      message.error("这个仓库缺少 forge_remote_id，无法启用");
      return;
    }
    setLookupLoading(true);
    try {
      const data = await api<{ repo: WoodpeckerRepo }>("/api/woodpecker/repos/activate", {
        method: "POST",
        body: JSON.stringify({ forge_remote_id: repo.forge_remote_id })
      });
      setLookupResult(data.repo);
      message.success(`Woodpecker 已启用 ${data.repo.full_name || repo.full_name}`);
      await saveRepo(data.repo);
    } catch (error) {
      message.error(errorText(error) || "启用仓库失败");
    } finally {
      setLookupLoading(false);
    }
  }

  const columns: ColumnsType<WoodpeckerRepo> = [
    {
      title: "仓库",
      render: (_, row) => (
        <Space direction="vertical" size={0}>
          <Text strong>{row.full_name || `${row.owner || ""}/${row.name || ""}`}</Text>
          <Text type="secondary">Repo ID {row.id} · {row.default_branch || "main"}</Text>
        </Space>
      )
    },
    {
      title: "状态",
      width: 160,
      render: (_, row) => (
        <Space wrap size={[4, 4]}>
          <Tag color={row.active ? "green" : "gold"}>{row.active ? "已启用" : "未启用"}</Tag>
          <Tag>{row.private ? "私有" : "公开"}</Tag>
          {configured[String(row.id)] && <Tag color="blue">Zephyr 已保存</Tag>}
        </Space>
      )
    },
    {
      title: "地址",
      render: (_, row) => row.forge_url ? <Text copyable>{row.forge_url}</Text> : <Text type="secondary">-</Text>
    },
    {
      title: "",
      width: 190,
      render: (_, row) => (
        <Space>
          <Button size="small" loading={savingID === row.id} onClick={() => saveRepo(row)}>
            保存到 Zephyr
          </Button>
          {row.forge_url && <Button size="small" href={row.forge_url} target="_blank" icon={<ExternalLink size={14} />} />}
        </Space>
      )
    }
  ];

  const zephyrEnabled = repos.some((repo) => (repo.full_name || "").toLowerCase() === "tangfire/zephyr");

  return (
    <Space direction="vertical" size={16} className="side-stack">
      {!zephyrEnabled && (
        <Alert
          type="warning"
          showIcon
          message="Woodpecker 里还没有启用 Zephyr 仓库"
          description="如果 lookup tangfire/zephyr 仍然 404，通常是 GitHub OAuth 没授权到这个仓库，先去 Woodpecker 的添加仓库或 GitHub OAuth 权限里处理。"
          action={state.links.woodpecker ? <Button size="small" href={state.links.woodpecker} target="_blank">打开 Woodpecker</Button> : undefined}
        />
      )}
      <Card title="查找并启用仓库">
        <Form form={lookupForm} layout="inline" onFinish={lookup} className="inline-create-form">
          <Form.Item label="Owner" name="owner" rules={[{ required: true, message: "请输入 owner" }]}>
            <Input placeholder="tangfire" />
          </Form.Item>
          <Form.Item label="仓库名" name="name" rules={[{ required: true, message: "请输入仓库名" }]}>
            <Input placeholder="zephyr" />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={lookupLoading}>
            查询
          </Button>
        </Form>
        {lookupResult && (
          <Card size="small" className="repo-lookup-card">
            <Space direction="vertical" size={10} className="side-stack">
              <Space wrap>
                <Text strong>{lookupResult.full_name || `${lookupResult.owner}/${lookupResult.name}`}</Text>
                <Tag>{lookupResult.private ? "私有" : "公开"}</Tag>
                <Tag>{lookupResult.default_branch || "main"}</Tag>
                {lookupResult.id ? <Tag color="green">已启用 Repo ID {lookupResult.id}</Tag> : <Tag color="gold">待启用</Tag>}
              </Space>
              <Text type="secondary">forge_remote_id：{lookupResult.forge_remote_id || "-"}</Text>
              <Space wrap>
                {lookupResult.id ? (
                  <Button type="primary" loading={savingID === lookupResult.id} onClick={() => saveRepo(lookupResult)}>
                    保存到 Zephyr
                  </Button>
                ) : (
                  <Button type="primary" loading={lookupLoading} onClick={() => activateAndSave(lookupResult)}>
                    启用并保存
                  </Button>
                )}
                {lookupResult.forge_url && <Button href={lookupResult.forge_url} target="_blank" icon={<ExternalLink size={14} />}>打开 GitHub</Button>}
              </Space>
            </Space>
          </Card>
        )}
      </Card>
      {errors.length > 0 && (
        <Alert type="warning" showIcon message="Woodpecker 仓库同步有问题" description={errors.join("；")} />
      )}
      <Card title="Woodpecker 已启用仓库" extra={<Button icon={<RefreshCw size={16} />} loading={loadingRepos} onClick={() => loadRepos({ notify: true })}>刷新</Button>}>
        <Table
          rowKey="id"
          size="small"
          loading={loadingRepos}
          columns={columns}
          dataSource={repos}
          pagination={false}
          scroll={{ x: 820 }}
        />
      </Card>
    </Space>
  );
}

function SetupConfigPanel({ onReload }: { onReload: () => Promise<void> }) {
  const { message } = AntApp.useApp();
  const [form] = Form.useForm<RuntimeConfigInput>();
  const [setup, setSetup] = useState<SetupConfigResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  async function loadSetup(options: { notify?: boolean } = {}) {
    setLoading(true);
    try {
      const data = await api<SetupConfigResponse>("/api/setup/config");
      setSetup(data);
      form.setFieldsValue(normalizeSetupFormValues(data.config));
      if (options.notify) message.success("接入配置已刷新");
    } catch (error) {
      message.error(errorText(error) || "接入配置加载失败");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadSetup();
  }, []);

  async function save(values: RuntimeConfigInput) {
    setSaving(true);
    try {
      const payload = normalizeSetupFormValues(values);
      const data = await api<SetupConfigResponse>("/api/setup/config", {
        method: "POST",
        body: JSON.stringify(payload)
      });
      setSetup(data);
      form.setFieldsValue(normalizeSetupFormValues(data.config));
      message.success("接入配置已保存");
      await onReload();
    } catch (error) {
      message.error(errorText(error) || "保存失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Space direction="vertical" size={16} className="side-stack">
      <Alert
        type="info"
        showIcon
        message="这里维护 Zephyr 的可视化接入配置"
        description="URL、监控主机、外部入口会回显；Woodpecker token 和 Beszel 密码只显示是否已配置，留空保存会保留原值。"
      />
      <ProCard
        title="接入状态"
        extra={<Button icon={<RefreshCw size={16} />} loading={loading} onClick={() => loadSetup({ notify: true })}>刷新</Button>}
      >
        <Row gutter={[12, 12]}>
          {(setup?.status || []).map((item) => (
            <Col xs={24} md={12} xl={8} key={item.id}>
              <Card size="small" className="setup-status-card">
                <Space direction="vertical" size={8} className="side-stack">
                  <Space align="center" className="setup-status-head">
                    <Tag color={setupStatusColor(item.status)}>{setupStatusText(item.status)}</Tag>
                    <Text strong>{item.title}</Text>
                  </Space>
                  <Text type="secondary">{item.message || "-"}</Text>
                  {item.action_url && (
                    <Button size="small" href={item.action_url} target="_blank" icon={<ExternalLink size={14} />}>
                      {item.action_label || "打开"}
                    </Button>
                  )}
                </Space>
              </Card>
            </Col>
          ))}
        </Row>
      </ProCard>

      <Form form={form} layout="vertical" onFinish={save} disabled={loading}>
        <ProCard title="核心服务" className="setup-form-card">
          <Row gutter={12}>
            <Col xs={24} lg={8}>
              <Form.Item label="Zephyr 公开地址" name="public_url" rules={[{ required: true, message: "请输入 Zephyr 公开地址" }]}>
                <Input placeholder="https://deploy.example.com" />
              </Form.Item>
            </Col>
            <Col xs={24} lg={8}>
              <Form.Item label="Woodpecker 内部地址" name="woodpecker_server" rules={[{ required: true, message: "请输入 Woodpecker 内部地址" }]}>
                <Input placeholder="http://woodpecker-server:8000" />
              </Form.Item>
            </Col>
            <Col xs={24} lg={8}>
              <Form.Item label="Woodpecker 公开地址" name="woodpecker_public_url" rules={[{ required: true, message: "请输入 Woodpecker 公开地址" }]}>
                <Input placeholder="https://ci.example.com" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={12}>
            <Col xs={24} lg={8}>
              <Form.Item
                label="Woodpecker API token"
                name="woodpecker_token"
                extra={setup?.secrets?.woodpecker_token ? "已配置；留空保存会保留原 token。" : "未配置；保存后 Zephyr 才能触发流水线。"}
              >
                <Input.Password placeholder={setup?.secrets?.woodpecker_token ? "留空保留原 token" : "填入 Woodpecker token"} autoComplete="new-password" />
              </Form.Item>
            </Col>
            <Col xs={24} lg={8}>
              <Form.Item label="Beszel 内部地址" name="beszel_base_url">
                <Input placeholder="http://beszel:8090" />
              </Form.Item>
            </Col>
            <Col xs={24} lg={8}>
              <Form.Item label="Beszel 公开地址" name="beszel_public_url">
                <Input placeholder="https://beszel.example.com" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={12}>
            <Col xs={24} lg={8}>
              <Form.Item label="Beszel API 邮箱" name="beszel_email">
                <Input placeholder="ops@example.com" />
              </Form.Item>
            </Col>
            <Col xs={24} lg={8}>
              <Form.Item
                label="Beszel API 密码"
                name="beszel_password"
                extra={setup?.secrets?.beszel_password ? "已配置；留空保存会保留原密码。" : "未配置；配置后可优先使用 Beszel 数据。"}
              >
                <Input.Password placeholder={setup?.secrets?.beszel_password ? "留空保留原密码" : "填入 Beszel 密码"} autoComplete="new-password" />
              </Form.Item>
            </Col>
            <Col xs={24} lg={8}>
              <Form.Item label="Grafana 公开地址" name="grafana_public_url">
                <Input placeholder="https://grafana.example.com" />
              </Form.Item>
            </Col>
          </Row>
        </ProCard>

        <ProCard title="监控阈值" className="setup-form-card">
          <Row gutter={12}>
            <Col xs={12} lg={6}>
              <Form.Item label="刷新秒数" name="monitor_refresh_seconds">
                <InputNumber min={5} max={300} style={{ width: "100%" }} />
              </Form.Item>
            </Col>
            <Col xs={12} lg={6}>
              <Form.Item label="磁盘提醒 %" name="monitor_warn_disk">
                <InputNumber min={1} max={100} style={{ width: "100%" }} />
              </Form.Item>
            </Col>
            <Col xs={12} lg={6}>
              <Form.Item label="磁盘严重 %" name="monitor_crit_disk">
                <InputNumber min={1} max={100} style={{ width: "100%" }} />
              </Form.Item>
            </Col>
            <Col xs={12} lg={6}>
              <Form.Item label="内存提醒 %" name="monitor_warn_memory">
                <InputNumber min={1} max={100} style={{ width: "100%" }} />
              </Form.Item>
            </Col>
          </Row>
        </ProCard>

        <ProCard
          title="被管机器"
          className="setup-form-card"
        >
          <Form.List name="monitor_hosts">
            {(fields, { add, remove }) => (
              <Space direction="vertical" size={12} className="side-stack">
                <div>
                  <Button icon={<Plus size={16} />} onClick={() => add({ id: "", name: "", role: "production", ssh_user: "codex", beszel_names: [], containers: [] })}>
                    新增机器
                  </Button>
                </div>
                {fields.map((field, index) => (
                  <Card
                    size="small"
                    key={field.key}
                    title={`机器 ${index + 1}`}
                    extra={<Button size="small" danger icon={<Trash2 size={14} />} onClick={() => remove(field.name)} />}
                  >
                    <Row gutter={12}>
                      <Col xs={24} lg={6}>
                        <Form.Item label="ID" name={[field.name, "id"]} rules={[{ required: true, message: "请输入 ID" }]}>
                          <Input placeholder="prod" />
                        </Form.Item>
                      </Col>
                      <Col xs={24} lg={6}>
                        <Form.Item label="名称" name={[field.name, "name"]} rules={[{ required: true, message: "请输入名称" }]}>
                          <Input placeholder="生产机" />
                        </Form.Item>
                      </Col>
                      <Col xs={24} lg={6}>
                        <Form.Item label="角色" name={[field.name, "role"]}>
                          <Input placeholder="production / builder" />
                        </Form.Item>
                      </Col>
                      <Col xs={24} lg={6}>
                        <Form.Item label="清理任务 ID" name={[field.name, "cleanup_task_id"]}>
                          <Input placeholder="production-cleanup" />
                        </Form.Item>
                      </Col>
                    </Row>
                    <Row gutter={12}>
                      <Col xs={24} lg={8}>
                        <Form.Item label="SSH 地址" name={[field.name, "ssh_host"]}>
                          <Input placeholder="1.2.3.4:22" />
                        </Form.Item>
                      </Col>
                      <Col xs={24} lg={4}>
                        <Form.Item label="SSH 用户" name={[field.name, "ssh_user"]}>
                          <Input placeholder="codex" />
                        </Form.Item>
                      </Col>
                      <Col xs={24} lg={6}>
                        <Form.Item label="Beszel 匹配名" name={[field.name, "beszel_names"]}>
                          <Select mode="tags" tokenSeparators={[","]} placeholder="prod, production" />
                        </Form.Item>
                      </Col>
                      <Col xs={24} lg={6}>
                        <Form.Item label="核心容器" name={[field.name, "containers"]}>
                          <Select mode="tags" tokenSeparators={[","]} placeholder="api, mysql, redis" />
                        </Form.Item>
                      </Col>
                    </Row>
                  </Card>
                ))}
                {!fields.length && <Alert type="warning" showIcon message="还没有配置被管机器" description="至少保留本机或一台生产机，这样监控页才有核心资源数据。" />}
              </Space>
            )}
          </Form.List>
        </ProCard>

        <ProCard
          title="外部入口"
          className="setup-form-card"
        >
          <Form.List name="external_links">
            {(fields, { add, remove }) => (
              <Space direction="vertical" size={10} className="side-stack">
                <div>
                  <Button icon={<Plus size={16} />} onClick={() => add({ id: "", title: "", url: "", group: "基础设施" })}>
                    新增入口
                  </Button>
                </div>
                {fields.map((field) => (
                  <Card size="small" key={field.key}>
                    <Row gutter={12}>
                      <Col xs={24} md={4}>
                        <Form.Item label="ID" name={[field.name, "id"]}>
                          <Input placeholder="grafana" />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={5}>
                        <Form.Item label="标题" name={[field.name, "title"]}>
                          <Input placeholder="Grafana" />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={5}>
                        <Form.Item label="分组" name={[field.name, "group"]}>
                          <Input placeholder="观测" />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={8}>
                        <Form.Item label="地址" name={[field.name, "url"]}>
                          <Input placeholder="https://grafana.example.com" />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={2}>
                        <Form.Item label=" ">
                          <Button danger icon={<Trash2 size={14} />} onClick={() => remove(field.name)} />
                        </Form.Item>
                      </Col>
                    </Row>
                    <Form.Item label="说明" name={[field.name, "description"]}>
                      <Input placeholder="查看日志、指标、链路和仪表盘" />
                    </Form.Item>
                  </Card>
                ))}
                {!fields.length && <Text type="secondary">暂无额外入口；Zephyr、Woodpecker、Beszel、Grafana 会自动显示。</Text>}
              </Space>
            )}
          </Form.List>
        </ProCard>

        <Button type="primary" htmlType="submit" loading={saving} size="large">
          保存接入配置
        </Button>
      </Form>

      <ProCard title="业务机接入命令">
        <Row gutter={[12, 12]}>
          {(setup?.commands || []).map((item) => (
            <Col xs={24} lg={12} key={item.id}>
              <Card size="small" title={item.title}>
                <Space direction="vertical" size={8} className="side-stack">
                  <Text type="secondary">{item.description}</Text>
                  <Typography.Paragraph copyable className="command-block">
                    {item.command}
                  </Typography.Paragraph>
                </Space>
              </Card>
            </Col>
          ))}
        </Row>
      </ProCard>

      <ProCard title="文档">
        <List
          dataSource={setup?.docs || []}
          renderItem={(item) => (
            <List.Item>
              <List.Item.Meta title={item.title} description={`${item.description} · ${item.path}`} />
            </List.Item>
          )}
        />
      </ProCard>
    </Space>
  );
}

function TaskConfigView({
  tasks,
  config,
  onAdd,
  onEdit,
  onDelete
}: {
  tasks: Task[];
  config: TaskConfig | null;
  onAdd: () => void;
  onEdit: (task: Task) => void;
  onDelete: (task: Task) => void;
}) {
  return (
    <Card
      title="持久化任务配置"
      extra={
        <Button type="primary" icon={<Plus size={16} />} onClick={onAdd}>
          新增任务
        </Button>
      }
    >
      <Descriptions column={1} bordered size="small">
        <Descriptions.Item label="配置文件">/data/tasks.json</Descriptions.Item>
        <Descriptions.Item label="已保存覆盖/自定义">{config?.tasks?.length || 0}</Descriptions.Item>
        <Descriptions.Item label="用途">这里维护默认任务配置；临时换分支可以在点“执行”时选择，不会改默认配置。</Descriptions.Item>
      </Descriptions>
      <Table
        className="config-task-table"
        rowKey="id"
        size="small"
        pagination={false}
        dataSource={tasks.filter((task) => !task.external_url)}
        columns={[
          {
            title: "任务",
            render: (_, row) => (
              <Space direction="vertical" size={0}>
                <Text strong>{row.title}</Text>
                <Space size={4}>
                  {row.builtin && <Tag>内置</Tag>}
                  {row.custom && <Tag color="blue">自定义</Tag>}
                  {row.overridden && <Tag color="gold">已覆盖</Tag>}
                </Space>
              </Space>
            )
          },
          { title: "模块", dataIndex: "group" },
          { title: "执行仓库", render: (_, row) => row.repo_name || `Repo ${row.repo_id}` },
          { title: "分支", dataIndex: "branch" },
          {
            title: "变量",
            render: (_, row) => (
              <Space wrap size={[4, 4]}>
                {Object.entries(row.variables || {}).map(([key, value]) => (
                  <Tag key={key}>{key}={value || "-"}</Tag>
                ))}
              </Space>
            )
          },
          {
            title: "",
            width: 150,
            render: (_, row) => (
              <Space>
                <Button size="small" icon={<Settings size={14} />} onClick={() => onEdit(row)} />
                {(row.custom || row.overridden) && (
                  <Popconfirm title={row.overridden ? "恢复这个内置任务的默认配置？" : "删除这个自定义任务？"} onConfirm={() => onDelete(row)}>
                    <Button size="small" icon={<Trash2 size={14} />} />
                  </Popconfirm>
                )}
              </Space>
            )
          }
        ]}
        scroll={{ x: 760 }}
      />
    </Card>
  );
}

export function Docs({ state }: { state: StateResponse }) {
  const data = [
    ["部署任务", "Woodpecker Repo ID / 默认分支", "DEPLOY_ACTION=deploy，建议设置 ZEPHYR_PROJECT_ID"],
    ["回退任务", "同一 Repo / 同一项目 ID", "DEPLOY_ACTION=rollback，确认词建议 ROLLBACK"],
    ["清理任务", "按项目自定义", "DEPLOY_ACTION=cleanup 或 disk-cleanup，确认词建议 CLEAN"],
    ["基础设施入口", "ZEPHYR_LINKS_JSON", "配置额外外部系统入口"],
    ["监控主机", "ZEPHYR_MONITOR_HOSTS_JSON", "配置机器、容器、Beszel 名称和 SSH 兜底"]
  ];
  return (
    <Space direction="vertical" size={16} className="side-stack">
      <Card title="部署参数手册" extra={<Button href={state.links.woodpecker} target="_blank" icon={<ExternalLink size={16} />}>打开 Woodpecker</Button>}>
        <Table
          rowKey={(row) => row[0]}
          pagination={false}
          columns={[
            { title: "模块", dataIndex: 0 },
            { title: "执行仓库/分支", dataIndex: 1 },
            { title: "关键变量", dataIndex: 2 }
          ]}
          dataSource={data}
        />
      </Card>
      <Alert
        type="info"
        showIcon
        icon={<FileText size={18} />}
        message="磁盘清理策略"
        description="建议把清理动作放进独立 Woodpecker 任务，并在脚本里保护正在运行的 CI 容器、Zephyr 镜像和业务关键卷。"
      />
    </Space>
  );
}

export function flattenPipelines(state: StateResponse | null): Pipeline[] {
  if (!state) return [];
  const rows: Pipeline[] = [];
  for (const [repoID, pipelines] of Object.entries(state.pipelines || {})) {
    for (const row of pipelines || []) {
      rows.push({ ...row, repo_id: Number(repoID), repo_name: state.repos[repoID] || `Repo ${repoID}` });
    }
  }
  return rows.sort((a, b) => pipelineSortTime(b) - pipelineSortTime(a));
}

export function recentFailedPipelineCount(rows: Pipeline[], nowMs: number): number {
  return rows.filter((row) => isRecentFailedPipeline(row, nowMs)).length;
}

function overviewPipelineRows(rows: Pipeline[], nowMs: number, limit: number): Pipeline[] {
  return rows
    .filter((row) => ["running", "pending"].includes(row.status) || isRecentFailedPipeline(row, nowMs))
    .slice(0, limit);
}

function isRecentFailedPipeline(row: Pipeline, nowMs: number): boolean {
  if (!["failure", "error"].includes(row.status)) return false;
  const at = pipelineSortTime(row);
  if (!at) return false;
  const now = Math.floor(nowMs / 1000);
  return now - at <= OVERVIEW_PIPELINE_LOOKBACK_SECONDS;
}

export function pipelineURL(base: string, row: Pipeline): string {
  return `${base.replace(/\/+$/, "")}/repos/${row.repo_id}/pipeline/${row.number}`;
}

function deploymentPipelineURL(base: string, row: DeploymentStatus): string {
  return `${base.replace(/\/+$/, "")}/repos/${row.repo_id}/pipeline/${row.pipeline}`;
}

function auditPipelineURL(base: string, row: AuditRecord): string {
  return `${base.replace(/\/+$/, "")}/repos/${row.repo_id}/pipeline/${row.pipeline}`;
}

function normalizeSetupFormValues(values: RuntimeConfigInput): RuntimeConfigInput {
  const externalLinks = (values.external_links || []).map((item) => ({
    id: String(item.id || "").trim(),
    title: String(item.title || "").trim(),
    url: String(item.url || "").trim(),
    group: String(item.group || "").trim(),
    description: String(item.description || "").trim()
  })).filter((item) => item.url || item.title || item.id);
  const monitorHosts = (values.monitor_hosts || []).map((item) => ({
    id: String(item.id || "").trim(),
    name: String(item.name || "").trim(),
    role: String(item.role || "").trim(),
    ssh_host: String(item.ssh_host || item.address || "").trim(),
    ssh_user: String(item.ssh_user || "").trim(),
    ssh_key_path: String(item.ssh_key_path || "").trim(),
    cleanup_task_id: String(item.cleanup_task_id || "").trim(),
    beszel_names: normalizeStringArray(item.beszel_names),
    containers: normalizeStringArray(item.containers),
    container_groups: item.container_groups || []
  })).filter((item) => item.id || item.name);
  return {
    public_url: String(values.public_url || "").trim(),
    woodpecker_server: String(values.woodpecker_server || "").trim(),
    woodpecker_public_url: String(values.woodpecker_public_url || "").trim(),
    woodpecker_token: String(values.woodpecker_token || "").trim(),
    beszel_base_url: String(values.beszel_base_url || "").trim(),
    beszel_public_url: String(values.beszel_public_url || "").trim(),
    beszel_email: String(values.beszel_email || "").trim(),
    beszel_password: String(values.beszel_password || "").trim(),
    grafana_public_url: String(values.grafana_public_url || "").trim(),
    external_links: externalLinks,
    monitor_hosts: monitorHosts,
    monitor_refresh_seconds: Number(values.monitor_refresh_seconds || 20),
    monitor_warn_disk: Number(values.monitor_warn_disk || 80),
    monitor_crit_disk: Number(values.monitor_crit_disk || 90),
    monitor_warn_memory: Number(values.monitor_warn_memory || 80)
  };
}

function normalizeStringArray(values?: string[]): string[] {
  return (values || [])
    .flatMap((item) => String(item || "").split(","))
    .map((item) => item.trim())
    .filter(Boolean);
}

function setupStatusColor(status: string): string {
  if (status === "ok") return "success";
  if (status === "error" || status === "critical") return "error";
  return "warning";
}

function setupStatusText(status: string): string {
  if (status === "ok") return "已就绪";
  if (status === "error" || status === "critical") return "异常";
  return "待配置";
}

export function branchOptionsForTask(state: StateResponse, task: Task): { value: string; label: string }[] {
  return branchOptionsForRepo(state, task.repo_id, task.branch || "main");
}

export function branchOptionsForRepo(state: StateResponse, repoID: number, configuredBranch: string): { value: string; label: string }[] {
  const configured = configuredBranch || "main";
  const branches = [...(state.branches?.[String(repoID)] || [])];
  if (!branches.includes(configured)) {
    branches.unshift(configured);
  }
  return Array.from(new Set(branches.filter(Boolean))).map((branch) => ({ value: branch, label: branch }));
}

function pipelineTaskText(row: Pipeline): string {
  if (row.zefire_task_title) return row.zefire_task_title;
  const variables = row.variables || {};
  const action = variables.DEPLOY_ACTION || variables.deploy_action || "";
  const target = variables.DEPLOY_TARGET || variables.deploy_target || "";
  if (action && target) return `${action} · ${target}`;
  if (action) return `DEPLOY_ACTION=${action}`;
  if (target) return `DEPLOY_TARGET=${target}`;
  if (row.event === "push") return "push 自动流水线";
  if (row.event === "manual") return "手动流水线";
  return row.event || "流水线";
}

function pipelineVariableHint(row: Pipeline): string {
  const variables = row.variables || {};
  const entries = Object.entries(variables).filter(([key]) => !isSensitiveVariable(key));
  if (!entries.length) return "";
  return entries
    .slice(0, 2)
    .map(([key, value]) => `${key}=${value || "-"}`)
    .join(" · ");
}

function isSensitiveVariable(key: string): boolean {
  return /PASSWORD|TOKEN|SECRET|KEY|PRIVATE|CREDENTIAL|ACCESS/i.test(key);
}

function pipelineTriggerText(row: Pipeline): string {
  if (!row.zefire_triggered_by) {
    const actor = row.author || row.sender || "";
    if (row.event === "push") return actor ? `Git push：${actor}` : "Git push";
    return actor ? `Woodpecker：${actor}` : "外部触发/未记录";
  }
  return ["Zephyr：" + row.zefire_triggered_by, formatShortTime(row.zefire_triggered_at)]
    .filter(Boolean)
    .join(" · ");
}

function pipelineTimeText(row: Pipeline): string {
  if (row.finished) return `完成 ${formatUnixTime(row.finished)}`;
  if (row.started) return `开始 ${formatUnixTime(row.started)}`;
  if (row.created) return `创建 ${formatUnixTime(row.created)}`;
  return "-";
}

function pipelineDurationText(row: Pipeline, nowMs: number): string {
  const now = Math.floor(nowMs / 1000);
  if (row.started && row.finished) return formatDuration(row.finished - row.started);
  if (row.started) return `运行 ${formatDuration(now - row.started)}`;
  if (row.created && ["pending", "running"].includes(row.status)) return `排队 ${formatDuration(now - row.created)}`;
  if (row.created && row.finished) return `总计 ${formatDuration(row.finished - row.created)}`;
  return "-";
}

function pipelineActivityMetaText(row: Pipeline, nowMs: number): string {
  if (["running", "pending"].includes(row.status)) return pipelineDurationText(row, nowMs);
  const at = pipelineSortTime(row);
  const age = at ? deployedAgeText(at, nowMs) : "";
  const duration = row.started && row.finished ? `耗时 ${formatDuration(row.finished - row.started)}` : "";
  return [age, duration].filter(Boolean).join(" · ") || pipelineDurationText(row, nowMs);
}

function pipelineStepTimeText(step: PipelineStep): string {
  if (step.started && step.finished) return `耗时 ${formatDuration(step.finished - step.started)}`;
  if (step.started) return `开始 ${formatUnixTime(step.started)}`;
  return step.type || "-";
}

function pipelineSortTime(row: Pipeline): number {
  return Math.max(row.finished || 0, row.started || 0, row.created || 0);
}

function formatDuration(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds || 0));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m ${secs}s`;
  return `${secs}s`;
}

function deployedAgeText(unixSeconds: number, nowMs: number): string {
  const seconds = Math.max(0, Math.floor(nowMs / 1000) - unixSeconds);
  if (seconds < 60) return "刚刚";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m 前`;
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  if (hours < 24) return remainingMinutes > 0 ? `${hours}h ${remainingMinutes}m 前` : `${hours}h 前`;
  const days = Math.floor(hours / 24);
  const remainingHours = hours % 24;
  return remainingHours > 0 ? `${days}d ${remainingHours}h 前` : `${days}d 前`;
}

function formatUnixTime(value?: number): string {
  if (!value) return "";
  return formatShortTime(new Date(value * 1000).toISOString());
}

function formatShortTime(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false
  }).format(date);
}

function pipelinePercent(row: Pipeline, nowMs: number): number {
  if (row.status === "success") return 100;
  if (["failure", "error", "killed", "skipped"].includes(row.status)) return 100;
  if (row.status === "pending") return 10;
  if (row.status === "running") {
    if (!row.started) return 35;
    const elapsed = Math.max(0, nowMs / 1000 - row.started);
    return Math.min(92, Math.max(25, Math.round((elapsed / 600) * 70)));
  }
  return 0;
}

function progressStatus(row: Pipeline): "success" | "exception" | "normal" | "active" {
  if (row.status === "success") return "success";
  if (["failure", "error", "killed"].includes(row.status)) return "exception";
  if (["running", "pending"].includes(row.status)) return "active";
  return "normal";
}

function repoName(state: StateResponse, task: Task): string {
  return task.repo_name || state.repos[String(task.repo_id)] || `Repo ${task.repo_id}`;
}

function taskToPipelinePreview(state: StateResponse, task: Task): Pipeline {
  return {
    number: 0,
    status: "",
    event: "manual",
    commit: "",
    branch: task.branch,
    created: 0,
    started: 0,
    finished: 0,
    message: task.title,
    variables: task.variables,
    repo_id: task.repo_id,
    repo_name: repoName(state, task)
  };
}

function repoNameByID(state: StateResponse, repoID: number): string {
  return state.repos[String(repoID)] || `Repo ${repoID}`;
}

function riskLabel(risk: Risk): string {
  return { normal: "普通", warning: "注意", danger: "高危", link: "入口" }[risk] || risk;
}

function statusText(status?: string): string {
  if (status === "not_deployed") return "未部署";
  if (status === "success") return "成功";
  if (status === "running") return "运行中";
  if (status === "pending") return "排队中";
  if (status === "failure" || status === "error") return "失败";
  if (status === "killed") return "已取消";
  return status || "-";
}

export function canRunTask(user: User, task: Task): boolean {
  if (task.disabled) return false;
  const roles = task.allowed_roles || [];
  if (!roles.length) return true;
  return roles.includes(user.role);
}

function isRollbackTask(task: Task): boolean {
  const variables = task.variables || {};
  return variables.DEPLOY_ACTION === "rollback" || Boolean(variables.ROLLBACK_VERSION) || /rollback|回退/i.test(task.id + task.title);
}

function deploymentStatusForTask(task: Task, statuses: DeploymentStatus[]): DeploymentStatus | undefined {
  const key = taskProjectKey(task);
  return statuses.find((item) => item.id === key || (item.repo_id === task.repo_id && normalizeKey(item.group || item.name) === normalizeKey(task.group || task.title)));
}

function deploymentActionsForStatus(row: DeploymentStatus, tasks: Task[]): { deploy?: Task; rollback?: Task } {
  const candidates = tasks.filter((task) => !task.external_url && (taskProjectKey(task) === row.id || (task.repo_id === row.repo_id && normalizeKey(task.group || task.title) === normalizeKey(row.group || row.name))));
  return {
    deploy: candidates.find((task) => !isRollbackTask(task) && !isCleanupTask(task)),
    rollback: candidates.find((task) => isRollbackTask(task))
  };
}

function taskProjectKey(task: Task): string {
  const variables = task.variables || {};
  const id = variables.ZEPHYR_PROJECT_ID || variables.PROJECT_ID || variables.SERVICE_ID || variables.DEPLOY_SERVICE || variables.APP || variables.PROJECT || task.group || task.id;
  return `repo-${task.repo_id}:${normalizeKey(id)}`;
}

function normalizeKey(value: string): string {
  return String(value || "").trim().toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "");
}

function isCleanupTask(task: Task): boolean {
  const action = String(task.variables?.DEPLOY_ACTION || "").toLowerCase();
  return action.includes("cleanup") || action.includes("clean") || action.includes("disk");
}

function mobileDeploymentActions(
  row: DeploymentStatus,
  tasks: Task[],
  currentUser: User,
  triggeringTaskIDSet: Set<string>,
  onRun: (task: Task) => void
) {
  const actions = deploymentActionsForStatus(row, tasks);
  const out: ReactNode[] = [];
  if (actions.deploy) {
    out.push(
      <Button key="deploy" size="small" type="primary" loading={triggeringTaskIDSet.has(actions.deploy.id)} disabled={!canRunTask(currentUser, actions.deploy)} onClick={() => onRun(actions.deploy!)}>
        部署
      </Button>
    );
  }
  if (actions.rollback) {
    out.push(
      <Button key="rollback" size="small" danger loading={triggeringTaskIDSet.has(actions.rollback.id)} disabled={!canRunTask(currentUser, actions.rollback)} onClick={() => onRun(actions.rollback!)}>
        回退
      </Button>
    );
  }
  return out;
}

function monitoringSourceText(source: string): string {
  if (source === "beszel") return "Beszel";
  if (source === "ssh_fallback") return "SSH 兜底";
  if (source === "degraded") return "已降级";
  return source || "未知";
}

function monitoringSourceColor(source: string): string {
  if (source === "beszel") return "success";
  if (source === "ssh_fallback") return "gold";
  if (source === "degraded") return "error";
  return "default";
}

function monitoringStatusColor(status: string): string {
  const value = String(status || "").toLowerCase();
  if (!value || value === "unknown") return "default";
  if (["ok", "up", "online", "healthy", "active"].includes(value)) return "success";
  if (["warning", "degraded"].includes(value)) return "gold";
  return "error";
}

function monitoringHostStatusText(status: string): string {
  const value = String(status || "").toLowerCase();
  if (!value || value === "unknown") return "未知";
  if (["ok", "up", "online", "healthy", "active"].includes(value)) return "正常";
  return status;
}

function monitoringRoleText(role: string): string {
  const value = String(role || "").toLowerCase();
  if (value === "production") return "生产节点";
  if (value === "builder") return "构建节点";
  if (value === "ops-builder") return "运维/测试节点";
  if (value === "edge") return "边缘入口";
  if (value === "infra") return "基础设施";
  return role || "被管节点";
}

function monitoringAlertColor(level: string): string {
  if (level === "critical") return "error";
  if (level === "warning") return "gold";
  return "blue";
}

function monitoringAlertText(level: string): string {
  if (level === "critical") return "紧急";
  if (level === "warning") return "提醒";
  return "信息";
}

function containerStatusColor(status: string): string {
  const value = String(status || "").toLowerCase();
  if (value.includes("up") || value.includes("healthy")) return "success";
  if (value.includes("restarting") || value.includes("starting")) return "gold";
  if (value.includes("missing") || value.includes("exited") || value.includes("dead")) return "error";
  return "default";
}

function metricProgressStatus(value: number): "success" | "exception" | "normal" | "active" {
  if (value >= 90) return "exception";
  if (value >= 80) return "active";
  return "normal";
}

function metricTone(value: number): "normal" | "warning" | "danger" {
  if (value >= 90) return "danger";
  if (value >= 80) return "warning";
  return "normal";
}

function highestHostMetric(hosts: MonitoringHost[], field: "cpu_percent" | "memory_percent" | "disk_percent"): { host?: MonitoringHost; value: number } {
  return hosts.reduce<{ host?: MonitoringHost; value: number }>((best, host) => {
    const value = Number(host[field] || 0);
    return value > best.value ? { host, value } : best;
  }, { value: 0 });
}

function highestMonitoringAlertLevel(alerts: MonitoringAlert[]): "info" | "warning" | "critical" {
  if (alerts.some((item) => item.level === "critical")) return "critical";
  if (alerts.some((item) => item.level === "warning")) return "warning";
  return "info";
}

function formatPercent(value: number): string {
  return Number(value || 0).toFixed(1);
}

function formatLoad(value?: number): string {
  return Number(value || 0).toFixed(2);
}

function formatBytes(value: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let next = Math.max(0, Number(value || 0));
  let index = 0;
  while (next >= 1024 && index < units.length - 1) {
    next /= 1024;
    index += 1;
  }
  return `${next >= 10 || index === 0 ? next.toFixed(0) : next.toFixed(1)}${units[index]}`;
}

function checkedAtText(value: string, nowMs: number): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const seconds = Math.max(0, Math.floor((nowMs - date.getTime()) / 1000));
  if (seconds < 60) return `${seconds}s 前`;
  return `${formatShortTime(value)} · ${deployedAgeText(Math.floor(date.getTime() / 1000), nowMs)}`;
}

export function variablesText(values: Record<string, string>): string {
  return Object.entries(values || {})
    .map(([key, value]) => `${key}=${value}`)
    .join("\n");
}

export function parseVariables(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const index = trimmed.indexOf("=");
    if (index <= 0) continue;
    out[trimmed.slice(0, index).trim()] = trimmed.slice(index + 1).trim();
  }
  return out;
}

function safeVariablesTextForDisplay(values: Record<string, string>): string {
  return variablesText(
    Object.fromEntries(
      Object.entries(values || {}).map(([key, value]) => [key, isSensitiveVariable(key) ? "***" : value])
    )
  );
}
