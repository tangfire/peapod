package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const cookieName = "peapod_session"

type authUserContextKey struct{}

type sessionPayload struct {
	Expires     int64  `json:"expires"`
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	Legacy      bool   `json:"legacy,omitempty"`
}

type Config struct {
	Addr                          string
	AppEnv                        string
	LogLevel                      string
	AccessLogMode                 string
	AccessLogSlowThresholdSeconds int
	ConfigPath                    string
	PublicURL                     string
	Password                      string
	SessionSecret                 string
	DBDSN                         string
	BootstrapUsername             string
	BootstrapPassword             string
	BootstrapDisplayName          string
	BootstrapEmail                string
	WoodpeckerServer              string
	WoodpeckerPublicURL           string
	WoodpeckerToken               string
	BeszelBaseURL                 string
	BeszelPublicURL               string
	BeszelEmail                   string
	BeszelPassword                string
	DozzleBaseURL                 string
	DozzlePublicURL               string
	DozzleUsername                string
	DozzlePassword                string
	GrafanaPublicURL              string
	LogStrategy                   string
	DockerLogMaxSize              string
	DockerLogMaxFile              string
	AlertWebhookURL               string
	ExternalLinksJSON             string
	MonitorHostsJSON              string
	MonitorSSHKeyPath             string
	MonitorRefreshSeconds         int
	MonitorWarnDisk               int
	MonitorCritDisk               int
	MonitorWarnMemory             int
	MonitorAutoCleanupLevel       string
	MonitorAutoCleanupDisk        int
	AuditPath                     string
	TasksPath                     string
	FrontendDir                   string
}

type RuntimeConfigFile struct {
	PublicURL             string               `json:"public_url,omitempty"`
	WoodpeckerServer      string               `json:"woodpecker_server,omitempty"`
	WoodpeckerPublicURL   string               `json:"woodpecker_public_url,omitempty"`
	WoodpeckerToken       string               `json:"woodpecker_token,omitempty"`
	BeszelBaseURL         string               `json:"beszel_base_url,omitempty"`
	BeszelPublicURL       string               `json:"beszel_public_url,omitempty"`
	BeszelEmail           string               `json:"beszel_email,omitempty"`
	BeszelPassword        string               `json:"beszel_password,omitempty"`
	DozzleBaseURL         string               `json:"dozzle_base_url,omitempty"`
	DozzlePublicURL       string               `json:"dozzle_public_url,omitempty"`
	DozzleUsername        string               `json:"dozzle_username,omitempty"`
	DozzlePassword        string               `json:"dozzle_password,omitempty"`
	GrafanaPublicURL      string               `json:"grafana_public_url,omitempty"`
	LogStrategy           string               `json:"log_strategy,omitempty"`
	DockerLogMaxSize      string               `json:"docker_log_max_size,omitempty"`
	DockerLogMaxFile      string               `json:"docker_log_max_file,omitempty"`
	AlertWebhookURL       string               `json:"alert_webhook_url,omitempty"`
	ExternalLinks         []ExternalLinkConfig `json:"external_links"`
	MonitorHosts          []MonitorHostConfig  `json:"monitor_hosts"`
	MonitorRefreshSeconds int                  `json:"monitor_refresh_seconds,omitempty"`
	MonitorWarnDisk       int                  `json:"monitor_warn_disk,omitempty"`
	MonitorCritDisk       int                  `json:"monitor_crit_disk,omitempty"`
	MonitorWarnMemory     int                  `json:"monitor_warn_memory,omitempty"`
	MonitorAutoCleanupLevel string              `json:"monitor_auto_cleanup_level,omitempty"`
	MonitorAutoCleanupDisk  int                  `json:"monitor_auto_cleanup_disk,omitempty"`
}

type RuntimeConfigInput struct {
	PublicURL             string               `json:"public_url"`
	WoodpeckerServer      string               `json:"woodpecker_server"`
	WoodpeckerPublicURL   string               `json:"woodpecker_public_url"`
	WoodpeckerToken       string               `json:"woodpecker_token"`
	BeszelBaseURL         string               `json:"beszel_base_url"`
	BeszelPublicURL       string               `json:"beszel_public_url"`
	BeszelEmail           string               `json:"beszel_email"`
	BeszelPassword        string               `json:"beszel_password"`
	DozzleBaseURL         string               `json:"dozzle_base_url"`
	DozzlePublicURL       string               `json:"dozzle_public_url"`
	DozzleUsername        string               `json:"dozzle_username"`
	DozzlePassword        string               `json:"dozzle_password"`
	GrafanaPublicURL      string               `json:"grafana_public_url"`
	LogStrategy           string               `json:"log_strategy"`
	DockerLogMaxSize      string               `json:"docker_log_max_size"`
	DockerLogMaxFile      string               `json:"docker_log_max_file"`
	AlertWebhookURL       string               `json:"alert_webhook_url"`
	ExternalLinks         []ExternalLinkConfig `json:"external_links"`
	MonitorHosts          []MonitorHostConfig  `json:"monitor_hosts"`
	MonitorRefreshSeconds int                  `json:"monitor_refresh_seconds"`
	MonitorWarnDisk       int                  `json:"monitor_warn_disk"`
	MonitorCritDisk       int                  `json:"monitor_crit_disk"`
	MonitorWarnMemory     int                  `json:"monitor_warn_memory"`
	MonitorAutoCleanupLevel string              `json:"monitor_auto_cleanup_level"`
	MonitorAutoCleanupDisk  int                  `json:"monitor_auto_cleanup_disk"`
}

type SetupConfigResponse struct {
	Config                        RuntimeConfigInput            `json:"config"`
	Secrets                       map[string]bool               `json:"secrets"`
	Readiness                     string                        `json:"readiness"`
	Status                        []SetupStatusItem             `json:"status"`
	Checklist                     []SetupChecklistItem          `json:"checklist"`
	DeploymentVerificationSummary DeploymentVerificationSummary `json:"deployment_verification_summary"`
	LogStrategy                   LogStrategyStatus             `json:"log_strategy"`
	Onboarding                    OnboardingProgress            `json:"onboarding"`
	Doctor                        DoctorSummary                 `json:"doctor"`
	Commands                      []SetupCommand                `json:"commands"`
	Docs                          []SetupDocLink                `json:"docs"`
	UpdatedAt                     string                        `json:"updated_at"`
}

type SetupStatusItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	ActionLabel string `json:"action_label,omitempty"`
	ActionURL   string `json:"action_url,omitempty"`
}

type SetupChecklistItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Fix         string `json:"fix,omitempty"`
	ActionLabel string `json:"action_label,omitempty"`
	ActionURL   string `json:"action_url,omitempty"`
}

type DeploymentVerificationSummary struct {
	TaskCount       int      `json:"task_count"`
	ConfiguredCount int      `json:"configured_count"`
	MissingCount    int      `json:"missing_count"`
	MissingTasks    []string `json:"missing_tasks"`
}

type LogStrategyStatus struct {
	Mode              string `json:"mode"`
	Label             string `json:"label"`
	Message           string `json:"message"`
	DozzleBaseURL     string `json:"dozzle_base_url,omitempty"`
	DozzlePublicURL   string `json:"dozzle_public_url,omitempty"`
	GrafanaPublicURL  string `json:"grafana_public_url,omitempty"`
	DozzleMCPReady    bool   `json:"dozzle_mcp_ready"`
	DozzleMCPMessage  string `json:"dozzle_mcp_message,omitempty"`
	DockerLogMaxSize  string `json:"docker_log_max_size"`
	DockerLogMaxFile  string `json:"docker_log_max_file"`
	DockerRetention   string `json:"docker_retention"`
	AlertWebhookReady bool   `json:"alert_webhook_ready"`
}

type OnboardingProgress struct {
	ReadyCount   int    `json:"ready_count"`
	TotalCount   int    `json:"total_count"`
	BlockedCount int    `json:"blocked_count"`
	WarningCount int    `json:"warning_count"`
	Percent      int    `json:"percent"`
	NextAction   string `json:"next_action,omitempty"`
}

type DoctorSummary struct {
	Readiness string        `json:"readiness"`
	Checks    []DoctorCheck `json:"checks"`
	UpdatedAt string        `json:"updated_at"`
}

type DoctorCheck struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Fix         string `json:"fix,omitempty"`
	ActionLabel string `json:"action_label,omitempty"`
	ActionURL   string `json:"action_url,omitempty"`
}

type SetupCommand struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Command     string `json:"command"`
}

type SetupDocLink struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

type Task struct {
	ID             string            `json:"id"`
	Group          string            `json:"group"`
	Title          string            `json:"title"`
	Description    string            `json:"description"`
	RepoID         int               `json:"repo_id"`
	RepoName       string            `json:"repo_name,omitempty"`
	Branch         string            `json:"branch"`
	Variables      map[string]string `json:"variables"`
	Risk           string            `json:"risk"`
	ConfirmText    string            `json:"confirm_text,omitempty"`
	AllowedRoles   []string          `json:"allowed_roles,omitempty"`
	Inputs         []TaskInput       `json:"inputs,omitempty"`
	Disabled       bool              `json:"disabled,omitempty"`
	DisabledReason string            `json:"disabled_reason,omitempty"`
	ExternalURL    string            `json:"external_url,omitempty"`
	Custom         bool              `json:"custom,omitempty"`
	Builtin        bool              `json:"builtin,omitempty"`
	Overridden     bool              `json:"overridden,omitempty"`
}

type TaskInput struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
	Required    bool   `json:"required"`
}

type Pipeline struct {
	Number            int64             `json:"number"`
	Status            string            `json:"status"`
	Event             string            `json:"event"`
	Commit            string            `json:"commit"`
	Branch            string            `json:"branch"`
	Author            string            `json:"author,omitempty"`
	Sender            string            `json:"sender,omitempty"`
	DeployTo          string            `json:"deploy_to,omitempty"`
	Created           int64             `json:"created"`
	Started           int64             `json:"started"`
	Finished          int64             `json:"finished"`
	Updated           int64             `json:"updated,omitempty"`
	Message           string            `json:"message"`
	Variables         map[string]string `json:"variables,omitempty"`
	PedpodTriggeredBy string            `json:"peapod_triggered_by,omitempty"`
	PedpodTriggeredAt string            `json:"peapod_triggered_at,omitempty"`
	PedpodTaskID      string            `json:"peapod_task_id,omitempty"`
	PedpodTaskTitle   string            `json:"peapod_task_title,omitempty"`
}

