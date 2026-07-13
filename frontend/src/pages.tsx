import {
  Alert,
  App as AntApp,
  Button,
  Card,
  Checkbox,
  Col,
  Descriptions,
  Divider,
  Drawer,
  Empty,
  Form,
  Grid,
  Input,
  InputNumber,
  List,
  Popconfirm,
  Progress,
  Radio,
  Row,
  Segmented,
  Select,
  Space,
  Spin,
  Table,
  Tabs,
  Tag,
  Tooltip,
  Typography
} from "antd";
import type { ColumnsType } from "antd/es/table";
import { ProCard, ProTable } from "@ant-design/pro-components";
import type { ProColumns } from "@ant-design/pro-components";
import { Virtuoso } from "react-virtuoso";
import {
  Activity,
  Clock3,
  Copy,
  Cpu,
  ExternalLink,
  FileText,
  Gauge,
  HardDrive,
  Home,
  Info,
  MemoryStick,
  Network,
  Play,
  Plus,
  RefreshCw,
  Rocket,
  ScrollText,
  Search,
  Server,
  Settings,
  Trash2,
  XCircle
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { ApiError, api, errorText, fetchDiskDiagnosis, fetchDiskCleanupPreview, executeDiskCleanup } from "./api";
import { PRODUCT_NAME, PRODUCT_REPO_NAME, PRODUCT_REPO_OWNER } from "./brand";
import type {
  AuditRecord,
  DeploymentRevision,
  DeploymentVerificationSummary,
  DeploymentStatus,
  LogContainer,
  LogContainersResponse,
  LogLine,
  LogQueryRequest,
  LogQueryResponse,
  LogSummaryResponse,
  LogStrategyStatus,
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
  TaskTemplate,
  TemplateApplyResponse,
  User,
  WoodpeckerRepo,
  WoodpeckerReposResponse,
  DiskDiagnosisResponse,
  DiskCleanupPreviewResponse,
  DiskCleanupRequest,
  DiskCleanupResponse
} from "./types";

type TaskRunHandler = (task: Task, presetInputs?: Record<string, string>) => void;

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

export function peapodNavItems() {
  return [
    { key: "overview", icon: <Home size={16} />, label: "总览" },
    { key: "deploy", icon: <Rocket size={16} />, label: "部署" },
    { key: "pipelines", icon: <Activity size={16} />, label: "流水线" },
    { key: "monitoring", icon: <Server size={16} />, label: "监控" },
    { key: "logs", icon: <ScrollText size={16} />, label: "日志" },
    { key: "settings", icon: <Settings size={16} />, label: "设置" }
  ];
}

function settingsSectionFromHash(): string {
  const parts = window.location.hash.replace(/^#\/?/, "").trim().split("/");
  return parts[0] === "settings" && parts[1] ? parts[1] : "setup";
}

function PageIntro({
  title,
  description,
  stats,
  actions
}: {
  title: string;
  description: string;
  stats?: Array<{ label: string; value: string; tone?: "normal" | "success" | "warning" | "danger" }>;
  actions?: ReactNode;
}) {
  const visibleStats = stats || [];
  return (
    <div className="page-intro">
      <div className="page-intro-copy">
        <Title level={3}>{title}</Title>
        <Text type="secondary">{description}</Text>
      </div>
      {(visibleStats.length > 0 || actions) && (
        <div className="page-intro-meta">
          {visibleStats.length > 0 && (
            <div className="page-intro-stats">
              {visibleStats.map((item) => (
                <div className={`page-intro-stat page-intro-stat-${item.tone || "normal"}`} key={item.label}>
                  <Text type="secondary">{item.label}</Text>
                  <Text strong>{item.value}</Text>
                </div>
              ))}
            </div>
          )}
          {actions && <div className="page-intro-actions">{actions}</div>}
        </div>
      )}
    </div>
  );
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
  onNavigate: (key: string) => void;
  onRefresh: () => void;
  onInspectPipeline: (row: Pipeline) => void;
}) {
  const alerts = monitoring?.alerts || [];
  const alertLevel = highestMonitoringAlertLevel(alerts);
  const attentionPipelines = overviewPipelineRows(pipelines, nowMs, 4);
  const highestDisk = highestHostMetric(monitoring?.hosts || [], "disk_percent");
  const highestMemory = highestHostMetric(monitoring?.hosts || [], "memory_percent");
  const healthyText = alertLevel === "critical" ? "需要处理" : alertLevel === "warning" ? "有提醒" : "可以上线";
  return (
    <Space direction="vertical" size={12} className="side-stack">
      <PageIntro
        title="总览"
        description="上线状态、队列、资源水位和最近版本集中在这里。"
        stats={[
          { label: "状态", value: healthyText, tone: alertLevel === "critical" ? "danger" : alertLevel === "warning" ? "warning" : "success" },
          { label: "数据源", value: monitoring?.source ? monitoringSourceText(monitoring.source) : "-" },
          { label: "检查", value: monitoring?.checked_at ? checkedAtText(monitoring.checked_at, nowMs) : "-" }
        ]}
        actions={
          <Space wrap>
            <Button type="primary" icon={<Rocket size={16} />} onClick={() => onNavigate("deploy")}>部署</Button>
            <Button icon={<RefreshCw size={16} />} onClick={onRefresh}>刷新</Button>
          </Space>
        }
      />

      <OverviewMetricGrid
        items={[
          { label: "运行中 / 排队", value: String(runningCount), meta: "当前队列", icon: <Activity size={18} /> },
          { label: "24h 失败", value: String(failedCount), meta: "未被成功覆盖", tone: failedCount ? "danger" : "success" },
          { label: "磁盘最高", value: `${formatPercent(highestDisk.value)}%`, meta: highestDisk.host?.name || "-", tone: metricTone(highestDisk.value) },
          { label: "内存最高", value: `${formatPercent(highestMemory.value)}%`, meta: highestMemory.host?.name || "-", tone: metricTone(highestMemory.value) }
        ]}
      />

      {alerts.length > 0 && (
        <Alert
          type={alertLevel === "critical" ? "error" : "warning"}
          showIcon
          message={alerts[0].title}
          description={alerts.slice(0, 3).map((item) => item.message).join("；")}
          action={<Button size="small" onClick={() => onNavigate("monitoring")}>查看监控</Button>}
        />
      )}

      <ProCard split="vertical" gutter={12} className="overview-split-card">
        <ProCard title="线上版本" extra={<Button type="link" onClick={() => onNavigate("deploy")}>进入部署</Button>}>
          <DeploymentVersionList rows={overviewDeploymentRows(deploymentStatuses, 6)} nowMs={nowMs} />
        </ProCard>
        <ProCard title="近期流水线" extra={<Button type="link" onClick={() => onNavigate("pipelines")}>查看全部</Button>}>
          <PipelineActivityList rows={attentionPipelines} nowMs={nowMs} onInspect={onInspectPipeline} />
        </ProCard>
      </ProCard>

      <HomeResourceStrip summary={monitoring} loading={monitoringLoading} onOpenMonitoring={() => onNavigate("monitoring")} />
    </Space>
  );
}

function OverviewMetricGrid({
  items
}: {
  items: Array<{ label: string; value: string; meta?: string; icon?: ReactNode; tone?: "normal" | "success" | "warning" | "danger" }>;
}) {
  return (
    <div className="overview-metric-grid">
      {items.map((item) => (
        <div className={`overview-metric-card overview-metric-${item.tone || "normal"}`} key={item.label}>
          <div className="overview-metric-label">
            {item.icon}
            <Text type="secondary">{item.label}</Text>
          </div>
          <Text strong className="overview-metric-value">{item.value}</Text>
          {item.meta && <Text type="secondary" className="overview-metric-meta">{item.meta}</Text>}
        </div>
      ))}
    </div>
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
                <Text strong>{productText(row.name)}</Text>
                <Tag color={deployVerifyColor(row)}>{deployVerifyText(row)}</Tag>
              </Space>
            }
            description={
              <Text type="secondary">
                {deploymentVersionText(row, nowMs)}
              </Text>
            }
          />
        </List.Item>
      )}
    />
  );
}

