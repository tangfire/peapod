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
  zefire_triggered_by?: string;
  zefire_triggered_at?: string;
  zefire_task_id?: string;
  zefire_task_title?: string;
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
  grafana_public_url: string;
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
  status: SetupStatusItem[];
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