type PipelineStep struct {
	ID       int64  `json:"id"`
	PID      int64  `json:"pid,omitempty"`
	PPID     int64  `json:"ppid,omitempty"`
	Name     string `json:"name"`
	State    string `json:"state"`
	Error    string `json:"error,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Started  int64  `json:"started,omitempty"`
	Finished int64  `json:"finished,omitempty"`
	Type     string `json:"type,omitempty"`
}

type PipelineSummary struct {
	Pipeline       Pipeline       `json:"pipeline"`
	Steps          []PipelineStep `json:"steps"`
	FailureSummary string         `json:"failure_summary,omitempty"`
	LogTail        []string       `json:"log_tail"`
	WoodpeckerURL  string         `json:"woodpecker_url"`
}

type WoodpeckerRepo struct {
	ID            int    `json:"id"`
	ForgeID       int    `json:"forge_id,omitempty"`
	ForgeRemoteID string `json:"forge_remote_id,omitempty"`
	Owner         string `json:"owner,omitempty"`
	Name          string `json:"name,omitempty"`
	FullName      string `json:"full_name,omitempty"`
	ForgeURL      string `json:"forge_url,omitempty"`
	CloneURL      string `json:"clone_url,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Visibility    string `json:"visibility,omitempty"`
	Private       bool   `json:"private,omitempty"`
	Active        bool   `json:"active"`
}

type WoodpeckerReposResponse struct {
	Repos      []WoodpeckerRepo `json:"repos"`
	Configured map[int]string   `json:"configured"`
	Errors     []string         `json:"errors,omitempty"`
}

type WoodpeckerRepoLookupRequest struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

type WoodpeckerRepoActivateRequest struct {
	ForgeRemoteID string `json:"forge_remote_id"`
}

type WoodpeckerRepoSaveRequest struct {
	RepoID   int    `json:"repo_id"`
	RepoName string `json:"repo_name"`
}

type DeploymentStatus struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Group               string               `json:"group"`
	RepoID              int                  `json:"repo_id"`
	RepoName            string               `json:"repo_name"`
	ConfiguredBranch    string               `json:"configured_branch"`
	CurrentBranch       string               `json:"current_branch"`
	CurrentCommit       string               `json:"current_commit"`
	LastAction          string               `json:"last_action"`
	LastStatus          string               `json:"last_status"`
	LastDeployedAt      int64                `json:"last_deployed_at"`
	Pipeline            int64                `json:"pipeline"`
	TriggeredBy         string               `json:"triggered_by,omitempty"`
	TriggeredAt         string               `json:"triggered_at,omitempty"`
	Variables           map[string]string    `json:"variables,omitempty"`
	DeployVerified      bool                 `json:"deploy_verified"`
	DeployDegraded      bool                 `json:"deploy_degraded,omitempty"`
	DeployVerifyStatus  string               `json:"deploy_verify_status,omitempty"`
	DeployVerifyMessage string               `json:"deploy_verify_message,omitempty"`
	ActualCommit        string               `json:"actual_commit,omitempty"`
	HealthURL           string               `json:"health_url,omitempty"`
	LatestAction        string               `json:"latest_action,omitempty"`
	LatestStatus        string               `json:"latest_status,omitempty"`
	LatestBranch        string               `json:"latest_branch,omitempty"`
	LatestCommit        string               `json:"latest_commit,omitempty"`
	LatestAt            int64                `json:"latest_at,omitempty"`
	LatestPipeline      int64                `json:"latest_pipeline,omitempty"`
	LatestTriggeredBy   string               `json:"latest_triggered_by,omitempty"`
	PreviousAction      string               `json:"previous_action,omitempty"`
	PreviousBranch      string               `json:"previous_branch,omitempty"`
	PreviousCommit      string               `json:"previous_commit,omitempty"`
	PreviousDeployedAt  int64                `json:"previous_deployed_at,omitempty"`
	PreviousPipeline    int64                `json:"previous_pipeline,omitempty"`
	Revisions           []DeploymentRevision `json:"revisions,omitempty"`
}

type DeploymentRevision struct {
	Pipeline    int64  `json:"pipeline"`
	Branch      string `json:"branch"`
	Commit      string `json:"commit"`
	DeployedAt  int64  `json:"deployed_at"`
	Action      string `json:"action"`
	Verified    bool   `json:"verified"`
	TriggeredBy string `json:"triggered_by,omitempty"`
	TriggeredAt string `json:"triggered_at,omitempty"`
}

type StateResponse struct {
	Tasks              []Task                 `json:"tasks"`
	Pipelines          map[int][]Pipeline     `json:"pipelines"`
	DeploymentStatuses []DeploymentStatus     `json:"deployment_statuses"`
	Repos              map[int]string         `json:"repos"`
	Branches           map[int][]string       `json:"branches"`
	Configurable       bool                   `json:"configurable"`
	CurrentUser        AuthUser               `json:"current_user"`
	AuthMode           string                 `json:"auth_mode"`
	Now                string                 `json:"now"`
	Links              map[string]string      `json:"links"`
	Health             map[string]interface{} `json:"health"`
}

type RunRequest struct {
	Inputs map[string]string `json:"inputs"`
	Branch string            `json:"branch"`
}

type CustomRunRequest struct {
	RepoID    int               `json:"repo_id"`
	RepoName  string            `json:"repo_name"`
	Branch    string            `json:"branch"`
	Variables map[string]string `json:"variables"`
}

type CustomTaskConfig struct {
	Repos map[int]string `json:"repos,omitempty"`
	Tasks []Task         `json:"tasks"`
}

type TaskTemplate struct {
	ID                   string            `json:"id"`
	Title                string            `json:"title"`
	Description          string            `json:"description"`
	Category             string            `json:"category"`
	DefaultGroup         string            `json:"default_group"`
	DefaultRisk          string            `json:"default_risk"`
	DefaultBranch        string            `json:"default_branch"`
	RequiresVerification bool              `json:"requires_verification"`
	Variables            map[string]string `json:"variables"`
	Inputs               []TemplateInput   `json:"inputs"`
}

type TemplateInput struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Default     string `json:"default,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Help        string `json:"help,omitempty"`
}

type TemplatesResponse struct {
	Templates []TaskTemplate `json:"templates"`
}

type TemplateApplyRequest struct {
	RepoID      int               `json:"repo_id"`
	RepoName    string            `json:"repo_name"`
	Branch      string            `json:"branch"`
	ProjectID   string            `json:"project_id"`
	ProjectName string            `json:"project_name"`
	Environment string            `json:"environment"`
	MarkerPath  string            `json:"marker_path"`
	HealthURL   string            `json:"health_url"`
	ConfirmText string            `json:"confirm_text"`
	Values      map[string]string `json:"values"`
}

type TemplateApplyResponse struct {
	Task   Task             `json:"task"`
	Config CustomTaskConfig `json:"config"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RunResponse struct {
	OK          bool     `json:"ok"`
	Task        Task     `json:"task"`
	Pipeline    Pipeline `json:"pipeline"`
	Woodpecker  string   `json:"woodpecker_url"`
	TriggeredAt string   `json:"triggered_at"`
}

type ErrorResponse struct {
	Error   string   `json:"error"`
	Details []string `json:"details,omitempty"`
}

type WoodpeckerRequestError struct {
	Operation  string
	RepoID     int
	Branch     string
	StatusCode int
	Body       string
}

func (e WoodpeckerRequestError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body != "" {
		return fmt.Sprintf("Woodpecker %s 失败：HTTP %d · %s", e.Operation, e.StatusCode, body)
	}
	return fmt.Sprintf("Woodpecker %s 失败：HTTP %d，服务没有返回错误内容", e.Operation, e.StatusCode)
}

func (e WoodpeckerRequestError) Details() []string {
	details := []string{
		"Woodpecker 操作：" + fallbackText(e.Operation, "请求"),
	}
	if e.Body == "" && e.StatusCode >= 500 {
		details = append(details, "Woodpecker 返回了空 5xx，常见原因是 Woodpecker Server 内部异常、仓库配置异常、分支不可触发，或 server 日志里有更具体的错误。")
	}
	return details
}

type AuditRecord struct {
	Time      string            `json:"time"`
	UserID    int64             `json:"user_id,omitempty"`
	Username  string            `json:"username,omitempty"`
	RemoteIP  string            `json:"remote_ip"`
	TaskID    string            `json:"task_id"`
	TaskTitle string            `json:"task_title"`
	RepoID    int               `json:"repo_id"`
	Branch    string            `json:"branch"`
	Variables map[string]string `json:"variables"`
	Pipeline  int64             `json:"pipeline"`
	Status    string            `json:"status"`
	Error     string            `json:"error,omitempty"`
}

type AuditListResponse struct {
	Records []AuditRecord `json:"records"`
}

type DockerImageInfo struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
	CreatedAt  string `json:"created_at"`
	ID         string `json:"id"`
	Dangling   bool   `json:"dangling"`
}

type DockerVolumeInfo struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
	Orphan     bool   `json:"orphan"`
}

type DiskWasteBreakdown struct {
	BuildCache       string `json:"build_cache"`
	DanglingImages   string `json:"dangling_images"`
	UnusedImages     string `json:"unused_images"`
	OrphanVolumes    string `json:"orphan_volumes"`
	ContainerLogs    string `json:"container_logs"`
	TotalReclaimable string `json:"total_reclaimable"`
}

type DiskDiagnosisResponse struct {
	Filesystems  []DiskFilesystemInfo `json:"filesystems"`
	Docker       DockerDiskInfo       `json:"docker"`
	TopDirs      []DiskUsageItem      `json:"top_dirs"`
	DockerOK     bool                 `json:"docker_ok"`
	CheckedAt    string               `json:"checked_at"`
	Images       []DockerImageInfo    `json:"images,omitempty"`
	Volumes      []DockerVolumeInfo   `json:"volumes,omitempty"`
	LogFiles     []DiskUsageItem      `json:"log_files,omitempty"`
	WasteBreakdown *DiskWasteBreakdown `json:"waste_breakdown,omitempty"`
}

type DiskFilesystemInfo struct {
	Mount   string `json:"mount"`
	Total   string `json:"total"`
	Used    string `json:"used"`
	Percent int    `json:"percent"`
}

type DockerDiskInfo struct {
	ImagesTotal        int    `json:"images_total"`
	ImagesActive       int    `json:"images_active"`
	ImagesSize         string `json:"images_size"`
	ImagesReclaimable  string `json:"images_reclaimable"`
	BuildCacheSize     string `json:"build_cache_size"`
	BuildReclaimable   string `json:"build_reclaimable"`
	VolumesSize        string `json:"volumes_size"`
	VolumesReclaimable string `json:"volumes_reclaimable"`
}

type DiskCleanupLevel struct {
	Level       string `json:"level"`
	Description string `json:"description"`
	Reclaimable string `json:"reclaimable"`
	Command     string `json:"command"`
	Risk        string `json:"risk"`
}

type DiskCleanupPreviewResponse struct {
	Levels         []DiskCleanupLevel `json:"levels"`
	Recommendation string             `json:"recommendation"`
	DockerOK       bool               `json:"docker_ok"`
}

type DiskCleanupRequest struct {
	Level   string `json:"level"`
	Confirm string `json:"confirm"`
}

