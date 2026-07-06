import {
  Alert,
  App as AntApp,
  Button,
  Card,
  Col,
  ConfigProvider,
  Drawer,
  Form,
  Input,
  Layout,
  Menu,
  Modal,
  Row,
  Select,
  Space,
  Tag,
  Typography,
  theme
} from "antd";
import { LogOut, Menu as MenuIcon } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { api, errorText } from "./api";
import { ZephyrLogo } from "./Logo";
import type {
  AuditRecord,
  DeploymentStatus,
  MonitoringSummary,
  Pipeline,
  PipelineSummary,
  Risk,
  RunResult,
  StateResponse,
  Task,
  TaskConfig
} from "./types";
import {
  DeployErrorContent,
  DeployPage,
  Docs,
  MonitoringView,
  OverviewPage,
  PipelinePage,
  PipelineSummaryDrawer,
  SettingsPage,
  InfrastructureLinks,
  TaskRunContext,
  branchOptionsForRepo,
  branchOptionsForTask,
  canRunTask,
  flattenPipelines,
  parseVariables,
  pipelineURL,
  variablesText,
  zephyrNavItems
} from "./pages";

const { Header, Content, Sider } = Layout;
const { Text, Title } = Typography;


export function App() {
  return (
    <ConfigProvider
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: "#ef6b44",
          borderRadius: 8,
          fontFamily:
            'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif'
        },
        components: {
          Card: { borderRadiusLG: 8 },
          Button: { borderRadius: 7 },
          Table: { headerBg: "#f7f9fb" }
        }
      }}
    >
      <AntApp>
        <Router />
      </AntApp>
    </ConfigProvider>
  );
}

function Router() {
  const path = window.location.pathname;
  if (path === "/login") return <LoginPage />;
  return <Shell page={path === "/docs" ? "docs" : "home"} />;
}