function PipelineActivityList({ rows, nowMs, onInspect }: { rows: Pipeline[]; nowMs: number; onInspect: (row: Pipeline) => void }) {
  if (!rows.length) return <Alert type="success" showIcon message="当前没有需要处理的流水线" description="已被后续成功覆盖的历史失败不会在总览里持续占位。" />;
  return (
    <List
      className="overview-pipeline-list"
      dataSource={rows}
      renderItem={(row) => (
        <List.Item actions={[<Button key="detail" size="small" onClick={() => onInspect(row)}>详情</Button>]}>
          <List.Item.Meta
            title={
              <Space wrap>
                <Text strong>{productText(row.repo_name)} #{row.number}</Text>
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
  pipelines,
  currentUser,
  triggeringTaskIds,
  refreshing,
  onRun,
  onRefresh,
  onConfigure,
  onCancel,
  onInspect
}: {
  state: StateResponse;
  rows: DeploymentStatus[];
  woodpecker: string;
  nowMs: number;
  tasks: Task[];
  pipelines: Pipeline[];
  currentUser: User;
  triggeringTaskIds: string[];
  refreshing: boolean;
  onRun: TaskRunHandler;
  onRefresh: () => void;
  onConfigure: () => void;
  onCancel: (row: Pipeline) => void;
  onInspect: (row: Pipeline) => void;
}) {
  const [query, setQuery] = useState("");
  const objects = useMemo(() => buildDeployObjects(state, rows, tasks, pipelines, nowMs), [state, rows, tasks, pipelines, nowMs]);
  const [selectedID, setSelectedID] = useState("");
  useEffect(() => {
    if (!objects.length) {
      setSelectedID("");
      return;
    }
    if (selectedID && !objects.some((item) => item.id === selectedID)) {
      setSelectedID("");
    }
  }, [objects, selectedID]);
  const filteredObjects = useMemo(() => filterDeployObjects(objects, query), [objects, query]);
  return (
    <Space direction="vertical" size={16} className="side-stack">
      <PageIntro
        title="部署"
        description="按项目和环境管理部署、回退与流水线，适合一个仓库部署到多台机器。"
        actions={
          <Space className="deploy-intro-actions" wrap>
            <Button icon={<RefreshCw size={16} />} loading={refreshing} onClick={onRefresh}>刷新</Button>
          </Space>
        }
      />
      <div className="deploy-console">
        <DeployObjectConsole
          objects={filteredObjects}
          allObjects={objects}
          query={query}
          selectedID={selectedID}
          state={state}
          woodpecker={woodpecker}
          nowMs={nowMs}
          currentUser={currentUser}
          triggeringTaskIds={triggeringTaskIds}
          onQueryChange={setQuery}
          onSelect={setSelectedID}
          onRun={onRun}
          onConfigure={onConfigure}
          onCancel={onCancel}
          onInspect={onInspect}
        />
      </div>
    </Space>
  );
}

type DeployObject = {
  id: string;
  kind: "deployment";
  title: string;
  subtitle: string;
  environmentLabel: string;
  branchLabel: string;
  statusLabel: string;
  statusColor: string;
  attention: boolean;
  risk: Risk;
  deployment?: DeploymentStatus;
  primaryTask?: Task;
  rollbackTask?: Task;
  extraTasks: Task[];
  pipelines: Pipeline[];
  searchText: string;
};

function DeployObjectConsole({
  objects,
  allObjects,
  query,
  selectedID,
  state,
  woodpecker,
  nowMs,
  currentUser,
  triggeringTaskIds,
  onQueryChange,
  onSelect,
  onRun,
  onConfigure,
  onCancel,
  onInspect
}: {
  objects: DeployObject[];
  allObjects: DeployObject[];
  query: string;
  selectedID: string;
  state: StateResponse;
  woodpecker: string;
  nowMs: number;
  currentUser: User;
  triggeringTaskIds: string[];
  onQueryChange: (value: string) => void;
  onSelect: (id: string) => void;
  onRun: TaskRunHandler;
  onConfigure: () => void;
  onCancel: (row: Pipeline) => void;
  onInspect: (row: Pipeline) => void;
}) {
  const triggeringTaskIDSet = useMemo(() => new Set(triggeringTaskIds), [triggeringTaskIds]);
  const selected = allObjects.find((item) => item.id === selectedID) || null;

  return (
    <Space direction="vertical" size={12} className="side-stack">
      {selected ? (
        <DeployObjectDetailView
          item={selected}
          state={state}
          woodpecker={woodpecker}
          nowMs={nowMs}
          currentUser={currentUser}
          triggeringTaskIDSet={triggeringTaskIDSet}
          onBack={() => onSelect("")}
          onRun={onRun}
          onCancel={onCancel}
          onInspect={onInspect}
        />
      ) : (
        <>
          <DeployObjectListTable
            objects={objects}
            allObjects={allObjects}
            state={state}
            woodpecker={woodpecker}
            nowMs={nowMs}
            currentUser={currentUser}
            triggeringTaskIDSet={triggeringTaskIDSet}
            onSelect={onSelect}
            onRun={onRun}
            onConfigure={onConfigure}
            query={query}
            onQueryChange={onQueryChange}
          />
          <DeployObjectMobileList
            objects={objects}
            state={state}
            woodpecker={woodpecker}
            nowMs={nowMs}
            currentUser={currentUser}
            triggeringTaskIDSet={triggeringTaskIDSet}
            onRun={onRun}
            onSelect={onSelect}
            query={query}
            onQueryChange={onQueryChange}
            onConfigure={onConfigure}
          />
        </>
      )}
    </Space>
  );
}

function DeployObjectListTable({
  objects,
  allObjects,
  state,
  woodpecker,
  nowMs,
  currentUser,
  triggeringTaskIDSet,
  onSelect,
  onRun,
  onConfigure,
  query,
  onQueryChange
}: {
  objects: DeployObject[];
  allObjects: DeployObject[];
  state: StateResponse;
  woodpecker: string;
  nowMs: number;
  currentUser: User;
  triggeringTaskIDSet: Set<string>;
  onSelect: (id: string) => void;
  onRun: TaskRunHandler;
  onConfigure: () => void;
  query: string;
  onQueryChange: (value: string) => void;
}) {
  const columns: ProColumns<DeployObject>[] = [
    {
      title: "项目",
      width: 280,
      sorter: (a, b) => a.title.localeCompare(b.title, "zh-CN"),
      render: (_, row) => (
        <div className="deploy-object-project-cell">
          <DeployObjectHealthMark item={row} />
          <Space direction="vertical" size={2} className="table-cell-stack">
            <Space wrap size={[6, 4]}>
              <Button type="link" className="deploy-object-name-link" onClick={(event) => { event.stopPropagation(); onSelect(row.id); }}>
                {row.title}
              </Button>
              <DeployObjectStateTags item={row} />
            </Space>
            <Text type="secondary" ellipsis={{ tooltip: row.subtitle }}>{row.subtitle}</Text>
          </Space>
        </div>
      )
    },
    {
      title: "环境 / 分支",
      width: 210,
      render: (_, row) => (
        <Space direction="vertical" size={0} className="table-cell-stack">
          <DeployObjectMetaCell value={row.environmentLabel} />
          <Text type="secondary" ellipsis={{ tooltip: row.branchLabel }}>{row.branchLabel}</Text>
        </Space>
      )
    },
    {
      title: "线上版本",
      width: 240,
      render: (_, row) => <DeployObjectVersionCell item={row} nowMs={nowMs} state={state} />
    },
    {
      title: "最近流水线",
      width: 230,
      sorter: (a, b) => pipelineSortValue(a.pipelines[0]) - pipelineSortValue(b.pipelines[0]),
      render: (_, row) => <DeployObjectPipelineOverview row={row.pipelines[0]} nowMs={nowMs} />
    },
    {
      title: "",
      width: 230,
      fixed: "right",
      render: (_, row) => (
        <DeployObjectRowActions item={row} woodpecker={woodpecker} currentUser={currentUser} triggeringTaskIDSet={triggeringTaskIDSet} onRun={onRun} onOpenDetail={() => onSelect(row.id)} />
      )
    }
  ];

  return (
    <Card className="deploy-object-index-card">
      <div className="deploy-object-index-head">
        <Space direction="vertical" size={1} className="table-cell-stack">
          <Text strong>部署项目</Text>
          <Text type="secondary">这里只展示可上线的项目/环境；清理、重启、状态检查放在维护动作里。</Text>
        </Space>
        <div className="deploy-object-toolbar">
          <Input
            allowClear
            prefix={<Search size={15} />}
            placeholder="搜索项目、环境、仓库、分支"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
          />
          <Button icon={<Settings size={15} />} onClick={onConfigure}>配置部署项目</Button>
          <Tag>{objects.length}/{allObjects.length}</Tag>
        </div>
      </div>
      <ProTable<DeployObject>
        className="desktop-deploy-object-index-table"
        rowKey="id"
        size="small"
        columns={columns}
        dataSource={objects}
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的部署项目" /> }}
        search={false}
        options={false}
        tableAlertRender={false}
        pagination={objects.length > 14 ? { pageSize: 14, showSizeChanger: true, pageSizeOptions: [14, 30, 60], showTotal: (total) => `共 ${total} 个对象` } : false}
        scroll={{ x: 1120 }}
        tableLayout="fixed"
        onRow={(row) => ({ onClick: () => onSelect(row.id) })}
      />
    </Card>
  );
}

function DeployObjectStateTags({ item }: { item: DeployObject }) {
  const latest = item.pipelines[0];
  return (
    <>
      <Tag color="blue">项目</Tag>
      <Tag color={item.statusColor}>{item.statusLabel}</Tag>
      {deployObjectRollbackTask(item) && <Tag color="volcano">可回滚</Tag>}
      {latest && ["running", "pending"].includes(latest.status) && <Tag color="processing">#{latest.number} {statusText(latest.status)}</Tag>}
      {item.attention && !["running", "pending"].includes(latest?.status || "") && <Tag color={item.risk === "danger" ? "red" : "gold"}>需关注</Tag>}
    </>
  );
}

function DeployObjectDetailView({
  item,
  state,
  woodpecker,
  nowMs,
  currentUser,
  triggeringTaskIDSet,
  onBack,
  onRun,
  onCancel,
  onInspect
}: {
  item: DeployObject;
  state: StateResponse;
  woodpecker: string;
  nowMs: number;
  currentUser: User;
  triggeringTaskIDSet: Set<string>;
  onBack: () => void;
  onRun: TaskRunHandler;
  onCancel: (row: Pipeline) => void;
  onInspect: (row: Pipeline) => void;
}) {
  const latest = item.pipelines[0];
  return (
    <div className="deploy-object-detail-shell">
      <div className="deploy-object-breadcrumb">
        <Button type="link" onClick={onBack}>部署</Button>
        <Text type="secondary">/</Text>
        <Text strong>{item.title}</Text>
      </div>
      <main className="deploy-object-detail-main">
        <div className="deploy-object-detail-head">
          <div className="deploy-object-detail-title">
            <Space size={8} wrap>
              <DeployObjectHealthMark item={item} />
              <Tag color="blue">部署项目</Tag>
              {item.attention && <Tag color={item.risk === "danger" ? "red" : "gold"}>需处理</Tag>}
              {latest && <Tag color={statusColors[latest.status] || "default"}>#{latest.number} {statusText(latest.status)}</Tag>}
            </Space>
            <Title level={3}>{item.title}</Title>
            <Text type="secondary">{item.subtitle}</Text>
            <div className="deploy-object-detail-meta">
              <Tag>{item.environmentLabel}</Tag>
              <Tag color="blue">{item.branchLabel}</Tag>
            </div>
          </div>
          <DeployObjectRowActions item={item} woodpecker={woodpecker} currentUser={currentUser} triggeringTaskIDSet={triggeringTaskIDSet} onRun={onRun} showRollback />
        </div>
        <DeployObjectStatusStrip item={item} state={state} nowMs={nowMs} />
        <DeployRollbackHistory item={item} currentUser={currentUser} triggeringTaskIDSet={triggeringTaskIDSet} nowMs={nowMs} onRun={onRun} />
        <DeployObjectExpandedPanel item={item} state={state} woodpecker={woodpecker} nowMs={nowMs} compact onCancel={onCancel} onInspect={onInspect} />
      </main>
    </div>
  );
}

function DeployRollbackHistory({
  item,
  currentUser,
  triggeringTaskIDSet,
  nowMs,
  onRun
}: {
  item: DeployObject;
  currentUser: User;
  triggeringTaskIDSet: Set<string>;
  nowMs: number;
  onRun: TaskRunHandler;
}) {
  const rollbackTask = deployObjectRollbackTask(item);
  if (!rollbackTask || !item.deployment) return null;
  const revisions = rollbackRevisionsForStatus(item.deployment).slice(0, 6);
  const canRun = canRunTask(currentUser, rollbackTask);
  if (!revisions.length) {
    return <Alert type="info" showIcon message="这个项目暂时没有可选的成功部署历史，不能指定版本回滚。" />;
  }
  return (
    <div className="deploy-rollback-panel">
      <div className="deploy-object-section-head">
        <Space size={8}>
          <RefreshCw size={16} />
          <Text strong>可回滚历史</Text>
          <Tag color="red">{revisions.length} 个候选</Tag>
        </Space>
        <Text type="secondary">只列出已验证成功的部署</Text>
      </div>
      <div className="deploy-rollback-list">
        {revisions.map((revision) => (
          <div className="deploy-rollback-item" key={rollbackRevisionKey(revision)}>
            <div className="deploy-rollback-meta">
              <Text strong>#{revision.pipeline || "-"}</Text>
              <Text ellipsis={{ tooltip: rollbackRevisionLabel(revision, nowMs) }}>{revision.branch || "-"} · {(revision.commit || "").slice(0, 8) || "-"}</Text>
              <Text type="secondary">{revision.deployed_at ? deployedAgeText(revision.deployed_at, nowMs) : "时间未知"}{revision.triggered_by ? ` · ${revision.triggered_by}` : ""}</Text>
            </div>
            <Tooltip title={canRun ? "按这个成功版本触发回滚" : taskDisabledTitle(currentUser, rollbackTask)}>
              <span>
                <Button
                  size="small"
                  danger
                  disabled={!canRun}
                  loading={triggeringTaskIDSet.has(rollbackTask.id)}
                  onClick={() => onRun(rollbackTask, rollbackPresetInputs(revision))}
                >
                  回滚到这里
                </Button>
              </span>
            </Tooltip>
          </div>
        ))}
      </div>
    </div>
  );
}

function DeployObjectHealthMark({ item }: { item: DeployObject }) {
  const latest = item.pipelines[0];
  const status = latest?.status || (item.deployment?.deploy_verified ? "success" : item.risk === "danger" ? "failure" : "pending");
  const tone = status === "success" ? "success" : ["failure", "error", "killed"].includes(status) ? "failure" : ["running", "pending"].includes(status) ? "running" : "unknown";
  return (
    <Tooltip title={deployObjectHealthText(status)}>
      <span className={`deploy-health deploy-health-${tone}`} />
    </Tooltip>
  );
}

function deployObjectHealthText(status: string): string {
  if (status === "success") return "最近一次流水线成功";
  if (["failure", "error"].includes(status)) return "最近一次流水线失败";
  if (status === "killed") return "最近一次流水线已取消";
  if (status === "running") return "流水线运行中";
  if (status === "pending") return "流水线排队中或等待验证";
  return "状态未知";
}

function DeployObjectMetaCell({ value }: { value: string }) {
  return <Text ellipsis={{ tooltip: value }}>{value || "未标注"}</Text>;
}

function DeployObjectPipelineOverview({ row, nowMs }: { row?: Pipeline; nowMs: number }) {
  if (!row) return <Text type="secondary">暂无流水线</Text>;
  return (
    <Space direction="vertical" size={1} className="table-cell-stack">
      <Space size={6} wrap>
        <Text strong>#{row.number}</Text>
        <Tag color={statusColors[row.status] || "default"}>{statusText(row.status)}</Tag>
        <Text type="secondary">{pipelineDurationText(row, nowMs)}</Text>
      </Space>
      <Text type="secondary" ellipsis={{ tooltip: `${row.branch || "-"} · ${(row.commit || "").slice(0, 8) || "-"}` }}>
        {row.branch || "-"} · {(row.commit || "").slice(0, 8) || "-"}
      </Text>
      <Text type="secondary" ellipsis={{ tooltip: pipelineTaskText(row) }}>{pipelineTaskText(row)}</Text>
    </Space>
  );
}

function DeployObjectStatusStrip({ item, state, nowMs }: { item: DeployObject; state: StateResponse; nowMs: number }) {
  const latest = item.pipelines[0];
  const latestText = latest ? `#${latest.number} · ${statusText(latest.status)} · ${latest.branch || "-"} · ${(latest.commit || "").slice(0, 8) || "-"}` : "暂无流水线";
  const branchText = item.deployment?.configured_branch || item.primaryTask?.branch || "main";
  const versionLabel = item.deployment ? "线上版本" : "部署目标";
  const versionValue = item.deployment ? deploymentVersionText(item.deployment, nowMs) : item.primaryTask ? `${repoName(state, item.primaryTask)} · 默认 ${branchText}` : "-";
  return (
    <div className="deploy-object-status-strip">
      <DeployObjectMetric label={versionLabel} value={versionValue} accent />
      <DeployObjectMetric label="最近尝试" value={latestText} />
      <DeployObjectMetric label="默认分支" value={branchText} />
    </div>
  );
}

function DeployVerifyInlineNotice({ row }: { row: DeploymentStatus }) {
  const tone = deployVerifyTone(row);
  return (
    <div className={`deploy-verify-inline deploy-verify-inline-${tone}`}>
      <Space size={8} align="start">
        <Info size={15} />
        <Space direction="vertical" size={1} className="deploy-verify-inline-copy">
          <Text strong>{shortDeploymentVerifyMessage(row)}</Text>
          <Text type="secondary" ellipsis={{ tooltip: row.deploy_verify_message }}>{normalizeDeployVerifyMessage(row.deploy_verify_message || "")}</Text>
        </Space>
      </Space>
    </div>
  );
}

function DeployObjectVersionCell({ item, state, nowMs }: { item: DeployObject; state: StateResponse; nowMs: number }) {
  if (item.deployment) {
    return (
      <Space direction="vertical" size={0} className="table-cell-stack">
        <Text ellipsis={{ tooltip: deploymentVersionText(item.deployment, nowMs) }}>{deploymentVersionText(item.deployment, nowMs)}</Text>
        <Text type="secondary">默认 {item.deployment.configured_branch || "main"}</Text>
      </Space>
    );
  }
  if (item.primaryTask) {
    return (
      <Space direction="vertical" size={0} className="table-cell-stack">
        <Text>未部署</Text>
        <Text type="secondary" ellipsis={{ tooltip: repoName(state, item.primaryTask) }}>{repoName(state, item.primaryTask)} · 默认 {item.primaryTask.branch || "main"}</Text>
      </Space>
    );
  }
  return <Text type="secondary">-</Text>;
}

function DeployObjectRowActions({
  item,
  woodpecker,
  currentUser,
  triggeringTaskIDSet,
  onRun,
  onOpenDetail,
  showRollback = false
}: {
  item: DeployObject;
  woodpecker: string;
  currentUser: User;
  triggeringTaskIDSet: Set<string>;
  onRun: TaskRunHandler;
  onOpenDetail?: () => void;
  showRollback?: boolean;
}) {
  const lastPipeline = item.pipelines[0];
  const rollbackTask = deployObjectRollbackTask(item);
  const rollbackTarget = firstRollbackRevision(item.deployment);
  const blocked = deployBlockedState(currentUser, item);
  return (
    <Space size={8} onClick={(event) => event.stopPropagation()}>
      {item.primaryTask && canRunTask(currentUser, item.primaryTask) ? (
        <Tooltip title={taskDisabledTitle(currentUser, item.primaryTask)}>
          <span>
            <Button
              size="small"
              type="primary"
              icon={<Rocket size={14} />}
              danger={item.primaryTask.risk === "danger"}
              loading={triggeringTaskIDSet.has(item.primaryTask.id)}
              disabled={!canRunTask(currentUser, item.primaryTask)}
              onClick={() => onRun(item.primaryTask!)}
            >
              部署
            </Button>
          </span>
        </Tooltip>
      ) : (
        <Tooltip title={blocked.title}>
          <Tag className="deploy-action-placeholder" color={blocked.color}>{blocked.label}</Tag>
        </Tooltip>
      )}
      {onOpenDetail && (
        <Tooltip title={rollbackTask ? "进入详情选择回滚历史" : "进入详情查看流水线和部署上下文"}>
          <Button size="small" icon={<ScrollText size={14} />} onClick={onOpenDetail}>
            {rollbackTask ? "详情/回滚" : "详情"}
          </Button>
        </Tooltip>
      )}
      {showRollback && rollbackTask && canRunTask(currentUser, rollbackTask) && rollbackTarget && (
        <Tooltip title={taskDisabledTitle(currentUser, rollbackTask)}>
          <span>
            <Button
              size="small"
              danger
              loading={triggeringTaskIDSet.has(rollbackTask.id)}
              disabled={!canRunTask(currentUser, rollbackTask)}
              onClick={() => onRun(rollbackTask, rollbackPresetInputs(rollbackTarget))}
            >
              回滚
            </Button>
          </span>
        </Tooltip>
      )}
      {showRollback && rollbackTask && !canRunTask(currentUser, rollbackTask) && (
        <Tooltip title={taskDisabledTitle(currentUser, rollbackTask)}>
          <Tag className="deploy-action-placeholder">回滚不可执行</Tag>
        </Tooltip>
      )}
      {lastPipeline ? <Button size="small" href={pipelineURL(woodpecker, lastPipeline)} target="_blank" icon={<ExternalLink size={14} />} /> : null}
    </Space>
  );
}

function deployBlockedState(user: User, item: DeployObject): { label: string; title: string; color: string } {
  const task = item.primaryTask;
  if (!task) {
    return {
      label: "待配置",
      title: "没有匹配到这个项目的部署任务，请在设置里补齐项目 ID 和动作配置",
      color: "gold"
    };
  }
  if (task.disabled) {
    return {
      label: "已停用",
      title: task.disabled_reason || "这个部署任务当前被停用",
      color: "default"
    };
  }
  if (!canRunTask(user, task)) {
    const roles = task.allowed_roles || [];
    return {
      label: roles.includes("admin") ? "仅管理员" : "无权限",
      title: taskDisabledTitle(user, task) || "当前账号没有权限执行这个部署任务",
      color: "default"
    };
  }
  return {
    label: "不可执行",
    title: taskDisabledTitle(user, task) || "这个部署任务当前不可执行",
    color: "default"
  };
}

function deployObjectRollbackTask(item: DeployObject): Task | undefined {
  if (!item.rollbackTask || !item.deployment) return undefined;
  const row = item.deployment;
  if (firstRollbackRevision(row) || row.previous_branch || row.previous_commit || row.previous_pipeline || row.previous_deployed_at) return item.rollbackTask;
  return undefined;
}

function DeployObjectExpandedPanel({
  item,
  state,
  woodpecker,
  nowMs,
  compact = false,
  onCancel,
  onInspect
}: {
  item: DeployObject;
  state: StateResponse;
  woodpecker: string;
  nowMs: number;
  compact?: boolean;
  onCancel: (row: Pipeline) => void;
  onInspect: (row: Pipeline) => void;
}) {
  const [pipelineView, setPipelineView] = useState<DeployPipelineView>("all");
  const running = item.pipelines.filter((row) => ["running", "pending"].includes(row.status)).length;
  const failed = item.pipelines.filter((row) => ["failure", "error", "killed"].includes(row.status)).length;
  const visiblePipelines = filterDeployPipelines(item.pipelines, pipelineView);
  return (
    <div className="deploy-object-expanded-panel">
      {!compact && (
        <div className="deploy-object-version-grid">
          {item.deployment ? (
            <>
              <DeployObjectMetric label="当前版本" value={deploymentVersionText(item.deployment, nowMs)} accent />
              <DeployObjectMetric label="最近动作" value={`${productText(item.deployment.latest_action || item.deployment.last_action || "-")} · ${statusText(item.deployment.latest_status || item.deployment.last_status)}`} />
              <DeployObjectMetric label="上一成功" value={item.deployment.previous_branch ? `${item.deployment.previous_branch} · ${(item.deployment.previous_commit || "").slice(0, 8) || "-"}` : "-"} />
              <DeployObjectMetric label="默认分支" value={item.deployment.configured_branch || "main"} />
            </>
          ) : item.primaryTask ? (
            <>
              <DeployObjectMetric label="部署任务" value={pipelineTaskText(taskToPipelinePreview(state, item.primaryTask))} accent />
              <DeployObjectMetric label="默认分支" value={item.primaryTask.branch || "main"} />
              <DeployObjectMetric label="确认词" value={item.primaryTask.confirm_text || "无需确认"} />
              <DeployObjectMetric label="最近流水" value={item.pipelines[0] ? `#${item.pipelines[0].number} · ${statusText(item.pipelines[0].status)}` : "-"} />
            </>
          ) : null}
        </div>
      )}
      {item.deployment?.deploy_verify_message && <DeployVerifyInlineNotice row={item.deployment} />}
      <div className="deploy-object-section-head">
        <Space size={8}>
          <Activity size={16} />
          <Text strong>相关流水线</Text>
          {running ? <Tag color="processing">{running} 运行中</Tag> : null}
          {failed ? <Tag color="error">{failed} 失败/取消</Tag> : null}
        </Space>
        <Segmented
          size="small"
          value={pipelineView}
          onChange={(value) => setPipelineView(value as DeployPipelineView)}
          options={[
            { label: `全部 ${item.pipelines.length}`, value: "all" },
            { label: `运行 ${running}`, value: "active" },
            { label: `失败 ${failed}`, value: "failed" }
          ]}
        />
      </div>
      {visiblePipelines.length ? (
        <DeployObjectPipelineTable rows={visiblePipelines} woodpecker={woodpecker} nowMs={nowMs} onCancel={onCancel} onInspect={onInspect} />
      ) : (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={item.pipelines.length ? "当前筛选下没有流水线" : "这个对象还没有相关流水线记录"} />
      )}
    </div>
  );
}

function DeployObjectMobileList({
  objects,
  state,
  woodpecker,
  nowMs,
  currentUser,
  triggeringTaskIDSet,
  onRun,
  onSelect,
  query,
  onQueryChange,
  onConfigure
}: {
  objects: DeployObject[];
  state: StateResponse;
  woodpecker: string;
  nowMs: number;
  currentUser: User;
  triggeringTaskIDSet: Set<string>;
  onRun: TaskRunHandler;
  onSelect: (id: string) => void;
  query: string;
  onQueryChange: (value: string) => void;
  onConfigure: () => void;
}) {
  return (
    <div className="mobile-deploy-object-list">
      <Card className="mobile-deploy-object-toolbar-card">
        <Space direction="vertical" size={10} className="side-stack">
          <Input
            allowClear
            prefix={<Search size={15} />}
            placeholder="搜索项目、环境、仓库、分支"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
          />
          <Button block icon={<Settings size={15} />} onClick={onConfigure}>配置部署项目</Button>
        </Space>
      </Card>
      <List
        dataSource={objects}
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的部署项目" /> }}
        pagination={objects.length > 8 ? { pageSize: 8, size: "small" } : false}
        renderItem={(item) => (
          <List.Item>
            <Space direction="vertical" size={10} className="side-stack">
              <List.Item.Meta
                title={
                  <Space wrap>
                    <Text strong>{item.title}</Text>
                    <DeployObjectStateTags item={item} />
                  </Space>
                }
                description={
                  <Space direction="vertical" size={4} className="side-stack">
                    <Text type="secondary">{item.subtitle}</Text>
                    <Text type="secondary">{item.environmentLabel} · {item.branchLabel}</Text>
                    <DeployObjectVersionCell item={item} nowMs={nowMs} state={state} />
                    <DeployObjectPipelineOverview row={item.pipelines[0]} nowMs={nowMs} />
                  </Space>
                }
              />
              <Space size={8} wrap>
                <DeployObjectRowActions item={item} woodpecker={woodpecker} currentUser={currentUser} triggeringTaskIDSet={triggeringTaskIDSet} onRun={onRun} />
                <Button size="small" icon={<ScrollText size={14} />} onClick={() => onSelect(item.id)}>{deployObjectRollbackTask(item) ? "详情/回滚" : "详情"}</Button>
              </Space>
            </Space>
          </List.Item>
        )}
      />
    </div>
  );
}

type DeployPipelineView = "active" | "failed" | "all";

function filterDeployPipelines(rows: Pipeline[], view: DeployPipelineView): Pipeline[] {
  if (view === "active") return rows.filter((row) => ["running", "pending"].includes(row.status));
  if (view === "failed") return rows.filter((row) => ["failure", "error", "killed"].includes(row.status));
  return rows;
}

function DeployObjectMetric({ label, value, accent = false }: { label: string; value: string; accent?: boolean }) {
  return (
    <div className={`deploy-object-metric ${accent ? "deploy-object-metric-accent" : ""}`}>
      <Text type="secondary">{label}</Text>
      <Text strong ellipsis={{ tooltip: value }}>{value}</Text>
    </div>
  );
}

function DeployObjectPipelineTable({
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
      width: 132,
      render: (_, row) => (
        <Space direction="vertical" size={0} className="table-cell-stack">
          <Space size={6}>
            <Text strong>#{row.number}</Text>
            <Tag color={statusColors[row.status] || "default"}>{statusText(row.status)}</Tag>
          </Space>
          <Text type="secondary">{pipelineKindText(row)}</Text>
        </Space>
      )
    },
    {
      title: "版本",
      width: 150,
      render: (_, row) => (
        <Space direction="vertical" size={0} className="table-cell-stack">
          <Text ellipsis={{ tooltip: row.branch || "-" }}>{row.branch || "-"}</Text>
          <Text type="secondary">{(row.commit || "").slice(0, 8) || "-"}</Text>
        </Space>
      )
    },
    {
      title: "动作",
      width: 190,
      render: (_, row) => (
        <Space direction="vertical" size={0} className="table-cell-stack">
          <Text ellipsis={{ tooltip: pipelineTaskText(row) }}>{pipelineTaskText(row)}</Text>
          {pipelineVariableHint(row) && (
            <Tooltip title={pipelineVariableHint(row)}>
              <Text type="secondary" className="pipeline-variable-hint">{pipelineVariableHint(row)}</Text>
            </Tooltip>
          )}
        </Space>
      )
    },
    {
      title: "时间",
      width: 150,
      render: (_, row) => (
        <Space direction="vertical" size={0} className="table-cell-stack">
          <Text type="secondary">{pipelineTimeText(row)}</Text>
          <Text>{pipelineDurationText(row, nowMs)}</Text>
        </Space>
      )
    },
    {
      title: "",
      width: 116,
      render: (_, row) => (
        <Space size={6}>
          <Button size="small" onClick={() => onInspect(row)}>详情</Button>
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
        className="desktop-pipeline-table deploy-object-pipeline-table"
        rowKey={(row) => `${row.repo_id}-${row.number}`}
        size="small"
        columns={columns}
        dataSource={rows}
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无相关流水线" /> }}
        search={false}
        options={false}
        tableAlertRender={false}
        pagination={rows.length > 8 ? { pageSize: 8, showSizeChanger: false, showTotal: (total) => `共 ${total} 条` } : false}
        scroll={{ x: 760 }}
        tableLayout="fixed"
      />
      <List
        className="mobile-pipeline-list deploy-object-mobile-pipeline-list"
        dataSource={rows}
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无相关流水线" /> }}
        pagination={rows.length > 8 ? { pageSize: 8, size: "small" } : false}
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
              title={<Space><Text strong>#{row.number}</Text><Tag color={statusColors[row.status] || "default"}>{statusText(row.status)}</Tag></Space>}
              description={
                <Space direction="vertical" size={4} className="side-stack">
                  <Text type="secondary">{row.branch || "-"} · {(row.commit || "").slice(0, 8) || "-"}</Text>
                  <Text>{pipelineTaskText(row)}</Text>
                  <Text type="secondary">{pipelineTimeText(row)} · {pipelineDurationText(row, nowMs)}</Text>
                </Space>
              }
            />
          </List.Item>
        )}
      />
    </>
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
  const runningCount = rows.filter((row) => row.status === "running").length;
  const pendingCount = rows.filter((row) => row.status === "pending").length;
  const failedCount = recentFailedPipelineCount(rows, nowMs);
  return (
    <Space direction="vertical" size={12} className="side-stack">
      <PageIntro
        title="流水线"
        description="队列、耗时、触发人、分支和失败摘要集中查看。"
        stats={[
          { label: "运行中", value: String(runningCount), tone: runningCount ? "warning" : "normal" },
          { label: "排队", value: String(pendingCount), tone: pendingCount ? "warning" : "normal" },
          { label: "24h 失败", value: String(failedCount), tone: failedCount ? "danger" : "success" }
        ]}
        actions={<Button icon={<RefreshCw size={16} />} loading={refreshing} onClick={onRefresh}>刷新</Button>}
      />
      {runningRows.length > 0 && (
        <Card title="运行中队列">
          <PipelineActivityList rows={runningRows} nowMs={nowMs} onInspect={onInspect} />
        </Card>
      )}
      <Card title="流水线历史">
        <PipelineTable rows={rows} woodpecker={woodpecker} nowMs={nowMs} onCancel={onCancel} onInspect={onInspect} />
      </Card>
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
  const screens = Grid.useBreakpoint();
  const compactSettingsNav = !screens.md;
  const [activeSetting, setActiveSetting] = useState(() => settingsSectionFromHash());
  const settingItems = [
    {
      key: "setup",
      label: "接入向导",
      shortLabel: "接入",
      children: state.current_user.role === "admin" ? <SetupConfigPanel onReload={onReload} /> : <Alert type="info" showIcon message="接入配置只允许管理员查看和修改" />
    },
    {
      key: "core",
      label: "核心服务",
      shortLabel: "核心",
      children: state.current_user.role === "admin" ? <SetupConfigPanel onReload={onReload} initialSection="core" /> : <Alert type="info" showIcon message="核心服务配置只允许管理员查看和修改" />
    },
    {
      key: "hosts",
      label: "环境与机器",
      shortLabel: "机器",
      children: state.current_user.role === "admin" ? <SetupConfigPanel onReload={onReload} initialSection="hosts" /> : <Alert type="info" showIcon message="机器配置只允许管理员查看和修改" />
    },
    {
      key: "repos",
      label: "仓库与任务",
      shortLabel: "仓库",
      children: (
        <Space direction="vertical" size={16} className="side-stack">
          {state.current_user.role === "admin" ? <RepositoryConfigPanel state={state} onReload={onReload} /> : <Alert type="info" showIcon message="仓库配置只允许管理员查看和修改" />}
          {state.current_user.role === "admin" && state.configurable && <TaskTemplatePanel state={state} onApplied={onReload} />}
          {state.configurable ? (
            <TaskConfigView state={state} config={customConfig} tasks={state.tasks || []} onAdd={onAddTask} onEdit={onEditTask} onDelete={onDeleteTask} />
          ) : (
            <Alert type="info" showIcon message="当前环境未开启任务配置文件" />
          )}
        </Space>
      )
    },
    { key: "links", label: "外部入口", shortLabel: "入口", children: <InfrastructureLinks tasks={state.tasks || []} compact /> },
    {
      key: "logs",
      label: "日志策略",
      shortLabel: "日志",
      children: state.current_user.role === "admin" ? <SetupConfigPanel onReload={onReload} initialSection="logs" /> : <Alert type="info" showIcon message="日志策略只允许管理员查看和修改" />
    },
    {
      key: "account",
      label: "账号与成员",
      shortLabel: "成员",
      children: (
        <Space direction="vertical" size={16} className="side-stack">
          <Profile state={state} onReload={onReload} />
          {state.current_user.role === "admin" && state.auth_mode === "db" && <Users />}
        </Space>
      )
    },
    { key: "audit", label: "操作历史", shortLabel: "历史", children: <AuditLogView records={auditRecords} loading={auditLoading} state={state} onRefresh={onAuditRefresh} /> },
    { key: "docs", label: "参数文档", shortLabel: "文档", children: <Docs state={state} compact /> }
  ];
  const activeSettingItem = settingItems.find((item) => item.key === activeSetting) || settingItems[0];
  useEffect(() => {
    function syncSettingsSection() {
      const next = settingsSectionFromHash();
      if (settingItems.some((item) => item.key === next)) {
        setActiveSetting(next);
      }
    }
    syncSettingsSection();
    window.addEventListener("hashchange", syncSettingsSection);
    window.addEventListener("popstate", syncSettingsSection);
    return () => {
      window.removeEventListener("hashchange", syncSettingsSection);
      window.removeEventListener("popstate", syncSettingsSection);
    };
  }, []);
  function changeSetting(key: string) {
    setActiveSetting(key);
    const nextHash = `#/settings/${key}`;
    if (window.location.hash !== nextHash) {
      window.history.pushState(null, "", `${window.location.pathname}${nextHash}`);
    }
  }
  return (
    <Space direction="vertical" size={16} className="side-stack">
      <PageIntro
        title="设置"
        description="成员、仓库、任务和底层入口集中维护。日常部署不用进入这里。"
      />
      {compactSettingsNav ? (
        <Space direction="vertical" size={12} className="side-stack">
          <Card className="settings-mobile-nav-card">
            <Space direction="vertical" size={8} className="side-stack">
              <Text type="secondary">设置分组</Text>
              <Select
                className="full-width"
                value={activeSetting}
                onChange={changeSetting}
                options={settingItems.map((item) => ({ value: item.key, label: item.label }))}
              />
            </Space>
          </Card>
          <div className="settings-mobile-content">{activeSettingItem.children}</div>
        </Space>
      ) : (
        <Tabs
          className="settings-tabs"
          activeKey={activeSetting}
          onChange={changeSetting}
          items={settingItems.map((item) => ({
            key: item.key,
            label: item.shortLabel,
            children: item.children
          }))}
        />
      )}
    </Space>
  );
}

export function TaskRunContext({ task, statuses, nowMs }: { task: Task; statuses: DeploymentStatus[]; nowMs: number }) {
  const status = deploymentStatusForTask(task, statuses);
  if (!status) return null;
  const rollback = isRollbackTask(task);
  const currentVersion = status.current_branch ? `${status.current_branch} · ${(status.current_commit || "").slice(0, 8) || "-"}` : "暂无成功部署";
  const currentAge = status.last_deployed_at ? deployedAgeText(status.last_deployed_at, nowMs) : "";
  const latestRun = `${status.latest_action || "-"} · ${statusText(status.latest_status)}`;
  const latestAge = status.latest_at ? deployedAgeText(status.latest_at, nowMs) : "";
  const rollbackTargets = rollbackRevisionsForStatus(status);
  const rollbackTarget = rollbackTargets[0] ? rollbackRevisionLabel(rollbackTargets[0], nowMs) : "没有可选成功版本";
  return (
    <div className="run-context-strip">
      <RunContextMetric label="线上" value={currentVersion} meta={currentAge} accent />
      <RunContextMetric label="最近" value={latestRun} meta={latestAge} />
      {rollback && <RunContextMetric label="回滚目标" value={rollbackTarget} meta={rollbackTargets.length > 1 ? `可选 ${rollbackTargets.length} 个历史版本` : undefined} />}
    </div>
  );
}

function RunContextMetric({ label, value, meta, accent = false }: { label: string; value: string; meta?: string; accent?: boolean }) {
  return (
    <div className={`run-context-metric ${accent ? "run-context-metric-accent" : ""}`}>
      <Text type="secondary">{label}</Text>
      <Text strong ellipsis={{ tooltip: value }}>{value}</Text>
      {meta && <Text type="secondary">{meta}</Text>}
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
  onRun: TaskRunHandler;
  onEdit?: (task: Task) => void;
  onDelete?: (task: Task) => void;
}) {
  const triggeringTaskIDSet = useMemo(() => new Set(triggeringTaskIds), [triggeringTaskIds]);
  const deploymentTaskIDs = useMemo(() => new Set(deploymentManagedTaskIDs(state.tasks || [], state.deployment_statuses || [])), [state.tasks, state.deployment_statuses]);
  const data = (state.tasks || []).filter((item) => {
    if (item.external_url) return false;
    if (isDeployProjectTask(item)) return false;
    if (deploymentTaskIDs.has(item.id)) return false;
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
  }).sort(taskLibrarySort);
  const columns: ProColumns<Task>[] = [
    {
      title: "动作",
      dataIndex: "title",
      width: 300,
      render: (_, row) => (
        <Space direction="vertical" size={0} className="table-cell-stack">
          <Text strong ellipsis={{ tooltip: productText(row.title) }}>{productText(row.title)}</Text>
          <Text type="secondary" ellipsis={{ tooltip: taskDescriptionLine(state, row) }}>{taskDescriptionLine(state, row)}</Text>
        </Space>
      )
    },
    {
      title: "上下文",
      width: 260,
      render: (_, row) => (
        <Space direction="vertical" size={0} className="table-cell-stack">
          <Text ellipsis={{ tooltip: taskGroupLabel(state, row) }}>{taskGroupLabel(state, row)}</Text>
          <Text type="secondary" ellipsis={{ tooltip: repoName(state, row) }}>{repoName(state, row)} · {row.branch || "main"}</Text>
        </Space>
      )
    },
    {
      title: "关键参数",
      width: 260,
      render: (_, row) => (
        <Space direction="vertical" size={0} className="table-cell-stack">
          <Text ellipsis={{ tooltip: pipelineTaskText(taskToPipelinePreview(state, row)) }}>{pipelineTaskText(taskToPipelinePreview(state, row))}</Text>
          {pipelineVariableHint(taskToPipelinePreview(state, row)) && (
            <Tooltip title={pipelineVariableHint(taskToPipelinePreview(state, row))}>
              <Text type="secondary" className="pipeline-variable-hint">{pipelineVariableHint(taskToPipelinePreview(state, row))}</Text>
            </Tooltip>
          )}
          {row.confirm_text && <Text type="secondary">确认词：{row.confirm_text}</Text>}
        </Space>
      )
    },
    {
      title: "风险",
      width: 96,
      render: (_, row) => (
        <Space direction="vertical" size={2}>
          <Tag color={riskColors[row.risk] || "default"}>{riskLabel(row.risk)}</Tag>
          {!canRunTask(state.current_user, row) && <Text type="secondary">仅管理员</Text>}
        </Space>
      )
    },
    {
      title: "",
      width: 150,
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
        size="small"
        columns={columns}
        dataSource={data}
        search={false}
        options={false}
        tableAlertRender={false}
        pagination={data.length > 8 ? {
          pageSize: 8,
          showSizeChanger: true,
          pageSizeOptions: [8, 16, 32],
          showTotal: (total) => `共 ${total} 个动作`
        } : false}
        scroll={{ x: 980 }}
        tableLayout="fixed"
        expandable={{
          expandedRowRender: (row) => <TaskDetailPanel state={state} task={row} />,
          rowExpandable: () => true
        }}
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
                  <Text strong>{productText(row.title)}</Text>
                  <Tag color={riskColors[row.risk] || "default"}>{riskLabel(row.risk)}</Tag>
                  {!canRunTask(state.current_user, row) && <Tag>仅管理员</Tag>}
                </Space>
              }
              description={
                <Space direction="vertical" size={4}>
                  <Text type="secondary">{productText(row.description)}</Text>
                  <Text>{taskGroupLabel(state, row)} · {repoName(state, row)} · {row.branch || "main"}</Text>
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

function TaskDetailPanel({ state, task }: { state: StateResponse; task: Task }) {
  const variables = safeVariablesTextForDisplay(task.variables || {});
  return (
    <div className="task-detail-panel">
      <Descriptions size="small" column={{ xs: 1, md: 2 }} bordered>
        <Descriptions.Item label="任务 ID">{task.id}</Descriptions.Item>
        <Descriptions.Item label="仓库">{repoName(state, task)}</Descriptions.Item>
        <Descriptions.Item label="模块">{taskGroupLabel(state, task)}</Descriptions.Item>
        <Descriptions.Item label="默认分支">{task.branch || "main"}</Descriptions.Item>
        <Descriptions.Item label="确认词">{task.confirm_text || "无需确认词"}</Descriptions.Item>
        <Descriptions.Item label="权限">{(task.allowed_roles || []).join("、") || "所有登录用户"}</Descriptions.Item>
      </Descriptions>
      {variables && (
        <pre className="task-variable-block">{variables}</pre>
      )}
    </div>
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
  const deploymentTaskIDs = new Set(deploymentManagedTaskIDs(state.tasks || [], state.deployment_statuses || []));
  const tasks = (state.tasks || []).filter((item) => !item.external_url && !isDeployProjectTask(item) && !deploymentTaskIDs.has(item.id));
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
        description={`${PRODUCT_NAME} 已经记录这次失败。优先检查 Woodpecker 仓库 ID、分支、token 权限、仓库 Trusted/Secrets 配置，以及 Woodpecker Server 日志。`}
      />
      <Descriptions size="small" column={1} bordered>
        <Descriptions.Item label="任务">{productText(task.title)}</Descriptions.Item>
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
      width: 190,
      render: (_, row) => (
        <Space direction="vertical" size={0} className="table-cell-stack">
          <Text strong ellipsis={{ tooltip: `${productText(row.repo_name)} #${row.number}` }}>{productText(row.repo_name)} #{row.number}</Text>
          <Text type="secondary">{pipelineKindText(row)}</Text>
        </Space>
      )
    },
    {
      title: "代码版本",
      width: 140,
      render: (_, row) => (
        <Space direction="vertical" size={0} className="table-cell-stack">
          <Text ellipsis={{ tooltip: row.branch || "-" }}>{row.branch || "-"}</Text>
          <Text type="secondary">{(row.commit || "").slice(0, 8) || "-"}</Text>
        </Space>
      )
    },
    {
      title: "动作",
      width: 170,
      render: (_, row) => (
        <Space direction="vertical" size={0} className="table-cell-stack">
          <Text ellipsis={{ tooltip: pipelineTaskText(row) }}>{pipelineTaskText(row)}</Text>
          {pipelineVariableHint(row) && (
            <Tooltip title={pipelineVariableHint(row)}>
              <Text type="secondary" className="pipeline-variable-hint">{pipelineVariableHint(row)}</Text>
            </Tooltip>
          )}
        </Space>
      )
    },
    {
      title: "触发",
      width: 140,
      render: (_, row) => <Text type="secondary" ellipsis={{ tooltip: pipelineTriggerText(row) }}>{pipelineTriggerText(row)}</Text>
    },
    {
      title: "时间",
      width: 130,
      render: (_, row) => <Text type="secondary" ellipsis={{ tooltip: pipelineTimeText(row) }}>{pipelineTimeText(row)}</Text>
    },
    {
      title: "耗时",
      width: 92,
      render: (_, row) => <Text type="secondary">{pipelineDurationText(row, nowMs)}</Text>
    },
    {
      title: "状态",
      width: 82,
      render: (_, row) => <Tag color={statusColors[row.status] || "default"}>{statusText(row.status)}</Tag>
    },
    {
      title: "进度",
      width: 96,
      render: (_, row) => <Progress percent={pipelinePercent(row, nowMs)} size="small" status={progressStatus(row)} />
    },
    {
      title: "",
      width: 118,
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
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无流水线" /> }}
        search={false}
        options={false}
        tableAlertRender={false}
        pagination={rows.length > 12 ? { pageSize: 12, showSizeChanger: true, pageSizeOptions: [12, 30], showTotal: (total) => `共 ${total} 条` } : false}
        scroll={{ x: 1060 }}
        tableLayout="fixed"
      />
      <List
        className="mobile-pipeline-list"
        dataSource={rows}
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无流水线" /> }}
        pagination={rows.length > 12 ? { pageSize: 12, size: "small" } : false}
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
              title={<Space><Text strong>{productText(row.repo_name)} #{row.number}</Text><Tag color={statusColors[row.status] || "default"}>{statusText(row.status)}</Tag></Space>}
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
      className="pipeline-summary-drawer"
      title={pipeline ? `${productText(pipeline.repo_name || "Repo")} #${pipeline.number}` : "流水线详情"}
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
  onRun: TaskRunHandler;
}) {
  const cleanupTasks = (state.tasks || []).filter(isCleanupTask);
  const links = summary?.links || state.links || {};
  const hosts = summary?.hosts || [];
  const alerts = summary?.alerts || [];
  const alertLevel = highestMonitoringAlertLevel(alerts);
  return (
    <Space direction="vertical" size={12} className="side-stack">
      <PageIntro
        title="监控"
        description="机器资源、核心容器和降级原因集中查看。"
        actions={
          <Space wrap>
            {links.beszel && <Button href={links.beszel} target="_blank" icon={<ExternalLink size={15} />}>Beszel</Button>}
            {links.dozzle && <Button href={links.dozzle} target="_blank" icon={<ExternalLink size={15} />}>Dozzle</Button>}
            {links.grafana && <Button href={links.grafana} target="_blank" icon={<ExternalLink size={15} />}>Grafana</Button>}
            <Button icon={<RefreshCw size={16} />} loading={loading} onClick={onRefresh}>刷新</Button>
            <DiskDiagnosisButton />
          </Space>
        }
      />

      {summary?.degraded_reason && (
        <Alert className="monitoring-alert-inline" type="warning" showIcon message="监控已降级" description={summary.degraded_reason} />
      )}
      <Card loading={!summary && loading}>
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
  const normalCount = hosts.filter((host) => ["success", "gold"].includes(monitoringStatusColor(host.status))).length;
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
              {alert.action_url && alert.action_label && (
                <Button type="link" size="small" onClick={() => window.location.hash = alert.action_url!}>
                  {alert.action_label}
                </Button>
              )}
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
  cleanupTasks: Task[];
  currentUser: User;
  onRun: TaskRunHandler;
}) {
  const columns: ColumnsType<MonitoringHost> = [
    {
      title: <MonitoringColumnTitle icon={<Server size={15} />} label="系统" />,
      width: 300,
      sorter: (a, b) => a.name.localeCompare(b.name),
      render: (_, row) => (
        <div className="monitor-system-cell">
          <span className={`system-dot system-dot-${monitoringStatusColor(row.status)}`} />
          <div className="monitor-system-copy">
            <Space size={6} wrap className="monitor-system-title">
              <Text strong>{row.name}</Text>
              <Tag color={monitoringStatusColor(row.status)}>{monitoringHostStatusText(row.status)}</Tag>
            </Space>
            <Text type="secondary" className="monitor-system-meta">{monitoringHostMeta(row)}</Text>
          </div>
        </div>
      )
    },
    {
      title: <MonitoringColumnTitle icon={<Cpu size={15} />} label="CPU" />,
      width: 118,
      sorter: (a, b) => (a.cpu_percent || 0) - (b.cpu_percent || 0),
      render: (_, row) => <CompactMetric value={row.cpu_percent || 0} />
    },
    {
      title: <MonitoringColumnTitle icon={<MemoryStick size={15} />} label="内存" />,
      width: 124,
      sorter: (a, b) => (a.memory_percent || 0) - (b.memory_percent || 0),
      render: (_, row) => <CompactMetric value={row.memory_percent || 0} />
    },
    {
      title: <MonitoringColumnTitle icon={<HardDrive size={15} />} label="磁盘" />,
      width: 124,
      sorter: (a, b) => (a.disk_percent || 0) - (b.disk_percent || 0),
      render: (_, row) => {
        const diskValue = row.disk_percent || 0;
        const reclaimable = row.docker_reclaimable_bytes;
        const tip = reclaimable ? `磁盘 ${diskValue}% · Docker 可回收 ${formatBytes(reclaimable)}` : `磁盘 ${diskValue}%`;
        return <Tooltip title={tip}><CompactMetric value={diskValue} /></Tooltip>;
      }
    },
    {
      title: <MonitoringColumnTitle icon={<Gauge size={15} />} label="负载" />,
      width: 130,
      sorter: (a, b) => (a.load_1 || 0) - (b.load_1 || 0),
      render: (_, row) => (
        <Space size={6} className="monitor-load-cell">
          <span className={`system-dot system-dot-${metricTone((row.load_1 || 0) * 25)}`} />
          <Text>{formatLoad(row.load_1)}</Text>
          <Text type="secondary">{formatLoad(row.load_5)}</Text>
          <Text type="secondary">{formatLoad(row.load_15)}</Text>
        </Space>
      )
    },
    {
      title: <MonitoringColumnTitle icon={<Network size={15} />} label="网络" />,
      width: 118,
      sorter: (a, b) => (a.network_bytes_per_second || 0) - (b.network_bytes_per_second || 0),
      render: (_, row) => <Text>{formatBytes(row.network_bytes_per_second || 0)}/s</Text>
    },
    {
      title: <MonitoringColumnTitle icon={<Clock3 size={15} />} label="运行时间" />,
      width: 112,
      sorter: (a, b) => (a.uptime_seconds || 0) - (b.uptime_seconds || 0),
      render: (_, row) => <Text>{row.uptime || "-"}</Text>
    },
    {
      title: "来源",
      width: 80,
      render: (_, row) => <Tag color={monitoringSourceColor(row.source)}>{monitoringSourceText(row.source)}</Tag>
    },
    {
      title: "操作",
      width: 104,
      render: (_, row) => (
        <MonitoringHostActions
          cleanupTask={cleanupTaskForMonitoringHost(row, cleanupTasks)}
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
        size="small"
        columns={columns}
        dataSource={rows}
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无机器资源" /> }}
        pagination={false}
        scroll={{ x: 1110 }}
      />
      <List
        className="mobile-monitor-system-list"
        dataSource={rows}
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无机器资源" /> }}
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
                  <Text type="secondary">{monitoringHostMeta(row)}</Text>
                  <div className="mobile-monitor-metrics">
                    <CompactMetric label="CPU" value={row.cpu_percent || 0} />
                    <CompactMetric label="内存" value={row.memory_percent || 0} />
                    <CompactMetric label="磁盘" value={row.disk_percent || 0} />
                  </div>
                  <div className="mobile-monitor-runline">
                    <Text type="secondary">负载 {formatLoad(row.load_1)} {formatLoad(row.load_5)} {formatLoad(row.load_15)}</Text>
                    <Text type="secondary">网络 {formatBytes(row.network_bytes_per_second || 0)}/s</Text>
                    <Text type="secondary">{row.uptime || "-"}</Text>
                  </div>
                  <MonitoringHostActions
                    cleanupTask={cleanupTaskForMonitoringHost(row, cleanupTasks)}
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

function MonitoringColumnTitle({ icon, label }: { icon: ReactNode; label: string }) {
  return (
    <Space size={6} className="monitor-column-title">
      {icon}
      <span>{label}</span>
    </Space>
  );
}

function MonitoringHostActions({
  cleanupTask,
  currentUser,
  onRun
}: {
  cleanupTask?: Task;
  currentUser: User;
  onRun: TaskRunHandler;
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

function DiskDiagnosisButton() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button icon={<HardDrive size={16} />} onClick={() => setOpen(true)}>磁盘诊断</Button>
      <DiskDiagnosisDrawer open={open} onClose={() => setOpen(false)} />
    </>
  );
}

function DiskDiagnosisDrawer({
  open,
  onClose
}: {
  open: boolean;
  onClose: () => void;
}) {
  const [loading, setLoading] = useState(false);
  const [diagnosis, setDiagnosis] = useState<DiskDiagnosisResponse | null>(null);
  const [preview, setPreview] = useState<DiskCleanupPreviewResponse | null>(null);
  const [cleanupLevel, setCleanupLevel] = useState<string>("safe");
  const [confirmText, setConfirmText] = useState("");
  const [forceRunning, setForceRunning] = useState(false);
  const [cleaning, setCleaning] = useState(false);
  const [cleanupResult, setCleanupResult] = useState<string>("");
  const [autoCleanupConfig, setAutoCleanupConfig] = useState<{ level: string; disk: number } | null>(null);

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    setCleanupResult("");
    Promise.all([
      fetchDiskDiagnosis().then(setDiagnosis),
      fetchDiskCleanupPreview().then(setPreview),
      api<{ config: { monitor_auto_cleanup_level: string; monitor_auto_cleanup_disk: number } }>("/api/setup/config").then(
        (data) => setAutoCleanupConfig({ level: data.config.monitor_auto_cleanup_level, disk: data.config.monitor_auto_cleanup_disk })
      ).catch(() => {})
    ]).finally(() => setLoading(false));
  }, [open]);

  const doCleanup = async () => {
    setCleaning(true);
    setCleanupResult("");
    try {
      const result = await executeDiskCleanup({ level: cleanupLevel, confirm: confirmText, force_running: forceRunning });
      setCleanupResult(`✅ 清理完成：回收 ${result.reclaimed} · ${result.details}`);
      setConfirmText("");
      const [newDiagnosis, newPreview] = await Promise.all([fetchDiskDiagnosis(), fetchDiskCleanupPreview()]);
      setDiagnosis(newDiagnosis);
      setPreview(newPreview);
    } catch (err: unknown) {
      setCleanupResult(`❌ 清理失败：${errorText(err)}`);
    } finally {
      setCleaning(false);
    }
  };

  const fs = diagnosis?.filesystems?.[0];
  const docker = diagnosis?.docker;
  const topDirs = diagnosis?.top_dirs || [];
  const levels = preview?.levels || [];
  const protectedImages = preview?.protected_images || [];
  const protectedVolumes = preview?.protected_volumes || [];

  return (
    <Drawer title="磁盘诊断" open={open} onClose={onClose} width={560} destroyOnClose>
      <Spin spinning={loading}>
        <Space direction="vertical" size={16} className="side-stack">
          {fs && (
            <Card size="small" title="文件系统">
              <Descriptions size="small" column={1}>
                <Descriptions.Item label="挂载点">{fs.mount}</Descriptions.Item>
                <Descriptions.Item label="总容量">{fs.total}</Descriptions.Item>
                <Descriptions.Item label="已用">{fs.used}</Descriptions.Item>
                <Descriptions.Item label="使用率">
                  <Progress percent={fs.percent} size="small" status={fs.percent >= 90 ? "exception" : fs.percent >= 80 ? "active" : "normal"} />
                </Descriptions.Item>
              </Descriptions>
            </Card>
          )}
          {docker && diagnosis?.docker_ok && (
            <Card size="small" title="Docker 空间">
              <Descriptions size="small" column={1}>
                <Descriptions.Item label="镜像">{docker.images_total} 个（{docker.images_size}，可回收 {docker.images_reclaimable}）</Descriptions.Item>
                <Descriptions.Item label="构建缓存">{docker.build_cache_size}（可回收 {docker.build_reclaimable}）</Descriptions.Item>
                <Descriptions.Item label="卷">{docker.volumes_size}（可回收 {docker.volumes_reclaimable}）</Descriptions.Item>
              </Descriptions>
            </Card>
          )}
          {diagnosis?.waste_breakdown && (
            <Card size="small" title="可回收空间汇总">
              <Descriptions size="small" column={1}>
                <Descriptions.Item label="构建缓存">{diagnosis.waste_breakdown.build_cache || "0B"}</Descriptions.Item>
                <Descriptions.Item label="悬空镜像">{diagnosis.waste_breakdown.dangling_images || "0B"}</Descriptions.Item>
                <Descriptions.Item label="孤儿卷">{diagnosis.waste_breakdown.orphan_volumes || "0B"}</Descriptions.Item>
                <Descriptions.Item label="容器日志">{diagnosis.waste_breakdown.container_logs || "0B"}</Descriptions.Item>
                <Descriptions.Item label={<Text strong>总计可回收</Text>}>{diagnosis.waste_breakdown.total_reclaimable}</Descriptions.Item>
              </Descriptions>
            </Card>
          )}
          {(diagnosis?.images || []).length > 0 && (
            <Card size="small" title={`大镜像排行（前 ${(diagnosis!.images || []).length}）`}>
              <List size="small" dataSource={diagnosis!.images!} renderItem={(item) => (
                <List.Item>
                  <Space direction="vertical" size={0} style={{ width: "100%" }}>
                    <Space>
                      <Text code ellipsis style={{ maxWidth: 260 }}>{item.dangling ? "<none>:<none>" : `${item.repository}:${item.tag}`}</Text>
                      {item.dangling && <Tag color="orange">悬空</Tag>}
                    </Space>
                    <Text type="secondary">{item.size} · {item.id.slice(0, 12)}</Text>
                  </Space>
                </List.Item>
              )} />
            </Card>
          )}
          {(diagnosis?.volumes || []).filter((v) => v.orphan).length > 0 && (
            <Card size="small" title="孤儿卷">
              <List size="small" dataSource={diagnosis!.volumes!.filter((v) => v.orphan)} renderItem={(item) => (
                <List.Item><Text code>{item.name}</Text><Text type="secondary">{item.driver}</Text></List.Item>
              )} />
            </Card>
          )}
          {(diagnosis?.log_files || []).length > 0 && (
            <Card size="small" title="容器日志文件（前 10）">
              <List size="small" dataSource={diagnosis!.log_files!} renderItem={(item) => (
                <List.Item><Text code>{item.path.split("/").pop()}</Text><Text>{item.size}</Text></List.Item>
              )} />
            </Card>
          )}
          {topDirs.length > 0 && (
            <Card size="small" title="大目录排行">
              <List size="small" dataSource={topDirs} renderItem={(item) => <List.Item><Text code>{item.path}</Text><Text>{item.size}</Text></List.Item>} />
            </Card>
          )}
          {levels.length > 0 && (
            <Card size="small" title="分级清理">
              <Radio.Group value={cleanupLevel} onChange={(e) => setCleanupLevel(e.target.value)}>
                <Space direction="vertical">
                  {levels.map((level) => (
                    <Radio key={level.level} value={level.level}>
                      <Space direction="vertical" size={2}>
                        <Text strong>{level.description}</Text>
                        <Text type="secondary">可回收 {level.reclaimable} · {level.risk}</Text>
                      </Space>
                    </Radio>
                  ))}
                </Space>
              </Radio.Group>
              <div style={{ marginTop: 12 }}>
                <Input.Password placeholder="输入 CLEAN 确认清理" value={confirmText} onChange={(e) => setConfirmText(e.target.value)} style={{ width: 240 }} />
                <Button type="primary" danger loading={cleaning} disabled={confirmText !== "CLEAN"} onClick={doCleanup} style={{ marginLeft: 8 }}>执行清理</Button>
              </div>
              {preview?.requires_force_running && (cleanupLevel === "standard" || cleanupLevel === "deep") && (
                <div style={{ marginTop: 12 }}>
                  <Checkbox checked={forceRunning} onChange={(e) => setForceRunning(e.target.checked)}>
                    强制停止运行中的容器（当前 {preview.running_containers} 个，{cleanupLevel} 会停止它们）
                  </Checkbox>
                </div>
              )}
              {(protectedImages.length > 0 || protectedVolumes.length > 0) && (
                <Alert style={{ marginTop: 12 }} type="warning" showIcon message="受保护资源不会被清理"
                  description={
                    <div>
                      {protectedImages.length > 0 && <div>镜像：{protectedImages.join("、")}</div>}
                      {protectedVolumes.length > 0 && <div>卷：{protectedVolumes.join("、")}</div>}
                    </div>
                  }
                />
              )}
              {cleanupResult && <Alert style={{ marginTop: 12 }} type={cleanupResult.startsWith("✅") ? "success" : "error"} message={cleanupResult} />}
            </Card>
          )}
          {!diagnosis?.docker_ok && (
            <Alert type="info" showIcon message="Docker 不可用" description="当前环境无法访问 Docker 守护进程，诊断和清理功能受限。请通过 SSH 监控查看磁盘状态。" />
          )}
          {autoCleanupConfig && (autoCleanupConfig.level || autoCleanupConfig.disk > 0) && (
            <Card size="small" title="自动清理配置">
              <Descriptions size="small" column={1}>
                <Descriptions.Item label="清理级别">{autoCleanupConfig.level || "禁用"}</Descriptions.Item>
                <Descriptions.Item label="触发阈值">{autoCleanupConfig.disk > 0 ? `${autoCleanupConfig.disk}%` : "禁用"}</Descriptions.Item>
                <Descriptions.Item label="冷却间隔">30 分钟</Descriptions.Item>
              </Descriptions>
            </Card>
          )}
        </Space>
      </Spin>
    </Drawer>
  );
}

function cleanupTaskForMonitoringHost(host: MonitoringHost, tasks: Task[]): Task | undefined {
  if (!tasks.length) return undefined;
  if (host.cleanup_task_id) {
    const explicit = tasks.find((task) => task.id === host.cleanup_task_id);
    if (explicit) return explicit;
  }
  const hostText = normalizeKey([host.id, host.name, host.role].filter(Boolean).join(" "));
  const scored = tasks
    .map((task) => ({ task, score: cleanupTaskHostMatchScore(hostText, task) }))
    .filter((item) => item.score > 0)
    .sort((a, b) => b.score - a.score || taskLibrarySort(a.task, b.task));
  return scored[0]?.task;
}

function cleanupTaskHostMatchScore(hostText: string, task: Task): number {
  const variables = task.variables || {};
  const taskText = normalizeKey([
    task.id,
    task.title,
    task.description,
    task.group,
    task.repo_name,
    task.branch,
    variables.DEPLOY_TARGET,
    variables.TARGET_HOST,
    variables.HOST,
    variables.MACHINE,
    variables.PEAPOD_PROJECT_ID,
    variables.PEAPOD_PROJECT_NAME,
    variables.ZEPHYR_PROJECT_ID,
    variables.ZEPHYR_PROJECT_NAME
  ].filter(Boolean).join(" "));
  if (!hostText || !taskText) return 0;
  let score = 0;
  const pairs: Array<[RegExp, RegExp, number]> = [
    [/novel|xzm|写书猫|production|生产/, /novel|xzm|写书猫|production|生产/, 8],
    [/estar|estack|e站|边缘|edge|campus|lehu/, /estar|estack|e站|边缘|edge|campus|lehu/, 8],
    [/builder|build|ops|test|测试|构建/, /builder|build|ops|test|测试|构建/, 8]
  ];
  for (const [hostPattern, taskPattern, value] of pairs) {
    if (hostPattern.test(hostText) && taskPattern.test(taskText)) score += value;
  }
  for (const token of hostText.split("-").filter((item) => item.length >= 2)) {
    if (taskText.includes(token)) score += 1;
  }
  return score;
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
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无核心容器" /> }}
        pagination={rows.length > 12 ? { pageSize: 12, showSizeChanger: true, pageSizeOptions: [12, 30], showTotal: (total) => `共 ${total} 个` } : false}
        scroll={{ x: 920 }}
      />
      <List
        className="mobile-monitor-container-list"
        dataSource={rows}
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无核心容器" /> }}
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

export function LogsPage({ state, nowMs }: { state: StateResponse; nowMs: number }) {
  const { message } = AntApp.useApp();
  const screens = Grid.useBreakpoint();
  const [form] = Form.useForm<LogQueryRequest>();
  const [summary, setSummary] = useState<LogSummaryResponse | null>(null);
  const [containers, setContainers] = useState<LogContainer[]>([]);
  const [result, setResult] = useState<LogQueryResponse | null>(null);
  const [containerSearch, setContainerSearch] = useState("");
  const [activeLine, setActiveLine] = useState<LogLine | null>(null);
  const [loading, setLoading] = useState(false);
  const [queryLoading, setQueryLoading] = useState(false);
  const selectedContainers = Form.useWatch("containers", form) || [];
  const keyword = Form.useWatch("keyword", form) || "";

  async function loadLogsConfig(options: { notify?: boolean; autoQuery?: boolean } = {}) {
    setLoading(true);
    try {
      const [summaryData, containersData] = await Promise.all([
        api<LogSummaryResponse>("/api/logs/summary"),
        api<LogContainersResponse>("/api/logs/containers")
      ]);
      setSummary(summaryData);
      setContainers(containersData.containers || []);
      const currentContainers = form.getFieldValue("containers") || [];
      const nextValues = {
        since_minutes: Number(form.getFieldValue("since_minutes") || 15),
        tail: Number(form.getFieldValue("tail") || 200),
        level: form.getFieldValue("level") || "all",
        stream: form.getFieldValue("stream") || "all",
        hosts: form.getFieldValue("hosts") || [],
        containers: currentContainers.length ? currentContainers : defaultLogContainerValues(containersData.containers || []),
        keyword: form.getFieldValue("keyword") || ""
      };
      form.setFieldsValue(nextValues);
      if (options.notify) message.success("日志配置已刷新");
      if (options.autoQuery) await runQuery(nextValues);
    } catch (error) {
      message.error(errorText(error) || "日志配置加载失败");
    } finally {
      setLoading(false);
    }
  }

  async function runQuery(values?: LogQueryRequest) {
    const payload = normalizeLogQueryValues(values || form.getFieldsValue());
    setQueryLoading(true);
    try {
      const data = await api<LogQueryResponse>("/api/logs/query", {
        method: "POST",
        body: JSON.stringify(payload)
      });
      setResult(data);
      if (!data.lines.length) {
        message.info("没有匹配的日志");
      }
    } catch (error) {
      message.error(errorText(error) || "日志查询失败");
    } finally {
      setQueryLoading(false);
    }
  }

  async function copyVisibleLogs() {
    const text = (result?.lines || []).map(formatLogLineForCopy).join("\n");
    if (!text) {
      message.info("当前没有可复制的日志");
      return;
    }
    await navigator.clipboard.writeText(text);
    message.success("已复制当前日志");
  }

  useEffect(() => {
    loadLogsConfig({ autoQuery: true });
  }, []);

  function toggleContainer(value: string) {
    const current = new Set((form.getFieldValue("containers") || []) as string[]);
    if (current.has(value)) {
      current.delete(value);
    } else {
      if (current.size >= (summary?.limits?.max_containers || 10)) {
        message.warning(`最多同时查询 ${summary?.limits?.max_containers || 10} 个容器`);
        return;
      }
      current.add(value);
    }
    form.setFieldsValue({ containers: Array.from(current) });
  }

  function clearContainers() {
    form.setFieldsValue({ containers: [] });
  }

  const lines = result?.lines || [];
  const activeSource = result?.source || summary?.source || "degraded";
  const degradedReason = result?.degraded_reason || summary?.degraded_reason || "";
  const selectedSet = new Set((selectedContainers as string[]).map(String));
  const selectedLogContainers = containers.filter((item) => selectedSet.has(logContainerValue(item)));
  const filteredContainers = filterLogContainers(containers, containerSearch);
  const groupedContainers = groupLogContainers(filteredContainers);
  const healthyContainers = containers.filter((item) => /(running|up|healthy)/i.test(`${item.state || ""} ${item.health || ""}`)).length;
  const errorLines = lines.filter((line) => inferLogLevel(line).includes("error") || inferLogLevel(line).includes("fatal") || inferLogLevel(line).includes("panic")).length;
  const warnLines = lines.filter((line) => inferLogLevel(line).includes("warn")).length;
  const querySourceText = logSourceText(activeSource);
  const isMobile = !screens.lg;

  return (
    <Space direction="vertical" size={16} className="side-stack">
      <PageIntro
        title="日志"
        description="聚合 Docker 已保留日志，快速筛选服务、错误和请求。"
        actions={
          <Space wrap>
            {summary?.dozzle_public_url && <Button href={summary.dozzle_public_url} target="_blank" icon={<ExternalLink size={15} />}>Dozzle</Button>}
            {summary?.grafana_public_url && <Button href={summary.grafana_public_url} target="_blank" icon={<ExternalLink size={15} />}>Grafana</Button>}
            <Button icon={<RefreshCw size={16} />} loading={loading} onClick={() => loadLogsConfig({ notify: true })}>
              刷新
            </Button>
          </Space>
        }
      />

      {degradedReason && <Alert type="warning" showIcon message="日志能力已降级" description={degradedReason} />}

      <Row gutter={[16, 16]} className="log-workspace">
        <Col xs={24} lg={7} xl={6}>
          <ProCard
            className="log-source-card"
            title={<Space size={8}><Server size={16} />服务</Space>}
            extra={<Button type="link" size="small" onClick={clearContainers}>清空</Button>}
            loading={loading}
          >
            <Input
              allowClear
              prefix={<Search size={14} />}
              placeholder="搜索容器、镜像、主机"
              value={containerSearch}
              onChange={(event) => setContainerSearch(event.target.value)}
            />
            <div className="log-source-summary">
              <Text type="secondary">已选 {selectedSet.size}/{summary?.limits?.max_containers || 10}</Text>
              {selectedLogContainers.length > 0 && (
                <Button size="small" onClick={() => runQuery(form.getFieldsValue())} loading={queryLoading}>
                  查询
                </Button>
              )}
            </div>
            <div className="log-source-list">
              {groupedContainers.length ? groupedContainers.map((group) => (
                <div className="log-source-host" key={group.host}>
                  <div className="log-source-host-head">
                    <Space size={6}>
                      <Server size={14} />
                      <Text strong>{group.hostName}</Text>
                    </Space>
                  </div>
                  <div className="log-source-items">
                        {group.items.map((item) => {
                          const value = logContainerValue(item);
                          const checked = selectedSet.has(value);
                          const meta = logContainerMeta(item);
                          return (
                            <label className={`log-source-item ${checked ? "log-source-item-active" : ""}`} key={value}>
                              <Checkbox checked={checked} onChange={() => toggleContainer(value)} />
                              <span className="log-source-item-main">
                                <Text strong ellipsis>{item.name}</Text>
                                {meta && <Text type="secondary" ellipsis>{meta}</Text>}
                              </span>
                            </label>
                          );
                        })}
                  </div>
                </div>
              )) : (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的容器" />
              )}
            </div>
          </ProCard>
        </Col>
        <Col xs={24} lg={17} xl={18}>
          <ProCard
            className="log-console-card"
            title={
              <Space size={8}>
                <FileText size={16} />
                <span>聚合日志</span>
                <Tag color={logSourceColor(activeSource)}>{querySourceText}</Tag>
              </Space>
            }
            extra={<Text type="secondary">{result?.checked_at ? checkedAtText(result.checked_at, nowMs) : `${containers.length} 个容器可选`}</Text>}
          >
            <Form
              form={form}
              layout="vertical"
              className="log-explore-form"
              onFinish={runQuery}
              initialValues={{ since_minutes: 15, tail: 200, level: "all", stream: "all", hosts: [], containers: [], keyword: "" }}
            >
              <Form.Item name="containers" hidden><Select mode="multiple" /></Form.Item>
              <Form.Item name="hosts" hidden><Select mode="multiple" /></Form.Item>
              <div className="log-explore-toolbar">
                <Form.Item name="keyword" className="log-search-field">
                  <Input allowClear size="large" prefix={<Search size={16} />} placeholder="搜索错误、接口、任务 ID、容器名" />
                </Form.Item>
                <Form.Item name="since_minutes" className="log-small-field">
                  <Select
                    size="large"
                    options={[
                      { value: 5, label: "5m" },
                      { value: 15, label: "15m" },
                      { value: 60, label: "1h" },
                      { value: 360, label: "6h" },
                      { value: 1440, label: "24h" }
                    ]}
                  />
                </Form.Item>
                <Button type="primary" size="large" htmlType="submit" loading={queryLoading} icon={<Search size={16} />}>
                  查询
                </Button>
              </div>
              <div className="log-filter-row">
                <Form.Item name="level" label="级别">
                  <Segmented
                    size={isMobile ? "small" : "middle"}
                    options={[
                      { value: "all", label: "全部" },
                      { value: "error", label: "Error" },
                      { value: "warn", label: "Warn" },
                      { value: "info", label: "Info" },
                      { value: "debug", label: "Debug" }
                    ]}
                  />
                </Form.Item>
                <Form.Item name="stream" label="流">
                  <Select
                    options={[
                      { value: "all", label: "全部" },
                      { value: "stdout", label: "stdout" },
                      { value: "stderr", label: "stderr" }
                    ]}
                  />
                </Form.Item>
                <Form.Item name="tail" label="行数">
                  <InputNumber min={20} max={summary?.limits?.max_lines || 1000} />
                </Form.Item>
                <Space className="log-action-row" wrap>
                  <Button icon={<Copy size={15} />} onClick={copyVisibleLogs}>
                    复制
                  </Button>
                  {summary?.dozzle_public_url && <Button href={summary.dozzle_public_url} target="_blank" icon={<ExternalLink size={15} />}>Dozzle</Button>}
                </Space>
              </div>
            </Form>

            <div className="log-result-meta">
              <Space wrap size={[8, 8]}>
                <Tag color={lines.length ? "blue" : "default"}>{lines.length} 行</Tag>
                {errorLines > 0 && <Tag color="red">{errorLines} error</Tag>}
                {warnLines > 0 && <Tag color="gold">{warnLines} warn</Tag>}
                {selectedLogContainers.slice(0, 5).map((item) => <Tag key={logContainerValue(item)}>{item.name}</Tag>)}
                {selectedLogContainers.length > 5 && <Tag>+{selectedLogContainers.length - 5}</Tag>}
              </Space>
              <Text type="secondary">最多返回 {summary?.limits?.max_lines || 1000} 行，日志内容已脱敏</Text>
            </div>

            {lines.length ? (
              <div className="log-console">
                <Virtuoso
                  className="log-virtual-list"
                  data={lines}
                  style={{ height: isMobile ? 520 : 640 }}
                  itemContent={(index, line) => (
                    <LogRow
                      line={line}
                      keyword={keyword}
                      index={index}
                      onOpen={() => setActiveLine(line)}
                    />
                  )}
                />
              </div>
            ) : (
              <div className="log-empty-state">
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description={result ? "没有匹配的日志" : "选择服务后查询日志"}
                />
                <Text type="secondary">轻量日志只读取 Docker 当前保留内容，不替代长期日志库。</Text>
              </div>
            )}
          </ProCard>
        </Col>
      </Row>

      <Drawer
        title="日志详情"
        width={screens.lg ? 640 : "100vw"}
        open={!!activeLine}
        onClose={() => setActiveLine(null)}
        destroyOnClose
      >
        {activeLine && (
          <Space direction="vertical" size={14} className="side-stack">
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label="时间">{activeLine.timestamp || "-"}</Descriptions.Item>
              <Descriptions.Item label="主机">{friendlyLogHostName(activeLine.host_name || activeLine.host)}</Descriptions.Item>
              <Descriptions.Item label="容器">{activeLine.container_name}</Descriptions.Item>
              <Descriptions.Item label="级别">{inferLogLevel(activeLine) || "-"}</Descriptions.Item>
              <Descriptions.Item label="输出流">{activeLine.stream || "-"}</Descriptions.Item>
            </Descriptions>
            <pre className="log-detail-message">{activeLine.message}</pre>
          </Space>
        )}
      </Drawer>
    </Space>
  );
}

function LogRow({ line, keyword, index, onOpen }: { line: LogLine; keyword: string; index: number; onOpen: () => void }) {
  const level = inferLogLevel(line);
  const container = line.container_name || line.container_id || "-";
  const message = summarizeLogMessage(line.message);
  return (
    <button type="button" className="log-row" onClick={onOpen}>
      <span className="log-row-index">{index + 1}</span>
      <span className="log-row-time">{logLineTime(line.timestamp)}</span>
      <span className={`log-row-level log-row-level-${logLevelTone(level)}`}>{level || "log"}</span>
      <span className="log-row-source" title={`${friendlyLogHostName(line.host_name || line.host)}/${container}`}>
        {container}
      </span>
      <span className="log-row-message" title={line.message}>{renderHighlightedLogMessage(message, keyword)}</span>
    </button>
  );
}

export function InfrastructureLinks({ tasks, compact = false }: { tasks: Task[]; compact?: boolean }) {
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
      {!compact && (
        <Card className="page-head-card">
          <Space direction="vertical" size={4}>
            <Title level={4}>基础设施入口</Title>
            <Text type="secondary">Woodpecker、监控、日志和外部控制台集中在这里。</Text>
          </Space>
        </Card>
      )}
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
          <Text strong>{productText(row.task_title || row.task_id)}</Text>
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
        pagination={records.length > 14 ? { pageSize: 14, showSizeChanger: true, pageSizeOptions: [14, 30, 60], showTotal: (total) => `共 ${total} 条` } : false}
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
    <Row gutter={[16, 16]} className="profile-grid">
      <Col xs={24} lg={12}>
        <Card title="账号资料" className="profile-card">
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
        <Card title="修改密码" className="profile-card">
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
      className="users-card"
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
        scroll={{ x: 620 }}
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
    lookupForm.setFieldsValue({ owner: PRODUCT_REPO_OWNER, name: PRODUCT_REPO_NAME });
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
          {configured[String(row.id)] && <Tag color="blue">{PRODUCT_NAME} 已保存</Tag>}
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
            保存到 {PRODUCT_NAME}
          </Button>
          {row.forge_url && <Button size="small" href={row.forge_url} target="_blank" icon={<ExternalLink size={14} />} />}
        </Space>
      )
    }
  ];

  const productRepoFullName = `${PRODUCT_REPO_OWNER}/${PRODUCT_REPO_NAME}`.toLowerCase();
  const productRepoEnabled = repos.some((repo) => {
    const fullName = (repo.full_name || "").toLowerCase();
    return fullName === productRepoFullName;
  });

  return (
    <Space direction="vertical" size={16} className="side-stack">
      {!productRepoEnabled && (
        <Alert
          type="warning"
          showIcon
          message={`Woodpecker 里还没有启用 ${PRODUCT_NAME} 仓库`}
          description={`如果 lookup ${PRODUCT_REPO_OWNER}/${PRODUCT_REPO_NAME} 仍然 404，通常是 GitHub OAuth 没授权到这个仓库，先去 Woodpecker 的添加仓库或 GitHub OAuth 权限里处理。`}
          action={state.links.woodpecker ? <Button size="small" href={state.links.woodpecker} target="_blank">打开 Woodpecker</Button> : undefined}
        />
      )}
      <Card title="查找并启用仓库">
        <Form form={lookupForm} layout="inline" onFinish={lookup} className="inline-create-form">
          <Form.Item label="Owner" name="owner" rules={[{ required: true, message: "请输入 owner" }]}>
            <Input placeholder={PRODUCT_REPO_OWNER} />
          </Form.Item>
          <Form.Item label="仓库名" name="name" rules={[{ required: true, message: "请输入仓库名" }]}>
            <Input placeholder={PRODUCT_REPO_NAME} />
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
              <Space wrap>
                {lookupResult.id ? (
                  <Button type="primary" loading={savingID === lookupResult.id} onClick={() => saveRepo(lookupResult)}>
                    保存到 {PRODUCT_NAME}
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

function SetupGuidePanel({
  setup,
  loading,
  doctorRunning,
  onRefresh,
  onDoctorRun
}: {
  setup: SetupConfigResponse | null;
  loading: boolean;
  doctorRunning: boolean;
  onRefresh: () => void;
  onDoctorRun: () => void;
}) {
  const checklist = setup?.checklist || [];
  const issueItems = checklist.filter((item) => !["ok", "optional"].includes(item.status));
  const quietCount = Math.max(0, checklist.length - issueItems.length);
  const visibleIssues = issueItems.slice(0, 3);
  const verification = setup?.deployment_verification_summary;
  const logStrategy = setup?.log_strategy;
  const onboarding = setup?.onboarding;
  const doctorChecks = setup?.doctor?.checks || [];
  const doctorIssueCount = doctorChecks.filter((item) => item.status !== "ok").length;
  return (
    <ProCard
      title="接入向导"
      extra={
        <Space wrap>
          <Button icon={<Gauge size={16} />} loading={doctorRunning} onClick={onDoctorRun}>
            运行体检
          </Button>
          <Button icon={<RefreshCw size={16} />} loading={loading} onClick={onRefresh}>
            刷新检查
          </Button>
        </Space>
      }
    >
      <Space direction="vertical" size={14} className="side-stack">
        <Row gutter={[12, 12]}>
          <Col xs={24} lg={8}>
            <Card size="small" className={`setup-readiness-card setup-readiness-${setup?.readiness || "warning"}`}>
              <Space direction="vertical" size={8}>
                <Tag color={readinessColor(setup?.readiness || "warning")}>{readinessText(setup?.readiness || "warning")}</Tag>
                <Text strong>上线准备度</Text>
                <Progress percent={onboarding?.percent || 0} size="small" status={setup?.readiness === "blocked" ? "exception" : "active"} />
                <Text type="secondary">{onboarding?.next_action || "阻断项会优先显示；warning 项不阻塞，但建议上线前处理。"}</Text>
              </Space>
            </Card>
          </Col>
          <Col xs={24} lg={8}>
            <VerificationSummaryCard summary={verification} />
          </Col>
          <Col xs={24} lg={8}>
            {logStrategy ? <LogStrategyCard status={logStrategy} compact /> : <Card size="small" loading />}
          </Col>
        </Row>
        <Card
          size="small"
          className="setup-focus-card"
          title={
            <Space wrap>
              <Text strong>待处理项</Text>
              {issueItems.length ? <Tag color="gold">{issueItems.length} 项</Tag> : <Tag color="green">无阻断</Tag>}
            </Space>
          }
          extra={quietCount ? <Text type="secondary">{quietCount} 项已就绪/可选，已收起</Text> : undefined}
        >
          {!checklist.length ? (
            <Alert type="info" showIcon message="正在加载接入检查" />
          ) : visibleIssues.length ? (
            <List
              size="small"
              className="setup-focus-list"
              dataSource={visibleIssues}
              renderItem={(item) => (
                <List.Item
                  actions={item.action_url ? [
                    <Button key="open" size="small" href={item.action_url} target="_blank" icon={<ExternalLink size={14} />}>
                      {item.action_label || "打开"}
                    </Button>
                  ] : undefined}
                >
                  <List.Item.Meta
                    title={
                      <Space wrap size={6}>
                        <Tag color={setupStatusColor(item.status)}>{setupStatusText(item.status)}</Tag>
                        <Text strong>{item.title}</Text>
                      </Space>
                    }
                    description={
                      <Space direction="vertical" size={2} className="side-stack">
                        <Text type="secondary">{item.message}</Text>
                        {item.fix && <Text className="checklist-fix">{item.fix}</Text>}
                      </Space>
                    }
                  />
                </List.Item>
              )}
            />
          ) : (
            <Alert type="success" showIcon message="核心接入项已就绪" description="可选入口和历史说明已收进对应设置分组，日常只需要关注阻断项。" />
          )}
          {issueItems.length > visibleIssues.length && (
            <Text type="secondary">还有 {issueItems.length - visibleIssues.length} 项已收起，可在对应设置分组里处理。</Text>
          )}
        </Card>
        {doctorChecks.length > 0 && (
          <Alert
            type={doctorIssueCount ? "warning" : "success"}
            showIcon
            message={doctorIssueCount ? `体检发现 ${doctorIssueCount} 个需确认项` : "体检未发现新的阻断项"}
            description="体检结果已归并到上面的待处理项，避免同一问题重复出现。"
          />
        )}
      </Space>
    </ProCard>
  );
}

function TaskTemplatePanel({ state, onApplied }: { state: StateResponse; onApplied: () => Promise<void> }) {
  const { message } = AntApp.useApp();
  const [form] = Form.useForm();
  const [templates, setTemplates] = useState<TaskTemplate[]>([]);
  const [loading, setLoading] = useState(false);
  const [applying, setApplying] = useState(false);
  const selectedID = Form.useWatch("template_id", form);
  const selected = templates.find((item) => item.id === selectedID) || templates[0];

  async function loadTemplates() {
    setLoading(true);
    try {
      const data = await api<{ templates: TaskTemplate[] }>("/api/templates");
      const rows = data.templates || [];
      setTemplates(rows);
      if (rows.length && !form.getFieldValue("template_id")) {
        form.setFieldsValue({
          template_id: rows[0].id,
          branch: rows[0].default_branch || "main",
          environment: "production"
        });
      }
    } catch (error) {
      message.error(errorText(error) || "任务模板加载失败");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadTemplates();
  }, []);

  useEffect(() => {
    if (!selected) return;
    const currentBranch = form.getFieldValue("branch");
    form.setFieldsValue({
      branch: currentBranch || selected.default_branch || "main",
      environment: form.getFieldValue("environment") || "production"
    });
  }, [selectedID]);

  async function applyTemplate(values: Record<string, unknown>) {
    const templateID = String(values.template_id || selected?.id || "");
    if (!templateID) {
      message.warning("请选择任务模板");
      return;
    }
    setApplying(true);
    try {
      const data = await api<TemplateApplyResponse>(`/api/templates/${encodeURIComponent(templateID)}/apply`, {
        method: "POST",
        body: JSON.stringify({
          repo_id: Number(values.repo_id || 0),
          repo_name: String(values.repo_name || ""),
          branch: String(values.branch || "main"),
          project_id: String(values.project_id || ""),
          project_name: String(values.project_name || ""),
          environment: String(values.environment || "production"),
          marker_path: String(values.marker_path || ""),
          health_url: String(values.health_url || ""),
          confirm_text: String(values.confirm_text || "")
        })
      });
      message.success(`已生成任务：${data.task.title}`);
      form.resetFields(["project_id", "project_name", "marker_path", "health_url", "confirm_text"]);
      await onApplied();
    } catch (error) {
      message.error(errorText(error) || "套用模板失败");
    } finally {
      setApplying(false);
    }
  }

  return (
    <ProCard
      title="从模板创建任务"
      className="template-panel"
      extra={<Button icon={<RefreshCw size={16} />} loading={loading} onClick={loadTemplates}>刷新模板</Button>}
    >
      <Row gutter={[16, 16]}>
        <Col xs={24} xl={8}>
          <Space direction="vertical" size={10} className="side-stack">
            <Text type="secondary">选择最接近的场景，{PRODUCT_NAME} 会生成 Woodpecker 变量、项目归并字段和部署验证项。</Text>
            {selected && (
              <Card size="small" className="template-preview-card">
                <Space direction="vertical" size={8} className="side-stack">
                  <Space wrap>
                    <Tag color={selected.requires_verification ? "green" : "blue"}>{selected.category}</Tag>
                    <Tag color={riskColors[selected.default_risk] || "default"}>{riskLabel(selected.default_risk)}</Tag>
                    {selected.requires_verification && <Tag color="success">可信部署</Tag>}
                  </Space>
                  <Text strong>{selected.title}</Text>
                  <Text type="secondary">{selected.description}</Text>
                  <Text className="checklist-fix">
                    默认变量：{Object.keys(selected.variables || {}).slice(0, 5).join("、") || "-"}
                  </Text>
                </Space>
              </Card>
            )}
          </Space>
        </Col>
        <Col xs={24} xl={16}>
          <Form form={form} layout="vertical" onFinish={applyTemplate} className="template-form">
            <Row gutter={12}>
              <Col xs={24} md={12}>
                <Form.Item label="模板" name="template_id" rules={[{ required: true, message: "请选择模板" }]}>
                  <Select
                    loading={loading}
                    options={templates.map((item) => ({ value: item.id, label: item.title }))}
                    placeholder="选择模板"
                  />
                </Form.Item>
              </Col>
              <Col xs={12} md={6}>
                <Form.Item label="Repo ID" name="repo_id" rules={[{ required: true, message: "请输入 Repo ID" }]}>
                  <InputNumber min={1} style={{ width: "100%" }} placeholder="3" />
                </Form.Item>
              </Col>
              <Col xs={12} md={6}>
                <Form.Item label="默认分支" name="branch" rules={[{ required: true, message: "请输入分支" }]}>
                  <Input placeholder="main" />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={12}>
              <Col xs={24} md={12}>
                <Form.Item label="仓库显示名" name="repo_name" rules={[{ required: true, message: "请输入仓库名" }]}>
                  <Input placeholder="owner/service" />
                </Form.Item>
              </Col>
              <Col xs={12} md={6}>
                <Form.Item label="项目 ID" name="project_id" rules={[{ required: true, message: "请输入项目 ID" }]}>
                  <Input placeholder="my-service" />
                </Form.Item>
              </Col>
              <Col xs={12} md={6}>
                <Form.Item label="环境" name="environment" rules={[{ required: true }]}>
                  <Select
                    options={[
                      { value: "operations", label: "运维机" },
                      { value: "production", label: "生产机" },
                      { value: "staging", label: "测试机" },
                      { value: "service", label: "业务机" }
                    ]}
                  />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={12}>
              <Col xs={24} md={12}>
                <Form.Item label="项目名称" name="project_name" rules={[{ required: true, message: "请输入项目名称" }]}>
                  <Input placeholder="业务服务" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item label="确认文字" name="confirm_text" extra="高危模板可留空，后端会按环境生成默认确认词。">
                  <Input placeholder="PRODUCTION" />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={12}>
              <Col xs={24} md={12}>
                <Form.Item label="版本 marker 路径" name="marker_path" extra="部署脚本写入实际 commit 的文件；部署模板未填时会给出默认路径。">
                  <Input placeholder="/opt/my-service/.deploy/current-source-sha" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item label="健康检查 URL" name="health_url" extra="返回 2xx/3xx 即算健康，可与 marker 同时使用。">
                  <Input placeholder="http://127.0.0.1:8080/healthz" />
                </Form.Item>
              </Col>
            </Row>
            <Space wrap>
              <Button type="primary" htmlType="submit" loading={applying} icon={<Plus size={16} />}>
                生成任务
              </Button>
              <Text type="secondary">生成后会出现在下方任务配置表，可继续编辑变量。</Text>
            </Space>
          </Form>
        </Col>
      </Row>
    </ProCard>
  );
}

function VerificationSummaryCard({ summary }: { summary?: DeploymentVerificationSummary }) {
  const missing = summary?.missing_count || 0;
  return (
    <Card size="small" className="setup-summary-card">
      <Space direction="vertical" size={8} className="side-stack">
        <Space>
          <Tag color={missing ? "error" : "success"}>{missing ? "有阻断" : "已闭环"}</Tag>
          <Text strong>部署可信验证</Text>
        </Space>
        <Text>{summary ? `${summary.configured_count}/${summary.task_count} 个部署任务已配置验证` : "-"}</Text>
        {missing > 0 ? (
          <Text type="danger" ellipsis={{ tooltip: summary?.missing_tasks?.join("、") }}>缺少：{summary?.missing_tasks?.slice(0, 3).join("、")}</Text>
        ) : (
          <Text type="secondary">部署入口会校验 marker 或 healthz。</Text>
        )}
      </Space>
    </Card>
  );
}

function LogStrategyCard({ status, compact = false }: { status: LogStrategyStatus; compact?: boolean }) {
  return (
    <Card size="small" className="setup-summary-card">
      <Space direction="vertical" size={compact ? 8 : 10} className="side-stack">
        <Space>
          <Tag color={logStrategyColor(status.mode)}>{status.label}</Tag>
          <Text strong>日志策略</Text>
        </Space>
        <Text type="secondary">{status.message}</Text>
        <Text>Docker 保留：{status.docker_retention}</Text>
        <Space wrap>
          <Tag color={status.dozzle_mcp_ready ? "green" : status.mode === "lightweight" ? "gold" : "default"}>
            MCP {status.dozzle_mcp_ready ? "可用" : "未确认"}
          </Tag>
          {status.dozzle_mcp_message && <Text type="secondary">{status.dozzle_mcp_message}</Text>}
        </Space>
        <Space wrap>
          {status.dozzle_public_url && <Button size="small" href={status.dozzle_public_url} target="_blank">Dozzle</Button>}
          {status.grafana_public_url && <Button size="small" href={status.grafana_public_url} target="_blank">Grafana</Button>}
          {status.alert_webhook_ready && <Tag color="green">告警已配置</Tag>}
        </Space>
      </Space>
    </Card>
  );
}

type SetupSection = "guide" | "core" | "hosts" | "logs";

function SetupConfigPanel({ onReload, initialSection = "guide" }: { onReload: () => Promise<void>; initialSection?: SetupSection }) {
  const { message } = AntApp.useApp();
  const [form] = Form.useForm<RuntimeConfigInput>();
  const [setup, setSetup] = useState<SetupConfigResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [doctorRunning, setDoctorRunning] = useState(false);
  const [dirty, setDirty] = useState(false);
  const focusTitle = initialSection === "hosts" ? "环境与机器" : initialSection === "logs" ? "日志策略" : initialSection === "core" ? "核心服务" : "接入向导";
  const focusDescription = initialSection === "hosts"
    ? "这里维护运维机、生产机、测试机和普通业务机。日志页会复用这些机器的 SSH 配置读取远端容器日志。"
    : initialSection === "logs"
      ? "这里选择轻量 Dozzle 或完整 Grafana/Loki，并配置日志入口与 Docker 日志保留策略。"
      : initialSection === "core"
        ? `这里配置 ${PRODUCT_NAME}、Woodpecker 和 Beszel 的服务地址与 API 凭据。`
        : "先看上线阻断项，再进入对应设置分组处理。";
  const showGuide = initialSection === "guide";
  const showCore = initialSection === "core";
  const showLogs = initialSection === "logs";
  const showHosts = initialSection === "hosts";
  const monitorHostCount = setup?.config?.monitor_hosts?.length || 0;

  async function loadSetup(options: { notify?: boolean } = {}) {
    setLoading(true);
    try {
      const data = await api<SetupConfigResponse>("/api/setup/config");
      setSetup(data);
      form.setFieldsValue(normalizeSetupFormValues(data.config));
      setDirty(false);
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
      setDirty(false);
      message.success("接入配置已保存");
      await onReload();
    } catch (error) {
      message.error(errorText(error) || "保存失败");
    } finally {
      setSaving(false);
    }
  }

  async function runDoctor() {
    setDoctorRunning(true);
    try {
      const doctor = await api<SetupConfigResponse["doctor"]>("/api/doctor/run", { method: "POST" });
      setSetup((previous) => previous ? { ...previous, doctor } : previous);
      message.success(doctor?.readiness === "blocked" ? "体检完成，有阻断项需要处理" : "体检完成");
    } catch (error) {
      message.error(errorText(error) || "体检失败");
    } finally {
      setDoctorRunning(false);
    }
  }

  return (
    <Space direction="vertical" size={16} className="side-stack">
      {showGuide ? (
        <SetupGuidePanel
          setup={setup}
          loading={loading}
          doctorRunning={doctorRunning}
          onRefresh={() => loadSetup({ notify: true })}
          onDoctorRun={runDoctor}
        />
      ) : (
        <ProCard className="setup-context-card">
          <Row gutter={[14, 14]} align="middle">
            <Col xs={24} lg={showLogs ? 14 : 16}>
              <Space direction="vertical" size={4} className="side-stack">
                <Space wrap>
                  <Tag color={showHosts ? "blue" : "green"}>{focusTitle}</Tag>
                  {dirty && <Tag color="gold">有未保存修改</Tag>}
                </Space>
                <Text strong>{focusTitle}配置</Text>
                <Text type="secondary">{focusDescription}</Text>
              </Space>
            </Col>
            {showHosts && (
              <Col xs={24} lg={8}>
                <Row gutter={8}>
                  <Col span={12}>
                    <Card size="small" className="setup-metric-card">
                      <Text type="secondary">被管机器</Text>
                      <Title level={4}>{monitorHostCount}</Title>
                    </Card>
                  </Col>
                  <Col span={12}>
                    <Card size="small" className="setup-metric-card">
                      <Text type="secondary">刷新间隔</Text>
                      <Title level={4}>{setup?.config?.monitor_refresh_seconds || 20}s</Title>
                    </Card>
                  </Col>
                </Row>
              </Col>
            )}
            {showLogs && setup?.log_strategy && (
              <Col xs={24} lg={10}>
                <LogStrategyCard status={setup.log_strategy} compact />
              </Col>
            )}
          </Row>
        </ProCard>
      )}
      {(showCore || showLogs || showHosts) && (
      <Form form={form} layout="vertical" onFinish={save} disabled={loading} onValuesChange={() => setDirty(true)}>
        <div className="setup-action-bar">
          <Space direction="vertical" size={2}>
            <Text strong>{focusTitle}</Text>
            <Text type="secondary">只保存当前表单里显示的配置项，Secret 留空会保留原值。</Text>
          </Space>
          <Space wrap className="setup-action-buttons">
            {dirty && <Tag color="gold">待保存</Tag>}
            {!showGuide && (
              <Button icon={<RefreshCw size={16} />} loading={loading} onClick={() => loadSetup({ notify: true })}>
                刷新
              </Button>
            )}
            <Button type="primary" htmlType="submit" loading={saving}>
              保存{focusTitle}
            </Button>
          </Space>
        </div>

        {showCore && (
        <ProCard title="核心服务" className="setup-form-card">
          <Row gutter={12}>
            <Col xs={24} lg={8}>
              <Form.Item label={`${PRODUCT_NAME} 公开地址`} name="public_url" rules={[{ required: true, message: `请输入 ${PRODUCT_NAME} 公开地址` }]}>
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
                extra={setup?.secrets?.woodpecker_token ? "已配置；留空保存会保留原 token。" : `未配置；保存后 ${PRODUCT_NAME} 才能触发流水线。`}
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
          </Row>
        </ProCard>
        )}

        {showLogs && (
        <ProCard title="日志策略" className="setup-form-card">
          <Row gutter={12}>
            <Col xs={24} lg={8}>
              <Form.Item label="当前日志模式" name="log_strategy" extra="轻量模式用 Dozzle；完整观测用 Grafana/Loki；外部模式只保留入口。">
                <Select
                  options={[
                    { value: "lightweight", label: "轻量模式 Dozzle" },
                    { value: "observability", label: "完整观测 Grafana/Loki" },
                    { value: "external", label: "外部日志平台" }
                  ]}
                />
              </Form.Item>
            </Col>
            <Col xs={12} lg={4}>
              <Form.Item label="Docker max-size" name="docker_log_max_size">
                <Input placeholder="20m" />
              </Form.Item>
            </Col>
            <Col xs={12} lg={4}>
              <Form.Item label="Docker max-file" name="docker_log_max_file">
                <Input placeholder="3" />
              </Form.Item>
            </Col>
            <Col xs={24} lg={8}>
              <Form.Item label="Dozzle 内部地址" name="dozzle_base_url" extra={`${PRODUCT_NAME} 通过这个地址访问 Dozzle MCP，默认 Docker 网络内为 http://dozzle:8080。`}>
                <Input placeholder="http://dozzle:8080" />
              </Form.Item>
            </Col>
            <Col xs={24} lg={8}>
              <Form.Item label="Dozzle 公开地址" name="dozzle_public_url" extra="轻量日志入口：查看 Docker 已保留日志，并实时跟随新日志，不落地集中日志库。">
                <Input placeholder="https://logs.example.com" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={12}>
            <Col xs={24} lg={8}>
              <Form.Item label="Dozzle 用户名" name="dozzle_username" extra={`Dozzle 开启 simple auth 后，${PRODUCT_NAME} 会用这个账号读取 MCP。`}>
                <Input placeholder="tangfire" autoComplete="username" />
              </Form.Item>
            </Col>
            <Col xs={24} lg={8}>
              <Form.Item
                label="Dozzle 密码"
                name="dozzle_password"
                extra={setup?.secrets?.dozzle_password ? "已配置；留空保存会保留原密码。" : "Dozzle 开启登录保护后需要配置。"}
              >
                <Input.Password placeholder={setup?.secrets?.dozzle_password ? "留空保留原密码" : "填入 Dozzle 密码"} autoComplete="new-password" />
              </Form.Item>
            </Col>
            <Col xs={24} lg={8}>
              <Form.Item label="Grafana 公开地址" name="grafana_public_url" extra="完整观测入口：历史日志、指标、链路和告警。轻量模式可留空或稍后启用。">
                <Input placeholder="https://grafana.example.com" />
              </Form.Item>
            </Col>
            <Col xs={24} lg={8}>
              <Form.Item
                label="告警 Webhook"
                name="alert_webhook_url"
                extra={setup?.secrets?.alert_webhook ? "已配置；留空保存会保留原 webhook。" : "可选；用于后续接入飞书/企业微信告警。"}
              >
                <Input.Password placeholder={setup?.secrets?.alert_webhook ? "留空保留原 webhook" : "https://..."} autoComplete="new-password" />
              </Form.Item>
            </Col>
          </Row>
          {setup?.log_strategy && <LogStrategyCard status={setup.log_strategy} />}
        </ProCard>
        )}

        {showHosts && (
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
          <Divider plain>自动清理</Divider>
          <Row gutter={12}>
            <Col xs={12} lg={6}>
              <Form.Item label="自动清理级别" name="monitor_auto_cleanup_level" extra="空=禁用，safe=仅 build cache，standard=含悬空镜像">
                <Select allowClear placeholder="禁用" options={[
                  { value: "", label: "禁用" },
                  { value: "safe", label: "safe — 仅 build cache" },
                  { value: "standard", label: "standard — 含悬空镜像" }
                ]} />
              </Form.Item>
            </Col>
            <Col xs={12} lg={6}>
              <Form.Item label="自动清理阈值 %" name="monitor_auto_cleanup_disk" extra="磁盘使用率达到此百分比时自动触发清理">
                <InputNumber min={0} max={100} style={{ width: "100%" }} placeholder="0=禁用" />
              </Form.Item>
            </Col>
          </Row>
        </ProCard>
        )}

        {showHosts && (
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
                          <Select
                            options={[
                              { value: "operations", label: "operations 运维/构建机" },
                              { value: "production", label: "production 生产机" },
                              { value: "staging", label: "staging 测试机" },
                              { value: "service", label: "service 业务机" }
                            ]}
                          />
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
        )}

      </Form>
      )}
    </Space>
  );
}

function TaskConfigView({
  state,
  tasks,
  config,
  onAdd,
  onEdit,
  onDelete
}: {
  state: StateResponse;
  tasks: Task[];
  config: TaskConfig | null;
  onAdd: () => void;
  onEdit: (task: Task) => void;
  onDelete: (task: Task) => void;
}) {
  const deployProjectRows = Array.from(groupDeployProjectTasks(tasks || []).entries())
    .map(([projectKey, projectTasks]) => ({ projectKey, projectTasks }))
    .sort((a, b) => taskProjectDisplayName(a.projectTasks).localeCompare(taskProjectDisplayName(b.projectTasks), "zh-CN"));
  const unassignedDeployTasks = (tasks || []).filter((task) => !task.external_url && !isDeployProjectTask(task) && isDeploymentLikeTask(task)).sort(taskLibrarySort);
  const maintenanceTasks = (tasks || []).filter((task) => !task.external_url && !isDeployProjectTask(task) && !isDeploymentLikeTask(task)).sort(taskLibrarySort);
  const taskActionCell = (task: Task) => (
    <Space>
      <Button size="small" icon={<Settings size={14} />} onClick={() => onEdit(task)} />
      {(task.custom || task.overridden) && (
        <Popconfirm title={task.overridden ? "恢复这个内置任务的默认配置？" : "删除这个自定义任务？"} onConfirm={() => onDelete(task)}>
          <Button size="small" icon={<Trash2 size={14} />} />
        </Popconfirm>
      )}
    </Space>
  );

  return (
    <Space direction="vertical" size={14} className="side-stack">
      <div className="config-task-summary">
        <Space direction="vertical" size={4} className="side-stack">
          <Text strong>部署页只展示项目/环境，不展示单次维护动作。</Text>
          <Text type="secondary">同一个项目 ID 下的部署、回退、强制部署会归并为一个部署项目；清理磁盘、重启、状态检查保留在维护动作。已保存覆盖/自定义 {config?.tasks?.length || 0} 个。</Text>
        </Space>
      </div>
      {unassignedDeployTasks.length > 0 && (
        <Card title="待归属部署任务">
          <Alert
            type="warning"
            showIcon
            message="这些任务看起来是部署/发布，但缺少项目 ID"
            description="补上 PEAPOD_PROJECT_ID 或 ZEPHYR_PROJECT_ID 后，它们会归并到部署页的项目列表里；否则只能作为单次任务保留。"
          />
          <Table
            className="config-task-table"
            rowKey="id"
            size="small"
            pagination={false}
            dataSource={unassignedDeployTasks}
            columns={[
              {
                title: "任务",
                render: (_, row) => (
                  <Space direction="vertical" size={0} className="table-cell-stack">
                    <Text strong>{productText(row.title)}</Text>
                    <Text type="secondary" ellipsis={{ tooltip: productText(row.description) }}>{productText(row.description) || "缺少项目归属"}</Text>
                  </Space>
                )
              },
              { title: "执行仓库", render: (_, row) => repoName(state, row) },
              { title: "分支", dataIndex: "branch" },
              {
                title: "需要补齐",
                render: () => <Text type="danger">PEAPOD_PROJECT_ID / ZEPHYR_PROJECT_ID</Text>
              },
              {
                title: "",
                width: 150,
                render: (_, row) => taskActionCell(row)
              }
            ]}
            scroll={{ x: 760 }}
          />
        </Card>
      )}
      <Card
        title="部署项目任务"
        extra={
          <Button type="primary" icon={<Plus size={16} />} onClick={onAdd}>
            新增任务
          </Button>
        }
      >
        <Table
          className="config-task-table"
          rowKey="projectKey"
          size="small"
          pagination={false}
          dataSource={deployProjectRows}
          locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有配置项目化部署任务" /> }}
          columns={[
            {
              title: "部署项目",
              width: 260,
              render: (_, row) => {
                const primary = preferredDeployTask(row.projectTasks) || row.projectTasks[0];
                return (
                  <Space direction="vertical" size={0} className="table-cell-stack">
                    <Text strong>{taskProjectDisplayName(row.projectTasks)}</Text>
                    <Text type="secondary" ellipsis={{ tooltip: taskProjectID(primary) }}>{taskProjectID(primary) || "未配置项目 ID"}</Text>
                  </Space>
                );
              }
            },
            {
              title: "环境 / 仓库",
              width: 260,
              render: (_, row) => {
                const primary = preferredDeployTask(row.projectTasks) || row.projectTasks[0];
                return (
                  <Space direction="vertical" size={0} className="table-cell-stack">
                    <Text>{deployObjectEnvironmentLabel(undefined, primary)}</Text>
                    <Text type="secondary" ellipsis={{ tooltip: repoName(state, primary) }}>{repoName(state, primary)} · {primary.branch || "main"}</Text>
                  </Space>
                );
              }
            },
            {
              title: "可用按钮",
              width: 250,
              render: (_, row) => (
                <Space wrap size={[4, 4]}>
                  {preferredDeployTask(row.projectTasks) && <Tag color="blue">部署</Tag>}
                  {row.projectTasks.some(isRollbackTask) && <Tag color="red">回退</Tag>}
                  {row.projectTasks.some(isForceDeployTask) && <Tag color="gold">强制部署</Tag>}
                  {row.projectTasks.filter((task) => !isRollbackTask(task) && !isForceDeployTask(task)).slice(1).map((task) => <Tag key={task.id}>{productText(task.title)}</Tag>)}
                </Space>
              )
            },
            {
              title: "验证",
              width: 180,
              render: (_, row) => {
                const deploy = preferredDeployTask(row.projectTasks);
                return deploy ? (
                  <Text type={taskHasVerification(deploy) ? "secondary" : "danger"}>
                    {taskHasVerification(deploy) ? "已配置 marker/healthz" : "缺少部署验证"}
                  </Text>
                ) : <Text type="secondary">无默认部署任务</Text>;
              }
            },
            {
              title: "任务来源",
              render: (_, row) => (
                <Space direction="vertical" size={4} className="table-cell-stack">
                  {row.projectTasks.map((task) => (
                    <Space key={task.id} size={6} wrap>
                      <Tag>{deployTaskRoleText(task)}</Tag>
                      <Text ellipsis={{ tooltip: productText(task.title) }}>{productText(task.title)}</Text>
                      {task.custom && <Tag color="blue">自定义</Tag>}
                      {task.overridden && <Tag color="gold">已覆盖</Tag>}
                      {task.disabled && <Tag color="red">不可执行</Tag>}
                    </Space>
                  ))}
                </Space>
              )
            },
            {
              title: "",
              width: 130,
              render: (_, row) => {
                const primary = preferredDeployTask(row.projectTasks) || row.projectTasks[0];
                return taskActionCell(primary);
              }
            }
          ]}
          scroll={{ x: 1080 }}
        />
      </Card>
      <Card title="维护动作" extra={<Button icon={<Plus size={16} />} onClick={onAdd}>新增维护动作</Button>}>
      <Table
        className="config-task-table"
        rowKey="id"
        size="small"
        pagination={false}
        dataSource={maintenanceTasks}
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无维护动作" /> }}
        columns={[
          {
            title: "维护动作",
            render: (_, row) => (
              <Space direction="vertical" size={0}>
                <Text strong>{productText(row.title)}</Text>
                <Space size={4}>
                  {row.builtin && <Tag>内置</Tag>}
                  {row.custom && <Tag color="blue">自定义</Tag>}
                  {row.overridden && <Tag color="gold">已覆盖</Tag>}
                  {row.disabled && <Tag color="red">{row.disabled_reason || "不可执行"}</Tag>}
                </Space>
              </Space>
            )
          },
          { title: "场景", render: (_, row) => maintenanceTaskKindText(row) },
          { title: "执行仓库", render: (_, row) => repoName(state, row) },
          { title: "分支", dataIndex: "branch" },
          {
            title: "配置摘要",
            render: (_, row) => (
              <Space direction="vertical" size={4} className="table-cell-stack">
                <Space wrap size={[4, 4]}>
                  {taskVariableSummaryTags(row).map((item) => (
                    <Tag key={item}>{item}</Tag>
                  ))}
                </Space>
                <Text type="secondary">不会作为部署项目出现在部署页主表。</Text>
              </Space>
            )
          },
          {
            title: "",
            width: 150,
            render: (_, row) => taskActionCell(row)
          }
        ]}
        scroll={{ x: 760 }}
      />
    </Card>
    </Space>
  );
}

export function Docs({ state, compact = false }: { state: StateResponse; compact?: boolean }) {
  const data = [
    ["部署任务", "Woodpecker Repo ID / 默认分支", "DEPLOY_ACTION=deploy，建议设置 PEAPOD_PROJECT_ID、PEAPOD_DEPLOY_MARKER_PATH、PEAPOD_DEPLOY_VERIFY_URL"],
    ["回退任务", "同一 Repo / 同一项目 ID", "DEPLOY_ACTION=rollback，确认词建议 ROLLBACK"],
    ["清理任务", "按项目自定义", "DEPLOY_ACTION=cleanup 或 disk-cleanup；普通部署不默认清理，确认词建议 CLEAN"],
    ["基础设施入口", "PEAPOD_LINKS_JSON", "配置额外外部系统入口"],
    ["监控主机", "PEAPOD_MONITOR_HOSTS_JSON", "配置机器、容器、Beszel 名称和 SSH 兜底"]
  ];
  return (
    <Space direction="vertical" size={16} className="side-stack">
      <Card title={compact ? "参数手册" : "部署参数手册"} extra={<Button href={state.links.woodpecker} target="_blank" icon={<ExternalLink size={16} />}>打开 Woodpecker</Button>}>
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
        description={`建议把清理动作放进独立 Woodpecker 任务，并在脚本里保护正在运行的 CI 容器、${PRODUCT_NAME} 镜像和业务关键卷。普通部署保持轻量；确实需要阈值触发时，再由目标项目显式开启 LOCAL_DOCKER_CLEANUP_AFTER_DEPLOY=auto。`}
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
  return rows.filter((row) => isRecentFailedPipeline(row, nowMs) && !hasNewerSuccessfulPipeline(rows, row)).length;
}

function overviewPipelineRows(rows: Pipeline[], nowMs: number, limit: number): Pipeline[] {
  return rows
    .filter((row) => ["running", "pending"].includes(row.status) || (isRecentFailedPipeline(row, nowMs) && !hasNewerSuccessfulPipeline(rows, row)))
    .slice(0, limit);
}

function hasNewerSuccessfulPipeline(rows: Pipeline[], failedRow: Pipeline): boolean {
  const failedAt = pipelineSortTime(failedRow);
  const failedKeys = pipelineAttentionKeys(failedRow);
  if (!failedAt || !failedKeys.length) return false;
  return rows.some((row) => {
    if (row.status !== "success") return false;
    if (pipelineSortTime(row) <= failedAt) return false;
    const successKeys = pipelineAttentionKeys(row);
    return successKeys.some((key) => failedKeys.includes(key));
  });
}

function pipelineAttentionKeys(row: Pipeline): string[] {
  const repo = row.repo_id || 0;
  const variables = row.variables || {};
  const keys = [
    row.peapod_task_id ? `${repo}:task:${row.peapod_task_id}` : "",
    variables.PEAPOD_PROJECT_ID ? `${repo}:project:${variables.PEAPOD_PROJECT_ID}` : "",
    variables.ZEPHYR_PROJECT_ID ? `${repo}:project:${variables.ZEPHYR_PROJECT_ID}` : "",
    variables.DEPLOY_ACTION ? `${repo}:action:${variables.DEPLOY_ACTION}:${variables.DEPLOY_TARGET || ""}:${variables.SOURCE_BRANCH || row.branch || ""}` : "",
    `${repo}:text:${pipelineTaskText(row)}:${row.branch || ""}`
  ];
  return Array.from(new Set(keys.filter(Boolean)));
}

function overviewDeploymentRows(rows: DeploymentStatus[], limit: number): DeploymentStatus[] {
  return sortDeploymentRows(rows)
    .filter((row) => row.deploy_verified && row.current_branch)
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
    dozzle_base_url: String(values.dozzle_base_url || "http://dozzle:8080").trim(),
    dozzle_public_url: String(values.dozzle_public_url || "").trim(),
    dozzle_username: String(values.dozzle_username || "").trim(),
    dozzle_password: String(values.dozzle_password || "").trim(),
    grafana_public_url: String(values.grafana_public_url || "").trim(),
    log_strategy: normalizeLogStrategyValue(values.log_strategy),
    docker_log_max_size: String(values.docker_log_max_size || "20m").trim(),
    docker_log_max_file: String(values.docker_log_max_file || "3").trim(),
    alert_webhook_url: String(values.alert_webhook_url || "").trim(),
    external_links: externalLinks,
    monitor_hosts: monitorHosts,
    monitor_refresh_seconds: Number(values.monitor_refresh_seconds || 20),
    monitor_warn_disk: Number(values.monitor_warn_disk || 80),
    monitor_crit_disk: Number(values.monitor_crit_disk || 90),
    monitor_warn_memory: Number(values.monitor_warn_memory || 80),
    monitor_auto_cleanup_level: String(values.monitor_auto_cleanup_level || "").trim(),
    monitor_auto_cleanup_disk: Number(values.monitor_auto_cleanup_disk || 0)
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
  if (status === "optional") return "default";
  if (status === "unknown") return "processing";
  if (status === "error" || status === "critical") return "error";
  return "warning";
}

function setupStatusText(status: string): string {
  if (status === "ok") return "已就绪";
  if (status === "optional") return "可选";
  if (status === "unknown") return "待确认";
  if (status === "error" || status === "critical") return "异常";
  return "待配置";
}

function readinessColor(value: string): string {
  if (value === "ready") return "success";
  if (value === "blocked") return "error";
  return "gold";
}

function readinessText(value: string): string {
  if (value === "ready") return "可以上线";
  if (value === "blocked") return "有阻断项";
  return "待补齐";
}

function logStrategyColor(value: string): string {
  if (value === "observability") return "blue";
  if (value === "external") return "purple";
  return "green";
}

function normalizeLogStrategyValue(value: unknown): "lightweight" | "observability" | "external" {
  const raw = String(value || "").trim();
  if (raw === "observability" || raw === "external") return raw;
  return "lightweight";
}

function normalizeLogQueryValues(values: Partial<LogQueryRequest>): LogQueryRequest {
  return {
    hosts: Array.isArray(values.hosts) ? values.hosts.map(String).filter(Boolean) : [],
    containers: Array.isArray(values.containers) ? values.containers.map(String).filter(Boolean) : [],
    keyword: String(values.keyword || "").trim(),
    level: String(values.level || "all"),
    since_minutes: Number(values.since_minutes || 15),
    tail: Number(values.tail || 200),
    stream: String(values.stream || "all")
  };
}

function logContainerValue(item: LogContainer): string {
  return `${item.host}|${item.id || item.name}`;
}

function defaultLogContainerValues(containers: LogContainer[]): string[] {
  const preferred = containers.filter((item) => {
    const name = `${item.host}/${item.host_name}/${item.name}`.toLowerCase();
    const running = ["running", "up"].some((state) => String(item.state || "").toLowerCase().includes(state));
    return running && /(peapod|woodpecker|api|server|worker|mysql|redis)/.test(name);
  });
  const pool = preferred.length ? preferred : containers.filter((item) => /(running|up)/i.test(item.state || ""));
  return pool.slice(0, 3).map(logContainerValue);
}

function filterLogContainers(containers: LogContainer[], keyword: string): LogContainer[] {
  const query = keyword.trim().toLowerCase();
  if (!query) return containers;
  return containers.filter((item) => {
    const haystack = [
      item.name,
      item.image,
      item.state,
      item.health,
      item.group,
      item.host,
      item.host_name
    ].filter(Boolean).join(" ").toLowerCase();
    return haystack.includes(query);
  });
}

function groupLogContainers(containers: LogContainer[]): Array<{ host: string; hostName: string; items: LogContainer[] }> {
  const rows = new Map<string, { host: string; hostName: string; items: LogContainer[] }>();
  for (const item of containers) {
    const key = item.host || item.host_name || "default";
    if (!rows.has(key)) {
      rows.set(key, { host: item.host, hostName: friendlyLogHostName(item.host_name || item.host || "本机"), items: [] });
    }
    rows.get(key)!.items.push(item);
  }
  return Array.from(rows.values()).map((group) => ({
    ...group,
    items: [...group.items].sort((left, right) => logContainerSortWeight(left) - logContainerSortWeight(right) || left.name.localeCompare(right.name))
  }));
}

function logContainerSortWeight(item: LogContainer): number {
  const name = item.name.toLowerCase();
  if (/peapod|woodpecker|api|server|worker/.test(name)) return 0;
  if (/mysql|redis|postgres/.test(name)) return 1;
  if (/beszel|dozzle|grafana|loki|prometheus/.test(name)) return 2;
  return 3;
}

function logContainerMeta(item: LogContainer): string {
  const image = shortImageName(item.image || "");
  const state = compactContainerStateText(item.state, item.health);
  return [state, image].filter(Boolean).join(" · ");
}

function compactContainerStateText(stateValue?: string, healthValue?: string): string {
  const state = String(stateValue || "").trim().toLowerCase();
  const health = String(healthValue || "").trim().toLowerCase();
  if (!state && !health) return "";
  if ([state, health].every((value) => !value || ["running", "up", "healthy", "configured"].includes(value))) return "";
  return [stateValue, healthValue].filter(Boolean).join(" / ");
}

function shortImageName(value: string): string {
  if (!value) return "";
  const withoutDigest = value.split("@")[0];
  const parts = withoutDigest.split("/");
  return parts[parts.length - 1] || withoutDigest;
}

function friendlyLogHostName(value?: string): string {
  const host = String(value || "").trim();
  if (!host) return "本机";
  if (isOpaqueLogHostID(host)) return `主机 ${host.slice(0, 8)}`;
  return host;
}

function isOpaqueLogHostID(value: string): boolean {
  return value.length >= 32 && (value.match(/-/g) || []).length >= 4;
}

function logSourceText(source: string): string {
  if (source === "dozzle_mcp") return "Dozzle MCP";
  if (source === "dozzle_mcp+ssh_fallback") return "Dozzle + SSH";
  if (source === "ssh_fallback") return "SSH 兜底";
  if (source === "monitoring_fallback") return "监控列表";
  if (source === "degraded") return "已降级";
  return source || "未知";
}

function logSourceColor(source: string): string {
  if (source === "dozzle_mcp") return "success";
  if (source === "dozzle_mcp+ssh_fallback") return "success";
  if (source === "ssh_fallback" || source === "monitoring_fallback") return "gold";
  if (source === "degraded") return "error";
  return "default";
}

function logLevelTone(level: string): "error" | "warn" | "debug" | "info" | "plain" {
  const value = String(level || "").toLowerCase();
  if (value.includes("error") || value.includes("fatal") || value.includes("panic")) return "error";
  if (value.includes("warn")) return "warn";
  if (value.includes("debug") || value.includes("trace")) return "debug";
  if (value.includes("info")) return "info";
  return "plain";
}

function inferLogLevel(line: LogLine): string {
  const level = String(line.level || "").trim().toLowerCase();
  if (level) return level;
  const message = String(line.message || "").toLowerCase();
  if (/\b(fatal|panic)\b/.test(message)) return "fatal";
  if (/\b(error|exception|failed|failure)\b/.test(message)) return "error";
  if (/\b(warn|warning)\b/.test(message)) return "warn";
  if (/\b(debug|trace)\b/.test(message)) return "debug";
  if (/\b(info)\b/.test(message)) return "info";
  return "log";
}

function summarizeLogMessage(message: string): string {
  const text = String(message || "").trim();
  if (!text) return "";
  if (!text.startsWith("{") && !text.startsWith("[")) return text;
  try {
    const payload = JSON.parse(text) as Record<string, unknown>;
    if (!payload || Array.isArray(payload)) return text;
    const method = stringField(payload, "method");
    const path = stringField(payload, "path") || stringField(payload, "uri") || stringField(payload, "url");
    const status = stringField(payload, "status") || stringField(payload, "status_code");
    const error = stringField(payload, "error") || stringField(payload, "err");
    const service = stringField(payload, "service_name") || stringField(payload, "service") || stringField(payload, "app");
    const duration = stringField(payload, "duration_ms") || stringField(payload, "latency") || stringField(payload, "elapsed");
    const msg = stringField(payload, "msg") || stringField(payload, "message");
    const requestID = stringField(payload, "request_id") || stringField(payload, "trace_id");
    const pieces: string[] = [];
    if (method || path) pieces.push([method, path].filter(Boolean).join(" "));
    if (status) pieces.push(`status ${status}`);
    if (error) pieces.push(`error ${error}`);
    if (msg && msg !== path) pieces.push(msg);
    if (service) pieces.push(service);
    if (duration) pieces.push(`${duration}ms`);
    if (requestID) pieces.push(`#${requestID.slice(0, 12)}`);
    return pieces.length ? pieces.join(" · ") : text;
  } catch {
    return text;
  }
}

function stringField(payload: Record<string, unknown>, key: string): string {
  const value = payload[key];
  if (value === null || value === undefined || value === "") return "";
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (typeof value === "string") return value.trim();
  return "";
}

function renderHighlightedLogMessage(message: string, keyword: string): ReactNode {
  const text = String(message || "");
  const query = keyword.trim();
  if (!query) return text;
  const lower = text.toLowerCase();
  const target = query.toLowerCase();
  const index = lower.indexOf(target);
  if (index < 0) return text;
  return (
    <>
      {text.slice(0, index)}
      <mark>{text.slice(index, index + query.length)}</mark>
      {text.slice(index + query.length)}
    </>
  );
}

function logLineTime(value?: string): string {
  if (!value) return "--:--:--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false
  }).format(date);
}

function formatLogLineForCopy(line: LogLine): string {
  return [
    line.timestamp || "",
    `${line.host_name || line.host}/${line.container_name}`,
    line.level || "",
    line.stream || "",
    line.message
  ].filter(Boolean).join(" ");
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
  if (row.peapod_task_title) return productText(row.peapod_task_title);
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

function pipelineKindText(row: Pipeline): string {
  if (row.event === "manual") return "手动触发";
  if (row.event === "push") return "Git push";
  return row.event || "流水线";
}

function pipelineVariableHint(row: Pipeline): string {
  const variables = row.variables || {};
  const preferredKeys = [
    "DEPLOY_ACTION",
    "DEPLOY_TARGET",
    "SOURCE_BRANCH",
    "BUILD_SERVICES",
    "DEPLOY_SERVICES",
    "DEPLOY_STRATEGY",
    "CLEANUP_MODE",
    "FORCE_DEPLOY"
  ];
  const preferred = preferredKeys
    .filter((key) => variables[key])
    .map((key) => [key, variables[key]] as [string, string]);
  const fallback = Object.entries(variables).filter(([key]) => !isSensitiveVariable(key) && !isNoisyPipelineVariable(key));
  const entries = [...preferred, ...fallback.filter(([key]) => !preferred.some(([used]) => used === key))];
  if (!entries.length) return "";
  return entries
    .slice(0, 2)
    .map(([key, value]) => `${key}=${shortPipelineVariableValue(value || "-")}`)
    .join(" · ");
}

function isNoisyPipelineVariable(key: string): boolean {
  return /MARKER|HEALTH|URL|PATH|DIR|SSH|CACHE|META|PREFIX/i.test(key);
}

function shortPipelineVariableValue(value: string): string {
  const text = String(value || "-");
  return text.length > 32 ? `${text.slice(0, 29)}...` : text;
}

function isSensitiveVariable(key: string): boolean {
  return /PASSWORD|TOKEN|SECRET|KEY|PRIVATE|CREDENTIAL|ACCESS/i.test(key);
}

function pipelineTriggerText(row: Pipeline): string {
  if (!row.peapod_triggered_by) {
    const actor = row.author || row.sender || "";
    if (row.event === "push") return actor ? `Git push：${actor}` : "Git push";
    return actor ? `Woodpecker：${actor}` : "外部触发/未记录";
  }
  return [PRODUCT_NAME + "：" + row.peapod_triggered_by, formatShortTime(row.peapod_triggered_at)]
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

function pipelineSortValue(row?: Pipeline): number {
  return row ? pipelineSortTime(row) : 0;
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

function cleanGroupLabel(value?: string): string {
  const label = String(value || "").trim();
  if (!label || label === "未归类" || label === "默认模块") return "";
  return productText(label);
}

function taskGroupLabel(state: StateResponse, task: Task): string {
  return cleanGroupLabel(task.group) || repoName(state, task);
}

function taskDescriptionLine(state: StateResponse, task: Task): string {
  return productText(task.description) || taskGroupLabel(state, task);
}

function deploymentScopeText(row: DeploymentStatus): string {
  const group = cleanGroupLabel(row.group);
  const repo = row.repo_name || `Repo ${row.repo_id}`;
  return group ? `${group} · ${repo}` : repo;
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

function productText(value?: string): string {
  return String(value || "")
    .replace(/Zefire(?:\s+(?:Deploy|Cloud))?/g, PRODUCT_NAME)
    .replace(/Zephyr/g, PRODUCT_NAME);
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

function deployVerifyColor(row: DeploymentStatus): string {
  if (!row.current_commit && row.latest_status !== "success") return "default";
  if (row.deploy_verified) return "success";
  switch (row.deploy_verify_status) {
    case "pipeline_only":
    case "marker_missing":
    case "marker_unavailable":
    case "external_marker":
      return "gold";
    case "marker_mismatch":
    case "health_failed":
      return "error";
    default:
      return row.last_status === "success" ? "gold" : statusColors[row.last_status] || "default";
  }
}

function deployVerifyText(row: DeploymentStatus): string {
  if (!row.current_commit && row.latest_status === "success" && row.deploy_verify_status) {
    if (row.deploy_verify_status === "pipeline_only") return "构建成功未验证";
    if (row.deploy_verify_status === "marker_mismatch") return "版本不一致";
    if (row.deploy_verify_status === "marker_missing") return "待接入验证";
    if (row.deploy_verify_status === "marker_unavailable") return "健康已确认";
    if (row.deploy_verify_status === "external_marker") return "外部部署已确认";
    if (row.deploy_verify_status === "health_failed") return "健康失败";
    return "待确认";
  }
  if (!row.current_commit) return "未部署";
  if (row.deploy_verified) return row.deploy_verify_status === "external_marker" ? "外部部署已确认" : row.deploy_degraded ? "健康已确认" : "已验证";
  switch (row.deploy_verify_status) {
    case "pipeline_only":
      return "构建成功未验证";
    case "marker_mismatch":
      return "版本不一致";
    case "marker_missing":
      return "待接入验证";
    case "marker_unavailable":
      return "健康已确认";
    case "external_marker":
      return "外部部署已确认";
    case "health_failed":
      return "健康失败";
    default:
      return row.last_status === "success" ? "待验证" : statusText(row.last_status);
  }
}

function deploymentVersionText(row: DeploymentStatus, nowMs: number): string {
  if (!row.current_branch) {
    if (row.latest_status === "success" && row.latest_commit) {
      const age = row.latest_at ? ` · ${deployedAgeText(row.latest_at, nowMs)}` : "";
      return `构建成功未验证 ${row.latest_branch || "-"} · ${(row.latest_commit || "").slice(0, 8)}${age}`;
    }
    return "暂无已验证部署";
  }
  const label = row.deploy_verified ? (row.deploy_verify_status === "external_marker" ? "外部部署已确认" : row.deploy_degraded ? "健康已确认" : "已验证") : row.deploy_verify_status === "pipeline_only" ? "流水线成功" : row.deploy_verify_status === "marker_missing" ? "待接入验证" : "待确认";
  const age = row.last_deployed_at ? ` · ${deployedAgeText(row.last_deployed_at, nowMs)}` : "";
  return `${label} ${row.current_branch} · ${(row.current_commit || "").slice(0, 8) || "-"}${age}`;
}

function sortDeploymentRows(rows: DeploymentStatus[]): DeploymentStatus[] {
  return [...rows].sort((a, b) => {
    const rank = deploymentAttentionRank(a) - deploymentAttentionRank(b);
    if (rank !== 0) return rank;
    const time = (b.last_deployed_at || b.latest_at || 0) - (a.last_deployed_at || a.latest_at || 0);
    if (time !== 0) return time;
    return (a.name || "").localeCompare(b.name || "", "zh-CN");
  });
}

function deploymentAttentionRank(row: DeploymentStatus): number {
  if (row.latest_status === "failure" || row.latest_status === "error" || row.last_status === "failure" || row.last_status === "error") return 1;
  if (row.deploy_verify_status === "marker_mismatch" || row.deploy_verify_status === "health_failed") return 1;
  if (row.deploy_verified) return 2;
  if (row.deploy_verify_status === "marker_missing" || row.deploy_verify_status === "marker_unavailable") return 3;
  if (!row.current_commit && row.latest_status === "success" && row.deploy_verify_status) return 3;
  if (row.current_commit) return 3;
  return 4;
}

function deployObjectEnvironmentLabel(row?: DeploymentStatus, task?: Task): string {
  const variables = { ...(row?.variables || {}), ...(task?.variables || {}) };
  const explicit = firstVariable(variables, [
    "PEAPOD_PROJECT_ENV",
    "ZEPHYR_PROJECT_ENV",
    "DEPLOY_ENV",
    "ENVIRONMENT",
    "APP_ENV",
    "DEPLOY_TARGET",
    "TARGET_HOST",
    "HOST",
    "MACHINE"
  ]);
  const source = [explicit, row?.group, row?.name, task?.group, task?.title, task?.description]
    .filter(Boolean)
    .join(" · ");
  const host = inferDeployHostLabel(source);
  const env = inferDeployEnvLabel(source);
  return [env, host].filter(Boolean).join(" / ") || "未标注";
}

function deployObjectBranchLabel(row?: DeploymentStatus, task?: Task): string {
  if (row?.current_branch) {
    const commit = (row.current_commit || "").slice(0, 8);
    const configured = row.configured_branch && row.configured_branch !== row.current_branch ? `，默认 ${row.configured_branch}` : "";
    return `线上 ${row.current_branch}${commit ? ` · ${commit}` : ""}${configured}`;
  }
  if (row?.latest_branch) {
    const commit = (row.latest_commit || "").slice(0, 8);
    return `最近 ${row.latest_branch}${commit ? ` · ${commit}` : ""}`;
  }
  const branch = task?.branch || row?.configured_branch || "main";
  return `默认 ${branch}`;
}

function firstVariable(variables: Record<string, string>, keys: string[]): string {
  for (const key of keys) {
    const value = String(variables[key] || variables[key.toLowerCase()] || "").trim();
    if (value) return value;
  }
  return "";
}

function inferDeployEnvLabel(source: string): string {
  const value = source.toLowerCase();
  if (/生产|prod|production/.test(value)) return "生产环境";
  if (/测试|test|staging|stage|dev|develop|development/.test(value)) return "测试环境";
  if (/构建|builder|build/.test(value)) return "构建环境";
  if (/观测|monitor|grafana|beszel|dozzle|observability/.test(value)) return "观测环境";
  return "";
}

function inferDeployHostLabel(source: string): string {
  const value = source.toLowerCase();
  if (/测试\/构建机|构建机|builder|build/.test(value)) return "测试/构建机";
  if (/生产机|production|prod/.test(value)) return "生产机";
  if (/e站|e站生产|estar|estack|lehu/.test(value)) return "e站机器";
  if (/写书猫|novelcat|xzm|context-flow/.test(value)) return "写书猫机器";
  if (/9router|router/.test(value)) return "9router";
  return "";
}

function shortDeploymentVerifyMessage(row: DeploymentStatus): string {
  if (!row.deploy_verify_message) return "";
  if (row.deploy_verified) return row.deploy_degraded ? "健康检查已通过，版本 marker 待接入" : "版本和健康检查已通过";
  if (row.deploy_verify_status === "pipeline_only") return "构建成功，部署未验证";
  if (row.deploy_verify_status === "marker_missing") return "版本验证待接入";
  if (row.deploy_verify_status === "marker_unavailable") return "健康已确认，marker 不可读";
  if (row.deploy_verify_status === "external_marker") return "外部部署已确认";
  if (row.deploy_verify_status === "marker_mismatch") return "版本不一致";
  if (row.deploy_verify_status === "health_failed") return "健康检查失败";
  return row.deploy_verify_message.length > 18 ? `${row.deploy_verify_message.slice(0, 18)}...` : row.deploy_verify_message;
}

function deployVerifyTone(row: DeploymentStatus): "success" | "warning" | "danger" | "muted" {
  if (row.deploy_verified && !row.deploy_degraded) return "success";
  if (row.deploy_verify_status === "health_failed" || row.deploy_verify_status === "marker_mismatch") return "danger";
  if (row.deploy_verify_status === "pipeline_only" || row.deploy_verify_status === "marker_missing" || row.deploy_verify_status === "marker_unavailable" || row.deploy_verify_status === "external_marker" || row.deploy_degraded) return "warning";
  return row.current_commit ? "warning" : "muted";
}

function normalizeDeployVerifyMessage(message: string): string {
  return message
    .replace(/版本 marker 读取失败/g, "版本 marker 暂不可读")
    .replace(/open ([^:；]+): no such file or directory/g, "未找到 marker 文件：$1")
    .replace(/open ([^:；]+): permission denied/g, "没有权限读取 marker 文件：$1");
}

function commitLooksSame(left?: string, right?: string): boolean {
  const a = (left || "").trim().toLowerCase();
  const b = (right || "").trim().toLowerCase();
  if (!a || !b) return false;
  return a.startsWith(b) || b.startsWith(a);
}

function buildDeployObjects(state: StateResponse, rows: DeploymentStatus[], tasks: Task[], pipelines: Pipeline[], nowMs: number): DeployObject[] {
  const deploymentRows = sortDeploymentRows(rows || []);
  const objects: DeployObject[] = deploymentRows.map((row) => {
    const actions = deploymentActionsForStatus(row, tasks || []);
    const actionTasks = [actions.deploy, actions.rollback, ...actions.extras].filter(Boolean) as Task[];
    const relatedPipelines = pipelines.filter((pipeline) => pipelineMatchesDeploymentObject(row, actionTasks, pipeline)).slice(0, 30);
    const status = row.deploy_verified ? "已验证" : deployVerifyText(row);
    const latestStatus = row.latest_status || row.last_status || "";
    const attention = deploymentAttentionRank(row) <= 1 || relatedPipelines.some((pipeline) => ["running", "pending"].includes(pipeline.status));
    return {
      id: row.id,
      kind: "deployment" as const,
      title: productText(row.name),
      subtitle: deploymentScopeText(row),
      environmentLabel: deployObjectEnvironmentLabel(row, actions.deploy || actions.rollback || actionTasks[0]),
      branchLabel: deployObjectBranchLabel(row, actions.deploy || actions.rollback || actionTasks[0]),
      statusLabel: status,
      statusColor: deployVerifyColor(row),
      attention,
      risk: latestStatus === "failure" || latestStatus === "error" ? "danger" : attention ? "warning" : "normal",
      deployment: row,
      primaryTask: actions.deploy,
      rollbackTask: actions.rollback,
      extraTasks: actions.extras,
      pipelines: relatedPipelines,
      searchText: [
        row.name,
        row.group,
        row.repo_name,
        row.configured_branch,
        row.current_branch,
        row.current_commit,
        row.latest_action,
        row.deploy_verify_message,
        actionTasks.map((task) => [task.id, task.title, variablesText(task.variables)].join(" ")).join(" ")
      ].join(" ").toLowerCase()
    };
  });

  const existingIDs = new Set(objects.map((item) => item.id));
  const configuredObjects = Array.from(groupDeployProjectTasks(tasks || []).entries())
    .filter(([projectKey]) => !existingIDs.has(projectKey))
    .map(([projectKey, projectTasks]) => buildConfiguredDeployObject(state, projectKey, projectTasks, pipelines));

  return [...objects, ...configuredObjects].sort((a, b) => {
    if (a.attention !== b.attention) return a.attention ? -1 : 1;
    const aTime = a.pipelines[0] ? pipelineSortTime(a.pipelines[0]) : 0;
    const bTime = b.pipelines[0] ? pipelineSortTime(b.pipelines[0]) : 0;
    if (aTime !== bTime) return bTime - aTime;
    return a.title.localeCompare(b.title, "zh-CN");
  });
}

function groupDeployProjectTasks(tasks: Task[]): Map<string, Task[]> {
  const groups = new Map<string, Task[]>();
  for (const task of tasks) {
    if (!isDeployProjectTask(task)) continue;
    const key = taskProjectKey(task);
    const current = groups.get(key) || [];
    current.push(task);
    groups.set(key, current);
  }
  for (const [key, rows] of groups.entries()) {
    groups.set(key, rows.sort(taskLibrarySort));
  }
  return groups;
}

function buildConfiguredDeployObject(state: StateResponse, projectKey: string, projectTasks: Task[], pipelines: Pipeline[]): DeployObject {
  const deploy = preferredDeployTask(projectTasks) || projectTasks.find((task) => !isRollbackTask(task)) || projectTasks[0];
  const rollback = projectTasks.find((task) => isRollbackTask(task));
  const actionTasks = [deploy, rollback, ...projectTasks.filter((task) => isDeploymentLikeTask(task) && task.id !== deploy?.id && task.id !== rollback?.id)].filter(Boolean) as Task[];
  const primary = deploy || actionTasks[0] || projectTasks[0];
  const relatedPipelines = pipelines.filter((pipeline) => actionTasks.some((task) => pipelineMatchesTask(task, pipeline))).slice(0, 30);
  const latest = relatedPipelines[0];
  const projectName = taskProjectDisplayName(projectTasks);
  const attention = ["running", "pending", "failure", "error"].includes(latest?.status || "");
  return {
    id: projectKey,
    kind: "deployment" as const,
    title: projectName,
    subtitle: `${taskGroupLabel(state, primary)} · ${repoName(state, primary)}`,
    environmentLabel: deployObjectEnvironmentLabel(undefined, primary),
    branchLabel: deployObjectBranchLabel(undefined, primary),
    statusLabel: latest ? statusText(latest.status) : "未部署",
    statusColor: latest ? statusColors[latest.status] || "default" : "default",
    attention,
    risk: ["failure", "error"].includes(latest?.status || "") ? "danger" : attention ? "warning" : primary.risk,
    primaryTask: deploy,
    rollbackTask: rollback,
    extraTasks: actionTasks.filter((task) => task.id !== deploy?.id && task.id !== rollback?.id),
    pipelines: relatedPipelines,
    searchText: [
      projectName,
      taskProjectID(primary),
      primary.group,
      repoName(state, primary),
      primary.branch,
      projectTasks.map((task) => [task.id, task.title, task.description, variablesText(task.variables)].join(" ")).join(" ")
    ].join(" ").toLowerCase()
  };
}

function filterDeployObjects(objects: DeployObject[], query: string): DeployObject[] {
  const q = query.trim().toLowerCase();
  return objects.filter((item) => {
    if (q && !item.searchText.includes(q)) return false;
    return true;
  });
}

function pipelineMatchesDeploymentObject(row: DeploymentStatus, tasks: Task[], pipeline: Pipeline): boolean {
  if (Number(pipeline.repo_id || 0) !== row.repo_id) return false;
  if (pipeline.number === row.pipeline || pipeline.number === row.latest_pipeline || pipeline.number === row.previous_pipeline) return true;
  const taskIDs = new Set(tasks.map((task) => task.id));
  if (pipeline.peapod_task_id && taskIDs.has(pipeline.peapod_task_id)) return true;
  const projectID = pipelineProjectID(pipeline);
  if (projectID && `repo-${row.repo_id}:${normalizeKey(projectID)}` === row.id) return true;
  return false;
}

function pipelineMatchesTask(task: Task, pipeline: Pipeline): boolean {
  if (Number(pipeline.repo_id || 0) !== task.repo_id) return false;
  if (pipeline.peapod_task_id === task.id) return true;
  if (pipeline.peapod_task_title && normalizeKey(pipeline.peapod_task_title) === normalizeKey(task.title)) return true;
  const taskProjectID = task.variables?.PEAPOD_PROJECT_ID || task.variables?.ZEPHYR_PROJECT_ID || "";
  const pipelineProject = pipelineProjectID(pipeline);
  if (taskProjectID && pipelineProject && normalizeKey(taskProjectID) === normalizeKey(pipelineProject)) return true;
  const action = String(task.variables?.DEPLOY_ACTION || "").toLowerCase();
  const pipelineAction = String(pipeline.variables?.DEPLOY_ACTION || "").toLowerCase();
  return Boolean(action && pipelineAction && action === pipelineAction && normalizeKey(task.group || task.title) === normalizeKey(pipeline.peapod_task_title || pipeline.message || ""));
}

function pipelineProjectID(pipeline: Pipeline): string {
  const variables = pipeline.variables || {};
  return variables.PEAPOD_PROJECT_ID || variables.ZEPHYR_PROJECT_ID || variables.PROJECT_ID || variables.SERVICE_ID || variables.DEPLOY_SERVICE || variables.APP || variables.PROJECT || "";
}

function taskLibrarySort(a: Task, b: Task): number {
  const riskRank: Record<string, number> = { danger: 0, warning: 1, normal: 2, link: 3 };
  const risk = (riskRank[a.risk] ?? 9) - (riskRank[b.risk] ?? 9);
  if (risk !== 0) return risk;
  const group = taskGroupSortText(a).localeCompare(taskGroupSortText(b), "zh-CN");
  if (group !== 0) return group;
  return productText(a.title).localeCompare(productText(b.title), "zh-CN");
}

function taskGroupSortText(task: Task): string {
  return cleanGroupLabel(task.group) || String(task.repo_name || task.repo_id || "");
}

export function canRunTask(user: User, task: Task): boolean {
  if (task.disabled) return false;
  const roles = task.allowed_roles || [];
  if (!roles.length) return true;
  return roles.includes(user.role);
}

function taskDisabledTitle(user: User, task: Task): string {
  if (task.disabled) return task.disabled_reason || "这个任务当前不可执行";
  if (!canRunTask(user, task)) return "当前账号没有权限执行";
  return "";
}

export function isRollbackTask(task: Task): boolean {
  const variables = task.variables || {};
  return variables.DEPLOY_ACTION === "rollback" || Boolean(variables.ROLLBACK_VERSION) || /rollback|回退|回滚/i.test(task.id + task.title);
}

export function deploymentStatusForTask(task: Task, statuses: DeploymentStatus[]): DeploymentStatus | undefined {
  const key = taskProjectKey(task);
  return statuses.find((item) => item.id === key || (item.repo_id === task.repo_id && normalizeKey(item.group || item.name) === normalizeKey(task.group || task.title)));
}

export function rollbackRevisionKey(revision: DeploymentRevision): string {
  return `${revision.pipeline}:${revision.branch || ""}:${revision.commit || ""}`;
}

export function rollbackRevisionLabel(revision: DeploymentRevision, nowMs: number): string {
  const commit = revision.commit ? revision.commit.slice(0, 8) : "-";
  const branch = revision.branch || "-";
  const age = revision.deployed_at ? deployedAgeText(revision.deployed_at, nowMs) : "时间未知";
  const actor = revision.triggered_by ? ` · ${revision.triggered_by}` : "";
  return `#${revision.pipeline} · ${branch} · ${commit} · ${age}${actor}`;
}

export function rollbackRevisionsForStatus(status?: DeploymentStatus): DeploymentRevision[] {
  if (!status) return [];
  const currentKey = `${status.pipeline}:${status.current_branch || ""}:${status.current_commit || ""}`;
  const revisions = (status.revisions || []).filter((revision) => {
    if (!revision.commit && !revision.pipeline) return false;
    return `${revision.pipeline}:${revision.branch || ""}:${revision.commit || ""}` !== currentKey;
  });
  if (revisions.length) return revisions;
  if (status.previous_branch || status.previous_commit || status.previous_pipeline || status.previous_deployed_at) {
    return [
      {
        pipeline: status.previous_pipeline || 0,
        branch: status.previous_branch || "-",
        commit: status.previous_commit || "",
        deployed_at: status.previous_deployed_at || 0,
        action: status.previous_action || "上一成功部署",
        verified: true
      }
    ];
  }
  return [];
}

function firstRollbackRevision(status?: DeploymentStatus): DeploymentRevision | undefined {
  return rollbackRevisionsForStatus(status)[0];
}

export function rollbackPresetInputs(revision: DeploymentRevision): Record<string, string> {
  const pipeline = revision.pipeline ? String(revision.pipeline) : "";
  const commit = revision.commit || "";
  const branch = revision.branch || "";
  return {
    rollback_revision: rollbackRevisionKey(revision),
    ROLLBACK_VERSION: commit,
    ROLLBACK_COMMIT: commit,
    ROLLBACK_BRANCH: branch,
    ROLLBACK_PIPELINE: pipeline,
    ROLLBACK_PIPELINE_NUMBER: pipeline,
    PEAPOD_ROLLBACK_COMMIT: commit,
    PEAPOD_ROLLBACK_BRANCH: branch,
    PEAPOD_ROLLBACK_PIPELINE: pipeline
  };
}

function deploymentActionsForStatus(row: DeploymentStatus, tasks: Task[]): { deploy?: Task; rollback?: Task; extras: Task[] } {
  let candidates = tasks.filter((task) => !task.external_url && taskMatchesDeploymentStatus(task, row));
  if (!candidates.length) {
    candidates = tasks.filter((task) => taskLooselyMatchesDeploymentStatus(task, row));
  }
  const deploy = preferredDeployTask(candidates);
  const rollback = candidates.find((task) => isRollbackTask(task));
  return {
    deploy,
    rollback,
    extras: candidates.filter((task) => isDeploymentLikeTask(task) && task.id !== deploy?.id && task.id !== rollback?.id)
  };
}

function deploymentManagedTaskIDs(tasks: Task[], statuses: DeploymentStatus[]): string[] {
  const ids = new Set<string>();
  for (const task of tasks) {
    if (isDeployProjectTask(task)) ids.add(task.id);
  }
  for (const status of statuses) {
    const actions = deploymentActionsForStatus(status, tasks);
    if (actions.deploy) ids.add(actions.deploy.id);
    if (actions.rollback) ids.add(actions.rollback.id);
    for (const task of actions.extras) ids.add(task.id);
  }
  return Array.from(ids);
}

function taskMatchesDeploymentStatus(task: Task, row: DeploymentStatus): boolean {
  if (taskProjectKey(task) === row.id) return true;
  if (task.repo_id !== row.repo_id) return false;
  const taskKey = normalizeKey([task.id, task.title, task.group, Object.values(task.variables || {}).join(" ")].join(" "));
  const rowKeys = [row.id, row.name, row.group]
    .map((item) => normalizeKey(item))
    .filter((item) => item.length >= 2);
  if (rowKeys.some((key) => taskKey.includes(key))) return true;
  const group = normalizeKey(task.group || "");
  return Boolean(group && rowKeys.includes(group));
}

function taskLooselyMatchesDeploymentStatus(task: Task, row: DeploymentStatus): boolean {
  if (!isDeployProjectTask(task) || task.repo_id !== row.repo_id) return false;
  const rowKeys = [row.id, row.name, row.group]
    .map((item) => normalizeKey(productText(item || "")))
    .filter((item) => item.length >= 3);
  if (!rowKeys.length) return false;
  const taskKeys = [
    taskProjectID(task),
    taskProjectDisplayName([task]),
    task.group,
    task.title
  ]
    .map((item) => normalizeKey(productText(item || "")))
    .filter((item) => item.length >= 3);
  return taskKeys.some((taskKey) => rowKeys.some((rowKey) => taskKey === rowKey || taskKey.includes(rowKey) || rowKey.includes(taskKey)));
}

function preferredDeployTask(tasks: Task[]): Task | undefined {
  const deployTasks = tasks.filter((task) => isDeploymentLikeTask(task) && !isRollbackTask(task) && !isCleanupTask(task));
  return deployTasks.find((task) => !isForceDeployTask(task)) || deployTasks[0];
}

function taskProjectKey(task: Task): string {
  const id = taskProjectID(task) || task.group || task.id;
  return `repo-${task.repo_id}:${normalizeKey(id)}`;
}

function taskProjectID(task: Task): string {
  const variables = task.variables || {};
  return variables.PEAPOD_PROJECT_ID || variables.ZEPHYR_PROJECT_ID || variables.PROJECT_ID || variables.SERVICE_ID || variables.DEPLOY_SERVICE || variables.APP || variables.PROJECT || "";
}

function taskProjectDisplayName(tasks: Task[]): string {
  const primary = preferredDeployTask(tasks) || tasks.find((task) => !isRollbackTask(task)) || tasks[0];
  if (!primary) return "未命名项目";
  const variables = primary.variables || {};
  const explicit = variables.PEAPOD_PROJECT_NAME || variables.ZEPHYR_PROJECT_NAME || variables.PROJECT_NAME || variables.SERVICE_NAME || "";
  if (explicit) return productText(explicit);
  const deployTitle = productText(primary.title || "").replace(/(强制)?部署|发布|回退|rollback|deploy|release/gi, "").replace(/[·｜|-]+$/g, "").trim();
  return deployTitle || taskProjectID(primary) || cleanGroupLabel(primary.group) || primary.repo_name || primary.id;
}

function isDeployProjectTask(task: Task): boolean {
  if (task.external_url) return false;
  if (!taskProjectID(task)) return false;
  return isDeploymentLikeTask(task);
}

function deployTaskRoleText(task: Task): string {
  if (isRollbackTask(task)) return "回退";
  if (isForceDeployTask(task)) return "强制部署";
  if (isDeploymentLikeTask(task)) return "部署";
  return "任务";
}

function maintenanceTaskKindText(task: Task): string {
  if (isCleanupTask(task)) return "清理";
  if (isRestartTask(task)) return "重启";
  const action = String(task.variables?.DEPLOY_ACTION || "").toLowerCase();
  if (action.includes("status") || /状态|检查|check|status/i.test(task.title)) return "状态检查";
  if (task.risk === "danger") return "高危操作";
  return "维护";
}

function normalizeKey(value: string): string {
  return String(value || "")
    .trim()
    .toLowerCase()
    .replace(/[^\p{L}\p{N}_-]+/gu, "-")
    .replace(/^-+|-+$/g, "");
}

function isCleanupTask(task: Task): boolean {
  const action = String(task.variables?.DEPLOY_ACTION || "").toLowerCase();
  return action.includes("cleanup") || action.includes("clean") || action.includes("disk");
}

function isRestartTask(task: Task): boolean {
  const action = String(task.variables?.DEPLOY_ACTION || "").toLowerCase();
  return action.includes("restart") || /重启|restart/i.test(task.title);
}

function isForceDeployTask(task: Task): boolean {
  const variables = task.variables || {};
  return String(variables.FORCE_DEPLOY || variables.FORCE || "").toLowerCase() === "true" || /强制|force/i.test(task.title);
}

function isDeploymentLikeTask(task: Task): boolean {
  if (isCleanupTask(task) || isRestartTask(task)) return false;
  if (isRollbackTask(task)) return true;
  const action = String(task.variables?.DEPLOY_ACTION || "").toLowerCase();
  if (["deploy", "site", "publish", "release", "zefire", "zephyr", "peapod"].includes(action)) return true;
  return /部署|发布|deploy|publish|release/i.test(task.title);
}

function taskHasVerification(task: Task): boolean {
  const variables = task.variables || {};
  return Boolean(
    variables.PEAPOD_DEPLOY_MARKER_PATH ||
      variables.PEAPOD_DEPLOY_VERIFY_URL ||
      variables.PEAPOD_HEALTH_URL ||
      variables.ZEPHYR_DEPLOY_MARKER_PATH ||
      variables.ZEPHYR_DEPLOY_VERIFY_URL ||
      variables.ZEPHYR_HEALTH_URL ||
      variables.DEPLOY_MARKER_PATH ||
      variables.DEPLOY_HEALTH_URL ||
      variables.HEALTH_URL
  );
}

function taskVariableSummaryTags(task: Task): string[] {
  const variables = task.variables || {};
  const keys = [
    "DEPLOY_ACTION",
    "PEAPOD_PROJECT_ENV",
    "PEAPOD_PROJECT_ID",
    "PEAPOD_PROJECT_NAME",
    "DEPLOY_STRATEGY",
    "CLEANUP_MODE"
  ];
  const tags = keys
    .filter((key) => variables[key])
    .map((key) => `${key}=${shortPipelineVariableValue(variables[key] || "-")}`);
  if (!tags.length) {
    const fallback = Object.entries(variables)
      .filter(([key]) => !isSensitiveVariable(key))
      .slice(0, 3)
      .map(([key, value]) => `${key}=${shortPipelineVariableValue(value || "-")}`);
    return fallback.length ? fallback : ["未配置变量"];
  }
  return tags;
}

function monitoringSourceText(source: string): string {
  if (source === "beszel") return "Beszel";
  if (source === "ssh_fallback") return "SSH 兜底";
  if (source === "degraded") return "已降级";
  if (source === "configured") return "已配置";
  return source || "未知";
}

function monitoringSourceColor(source: string): string {
  if (source === "beszel") return "success";
  if (source === "ssh_fallback") return "gold";
  if (source === "degraded") return "error";
  if (source === "configured") return "default";
  return "default";
}

function monitoringStatusColor(status: string): string {
  const value = String(status || "").toLowerCase();
  if (!value || value === "unknown") return "default";
  if (["ok", "up", "online", "healthy", "active", "normal", "success", "configured"].includes(value)) return "success";
  if (["warning", "degraded"].includes(value)) return "gold";
  return "error";
}

function monitoringHostStatusText(status: string): string {
  const value = String(status || "").toLowerCase();
  if (!value || value === "unknown") return "未知";
  if (value === "configured") return "已配置";
  if (["ok", "up", "online", "healthy", "active", "normal", "success"].includes(value)) return "正常";
  if (["warning", "degraded"].includes(value)) return "提醒";
  return status;
}

function monitoringHostMeta(row: MonitoringHost): string {
  const message = String(row.message || "").trim();
  if (!message || message.toLowerCase() === "configured") return monitoringRoleText(row.role);
  return `${monitoringRoleText(row.role)} · ${message}`;
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