type DiskCleanupBreakdownItem struct {
	Category   string `json:"category"`
	Reclaimed  string `json:"reclaimed"`
	Command    string `json:"command"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

type DiskCleanupResponse struct {
	OK        bool                     `json:"ok"`
	Level     string                   `json:"level"`
	Reclaimed string                   `json:"reclaimed"`
	Details   string                   `json:"details"`
	Breakdown []DiskCleanupBreakdownItem `json:"breakdown,omitempty"`
}

var repos = map[int]string{}

var tasks = []Task{}

func main() {
	cfg := loadConfig()
	logger, cleanupLogger, err := initAppLogger(cfg)
	if err != nil {
		panic(err)
	}
	defer cleanupLogger()
	if runtimeCfg, err := loadRuntimeConfigFile(cfg.ConfigPath); err == nil {
		applyRuntimeConfig(&cfg, runtimeCfg)
	} else if !errors.Is(err, os.ErrNotExist) {
		logger.Warn("load runtime config failed", zap.Error(err))
	}
	if err := cfg.validate(); err != nil {
		logger.Fatal("invalid configuration", zap.Error(err))
	}
	store, err := OpenUserStore(context.Background(), cfg)
	if err != nil {
		logger.Fatal("open user store failed", zap.Error(err))
	}
	if store != nil {
		defer store.Close()
	}
	app := &App{cfg: cfg, client: &http.Client{Timeout: 20 * time.Second}, store: store}
	app.monitor = NewMonitoringService(cfg, app.client)
	mux := http.NewServeMux()
	mux.HandleFunc("/", app.index)
	mux.HandleFunc("/docs", app.docs)
	mux.HandleFunc("/login", app.login)
	mux.HandleFunc("/logout", app.logout)
	mux.HandleFunc("/peapod-logo.svg", app.frontendStatic("peapod-logo.svg"))
	mux.Handle("/assets/", app.frontendAssets())
	mux.HandleFunc("/api/login", app.apiLogin)
	mux.HandleFunc("/api/logout", app.apiLogout)
	mux.HandleFunc("/api/state", app.auth(app.state))
	mux.HandleFunc("/api/monitoring/summary", app.auth(app.monitoringSummary))
	mux.HandleFunc("/api/logs/summary", app.auth(app.logsSummary))
	mux.HandleFunc("/api/logs/containers", app.auth(app.logsContainers))
	mux.HandleFunc("/api/logs/query", app.auth(app.logsQuery))
	mux.HandleFunc("/api/users", app.auth(app.users))
	mux.HandleFunc("/api/users/", app.auth(app.userByID))
	mux.HandleFunc("/api/me", app.auth(app.me))
	mux.HandleFunc("/api/me/password", app.auth(app.changeOwnPassword))
	mux.HandleFunc("/api/setup/config", app.auth(app.setupConfig))
	mux.HandleFunc("/api/doctor/run", app.auth(app.doctorRun))
	mux.HandleFunc("/api/templates", app.auth(app.templates))
	mux.HandleFunc("/api/templates/", app.auth(app.templateAction))
	mux.HandleFunc("/api/woodpecker/repos", app.auth(app.woodpeckerRepos))
	mux.HandleFunc("/api/woodpecker/repos/", app.auth(app.woodpeckerRepoAction))
	mux.HandleFunc("/api/tasks/", app.auth(app.runTask))
	mux.HandleFunc("/api/config/tasks", app.auth(app.customTasks))
	mux.HandleFunc("/api/config/tasks/", app.auth(app.customTaskByID))
	mux.HandleFunc("/api/custom-run", app.auth(app.customRun))
	mux.HandleFunc("/api/pipelines/", app.auth(app.pipelineAction))
	mux.HandleFunc("/api/audit", app.auth(app.audit))
	mux.HandleFunc("/api/system/disk-diagnosis", app.auth(app.diskDiagnosis))
	mux.HandleFunc("/api/system/disk-cleanup-preview", app.auth(app.diskCleanupPreview))
	mux.HandleFunc("/api/system/disk-cleanup", app.auth(app.diskCleanup))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"peapod"}`))
	})
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           accessLogMiddleware(logger, cfg, securityHeaders(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("Pedpod listening", zap.String("addr", cfg.Addr))
	if err := server.ListenAndServe(); err != nil {
		logger.Fatal("server stopped", zap.Error(err))
	}
}

type App struct {
	cfg     Config
	client  *http.Client
	store   *UserStore
	monitor *MonitoringService
}


func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		if isHTTPS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) frontendAssets() http.Handler {
	return http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(a.cfg.FrontendDir, "assets"))))
}

func (a *App) frontendStatic(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(a.cfg.FrontendDir, name)
		if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		http.NotFound(w, r)
	}
}

func (a *App) serveFrontend(w http.ResponseWriter, r *http.Request, fallback *template.Template) {
	indexPath := filepath.Join(a.cfg.FrontendDir, "index.html")
	if stat, err := os.Stat(indexPath); err == nil && !stat.IsDir() {
		http.ServeFile(w, r, indexPath)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := map[string]any{
		"Error":         r.URL.Query().Get("error"),
		"DBMode":        a.store != nil,
		"WoodpeckerURL": a.cfg.WoodpeckerPublicURL,
	}
	_ = fallback.Execute(w, data)
}

func (a *App) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if _, ok := a.currentUser(r); !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	a.serveFrontend(w, r, indexTemplate)
}

func (a *App) docs(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/docs" {
		http.NotFound(w, r)
		return
	}
	if _, ok := a.currentUser(r); !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	a.serveFrontend(w, r, docsTemplate)
}

// buildAuditRecord constructs an audit record for the current user with the
// common fields pre-filled. Callers override Status, Error, or Pipeline as needed.
func buildAuditRecord(user AuthUser, r *http.Request, taskID, taskTitle string, repoID int, branch string, pipeline int64, variables map[string]string) AuditRecord {
	return AuditRecord{
		Time:      time.Now().Format(time.RFC3339),
		UserID:    user.ID,
		Username:  user.Username,
		RemoteIP:  remoteIP(r),
		TaskID:    taskID,
		TaskTitle: taskTitle,
		RepoID:    repoID,
		Branch:    branch,
		Pipeline:  pipeline,
		Variables: variables,
		Status:    "ok",
	}
}

func (a *App) state(w http.ResponseWriter, r *http.Request) {
	user := authUserFromRequest(r)
	pipelines := map[int][]Pipeline{}
	branches := map[int][]string{}
	health := map[string]interface{}{
		"checked_at": time.Now().Format(time.RFC3339),
		"auth_mode":  a.authMode(),
		"database":   healthStatus(a.store != nil, "数据库账号模式", "共享密码模式"),
	}
	woodpeckerErrors := []string{}
	visibleRepos := a.configuredRepos()
	type repoStateResult struct {
		repoID    int
		pipelines []Pipeline
		branches  []string
		errors    []string
	}
	results := make(chan repoStateResult, len(visibleRepos))
	var wg sync.WaitGroup
	for repoID := range visibleRepos {
		wg.Add(1)
		go func(repoID int) {
			defer wg.Done()
			result := repoStateResult{repoID: repoID}
			if rows, err := a.listPipelines(repoID, 24); err == nil {
				result.pipelines = rows
			} else {
				result.errors = append(result.errors, fmt.Sprintf("Repo %d 流水线：%v", repoID, err))
			}
			if rows, err := a.listBranches(repoID); err == nil {
				result.branches = rows
			} else {
				result.errors = append(result.errors, fmt.Sprintf("Repo %d 分支：%v", repoID, err))
			}
			results <- result
		}(repoID)
	}
	wg.Wait()
	close(results)
	for result := range results {
		if result.pipelines != nil {
			pipelines[result.repoID] = result.pipelines
		}
		if result.branches != nil {
			branches[result.repoID] = result.branches
		}
		woodpeckerErrors = append(woodpeckerErrors, result.errors...)
	}
	if len(woodpeckerErrors) == 0 {
		health["woodpecker"] = map[string]interface{}{"status": "ok", "message": "Woodpecker 状态已同步"}
	} else {
		health["woodpecker"] = map[string]interface{}{"status": "degraded", "message": "部分状态同步失败", "errors": woodpeckerErrors}
	}
	if records, err := a.listAudit(200); err == nil {
		annotatePipelinesWithAudit(pipelines, records)
		health["audit"] = map[string]interface{}{"status": "ok", "message": "操作历史可用"}
	} else {
		health["audit"] = map[string]interface{}{"status": "degraded", "message": "操作历史读取失败", "error": err.Error()}
	}
	configuredTasks := a.configuredTasks()
	resp := StateResponse{
		Tasks:              configuredTasks,
		Pipelines:          pipelines,
		DeploymentStatuses: deploymentStatuses(configuredTasks, visibleRepos, pipelines),
		Repos:              visibleRepos,
		Branches:           branches,
		Configurable:       user.Role == "admin",
		CurrentUser:        user,
		AuthMode:           a.authMode(),
		Now:                time.Now().Format(time.RFC3339),
		Links:              a.configuredLinks(),
		Health:             health,
	}
	writeJSON(w, resp)
}

func (a *App) runTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tasks/"), "/")
	id = strings.TrimSuffix(id, "/run")
	id = strings.TrimSuffix(id, "/")
	task, ok := a.findTask(id)
	if !ok || task.Disabled {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	user := authUserFromRequest(r)
	if !canRunTask(user, task) {
		writeError(w, http.StatusForbidden, taskForbiddenMessage(task))
		return
	}
	var req RunRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = task.Branch
	}
	if branch == "" {
		branch = "main"
	}
	variables := cloneMap(task.Variables)
	declaredInputs := map[string]bool{}
	for _, input := range task.Inputs {
		declaredInputs[input.Name] = true
		value := strings.TrimSpace(req.Inputs[input.Name])
		if input.Required && value == "" {
			http.Error(w, "missing input: "+input.Name, http.StatusBadRequest)
			return
		}
		if value != "" {
			variables[input.Name] = value
		}
	}
	if isRollbackTask(task) {
		for key, rawValue := range req.Inputs {
			key = strings.TrimSpace(key)
			if declaredInputs[key] || !isAllowedRollbackInput(key) {
				continue
			}
			if value := strings.TrimSpace(rawValue); value != "" {
				variables[key] = value
			}
		}
	}
	pipeline, err := a.createPipeline(task.RepoID, branch, variables)
	record := buildAuditRecord(user, r, task.ID, task.Title, task.RepoID, branch, 0, variables)
	if err != nil {
		record.Status = "error"
		record.Error = err.Error()
		_ = a.writeAudit(record)
		errorTask := task
		errorTask.Branch = branch
		writeError(w, http.StatusBadGateway, friendlyErrorMessage(err), friendlyErrorDetails(err, errorTask, variables)...)
		return
	}
	record.Pipeline = pipeline.Number
	_ = a.writeAudit(record)
	responseTask := task
	responseTask.Branch = branch
	writeJSON(w, RunResponse{
		OK:          true,
		Task:        responseTask,
		Pipeline:    pipeline,
		Woodpecker:  a.pipelineURL(task.RepoID, pipeline.Number),
		TriggeredAt: time.Now().Format(time.RFC3339),
	})
}

