export type Risk = "normal" | "warning" | "danger" | "link";

export type TaskInput = {
  name: string;
  label: string;
  placeholder?: string;
  required?: boolean;
};

export type Task = {
  id: string;
  group: string;
  title: string;
  description: string;
  repo_id: number;
  repo_name?: string;
  branch: string;
  variables: Record<string, string>;
  risk: Risk;
  confirm_text?: string;
  allowed_roles?: string[];
  inputs?: TaskInput[];
  disabled?: boolean;
  disabled_reason?: string;
  external_url?: string;
  custom?: boolean;
  builtin?: boolean;
  overridden?: boolean;
};

export type Pipeline = {
  number: number;
  status: string;
  event: string;
  commit: string;
  branch: string;
  author?: string;
  sender?: string;
  deploy_to?: string;
  created: number;
  started: number;
  finished: number;
  updated?: number;
  message: string;
  variables?: Record<string, string>;
  repo_id?: number;
  repo_name?: string;
  peapod_triggered_by?: string;
  peapod_triggered_at?: string;
  peapod_task_id?: string;
  peapod_task_title?: string;
};

export type PipelineStep = {
  id: number;
  pid?: number;
  ppid?: number;
  name: string;
  state: string;
  error?: string;
  exit_code?: number;
  started?: number;
  finished?: number;
  type?: string;
};

export type PipelineSummary = {
  pipeline: Pipeline;
  steps: PipelineStep[];
  failure_summary?: string;
  log_tail: string[];
  woodpecker_url: string;
};

export type WoodpeckerRepo = {
  id: number;
  forge_id?: number;
  forge_remote_id?: string;
  owner?: string;
  name?: string;
  full_name?: string;
  forge_url?: string;
  clone_url?: string;
  default_branch?: string;
  visibility?: string;
  private?: boolean;
  active?: boolean;
};

export type WoodpeckerReposResponse = {
  repos: WoodpeckerRepo[];
  configured: Record<string, string>;
  errors?: string[];
};

export type DeploymentStatus = {
  id: string;
  name: string;
  group: string;
  repo_id: number;
  repo_name: string;
  configured_branch: string;
  current_branch: string;
  current_commit: string;
  last_action: string;
  last_status: string;
  last_deployed_at: number;
  pipeline: number;
  triggered_by?: string;
  triggered_at?: string;
  variables?: Record<string, string>;
  deploy_verified?: boolean;
  deploy_degraded?: boolean;
  deploy_verify_status?: string;
  deploy_verify_message?: string;
  actual_commit?: string;
  health_url?: string;
  latest_action?: string;
  latest_status?: string;
  latest_branch?: string;
  latest_commit?: string;
  latest_at?: number;
  latest_pipeline?: number;
  latest_triggered_by?: string;
  previous_action?: string;
  previous_branch?: string;
  previous_commit?: string;
  previous_deployed_at?: number;
  previous_pipeline?: number;
};

export type User = {
  id: number;
  username: string;
  display_name: string;
  email: string;
  role: "admin" | "operator";
  active: boolean;
  legacy?: boolean;
};

export type StateResponse = {
  tasks: Task[];
  pipelines: Record<string, Pipeline[]>;
  deployment_statuses: DeploymentStatus[];
  repos: Record<string, string>;
  branches: Record<string, string[]>;
  configurable: boolean;
  current_user: User;
  auth_mode: "db" | "legacy";
  now: string;
  links: Record<string, string>;
  health?: Record<string, unknown>;
};

export type MonitoringSummary = {
  hosts: MonitoringHost[];
  containers: MonitoringContainer[];
  alerts: MonitoringAlert[];
  links: Record<string, string>;
  checked_at: string;
  source: string;
  degraded_reason?: string;
};

export type MonitoringHost = {
  id: string;
  name: string;
  role: string;
  status: string;
  source: string;
  cpu_percent?: number;
  memory_used_bytes?: number;
  memory_total_bytes?: number;
  memory_percent?: number;
  disk_used_bytes?: number;
  disk_total_bytes?: number;
  disk_percent?: number;
  load_1?: number;
  load_5?: number;
  load_15?: number;
  network_bytes_per_second?: number;
  uptime_seconds?: number;
  uptime?: string;
  message?: string;
  cleanup_task_id?: string;
  checked_at?: string;
};

export type MonitoringContainer = {
  host_id: string;
  host_name: string;
  name: string;
  status: string;
  cpu_percent?: number;
  memory_usage?: string;
  memory_percent?: number;
  configured: boolean;
  message?: string;
};

export type MonitoringAlert = {
  level: "info" | "warning" | "critical";
  host_id?: string;
  host_name?: string;
  metric?: string;
  title: string;
  message: string;
};

export type ExternalLinkConfig = {
  id: string;
  title: string;
  url: string;
  description?: string;
  group?: string;
};

export type MonitorHostConfig = {
  id: string;
  name: string;
  role?: string;
  ssh_host?: string;
  address?: string;
  ssh_user?: string;
  ssh_key_path?: string;
  beszel_names?: string[];
  containers?: string[];
  container_groups?: string[][];
  cleanup_task_id?: string;
};