function LoginPage() {
  const { message } = AntApp.useApp();
  const [loading, setLoading] = useState(false);

  async function submit(values: { username: string; password: string }) {
    setLoading(true);
    try {
      await api("/api/login", {
        method: "POST",
        body: JSON.stringify(values)
      });
      window.location.href = "/";
    } catch (error) {
      message.error(errorText(error) || "账号、邮箱或密码不正确");
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="login-page">
      <Card className="login-card">
        <ZephyrLogo className="login-logo" title="Zephyr" />
        <Title level={2}>Zephyr</Title>
        <Text type="secondary">基础设施部署控制台</Text>
        <Form layout="vertical" onFinish={submit} className="login-form">
          <Form.Item label="账号或邮箱" name="username">
            <Input autoComplete="username" autoFocus />
          </Form.Item>
          <Form.Item label="密码" name="password" rules={[{ required: true, message: "请输入密码" }]}>
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={loading}>
            进入控制台
          </Button>
        </Form>
      </Card>
    </main>
  );
}

function Shell({ page }: { page: "home" | "docs" }) {
  const { message, modal } = AntApp.useApp();
  const [state, setState] = useState<StateResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [runTask, setRunTask] = useState<Task | null>(null);
  const [runForm] = Form.useForm();
  const [taskDrawerOpen, setTaskDrawerOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<Task | null>(null);
  const [taskForm] = Form.useForm();
  const taskFormRepoID = Form.useWatch("repo_id", taskForm);
  const taskFormBranch = Form.useWatch("branch", taskForm);
  const [customConfig, setCustomConfig] = useState<TaskConfig | null>(null);
  const [triggeringTaskIds, setTriggeringTaskIds] = useState<string[]>([]);
  const [activePage, setActivePage] = useState("overview");
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [auditRecords, setAuditRecords] = useState<AuditRecord[]>([]);
  const [auditLoading, setAuditLoading] = useState(false);
  const [monitoring, setMonitoring] = useState<MonitoringSummary | null>(null);
  const [monitoringLoading, setMonitoringLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [pipelineSummaryOpen, setPipelineSummaryOpen] = useState(false);
  const [pipelineSummary, setPipelineSummary] = useState<PipelineSummary | null>(null);
  const [pipelineSummaryLoading, setPipelineSummaryLoading] = useState(false);
  const [nowMs, setNowMs] = useState(() => Date.now());
  const triggeringTaskIdsRef = useRef<Set<string>>(new Set());

  function setTaskTriggering(taskID: string, active: boolean) {
    if (active) {
      triggeringTaskIdsRef.current.add(taskID);
    } else {
      triggeringTaskIdsRef.current.delete(taskID);
    }
    setTriggeringTaskIds(Array.from(triggeringTaskIdsRef.current));
  }

  async function loadState(options: { notify?: boolean } = {}) {
    if (options.notify) {
      setRefreshing(true);
      message.open({ type: "loading", content: "正在刷新状态", key: "state-refresh", duration: 0 });
    }
    try {
      const data = await api<StateResponse>("/api/state");
      setState(data);
      if (options.notify) {
        message.open({ type: "success", content: "状态已刷新", key: "state-refresh", duration: 1.8 });
      }
    } catch (error) {
      if (String(error).includes("unauthorized")) {
        window.location.href = "/login";
        return;
      }
      if (options.notify) {
        message.open({ type: "error", content: errorText(error) || "刷新失败", key: "state-refresh", duration: 4 });
      } else {
        message.error(errorText(error) || "状态加载失败");
      }
    } finally {
      setLoading(false);
      if (options.notify) {
        setRefreshing(false);
      }
    }
  }

  async function refreshState() {
    if (refreshing) return;
    await loadState({ notify: true });
  }

  async function loadCustomConfig() {
    if (!state?.configurable) return;
    try {
      setCustomConfig(await api<TaskConfig>("/api/config/tasks"));
    } catch {
      setCustomConfig({ tasks: [] });
    }
  }

  useEffect(() => {
    loadState();
    loadMonitoring();
    const timer = window.setInterval(loadState, 12000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    const timer = window.setInterval(() => setNowMs(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    loadCustomConfig();
  }, [state?.configurable]);

  useEffect(() => {
    if (activePage === "settings") {
      loadAudit();
    }
    if (activePage === "monitoring") {
      loadMonitoring();
    }
  }, [activePage]);

  useEffect(() => {
    if (activePage !== "monitoring") return;
    const timer = window.setInterval(() => loadMonitoring(), 20000);
    return () => window.clearInterval(timer);
  }, [activePage]);

  async function loadAudit(options: { notify?: boolean } = {}) {
    setAuditLoading(true);
    try {
      const data = await api<{ records: AuditRecord[] }>("/api/audit?limit=120");
      setAuditRecords(data.records || []);
      if (options.notify) message.success("操作历史已刷新");
    } catch (error) {
      message.error(errorText(error) || "操作历史加载失败");
    } finally {
      setAuditLoading(false);
    }
  }

  async function loadMonitoring(options: { notify?: boolean } = {}) {
    setMonitoringLoading(true);
    if (options.notify) {
      message.open({ type: "loading", content: "正在刷新监控", key: "monitoring-refresh", duration: 0 });
    }
    try {
      const data = await api<MonitoringSummary>("/api/monitoring/summary");
      setMonitoring(data);
      if (options.notify) {
        message.open({ type: "success", content: "监控已刷新", key: "monitoring-refresh", duration: 1.8 });
      }
    } catch (error) {
      if (options.notify) {
        message.open({ type: "error", content: errorText(error) || "监控刷新失败", key: "monitoring-refresh", duration: 4 });
      } else {
        message.error(errorText(error) || "监控加载失败");
      }
    } finally {
      setMonitoringLoading(false);
    }
  }

  function openNewTask() {
    setEditingTask(null);
    taskForm.resetFields();
    taskForm.setFieldsValue({ branch: "main", risk: "normal", group: "自定义任务" });
    setTaskDrawerOpen(true);
  }

  function openRunTask(task: Task) {
    if (triggeringTaskIdsRef.current.has(task.id)) return;
    if (state && !canRunTask(state.current_user, task)) {
      message.warning("这个动作只允许管理员执行");
      return;
    }
    setRunTask(task);
    runForm.resetFields();
    runForm.setFieldsValue({ branch: task.branch || "main" });
  }

  function openEditTask(task: Task) {
    setEditingTask(task);
    taskForm.setFieldsValue({
      ...task,
      variables: variablesText(task.variables)
    });
    setTaskDrawerOpen(true);
  }

  async function logout() {
    await api("/api/logout", { method: "POST" }).catch(() => undefined);
    window.location.href = "/login";
  }

  function appendOptimisticPipeline(task: Task, branch: string, inputs: Record<string, string>, pipeline: Pipeline, triggeredAt?: string) {
    setState((previous) => {
      if (!previous) return previous;
      const repoKey = String(task.repo_id);
      const variables = { ...(task.variables || {}) };
      for (const [key, value] of Object.entries(inputs || {})) {
        if (value) variables[key] = value;
      }
      const nowSeconds = Math.floor(Date.now() / 1000);
      const optimistic: Pipeline = {
        ...pipeline,
        repo_id: task.repo_id,
        repo_name: task.repo_name || previous.repos[repoKey] || `Repo ${task.repo_id}`,
        branch: pipeline.branch || branch || task.branch || "main",
        event: pipeline.event || "manual",
        status: pipeline.status || "pending",
        created: pipeline.created || nowSeconds,
        started: pipeline.started || 0,
        finished: pipeline.finished || 0,
        variables,
        zefire_triggered_by: previous.current_user.username,
        zefire_triggered_at: triggeredAt || new Date().toISOString(),
        zefire_task_id: task.id,
        zefire_task_title: task.title
      };
      const repoPipelines = previous.pipelines?.[repoKey] || [];
      const nextRepoPipelines = [optimistic, ...repoPipelines.filter((item) => item.number !== optimistic.number)];
      return {
        ...previous,
        pipelines: {
          ...(previous.pipelines || {}),
          [repoKey]: nextRepoPipelines
        }
      };
    });
  }

  async function executeTask(values: Record<string, string>) {
    const task = runTask;
    if (!task || triggeringTaskIdsRef.current.has(task.id)) return;
    if (task.confirm_text && values.confirm_text !== task.confirm_text) {
      message.warning("确认文字不匹配");
      return;
    }
    const inputs: Record<string, string> = {};
    for (const item of task.inputs || []) {
      inputs[item.name] = values[item.name] || "";
    }
    const branch = String(values.branch || task.branch || "main").trim();
    setTaskTriggering(task.id, true);
    setRunTask(null);
    runForm.resetFields();
    const messageKey = `run-${task.id}`;
    message.open({ type: "loading", content: `正在触发 ${task.title}`, key: messageKey, duration: 0 });
    try {
      const result = await api<RunResult>(`/api/tasks/${task.id}/run`, {
        method: "POST",
        body: JSON.stringify({ inputs, branch })
      });
      appendOptimisticPipeline(result.task || task, branch, inputs, result.pipeline, result.triggered_at);
      message.open({ type: "success", content: `已触发流水线 #${result.pipeline.number}`, key: messageKey, duration: 3 });
      await loadState();
    } catch (error) {
      const text = errorText(error) || "触发失败";
      message.open({ type: "error", content: text, key: messageKey, duration: 4 });
      modal.error({
        title: `${task.title} 触发失败`,
        content: <DeployErrorContent error={error} task={task} />
      });
    } finally {
      setTaskTriggering(task.id, false);
    }
  }

  async function cancelPipeline(row: Pipeline) {
    try {
      await api(`/api/pipelines/${row.repo_id}/${row.number}/cancel`, { method: "POST" });
      message.success(`已请求取消 #${row.number}`);
      await loadState();
    } catch (error) {
      message.error(errorText(error) || "取消失败");
    }
  }

  async function openPipelineSummary(row: Pipeline) {
    if (!row.repo_id || !row.number) return;
    setPipelineSummaryOpen(true);
    setPipelineSummary(null);
    setPipelineSummaryLoading(true);
    try {
      setPipelineSummary(await api<PipelineSummary>(`/api/pipelines/${row.repo_id}/${row.number}/summary`));
    } catch (error) {
      message.error(errorText(error) || "流水线详情加载失败");
      setPipelineSummary({
        pipeline: row,
        steps: [],
        failure_summary: errorText(error) || "流水线详情加载失败",
        log_tail: [],
        woodpecker_url: pipelineURL(state?.links.woodpecker || "", row)
      });
    } finally {
      setPipelineSummaryLoading(false);
    }
  }

  async function saveCustomTask(values: Record<string, unknown>) {
    const variables = parseVariables(String(values.variables || ""));
    const task: Task = {
      id: String(values.id || editingTask?.id || ""),
      title: String(values.title || ""),
      group: String(values.group || "自定义任务"),
      description: String(values.description || ""),
      repo_id: Number(values.repo_id || 0),
      repo_name: String(values.repo_name || ""),
      branch: String(values.branch || "main"),
      risk: String(values.risk || "normal") as Risk,
      confirm_text: String(values.confirm_text || ""),
      variables
    };
    try {
      await api("/api/config/tasks", {
        method: "POST",
        body: JSON.stringify(task)
      });
      message.success(editingTask?.builtin ? "内置任务覆盖配置已保存" : "任务配置已保存");
      setTaskDrawerOpen(false);
      setEditingTask(null);
      taskForm.resetFields();
      await loadState();
      await loadCustomConfig();
    } catch (error) {
      message.error(errorText(error) || "保存失败");
    }
  }

  async function deleteCustomTask(task: Task) {
    try {
      await api(`/api/config/tasks/${encodeURIComponent(task.id)}`, { method: "DELETE" });
      message.success(task.overridden ? "已恢复默认配置" : "任务已删除");
      await loadState();
      await loadCustomConfig();
    } catch (error) {
      message.error(errorText(error) || "删除失败");
    }
  }

  const pipelines = useMemo(() => flattenPipelines(state), [state]);
  const deploymentStatuses = state?.deployment_statuses || [];
  const runningCount = pipelines.filter((item) => ["running", "pending"].includes(item.status)).length;
  const failedCount = pipelines.filter((item) => ["failure", "error"].includes(item.status)).length;
  const navItems = zephyrNavItems();

  function navigate(key: string) {
    setActivePage(key);
    setMobileNavOpen(false);
  }

  function renderPage() {
    if (!state) return null;
    switch (activePage) {
      case "deploy":
        return (
          <DeployPage
            state={state}
            rows={deploymentStatuses}
            woodpecker={state.links.woodpecker}
            nowMs={nowMs}
            tasks={state.tasks || []}
            currentUser={state.current_user}
            triggeringTaskIds={triggeringTaskIds}
            refreshing={refreshing}
            onRun={openRunTask}
            onRefresh={refreshState}
          />
        );
      case "pipelines":
        return (
          <PipelinePage
            rows={pipelines}
            woodpecker={state.links.woodpecker}
            nowMs={nowMs}
            refreshing={refreshing}
            onRefresh={refreshState}
            onCancel={cancelPipeline}
            onInspect={openPipelineSummary}
          />
        );
      case "monitoring":
        return (
          <MonitoringView
            state={state}
            summary={monitoring}
            loading={monitoringLoading}
            nowMs={nowMs}
            onRefresh={() => loadMonitoring({ notify: true })}
            onRun={openRunTask}
          />
        );
      case "links":
        return <InfrastructureLinks tasks={state.tasks || []} />;
      case "docs":
        return <Docs state={state} />;
      case "settings":
        return (
          <SettingsPage
            state={state}
            customConfig={customConfig}
            auditRecords={auditRecords}
            auditLoading={auditLoading}
            onReload={loadState}
            onAuditRefresh={() => loadAudit({ notify: true })}
            onAddTask={openNewTask}
            onEditTask={openEditTask}
            onDeleteTask={deleteCustomTask}
          />
        );
      default:
        return (
          <OverviewPage
            state={state}
            monitoring={monitoring}
            monitoringLoading={monitoringLoading}
            pipelines={pipelines}
            deploymentStatuses={deploymentStatuses}
            runningCount={runningCount}
            failedCount={failedCount}
            nowMs={nowMs}
            triggeringTaskIds={triggeringTaskIds}
            onRun={openRunTask}
            onNavigate={navigate}
            onRefresh={refreshState}
            onInspectPipeline={openPipelineSummary}
          />
        );
    }
  }

  if (loading || !state) {
    return <LoadingShell />;
  }

  return (
    <Layout className="app-layout">
      <Header className="app-header">
        <Space size={12}>
          <Button className="mobile-nav-button" icon={<MenuIcon size={18} />} onClick={() => setMobileNavOpen(true)} />
          <ZephyrLogo className="header-logo" title="Zephyr" />
          <div>
            <Text className="eyebrow">Infrastructure Console</Text>
            <Title level={4} className="header-title">
              Zephyr
            </Title>
          </div>
        </Space>
        <Space className="header-actions" size={10}>
          <Tag className="user-pill">{state.current_user.display_name || state.current_user.username}</Tag>
          <Button icon={<LogOut size={16} />} onClick={logout}>
            退出
          </Button>
        </Space>
      </Header>

      {page === "docs" ? (
        <Content className="standalone-content">
          <Docs state={state} />
        </Content>
      ) : (
        <Layout className="app-main-layout">
          <Sider width={226} className="app-sidebar">
            <Menu mode="inline" selectedKeys={[activePage]} items={navItems} onClick={({ key }) => navigate(key)} />
          </Sider>
          <Content className="app-content">
            {renderPage()}
          </Content>
        </Layout>
      )}

      <Drawer
        className="mobile-nav-drawer"
        placement="left"
        width="100%"
        open={mobileNavOpen}
        onClose={() => setMobileNavOpen(false)}
        title={
          <Space size={10}>
            <ZephyrLogo className="drawer-logo" title="Zephyr" />
            <Text strong>Zephyr</Text>
          </Space>
        }
      >
        <Menu mode="inline" selectedKeys={[activePage]} items={navItems} onClick={({ key }) => navigate(key)} />
      </Drawer>

      <Modal
        title={runTask?.title}
        open={!!runTask}
        onCancel={() => setRunTask(null)}
        onOk={() => runForm.submit()}
        okText="执行"
        cancelText="取消"
        forceRender
        okButtonProps={{ danger: runTask?.risk === "danger" }}
      >
        <Form form={runForm} layout="vertical" onFinish={executeTask}>
          {runTask && (
            <>
            <Alert type={runTask.risk === "danger" ? "error" : runTask.risk === "warning" ? "warning" : "info"} showIcon message={runTask.description} />
            <TaskRunContext task={runTask} statuses={deploymentStatuses} nowMs={nowMs} />
            <Form.Item
              label="本次执行分支"
              name="branch"
              extra={`默认配置：${runTask.branch || "main"}`}
              rules={[{ required: true, message: "请选择分支" }]}
            >
              <Select
                showSearch
                optionFilterProp="label"
                options={branchOptionsForTask(state, runTask)}
                placeholder="选择本次部署分支"
              />
            </Form.Item>
            {(runTask.inputs || []).map((item) => (
              <Form.Item
                key={item.name}
                label={item.label}
                name={item.name}
                rules={item.required ? [{ required: true, message: `请输入${item.label}` }] : undefined}
              >
                <Input placeholder={item.placeholder} />
              </Form.Item>
            ))}
            {runTask.confirm_text && (
              <Form.Item
                label={`输入 ${runTask.confirm_text} 确认`}
                name="confirm_text"
                rules={[{ required: true, message: "请输入确认文字" }]}
            >
              <Input />
            </Form.Item>
          )}
            </>
          )}
        </Form>
      </Modal>

      <Drawer
        title={editingTask ? `编辑 ${editingTask.title}` : "新增部署任务"}
        open={taskDrawerOpen}
        onClose={() => {
          setTaskDrawerOpen(false);
          setEditingTask(null);
          taskForm.resetFields();
        }}
        width={560}
        forceRender
        destroyOnClose
      >
        <Form form={taskForm} layout="vertical" onFinish={saveCustomTask} initialValues={{ branch: "main", risk: "normal", group: "自定义任务" }}>
          <Form.Item label="任务 ID" name="id" extra={editingTask?.builtin ? "内置任务 ID 已锁定；保存后会覆盖默认配置，可随时恢复默认。" : "留空时会按标题生成；后续编辑建议保持不变。"}>
            <Input placeholder="deploy-my-service" disabled={Boolean(editingTask?.builtin)} />
          </Form.Item>
          <Form.Item label="任务标题" name="title" rules={[{ required: true, message: "请输入任务标题" }]}>
            <Input placeholder="部署新服务" />
          </Form.Item>
          <Form.Item label="模块" name="group">
            <Input placeholder="例如 业务服务 / 基础设施 / Zephyr" />
          </Form.Item>
          <Form.Item label="说明" name="description">
            <Input.TextArea rows={2} />
          </Form.Item>
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item label="Woodpecker Repo ID" name="repo_id" rules={[{ required: true, message: "请输入 Repo ID" }]}>
                <Input type="number" placeholder="例如 3" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item label="执行仓库显示名" name="repo_name">
                <Input placeholder="例如 infra-platform / app-service" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item
                label="默认执行分支"
                name="branch"
                extra="保存后作为这个任务的默认分支；点执行时仍可临时选择其他分支。"
                rules={[{ required: true, message: "请选择默认分支" }]}
              >
                <Select
                  showSearch
                  optionFilterProp="label"
                  options={branchOptionsForRepo(
                    state,
                    Number(taskFormRepoID || editingTask?.repo_id || 0),
                    String(taskFormBranch || editingTask?.branch || "main")
                  )}
                  placeholder="选择默认分支"
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item label="风险级别" name="risk">
                <Select
                  options={[
                    { value: "normal", label: "普通" },
                    { value: "warning", label: "注意" },
                    { value: "danger", label: "高危" }
                  ]}
                />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item label="确认文字" name="confirm_text" extra="高危任务建议填写，例如 PROD / ROLLBACK。">
            <Input />
          </Form.Item>
          <Form.Item label="Woodpecker 变量" name="variables" rules={[{ required: true, message: "请输入变量" }]}>
            <Input.TextArea rows={6} placeholder={"DEPLOY_ACTION=deploy\nDEPLOY_TARGET=production"} />
          </Form.Item>
          <Button type="primary" htmlType="submit" block>
            保存任务配置
          </Button>
        </Form>
      </Drawer>

      <PipelineSummaryDrawer
        open={pipelineSummaryOpen}
        loading={pipelineSummaryLoading}
        summary={pipelineSummary}
        nowMs={nowMs}
        onClose={() => setPipelineSummaryOpen(false)}
      />
    </Layout>
  );
}

function LoadingShell() {
  return (
    <Layout className="app-layout">
      <Content className="loading-shell">
        <div className="loading-scene">
          <div className="loading-breeze" aria-hidden="true">
            <span className="breeze-line breeze-line-a" />
            <span className="breeze-line breeze-line-b" />
            <span className="breeze-line breeze-line-c" />
            <span className="breeze-node breeze-node-a" />
            <span className="breeze-node breeze-node-b" />
          </div>
          <ZephyrLogo className="loading-logo loading-logo-active" title="Zephyr" />
          <Text className="eyebrow">Infrastructure Console</Text>
          <Title level={4} className="loading-title">
            Zephyr
          </Title>
          <Text type="secondary" className="loading-caption">
            同步状态中
          </Text>
        </div>
      </Content>
    </Layout>
  );
}