func (a *App) customRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		writeError(w, http.StatusForbidden, "自定义触发只允许管理员执行")
		return
	}
	var req CustomRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.RepoID <= 0 {
		http.Error(w, "repo_id is required", http.StatusBadRequest)
		return
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}
	variables := map[string]string{}
	for key, value := range req.Variables {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		variables[key] = strings.TrimSpace(value)
	}
	if len(variables) == 0 {
		http.Error(w, "at least one variable is required", http.StatusBadRequest)
		return
	}
	pipeline, err := a.createPipeline(req.RepoID, branch, variables)
	record := buildAuditRecord(user, r, "custom-run", "自定义部署", req.RepoID, branch, 0, variables)
	if err != nil {
		record.Status = "error"
		record.Error = err.Error()
		_ = a.writeAudit(record)
		writeError(w, http.StatusBadGateway, friendlyErrorMessage(err), friendlyErrorDetails(err, Task{ID: "custom-run", Title: "高级触发", RepoID: req.RepoID, Branch: branch}, variables)...)
		return
	}
	record.Pipeline = pipeline.Number
	_ = a.writeAudit(record)
	writeJSON(w, RunResponse{
		OK:          true,
		Task:        Task{ID: "custom-run", Group: "高级触发", Title: "高级触发", RepoID: req.RepoID, RepoName: strings.TrimSpace(req.RepoName), Branch: branch, Variables: variables},
		Pipeline:    pipeline,
		Woodpecker:  a.pipelineURL(req.RepoID, pipeline.Number),
		TriggeredAt: time.Now().Format(time.RFC3339),
	})
}

func (a *App) users(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		http.Error(w, "database auth is not enabled", http.StatusNotFound)
		return
	}
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		users, err := a.store.ListUsers(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"users": users})
	case http.MethodPost:
		var input UserInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		created, err := a.store.CreateUser(r.Context(), input)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"user": created})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) templates(w http.ResponseWriter, r *http.Request) {
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, TemplatesResponse{Templates: taskTemplates()})
}

func (a *App) templateAction(w http.ResponseWriter, r *http.Request) {
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/templates/"), "/")
	if !strings.HasSuffix(path, "/apply") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSuffix(path, "/apply")
	id = strings.Trim(id, "/")
	template, ok := findTaskTemplate(id)
	if !ok {
		http.Error(w, "template not found", http.StatusNotFound)
		return
	}
	var req TemplateApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	task, err := buildTaskFromTemplate(template, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := a.upsertTaskIntoConfig(task)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	_ = a.writeAudit(buildAuditRecord(user, r, "template-apply", "套用任务模板", task.RepoID, task.Branch, 0, map[string]string{
		"template": template.ID,
		"task":     task.ID,
		"project":  variableValue(task.Variables, "PEAPOD_PROJECT_ID"),
	}))
	writeJSON(w, TemplateApplyResponse{Task: task, Config: cfg})
}


func (a *App) loadCustomTaskConfig() (CustomTaskConfig, error) {
	cfg := CustomTaskConfig{Repos: map[int]string{}, Tasks: []Task{}}
	if a.cfg.TasksPath == "" {
		return cfg, nil
	}
	payload, err := os.ReadFile(a.cfg.TasksPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(payload, &cfg); err != nil {
		var rows []Task
		if err2 := json.Unmarshal(payload, &rows); err2 != nil {
			return cfg, err
		}
		cfg.Tasks = rows
	}
	if cfg.Repos == nil {
		cfg.Repos = map[int]string{}
	}
	return cfg, nil
}

func (a *App) saveCustomTaskConfig(cfg CustomTaskConfig) error {
	if a.cfg.TasksPath == "" {
		return errors.New("PEAPOD_TASKS_PATH is not configured")
	}
	if cfg.Repos == nil {
		cfg.Repos = map[int]string{}
	}
	sort.SliceStable(cfg.Tasks, func(i, j int) bool {
		if cfg.Tasks[i].Group == cfg.Tasks[j].Group {
			return cfg.Tasks[i].Title < cfg.Tasks[j].Title
		}
		return cfg.Tasks[i].Group < cfg.Tasks[j].Group
	})
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(a.cfg.TasksPath), 0o755); err != nil {
		return err
	}
	tmp := a.cfg.TasksPath + ".tmp"
	if err := os.WriteFile(tmp, append(payload, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.cfg.TasksPath)
}

func (a *App) saveConfiguredRepo(repoID int, repoName string) error {
	if repoID <= 0 {
		return errors.New("Repo ID 必须大于 0")
	}
	repoName = strings.TrimSpace(repoName)
	if repoName == "" {
		return errors.New("仓库名称不能为空")
	}
	cfg, err := a.loadCustomTaskConfig()
	if err != nil {
		return err
	}
	if cfg.Repos == nil {
		cfg.Repos = map[int]string{}
	}
	cfg.Repos[repoID] = repoName
	return a.saveCustomTaskConfig(cfg)
}

// upsertTaskIntoConfig loads the custom task configuration, merges the given task
// (replacing any existing task with the same ID), records its repo mapping, and
// persists the result. It returns the saved configuration.
func (a *App) upsertTaskIntoConfig(task Task) (CustomTaskConfig, error) {
	cfg, err := a.loadCustomTaskConfig()
	if err != nil {
		return cfg, err
	}
	if cfg.Repos == nil {
		cfg.Repos = map[int]string{}
	}
	if task.RepoName != "" {
		cfg.Repos[task.RepoID] = task.RepoName
	}
	replaced := false
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == task.ID {
			cfg.Tasks[i] = task
			replaced = true
			break
		}
	}
	if !replaced {
		cfg.Tasks = append(cfg.Tasks, task)
	}
	if err := a.saveCustomTaskConfig(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func taskTemplates() []TaskTemplate {
	commonInputs := []TemplateInput{
		{Name: "repo_id", Label: "Woodpecker Repo ID", Type: "number", Required: true, Placeholder: "3"},
		{Name: "repo_name", Label: "仓库显示名", Required: true, Placeholder: "owner/service"},
		{Name: "branch", Label: "默认分支", Default: "main", Required: true, Placeholder: "main"},
		{Name: "project_id", Label: "项目 ID", Required: true, Placeholder: "my-service", Help: "用于归并部署、回退和线上版本状态。"},
		{Name: "project_name", Label: "项目名称", Required: true, Placeholder: "业务服务"},
		{Name: "environment", Label: "所属环境", Type: "environment", Default: "production", Required: true},
		{Name: "marker_path", Label: "版本 marker 路径", Placeholder: "/opt/my-service/.deploy/current-source-sha", Help: "部署脚本落地后写入实际 commit。"},
		{Name: "health_url", Label: "健康检查 URL", Placeholder: "http://127.0.0.1:8080/healthz", Help: "返回 2xx/3xx 才算部署可信。"},
	}
	return []TaskTemplate{
		{
			ID:                   "docker-compose-service",
			Title:                "Docker Compose 服务部署",
			Description:          "适合 Go 后端、API、worker 等 compose 服务。默认生成可信部署变量。",
			Category:             "部署",
			DefaultGroup:         "业务服务",
			DefaultRisk:          "warning",
			DefaultBranch:        "main",
			RequiresVerification: true,
			Variables: map[string]string{
				"DEPLOY_ACTION":        "deploy",
				"DEPLOY_STRATEGY":      "compose",
				"PEAPOD_PROJECT_TYPE":  "docker-compose",
				"PEAPOD_PROJECT_GROUP": "service",
			},
			Inputs: append([]TemplateInput{}, commonInputs...),
		},
		{
			ID:                   "static-frontend",
			Title:                "静态前端部署",
			Description:          "适合官网、管理台、Vite/React 构建产物发布。",
			Category:             "部署",
			DefaultGroup:         "前端站点",
			DefaultRisk:          "warning",
			DefaultBranch:        "main",
			RequiresVerification: true,
			Variables: map[string]string{
				"DEPLOY_ACTION":        "deploy",
				"DEPLOY_STRATEGY":      "static",
				"PEAPOD_PROJECT_TYPE":  "static-site",
				"PEAPOD_PROJECT_GROUP": "site",
			},
			Inputs: append([]TemplateInput{}, commonInputs...),
		},
		{
			ID:                   "go-backend",
			Title:                "Go 后端部署",
			Description:          "适合 Go 服务构建镜像或二进制后部署。",
			Category:             "部署",
			DefaultGroup:         "后端服务",
			DefaultRisk:          "warning",
			DefaultBranch:        "main",
			RequiresVerification: true,
			Variables: map[string]string{
				"DEPLOY_ACTION":        "deploy",
				"BUILD_RUNTIME":        "go",
				"DEPLOY_STRATEGY":      "compose",
				"PEAPOD_PROJECT_TYPE":  "go-backend",
				"PEAPOD_PROJECT_GROUP": "service",
			},
			Inputs: append([]TemplateInput{}, commonInputs...),
		},
		{
			ID:                   "blue-green",
			Title:                "蓝绿部署",
			Description:          "适合需要槽位切换、健康检查和快速回退的服务。",
			Category:             "部署",
			DefaultGroup:         "业务服务",
			DefaultRisk:          "danger",
			DefaultBranch:        "main",
			RequiresVerification: true,
			Variables: map[string]string{
				"DEPLOY_ACTION":        "deploy",
				"DEPLOY_STRATEGY":      "blue-green",
				"PEAPOD_PROJECT_TYPE":  "blue-green",
				"PEAPOD_PROJECT_GROUP": "service",
			},
			Inputs: append([]TemplateInput{}, commonInputs...),
		},
		{
			ID:                   "disk-cleanup",
			Title:                "磁盘清理",
			Description:          "适合清理 Docker build cache、悬空镜像和明确允许的临时目录。",
			Category:             "维护",
			DefaultGroup:         "运维维护",
			DefaultRisk:          "danger",
			DefaultBranch:        "main",
			RequiresVerification: false,
			Variables: map[string]string{
				"DEPLOY_ACTION":      "cleanup",
				"CLEANUP_MODE":       "safe",
				"CLEANUP_SHOW_STATS": "1",
			},
			Inputs: []TemplateInput{
				{Name: "repo_id", Label: "Woodpecker Repo ID", Type: "number", Required: true, Placeholder: "3"},
				{Name: "repo_name", Label: "仓库显示名", Required: true, Placeholder: "owner/ops"},
				{Name: "branch", Label: "默认分支", Default: "main", Required: true},
				{Name: "project_id", Label: "维护目标 ID", Required: true, Placeholder: "prod-host"},
				{Name: "project_name", Label: "维护目标名称", Required: true, Placeholder: "生产机磁盘清理"},
				{Name: "environment", Label: "所属环境", Type: "environment", Default: "operations", Required: true},
			},
		},
		{
			ID:                   "peapod-self-deploy",
			Title:                "Pedpod 自部署",
			Description:          "让 Pedpod 自己也走 Woodpecker 部署和健康验证。",
			Category:             "运维",
			DefaultGroup:         "Pedpod",
			DefaultRisk:          "danger",
			DefaultBranch:        "main",
			RequiresVerification: true,
			Variables: map[string]string{
				"DEPLOY_ACTION":       "deploy",
				"PEAPOD_DEPLOY_DIR":   "/opt/peapod",
				"PEAPOD_HEALTH_URL":   "http://127.0.0.1:8095/healthz",
				"PEAPOD_PROJECT_TYPE": "peapod",
			},
			Inputs: append([]TemplateInput{}, commonInputs...),
		},
	}
}

func findTaskTemplate(id string) (TaskTemplate, bool) {
	id = strings.TrimSpace(id)
	for _, template := range taskTemplates() {
		if template.ID == id {
			return template, true
		}
	}
	return TaskTemplate{}, false
}

func buildTaskFromTemplate(template TaskTemplate, req TemplateApplyRequest) (Task, error) {
	projectID := normalizeTaskID(firstNonEmptyString(req.ProjectID, req.Values["project_id"]))
	if projectID == "" {
		return Task{}, errors.New("项目 ID 不能为空")
	}
	projectName := strings.TrimSpace(firstNonEmptyString(req.ProjectName, req.Values["project_name"]))
	if projectName == "" {
		return Task{}, errors.New("项目名称不能为空")
	}
	repoID := req.RepoID
	if repoID <= 0 {
		if parsed, err := strconv.Atoi(strings.TrimSpace(req.Values["repo_id"])); err == nil {
			repoID = parsed
		}
	}
	if repoID <= 0 {
		return Task{}, errors.New("Woodpecker Repo ID 必须大于 0")
	}
	repoName := strings.TrimSpace(firstNonEmptyString(req.RepoName, req.Values["repo_name"]))
	if repoName == "" {
		repoName = fmt.Sprintf("Repo %d", repoID)
	}
	branch := strings.TrimSpace(firstNonEmptyString(req.Branch, req.Values["branch"], template.DefaultBranch, "main"))
	environment := normalizeEnvironment(firstNonEmptyString(req.Environment, req.Values["environment"], "production"))
	markerPath := strings.TrimSpace(firstNonEmptyString(req.MarkerPath, req.Values["marker_path"]))
	healthURL := strings.TrimSpace(firstNonEmptyString(req.HealthURL, req.Values["health_url"]))
	if template.RequiresVerification && markerPath == "" && healthURL == "" {
		markerPath = fmt.Sprintf("/opt/%s/.deploy/current-source-sha", projectID)
	}
	variables := cloneMap(template.Variables)
	for key, value := range req.Values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || isReservedTemplateInput(key) {
			continue
		}
		variables[key] = value
	}
	variables["PEAPOD_PROJECT_ID"] = projectID
	variables["PEAPOD_PROJECT_NAME"] = projectName
	variables["PEAPOD_PROJECT_ENV"] = environment
	if markerPath != "" {
		variables["PEAPOD_DEPLOY_MARKER_PATH"] = markerPath
	}
	if healthURL != "" {
		variables["PEAPOD_DEPLOY_VERIFY_URL"] = healthURL
	}
	confirm := strings.TrimSpace(req.ConfirmText)
	if confirm == "" && template.DefaultRisk == "danger" {
		confirm = strings.ToUpper(environment)
		if confirm == "OPERATIONS" {
			confirm = "OPS"
		}
	}
	task := Task{
		ID:          normalizeTaskID(template.ID + "-" + environment + "-" + projectID),
		Group:       fallbackText(environmentLabel(environment), template.DefaultGroup),
		Title:       template.Title + " · " + projectName,
		Description: template.Description,
		RepoID:      repoID,
		RepoName:    repoName,
		Branch:      branch,
		Variables:   variables,
		Risk:        fallbackText(template.DefaultRisk, "normal"),
		ConfirmText: confirm,
	}
	if task.Group == "" {
		task.Group = template.DefaultGroup
	}
	if err := normalizeTaskConfig(&task); err != nil {
		return Task{}, err
	}
	task.Custom = true
	return task, nil
}

func isReservedTemplateInput(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "repo_id", "repo_name", "branch", "project_id", "project_name", "environment", "marker_path", "health_url", "confirm_text":
		return true
	default:
		return false
	}
}

func normalizeEnvironment(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ops", "operation", "operations", "builder", "build":
		return "operations"
	case "prod", "production":
		return "production"
	case "stage", "staging", "test", "testing", "dev":
		return "staging"
	case "service", "business":
		return "service"
	default:
		if strings.TrimSpace(value) == "" {
			return "production"
		}
		return normalizeTaskID(value)
	}
}