export type RuntimeConfigInput = {
  public_url: string;
  woodpecker_server: string;
  woodpecker_public_url: string;
  woodpecker_token?: string;
  beszel_base_url: string;
  beszel_public_url: string;
  beszel_email?: string;
  beszel_password?: string;
  dozzle_base_url: string;
  dozzle_public_url: string;
  dozzle_username?: string;
  dozzle_password?: string;
  grafana_public_url: string;
  log_strategy: "lightweight" | "observability" | "external";
  docker_log_max_size: string;
  docker_log_max_file: string;
  alert_webhook_url?: string;
  external_links: ExternalLinkConfig[];
  monitor_hosts: MonitorHostConfig[];
  monitor_refresh_seconds: number;
  monitor_warn_disk: number;
  monitor_crit_disk: number;
  monitor_warn_memory: number;
};

export type SetupStatusItem = {
  id: string;
  title: string;
  status: "ok" | "warning" | "error" | string;
  message: string;
  action_label?: string;
  action_url?: string;
};

export type SetupChecklistItem = {
  id: string;
  title: string;
  status: "ok" | "warning" | "error" | "unknown" | "optional" | string;
  severity: "ok" | "warning" | "error" | string;
  message: string;
  fix?: string;
  action_label?: string;
  action_url?: string;
};

export type DeploymentVerificationSummary = {
  task_count: number;
  configured_count: number;
  missing_count: number;
  missing_tasks: string[];
};

export type LogStrategyStatus = {
  mode: "lightweight" | "observability" | "external" | string;
  label: string;
  message: string;
  dozzle_base_url?: string;
  dozzle_public_url?: string;
  grafana_public_url?: string;
  dozzle_mcp_ready?: boolean;
  dozzle_mcp_message?: string;
  docker_log_max_size: string;
  docker_log_max_file: string;
  docker_retention: string;
  alert_webhook_ready?: boolean;
};

export type OnboardingProgress = {
  ready_count: number;
  total_count: number;
  blocked_count: number;
  warning_count: number;
  percent: number;
  next_action?: string;
};

export type DoctorCheck = {
  id: string;
  title: string;
  status: string;
  severity: string;
  message: string;
  fix?: string;
  action_label?: string;
  action_url?: string;
};

export type DoctorSummary = {
  readiness: "ready" | "warning" | "blocked" | string;
  checks: DoctorCheck[];
  updated_at: string;
};

export type SetupCommand = {
  id: string;
  title: string;
  description: string;
  command: string;
};

export type SetupDocLink = {
  title: string;
  description: string;
  path: string;
};

export type SetupConfigResponse = {
  config: RuntimeConfigInput;
  secrets: Record<string, boolean>;
  readiness: "ready" | "warning" | "blocked" | string;
  status: SetupStatusItem[];
  checklist: SetupChecklistItem[];
  deployment_verification_summary: DeploymentVerificationSummary;
  log_strategy: LogStrategyStatus;
  onboarding?: OnboardingProgress;
  doctor?: DoctorSummary;
  commands: SetupCommand[];
  docs: SetupDocLink[];
  updated_at: string;
};

export type RunResult = {
  task: Task;
  pipeline: Pipeline;
  triggered_at: string;
  woodpecker_url?: string;
};

export type TaskConfig = {
  repos?: Record<string, string>;
  tasks: Task[];
};

export type LogQueryLimits = {
  max_lines: number;
  max_containers: number;
  timeout_seconds: number;
};

export type LogSummaryResponse = {
  mode: string;
  label: string;
  message: string;
  source: string;
  dozzle_public_url?: string;
  grafana_public_url?: string;
  dozzle_mcp_ready: boolean;
  dozzle_mcp_message?: string;
  docker_log_max_size: string;
  docker_log_max_file: string;
  docker_retention: string;
  container_count: number;
  host_count: number;
  limits: LogQueryLimits;
  checked_at: string;
  degraded_reason?: string;
};

export type LogContainer = {
  id: string;
  name: string;
  image?: string;
  state?: string;
  health?: string;
  host: string;
  host_name?: string;
  group?: string;
  created?: string;
  source: string;
};

export type LogContainersResponse = {
  containers: LogContainer[];
  source: string;
  checked_at: string;
  degraded_reason?: string;
};

export type LogQueryRequest = {
  hosts: string[];
  containers: string[];
  keyword: string;
  level: string;
  since_minutes: number;
  tail: number;
  stream: string;
};

export type LogLine = {
  timestamp?: string;
  level?: string;
  stream?: string;
  type?: string;
  message: string;
  host: string;
  host_name?: string;
  container_id: string;
  container_name: string;
};

export type LogQueryResponse = {
  lines: LogLine[];
  source: string;
  containers: LogContainer[];
  checked_at: string;
  degraded_reason?: string;
};

export type TemplateInput = {
  name: string;
  label: string;
  type?: string;
  placeholder?: string;
  default?: string;
  required?: boolean;
  help?: string;
};

export type TaskTemplate = {
  id: string;
  title: string;
  description: string;
  category: string;
  default_group: string;
  default_risk: Risk;
  default_branch: string;
  requires_verification: boolean;
  variables: Record<string, string>;
  inputs: TemplateInput[];
};

export type TemplateApplyRequest = {
  repo_id: number;
  repo_name: string;
  branch: string;
  project_id: string;
  project_name: string;
  environment: string;
  marker_path?: string;
  health_url?: string;
  confirm_text?: string;
  values?: Record<string, string>;
};

export type TemplateApplyResponse = {
  task: Task;
  config: TaskConfig;
};

export type AuditRecord = {
  time: string;
  user_id?: number;
  username?: string;
  task_id: string;
  task_title: string;
  repo_id: number;
  branch: string;
  variables: Record<string, string>;
  pipeline: number;
  status: string;
  error?: string;
};