func environmentLabel(value string) string {
	switch normalizeEnvironment(value) {
	case "operations":
		return "运维机"
	case "production":
		return "生产机"
	case "staging":
		return "测试机"
	case "service":
		return "业务机"
	default:
		return value
	}
}

func normalizeTaskConfig(task *Task) error {
	task.ID = normalizeTaskID(task.ID)
	if task.ID == "" {
		task.ID = normalizeTaskID(task.Title)
	}
	if task.ID == "" {
		return errors.New("任务 ID 或标题不能为空")
	}
	task.Title = strings.TrimSpace(task.Title)
	if task.Title == "" {
		return errors.New("任务标题不能为空")
	}
	if task.RepoID <= 0 {
		return errors.New("Woodpecker Repo ID 必须大于 0")
	}
	task.Group = strings.TrimSpace(task.Group)
	if task.Group == "" {
		task.Group = "自定义任务"
	}
	task.Branch = strings.TrimSpace(task.Branch)
	if task.Branch == "" {
		task.Branch = "main"
	}
	task.RepoName = strings.TrimSpace(task.RepoName)
	task.Description = strings.TrimSpace(task.Description)
	task.Risk = strings.TrimSpace(task.Risk)
	switch task.Risk {
	case "", "normal":
		task.Risk = "normal"
	case "warning", "danger":
	default:
		return errors.New("风险级别只支持 normal / warning / danger")
	}
	if task.Variables == nil {
		task.Variables = map[string]string{}
	}
	cleanVariables := map[string]string{}
	for key, value := range task.Variables {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		cleanVariables[key] = strings.TrimSpace(value)
	}
	if len(cleanVariables) == 0 {
		return errors.New("至少需要配置一个 Woodpecker 变量")
	}
	task.Variables = cleanVariables
	if deploymentTaskRequiresVerification(*task) && !taskHasDeploymentVerification(*task) {
		return errors.New("部署类任务必须配置 PEAPOD_DEPLOY_MARKER_PATH 或 PEAPOD_DEPLOY_VERIFY_URL")
	}
	task.ConfirmText = strings.TrimSpace(task.ConfirmText)
	task.AllowedRoles = normalizeAllowedRoles(task.AllowedRoles)
	task.Disabled = false
	task.DisabledReason = ""
	task.ExternalURL = ""
	return nil
}

func normalizeAllowedRoles(roles []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, role := range roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role != "admin" && role != "operator" {
			continue
		}
		if seen[role] {
			continue
		}
		seen[role] = true
		out = append(out, role)
	}
	return out
}

func isBuiltinTaskID(id string) bool {
	id = strings.TrimSpace(id)
	for _, task := range tasks {
		if task.ID == id {
			return true
		}
	}
	return false
}

func normalizeTaskID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func authUserFromRequest(r *http.Request) AuthUser {
	user, _ := r.Context().Value(authUserContextKey{}).(AuthUser)
	return user
}

func (a *App) findTask(id string) (Task, bool) {
	for _, task := range a.configuredTasks() {
		if task.ID == id {
			return task, true
		}
	}
	return Task{}, false
}

func cloneMap(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func remoteIP(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			return strings.Split(value, ",")[0]
		}
	}
	return r.RemoteAddr
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string, details ...string) {
	cleanDetails := make([]string, 0, len(details))
	for _, detail := range details {
		detail = strings.TrimSpace(detail)
		if detail != "" {
			cleanDetails = append(cleanDetails, detail)
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: strings.TrimSpace(message), Details: cleanDetails})
}

func friendlyErrorMessage(err error) string {
	var woodpeckerErr WoodpeckerRequestError
	if errors.As(err, &woodpeckerErr) {
		return woodpeckerErr.Error()
	}
	return err.Error()
}

func friendlyErrorDetails(err error, task Task, variables map[string]string) []string {
	details := []string{
		"任务：" + fallbackText(task.Title, task.ID),
		fmt.Sprintf("Repo ID：%d", task.RepoID),
		"分支：" + fallbackText(task.Branch, "main"),
	}
	if len(variables) > 0 {
		details = append(details, "变量："+safeVariablesText(variables))
	}
	var woodpeckerErr WoodpeckerRequestError
	if errors.As(err, &woodpeckerErr) {
		details = append(details, woodpeckerErr.Details()...)
	}
	return details
}

func safeVariablesText(variables map[string]string) string {
	if len(variables) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := variables[key]
		if isSensitiveVariable(key) {
			value = "***"
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, " · ")
}

func isSensitiveVariable(key string) bool {
	upper := strings.ToUpper(key)
	for _, marker := range []string{"PASSWORD", "TOKEN", "SECRET", "KEY", "PRIVATE", "CREDENTIAL", "ACCESS"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

func fallbackText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

var loginTemplate = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Pedpod</title>
  <link rel="icon" type="image/svg+xml" href="` + faviconPath + `" />
  <style>{{template "styles"}}</style>
</head>
<body class="login-page">
  <main class="login-card">
    <div class="brand-mark" aria-hidden="true">` + peapodLogo + `</div>
    <h1>Pedpod</h1>
    <p>基础设施部署控制台</p>
    {{if .Error}}<div class="error">密码不正确。</div>{{end}}
    <form method="post" action="/login">
      {{if .DBMode}}
      <label>账号或邮箱</label>
      <input name="username" type="text" autocomplete="username" autofocus />
      {{end}}
      <label>密码</label>
      <input name="password" type="password" autocomplete="current-password" {{if not .DBMode}}autofocus{{end}} />
      <button type="submit">进入控制台</button>
    </form>
  </main>
</body>
</html>
{{define "styles"}}` + css + `{{end}}`))

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Pedpod</title>
  <link rel="icon" type="image/svg+xml" href="` + faviconPath + `" />
  <style>{{template "styles"}}</style>
</head>
<body>
  <header class="topbar">
    <div class="brand-lockup">
      <div class="brand-mark brand-mark-small" aria-hidden="true">` + peapodLogo + `</div>
      <div>
        <div class="eyebrow">Infrastructure Console</div>
        <h1>Pedpod</h1>
      </div>
    </div>
    <nav>
      <span id="currentUserBadge" class="nav-user"></span>
      <a class="nav-link" href="/">控制台</a>
      <a class="nav-link" href="/docs">部署文档</a>
      <a href="/logout">退出</a>
    </nav>
  </header>
  <main class="shell">
    <section class="hero-panel compact">
      <div>
        <div class="eyebrow">Deploy Workspace</div>
        <h2>基础设施部署工作台</h2>
        <p>统一触发部署、回退、清理和自定义 Woodpecker 任务。</p>
      </div>
      <div class="status-row" id="statusRow"></div>
    </section>
    <section id="loadError" class="error-panel" hidden></section>
    <section class="ops-layout">
      <section class="panel deploy-panel">
        <div class="panel-head">
          <div>
            <h2>任务编排</h2>
            <p>表格化查看任务模块、执行仓库、变量和风险级别。</p>
          </div>
          <a class="button ghost" href="/docs">查看参数</a>
        </div>
        <div class="table-wrap">
          <table class="ops-table">
            <thead><tr><th>动作</th><th>模块/执行仓库</th><th>变量</th><th>风险</th><th></th></tr></thead>
            <tbody id="taskTable"></tbody>
          </table>
        </div>
      </section>
      <aside class="side-column">
        <section class="panel">
          <div class="panel-head">
            <h2>最近流水线</h2>
            <button class="ghost" onclick="loadState()">刷新</button>
          </div>
          <div class="table-wrap compact-table">
            <table class="ops-table">
              <thead><tr><th>流水线</th><th>状态</th><th>进度</th><th></th></tr></thead>
              <tbody id="pipelineTable"></tbody>
            </table>
          </div>
        </section>
        <section class="panel">
          <div class="panel-head">
            <h2>自定义触发</h2>
          </div>
          <div class="custom-run-grid">
            <select id="customRepo"></select>
            <input id="customBranch" placeholder="分支，默认 main" />
            <textarea id="customVariables" placeholder="变量，每行一个：DEPLOY_ACTION=deploy"></textarea>
            <button onclick="runCustom()">触发</button>
          </div>
        </section>
        <section class="panel">
          <div class="panel-head">
            <h2>基础设施入口</h2>
          </div>
          <div id="quickLinks" class="quick-links"></div>
        </section>
      </aside>
    </section>
    <section class="panel" id="accountPanel" hidden>
      <div class="panel-head">
        <h2>我的账号</h2>
        <span id="authModeBadge" class="badge"></span>
      </div>
      <div class="inline-form profile-form">
        <input id="profileUsername" placeholder="账号名" />
        <input id="profileDisplayName" placeholder="姓名/昵称" />
        <input id="profileEmail" placeholder="邮箱" />
        <button class="ghost" onclick="saveProfile()">保存资料</button>
      </div>
      <div class="inline-form">
        <input id="oldPassword" type="password" placeholder="旧密码" autocomplete="current-password" />
        <input id="newPassword" type="password" placeholder="新密码，至少 8 位" autocomplete="new-password" />
        <button class="ghost" onclick="changeOwnPassword()">修改密码</button>
      </div>
    </section>
    <section class="panel" id="usersPanel" hidden>
      <div class="panel-head">
        <h2>成员账号</h2>
        <button class="ghost" onclick="loadUsers()">刷新成员</button>
      </div>
      <div class="inline-form">
        <input id="newUsername" placeholder="账号，例如 tangfire" />
        <input id="newDisplayName" placeholder="姓名/昵称" />
        <input id="newEmail" placeholder="邮箱，可选" />
        <input id="newUserPassword" type="password" placeholder="初始密码" />
        <select id="newUserRole">
          <option value="operator">成员</option>
          <option value="admin">管理员</option>
        </select>
        <button onclick="createUser()">创建成员</button>
      </div>
      <div id="usersTable" class="user-table"></div>
    </section>
  </main>
  <dialog id="runDialog">
    <form method="dialog" id="runForm">
      <h3 id="dialogTitle"></h3>
      <p id="dialogDesc"></p>
      <div id="dialogInputs"></div>
      <label id="confirmLabel" class="confirm-label"></label>
      <input id="confirmInput" autocomplete="off" />
      <menu>
        <button value="cancel" class="ghost">取消</button>
        <button id="confirmButton" value="default">执行</button>
      </menu>
    </form>
  </dialog>
  <script>{{template "script"}}</script>
</body>
</html>
{{define "styles"}}` + css + `{{end}}
{{define "script"}}` + js + `{{end}}`))

var docsTemplate = template.Must(template.New("docs").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Pedpod · 部署文档</title>
  <link rel="icon" type="image/svg+xml" href="` + faviconPath + `" />
  <style>{{template "styles"}}</style>
</head>
<body>
  <header class="topbar">
    <div class="brand-lockup">
      <div class="brand-mark brand-mark-small" aria-hidden="true">` + peapodLogo + `</div>
      <div>
        <div class="eyebrow">Runbook</div>
        <h1>部署文档</h1>
      </div>
    </div>
    <nav>
      <a class="nav-link" href="/">控制台</a>
      <a class="nav-link" href="/docs">部署文档</a>
      <a href="/logout">退出</a>
    </nav>
  </header>
  <main class="shell docs-shell">
    <section class="hero-panel">
      <div>
        <div class="eyebrow">Woodpecker Parameters</div>
        <h2>通用手动部署参数</h2>
        <p>Pedpod 的部署动作来自 <code>PEAPOD_TASKS_PATH</code> 指向的任务配置。面板不可用时，可以到 Woodpecker 手动触发同一个仓库、分支和变量。</p>
      </div>
      <a class="button" target="_blank" rel="noreferrer" href="{{.WoodpeckerURL}}">打开 Woodpecker</a>
    </section>

    <section class="docs-grid">
      <article class="doc-card">
        <h2>任务配置</h2>
        <p>每个任务至少包含 Repo ID、默认分支、变量和风险级别。建议为同一项目的部署和回退设置相同的 <code>PEAPOD_PROJECT_ID</code>，这样项目状态会自动归并。</p>
        <div class="code-block">{
  "repos": {"1": "your-repo"},
  "tasks": [{
    "id": "app-deploy",
    "group": "业务服务",
    "title": "部署业务服务",
    "repo_id": 1,
    "branch": "main",
    "risk": "normal",
    "variables": {
      "DEPLOY_ACTION": "deploy",
      "PEAPOD_PROJECT_ID": "app",
      "PEAPOD_PROJECT_NAME": "业务服务"
    }
  }]
}</div>
      </article>

      <article class="doc-card">
        <h2>底层系统</h2>
        <p>Pedpod 只做统一入口和轻量诊断，真正执行仍由 Woodpecker、Beszel、Dozzle，以及可选 Grafana/Loki/Prometheus/Tempo 完成。</p>
        <table class="param-table">
          <thead><tr><th>系统</th><th>用途</th><th>配置</th></tr></thead>
          <tbody>
            <tr><td>Woodpecker</td><td>流水线执行、取消、日志</td><td><code>WOODPECKER_SERVER</code> / <code>WOODPECKER_TOKEN</code></td></tr>
            <tr><td>Beszel</td><td>机器资源和容器状态</td><td><code>PEAPOD_BESZEL_*</code></td></tr>
            <tr><td>Dozzle</td><td>轻量查看 Docker 已保留日志并实时跟随</td><td><code>PEAPOD_DOZZLE_PUBLIC_URL</code></td></tr>
            <tr><td>Grafana</td><td>完整历史日志、指标、链路面板</td><td><code>PEAPOD_GRAFANA_PUBLIC_URL</code></td></tr>
          </tbody>
        </table>
      </article>

      <article class="doc-card">
        <h2>监控主机</h2>
        <p>通过 <code>PEAPOD_MONITOR_HOSTS_JSON</code> 配置需要观察的机器、Beszel 名称、SSH 只读兜底和核心容器。</p>
        <div class="code-block">[{"id":"prod","name":"生产机","role":"production","ssh_host":"example.com:22","ssh_user":"ops","containers":["api","worker","mysql"]}]</div>
      </article>
    </section>
  </main>
</body>
</html>
{{define "styles"}}` + css + `{{end}}`))

const faviconPath = `/peapod-logo.svg?v=pea`

const peapodLogo = `
<img class="peapod-logo" src="/peapod-logo.svg?v=pea" alt="" draggable="false" />`

const css = `
:root { color-scheme: light; --bg:#f5f8f2; --panel:#fbfdf9; --ink:#1f2a22; --muted:#68736a; --line:#dfe8d9; --accent:#3d721d; --ok:#5ea53a; --warn:#ba7a17; --danger:#bd2c2c; }
* { box-sizing: border-box; }
body { margin:0; min-height:100vh; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color:var(--ink); background:linear-gradient(180deg,#fbfef8,#edf5e8); }
body::before { content:""; position:fixed; inset:0; pointer-events:none; opacity:.55; background-image:linear-gradient(var(--line) 1px,transparent 1px),linear-gradient(90deg,var(--line) 1px,transparent 1px); background-size:42px 42px; }
a { color:inherit; text-decoration:none; }
button, input, select { font:inherit; }
.topbar { position:sticky; top:0; z-index:3; display:flex; align-items:center; justify-content:space-between; padding:22px 28px; backdrop-filter:blur(16px); background:rgba(255,255,255,.82); border-bottom:1px solid var(--line); }
.topbar nav { display:flex; align-items:center; gap:16px; }
.brand-lockup { display:flex; align-items:center; gap:12px; }
.nav-user { color:var(--muted); font-weight:800; }
.nav-link { height:34px; display:inline-flex; align-items:center; padding:0 10px; border-radius:7px; background:rgba(255,255,255,.66); border:1px solid rgba(223,232,217,.9); font-weight:800; }
.eyebrow { color:var(--accent); font-size:12px; font-weight:800; letter-spacing:.12em; text-transform:uppercase; }
h1 { margin:4px 0 0; font-size:28px; letter-spacing:0; }
h2 { margin:0; font-size:18px; }
h3 { margin:0 0 8px; font-size:20px; }
.panel-head p, .hero-panel p, .doc-card p { margin:6px 0 0; color:var(--muted); line-height:1.55; }
.shell { position:relative; z-index:1; width:min(1240px, calc(100vw - 32px)); margin:24px auto 56px; display:grid; gap:18px; }
.hero-panel { display:grid; grid-template-columns:minmax(0, .95fr) minmax(420px, 1.05fr); gap:16px; align-items:stretch; padding:22px; background:rgba(251,253,249,.94); border:1px solid var(--line); border-radius:8px; box-shadow:0 18px 48px rgba(39,78,31,.08); }
.hero-panel h2 { margin:4px 0 0; font-size:26px; }
.ops-layout { display:grid; grid-template-columns:minmax(0, 1fr) 340px; gap:18px; align-items:start; }
.side-column { display:grid; gap:18px; position:sticky; top:108px; }
.status-row { display:grid; grid-template-columns:repeat(3, minmax(0, 1fr)); gap:12px; }
.metric, .panel, .task-card { background:rgba(251,253,249,.94); border:1px solid var(--line); border-radius:8px; box-shadow:0 18px 48px rgba(39,78,31,.08); }
.metric { padding:15px; min-height:82px; background:#fff; }
.metric b { display:block; font-size:22px; margin-bottom:4px; }
.metric span, .task-card p, .pipeline small, .login-card p { color:var(--muted); }
.panel { padding:18px; }
.panel-head { display:flex; align-items:center; justify-content:space-between; gap:12px; margin-bottom:14px; }
.error-panel { padding:14px 16px; border:1px solid #ffd0cd; background:#fff0ef; color:var(--danger); border-radius:8px; font-weight:800; }
.task-groups { display:grid; grid-template-columns:repeat(2, minmax(0,1fr)); gap:14px; }
.task-section { min-width:0; padding:14px; border:1px solid var(--line); border-radius:8px; background:#fff; }
.task-section h2 { margin:0; font-size:17px; }
.task-section p { margin:5px 0 12px; color:var(--muted); line-height:1.45; }
.task-grid { display:grid; grid-template-columns:repeat(auto-fit, minmax(220px,1fr)); gap:10px; }
.task-card { min-height:158px; padding:14px; display:flex; flex-direction:column; gap:10px; background:#fbfdf9; box-shadow:none; }
.task-card h3 { font-size:16px; margin:0; }
.task-card p { margin:0; line-height:1.55; flex:1; }
.task-meta { display:flex; align-items:center; justify-content:space-between; gap:10px; }
.task-vars { display:flex; flex-wrap:wrap; gap:6px; min-height:24px; }
.badge { display:inline-flex; align-items:center; height:24px; padding:0 8px; border-radius:999px; font-size:12px; font-weight:700; background:#edf5e8; color:#3d721d; }
.badge.normal { background:#e5f4ec; color:var(--ok); }
.badge.warning { background:#fff2d8; color:var(--warn); }
.badge.danger { background:#ffe1df; color:var(--danger); }
.badge.link { background:#e8eef8; color:#365b8f; }
button, .button { border:0; border-radius:7px; background:var(--accent); color:white; height:38px; padding:0 14px; font-weight:800; cursor:pointer; display:inline-flex; align-items:center; justify-content:center; }
button:hover, .button:hover { filter:brightness(.98); }
button:disabled { opacity:.55; cursor:not-allowed; }
.ghost { background:#eef5e9; color:#243027; }
.danger-button { background:var(--danger); }
.quick-links { display:grid; gap:10px; }
.quick-link { display:flex; align-items:center; justify-content:space-between; gap:10px; padding:12px; border:1px solid var(--line); border-radius:8px; background:#fff; }
.quick-link strong { display:block; margin-bottom:4px; }
.quick-link span { color:var(--muted); font-size:13px; line-height:1.35; }
.quick-link .button { height:34px; flex:0 0 auto; }
.pipeline-grid { display:grid; grid-template-columns:1fr; gap:12px; }
.pipeline { border:1px solid var(--line); border-radius:8px; padding:12px; background:#fff; min-height:108px; }
.pipeline strong { display:block; margin-bottom:4px; }
.status { font-weight:800; }
.status.success { color:var(--ok); }
.status.failure, .status.error, .status.killed { color:var(--danger); }
.status.running, .status.pending { color:var(--warn); }
.login-page { display:grid; place-items:center; padding:20px; }
.login-card { position:relative; z-index:1; width:min(420px,100%); padding:28px; background:rgba(251,253,249,.96); border:1px solid var(--line); border-radius:8px; box-shadow:0 28px 70px rgba(39,78,31,.12); }
.brand-mark { width:58px; height:58px; display:grid; place-items:center; margin-bottom:14px; }
.brand-mark-small { width:44px; height:44px; margin-bottom:0; flex:0 0 auto; }
.peapod-logo { width:100%; height:100%; display:block; object-fit:contain; user-select:none; filter:drop-shadow(0 10px 22px rgba(39,78,31,.18)); }
.brand-mark-small .peapod-logo { filter:drop-shadow(0 8px 16px rgba(39,78,31,.14)); }
label { display:block; margin:14px 0 8px; font-weight:800; }
input, select { width:100%; height:42px; border:1px solid var(--line); border-radius:7px; padding:0 12px; background:#fff; color:var(--ink); }
.inline-form { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)) auto; gap:10px; align-items:center; }
.inline-form button { white-space:nowrap; }
.user-table { display:grid; gap:10px; margin-top:14px; }
.user-row { display:grid; grid-template-columns:1.2fr 1fr 1.4fr .8fr .8fr 1.4fr; gap:8px; align-items:center; padding:10px; border:1px solid var(--line); border-radius:8px; background:#fff; }
.user-row.header { color:var(--muted); font-size:12px; font-weight:900; background:#f3f8ef; }
.user-row input, .user-row select { height:36px; }
.row-actions { display:flex; gap:8px; justify-content:flex-end; }
.login-card button { width:100%; margin-top:16px; }
.error { margin:12px 0; padding:10px; border-radius:7px; background:#ffe1df; color:var(--danger); }
dialog { border:1px solid var(--line); border-radius:8px; padding:0; width:min(460px,calc(100vw - 30px)); box-shadow:0 30px 80px rgba(0,0,0,.18); }
dialog::backdrop { background:rgba(20,24,25,.32); backdrop-filter:blur(3px); }
#runForm { padding:22px; }
#dialogDesc { color:var(--muted); line-height:1.55; }
.confirm-label { color:var(--danger); }
menu { display:flex; justify-content:flex-end; gap:10px; padding:0; margin:18px 0 0; }
.toast { position:fixed; right:18px; bottom:18px; z-index:8; padding:12px 14px; background:#1e282b; color:white; border-radius:8px; box-shadow:0 16px 42px rgba(0,0,0,.2); }
.docs-shell { max-width:1180px; }
.docs-grid { display:grid; grid-template-columns:repeat(2, minmax(0,1fr)); gap:16px; }
.doc-card { background:rgba(251,253,249,.94); border:1px solid var(--line); border-radius:8px; padding:18px; box-shadow:0 18px 48px rgba(39,78,31,.08); }
.param-table { width:100%; border-collapse:separate; border-spacing:0; margin-top:14px; overflow:hidden; border:1px solid var(--line); border-radius:8px; background:#fff; }
.param-table th, .param-table td { text-align:left; vertical-align:top; padding:10px 12px; border-bottom:1px solid var(--line); line-height:1.55; }
.param-table th { background:#f3f8ef; font-size:12px; color:#3d721d; text-transform:uppercase; }
.param-table tr:last-child td { border-bottom:0; }
code { display:inline-block; padding:2px 6px; border-radius:6px; background:#eef5e9; color:#243027; font-family:ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size:12px; }
.code-block { white-space:pre-wrap; margin-top:14px; padding:12px; border-radius:8px; background:#1d291e; color:#f5ffed; font-family:ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size:12px; line-height:1.6; }
@media (max-width: 1100px) { .hero-panel, .ops-layout, .docs-grid { grid-template-columns:1fr; } .side-column { position:static; } .task-groups { grid-template-columns:1fr; } }
@media (max-width: 900px) { .status-row, .task-grid, .pipeline-grid, .inline-form, .user-row { grid-template-columns:1fr; } .topbar { padding:18px; } .topbar nav { align-items:flex-end; flex-direction:column; gap:8px; } .row-actions { justify-content:flex-start; } }
`

const js = `
let state = null;
let selectedTask = null;
let usersLoaded = false;

async function loadState() {
  try {
    const res = await fetch('/api/state', { credentials: 'same-origin' });
    if (res.status === 401) {
      location.href = '/login';
      return;
    }
    if (!res.ok) throw new Error(await res.text() || '状态接口异常');
    state = await res.json();
    document.getElementById('loadError').hidden = true;
    render();
  } catch (err) {
    showLoadError(err.message || '加载失败');
  }
}

function render() {
  renderCurrentUser();
  renderStatus();
  renderTasks();
  renderQuickLinks();
  renderAccount();
  renderPipelines();
  if (state.auth_mode === 'db' && state.current_user && state.current_user.role === 'admin' && !usersLoaded) {
    loadUsers();
  }
}

function renderCurrentUser() {
  const user = state.current_user || {};
  document.getElementById('currentUserBadge').textContent = user.username ? (user.display_name || user.username) + ' · ' + roleLabel(user.role) : '';
}

function renderStatus() {
  const pipelines = Object.values(state.pipelines || {}).flat();
  const running = pipelines.filter(p => ['running','pending'].includes(p.status)).length;
  let latestSuccess = null;
  let latestSuccessRepo = '';
  for (const [repoId, rows] of Object.entries(state.pipelines || {})) {
    const hit = rows.find(p => p.status === 'success');
    if (hit) {
      latestSuccess = hit;
      latestSuccessRepo = state.repos[repoId] || ('Repo ' + repoId);
      break;
    }
  }
  document.getElementById('statusRow').innerHTML = [
    metric('Woodpecker', state.links.woodpecker ? '已连接' : '未配置', '底层执行器'),
    metric('运行中', String(running), '正在执行的流水线'),
    metric('最近成功', latestSuccess ? '#' + latestSuccess.number : '-', latestSuccess ? latestSuccessRepo : '暂无')
  ].join('');
}

function showLoadError(message) {
  document.getElementById('statusRow').innerHTML = [
    metric('加载失败', '请刷新', '如果持续失败，打开 Woodpecker 查看服务状态')
  ].join('');
  const el = document.getElementById('loadError');
  el.hidden = false;
  el.textContent = 'Pedpod 暂时没拿到部署数据：' + message;
  document.getElementById('taskGroups').innerHTML = '';
  document.getElementById('quickLinks').innerHTML = '';
  document.getElementById('pipelines').innerHTML = '<p>暂无流水线。</p>';
}

function metric(title, value, hint) {
  return '<div class="metric"><span>' + esc(title) + '</span><b>' + esc(value) + '</b><span>' + esc(hint || '') + '</span></div>';
}

function renderTasks() {
  const groups = {};
  for (const task of state.tasks || []) {
    if (task.external_url) continue;
    (groups[task.group] ||= []).push(task);
  }
  const order = ['业务服务', '基础设施', 'Pedpod', '磁盘维护'];
  const html = Object.entries(groups).sort((a, b) => groupIndex(a[0], order) - groupIndex(b[0], order)).map(([group, tasks]) => {
    return '<section class="task-section"><h2>' + esc(group) + '</h2><p>' + esc(groupNote(group)) + '</p><div class="task-grid">' + tasks.map(taskCard).join('') + '</div></section>';
  }).join('');
  document.getElementById('taskGroups').innerHTML = html;
}

function taskCard(task) {
  const action = '<button class="' + (task.risk === 'danger' ? 'danger-button' : '') + '" data-task-id="' + esc(task.id) + '">执行</button>';
  const vars = Object.entries(task.variables || {}).map(([key, value]) => '<span class="badge">' + esc(key) + '=' + esc(value || '-') + '</span>').join('');
  const repo = task.repo_id ? esc(state.repos[task.repo_id] || ('Repo ' + task.repo_id)) : '外部';
  return '<article class="task-card"><div class="task-meta"><h3>' + esc(task.title) + '</h3><span class="badge ' + esc(task.risk) + '">' + riskLabel(task.risk) + '</span></div><p>' + esc(task.description) + '</p><div class="task-vars">' + vars + '</div><div class="task-meta"><span class="badge">' + esc(task.group || '默认模块') + ' · ' + repo + '</span>' + action + '</div></article>';
}

function renderQuickLinks() {
  const links = (state.tasks || []).filter(task => task.external_url);
  document.getElementById('quickLinks').innerHTML = links.map(task => {
    return '<a class="quick-link" target="_blank" rel="noreferrer" href="' + esc(task.external_url) + '"><span><strong>' + esc(task.title) + '</strong><span>' + esc(task.description) + '</span></span><span class="button ghost">打开</span></a>';
  }).join('') || '<p>暂无外部入口。</p>';
}

function groupIndex(group, order) {
  const index = order.indexOf(group);
  return index === -1 ? 999 : index;
}

function groupNote(group) {
  const notes = {
    '业务服务': '业务服务部署、回退和重启。',
    '基础设施': 'Grafana、Loki、Tempo、Prometheus、Beszel、Woodpecker 配置刷新。',
    'Pedpod': '部署平台自更新。',
    '磁盘维护': '构建缓存和无用镜像清理。'
  };
  return notes[group] || '基础设施操作。';
}

function renderPipelines() {
  const allRows = [];
  for (const [repoId, repoRows] of Object.entries(state.pipelines || {})) {
    for (const p of repoRows) {
      allRows.push({...p, repo_id: repoId, repo_name: state.repos[repoId] || ('Repo ' + repoId)});
    }
  }
  const cards = allRows.sort((a, b) => ((b.started || b.finished || 0) - (a.started || a.finished || 0))).slice(0, 8).map(p => {
    return '<article class="pipeline"><strong>' + esc(p.repo_name) + ' #' + p.number + '</strong><span class="status ' + esc(p.status) + '">' + esc(p.status) + '</span><br><small>' + esc(p.event) + ' · ' + esc(p.branch || '-') + ' · ' + esc((p.commit || '').slice(0,8)) + '</small><br><a target="_blank" rel="noreferrer" href="' + esc(state.links.woodpecker.replace(/\\/+$/, '') + '/repos/' + p.repo_id + '/pipeline/' + p.number) + '">查看流水线</a></article>';
  });
  document.getElementById('pipelines').innerHTML = cards.join('') || '<p>暂无流水线。</p>';
}

function renderAccount() {
  const accountPanel = document.getElementById('accountPanel');
  const usersPanel = document.getElementById('usersPanel');
  const dbMode = state.auth_mode === 'db';
  accountPanel.hidden = !dbMode;
  usersPanel.hidden = !(dbMode && state.current_user && state.current_user.role === 'admin');
  document.getElementById('authModeBadge').textContent = dbMode ? '数据库账号' : '共享密码';
}

async function loadUsers() {
  if (!state || state.auth_mode !== 'db' || !state.current_user || state.current_user.role !== 'admin') return;
  try {
    const res = await fetch('/api/users', { credentials: 'same-origin' });
    if (!res.ok) throw new Error(await res.text() || '成员加载失败');
    const data = await res.json();
    usersLoaded = true;
    renderUsers(data.users || []);
  } catch (err) {
    document.getElementById('usersTable').innerHTML = '<div class="error-panel">' + esc(err.message || '成员加载失败') + '</div>';
  }
}

function renderUsers(users) {
  const rows = ['<div class="user-row header"><span>账号</span><span>姓名</span><span>邮箱</span><span>角色</span><span>状态</span><span>操作</span></div>'];
  for (const user of users) {
    const id = String(user.id);
    rows.push(
      '<div class="user-row">' +
      '<input data-user-field="username" data-user-id="' + esc(id) + '" value="' + esc(user.username) + '">' +
      '<input data-user-field="display_name" data-user-id="' + esc(id) + '" value="' + esc(user.display_name || '') + '">' +
      '<input data-user-field="email" data-user-id="' + esc(id) + '" value="' + esc(user.email || '') + '">' +
      '<select data-user-field="role" data-user-id="' + esc(id) + '">' +
      '<option value="operator"' + selected(user.role === 'operator') + '>成员</option>' +
      '<option value="admin"' + selected(user.role === 'admin') + '>管理员</option>' +
      '</select>' +
      '<select data-user-field="active" data-user-id="' + esc(id) + '">' +
      '<option value="true"' + selected(user.active) + '>启用</option>' +
      '<option value="false"' + selected(!user.active) + '>停用</option>' +
      '</select>' +
      '<div class="row-actions"><input data-user-field="password" data-user-id="' + esc(id) + '" type="password" placeholder="新密码"><button class="ghost" data-user-action="save" data-user-id="' + esc(id) + '">保存</button><button class="ghost" data-user-action="password" data-user-id="' + esc(id) + '">改密</button></div>' +
      '</div>'
    );
  }
  document.getElementById('usersTable').innerHTML = rows.join('');
}

async function createUser() {
  const body = {
    username: document.getElementById('newUsername').value.trim(),
    display_name: document.getElementById('newDisplayName').value.trim(),
    email: document.getElementById('newEmail').value.trim(),
    password: document.getElementById('newUserPassword').value,
    role: document.getElementById('newUserRole').value
  };
  try {
    const res = await fetch('/api/users', { method:'POST', headers:{'Content-Type':'application/json'}, credentials:'same-origin', body: JSON.stringify(body) });
    if (!res.ok) throw new Error(await res.text() || '创建失败');
    document.getElementById('newUsername').value = '';
    document.getElementById('newDisplayName').value = '';
    document.getElementById('newEmail').value = '';
    document.getElementById('newUserPassword').value = '';
    toast('成员已创建');
    await loadUsers();
  } catch (err) {
    toast(err.message || '创建失败');
  }
}

async function saveUser(id) {
  const body = readUserRow(id);
  try {
    const res = await fetch('/api/users/' + id, { method:'PATCH', headers:{'Content-Type':'application/json'}, credentials:'same-origin', body: JSON.stringify(body) });
    if (!res.ok) throw new Error(await res.text() || '保存失败');
    toast('成员已保存');
    await loadUsers();
  } catch (err) {
    toast(err.message || '保存失败');
  }
}

async function resetUserPassword(id) {
  const input = document.querySelector('[data-user-field="password"][data-user-id="' + id + '"]');
  const password = input ? input.value : '';
  if (!password) {
    toast('请输入新密码');
    return;
  }
  try {
    const res = await fetch('/api/users/' + id + '/password', { method:'POST', headers:{'Content-Type':'application/json'}, credentials:'same-origin', body: JSON.stringify({new_password: password}) });
    if (!res.ok) throw new Error(await res.text() || '改密失败');
    input.value = '';
    toast('密码已更新');
  } catch (err) {
    toast(err.message || '改密失败');
  }
}

async function changeOwnPassword() {
  const oldPassword = document.getElementById('oldPassword').value;
  const newPassword = document.getElementById('newPassword').value;
  try {
    const res = await fetch('/api/me/password', { method:'POST', headers:{'Content-Type':'application/json'}, credentials:'same-origin', body: JSON.stringify({old_password: oldPassword, new_password: newPassword}) });
    if (!res.ok) throw new Error(await res.text() || '修改失败');
    document.getElementById('oldPassword').value = '';
    document.getElementById('newPassword').value = '';
    toast('密码已修改');
  } catch (err) {
    toast(err.message || '修改失败');
  }
}

function readUserRow(id) {
  const value = (field) => {
    const el = document.querySelector('[data-user-field="' + field + '"][data-user-id="' + id + '"]');
    return el ? el.value : '';
  };
  return {
    username: value('username').trim(),
    display_name: value('display_name').trim(),
    email: value('email').trim(),
    role: value('role'),
    active: value('active') === 'true'
  };
}

function openRun(id) {
  selectedTask = state.tasks.find(t => t.id === id);
  if (!selectedTask) return;
  document.getElementById('dialogTitle').textContent = selectedTask.title;
  document.getElementById('dialogDesc').textContent = selectedTask.description;
  const inputs = (selectedTask.inputs || []).map(input => '<label>' + esc(input.label) + '</label><input data-input="' + esc(input.name) + '" placeholder="' + esc(input.placeholder || '') + '">').join('');
  document.getElementById('dialogInputs').innerHTML = inputs;
  const confirmLabel = document.getElementById('confirmLabel');
  const confirmInput = document.getElementById('confirmInput');
  if (selectedTask.confirm_text) {
    confirmLabel.style.display = 'block';
    confirmInput.style.display = 'block';
    confirmLabel.textContent = '请输入 ' + selectedTask.confirm_text + ' 确认执行';
    confirmInput.value = '';
  } else {
    confirmLabel.style.display = 'none';
    confirmInput.style.display = 'none';
    confirmInput.value = '';
  }
  document.getElementById('runDialog').showModal();
}

document.getElementById('confirmButton').addEventListener('click', async (event) => {
  event.preventDefault();
  if (!selectedTask) return;
  if (selectedTask.confirm_text && document.getElementById('confirmInput').value.trim() !== selectedTask.confirm_text) {
    toast('确认文字不匹配');
    return;
  }
  const inputs = {};
  document.querySelectorAll('[data-input]').forEach(input => { inputs[input.dataset.input] = input.value.trim(); });
  const button = document.getElementById('confirmButton');
  button.disabled = true;
  try {
    const res = await fetch('/api/tasks/' + selectedTask.id + '/run', {
      method: 'POST',
      headers: {'Content-Type':'application/json'},
      credentials: 'same-origin',
      body: JSON.stringify({inputs})
    });
    const text = await res.text();
    if (!res.ok) throw new Error(text || '执行失败');
    const data = JSON.parse(text);
    document.getElementById('runDialog').close();
    toast('已触发流水线 #' + data.pipeline.number);
    await loadState();
  } catch (err) {
    toast(err.message || '执行失败');
  } finally {
    button.disabled = false;
  }
});

document.addEventListener('click', (event) => {
  const taskButton = event.target.closest('[data-task-id]');
  if (taskButton) {
    openRun(taskButton.dataset.taskId);
    return;
  }
  const userButton = event.target.closest('[data-user-action]');
  if (userButton) {
    const id = userButton.dataset.userId;
    if (userButton.dataset.userAction === 'save') saveUser(id);
    if (userButton.dataset.userAction === 'password') resetUserPassword(id);
  }
});

function toast(message) {
  const el = document.createElement('div');
  el.className = 'toast';
  el.textContent = message;
  document.body.appendChild(el);
  setTimeout(() => el.remove(), 3600);
}

function repoName(id) { return state.repos[id] || ('Repo ' + id); }
function riskLabel(risk) { return ({normal:'普通', warning:'注意', danger:'高危', link:'入口'}[risk] || risk); }
function roleLabel(role) { return ({admin:'管理员', operator:'成员'}[role] || role || '成员'); }
function selected(value) { return value ? ' selected' : ''; }
function esc(value) {
  return String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
}

loadState();
setInterval(loadState, 15000);
`
