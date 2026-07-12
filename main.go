package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const cookieName = "peapod_session"
const productName = "Pedpod"

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

type DiskDiagnosisResponse struct {
	Filesystems []DiskFilesystemInfo `json:"filesystems"`
	Docker      DockerDiskInfo       `json:"docker"`
	TopDirs     []DiskUsageItem      `json:"top_dirs"`
	DockerOK    bool                 `json:"docker_ok"`
	CheckedAt   string               `json:"checked_at"`
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

type DiskCleanupResponse struct {
	OK        bool   `json:"ok"`
	Level     string `json:"level"`
	Reclaimed string `json:"reclaimed"`
	Details   string `json:"details"`
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

func loadConfig() Config {
	cfg := Config{
		Addr:                          envFirst(":8095", "PEAPOD_ADDR", "ZEPHYR_ADDR", "ZEFIRE_ADDR"),
		AppEnv:                        envFirst("production", "PEAPOD_APP_ENV", "APP_ENV"),
		LogLevel:                      envFirst("info", "PEAPOD_LOG_LEVEL", "LOG_LEVEL"),
		AccessLogMode:                 envFirst("attention", "PEAPOD_ACCESS_LOG_MODE", "ACCESS_LOG_MODE"),
		AccessLogSlowThresholdSeconds: envIntFirst(3, "PEAPOD_ACCESS_LOG_SLOW_THRESHOLD_SECONDS", "ACCESS_LOG_SLOW_THRESHOLD_SECONDS"),
		ConfigPath:                    envFirst("/data/config.json", "PEAPOD_CONFIG_PATH", "ZEPHYR_CONFIG_PATH", "ZEFIRE_CONFIG_PATH"),
		PublicURL:                     strings.TrimRight(envFirst("http://127.0.0.1:8095", "PEAPOD_PUBLIC_URL", "ZEPHYR_PUBLIC_URL", "ZEFIRE_PUBLIC_URL"), "/"),
		Password:                      envFirst("", "PEAPOD_PASSWORD", "ZEPHYR_PASSWORD", "ZEFIRE_PASSWORD"),
		SessionSecret:                 envFirst("", "PEAPOD_SESSION_SECRET", "ZEPHYR_SESSION_SECRET", "ZEFIRE_SESSION_SECRET"),
		DBDSN:                         envFirst("", "PEAPOD_DB_DSN", "ZEPHYR_DB_DSN", "ZEFIRE_DB_DSN"),
		BootstrapUsername:             envFirst("admin", "PEAPOD_BOOTSTRAP_USERNAME", "ZEPHYR_BOOTSTRAP_USERNAME", "ZEFIRE_BOOTSTRAP_USERNAME"),
		BootstrapPassword:             envFirst("", "PEAPOD_BOOTSTRAP_PASSWORD", "ZEPHYR_BOOTSTRAP_PASSWORD", "ZEFIRE_BOOTSTRAP_PASSWORD"),
		BootstrapDisplayName:          envFirst("管理员", "PEAPOD_BOOTSTRAP_DISPLAY_NAME", "ZEPHYR_BOOTSTRAP_DISPLAY_NAME", "ZEFIRE_BOOTSTRAP_DISPLAY_NAME"),
		BootstrapEmail:                envFirst("", "PEAPOD_BOOTSTRAP_EMAIL", "ZEPHYR_BOOTSTRAP_EMAIL", "ZEFIRE_BOOTSTRAP_EMAIL"),
		WoodpeckerServer:              strings.TrimRight(env("WOODPECKER_SERVER", "http://127.0.0.1:8000"), "/"),
		WoodpeckerPublicURL:           strings.TrimRight(env("WOODPECKER_PUBLIC_URL", env("WOODPECKER_SERVER", "http://127.0.0.1:8000")), "/"),
		WoodpeckerToken:               env("WOODPECKER_TOKEN", ""),
		BeszelBaseURL:                 strings.TrimRight(envFirst("http://beszel:8090", "PEAPOD_BESZEL_BASE_URL", "ZEPHYR_BESZEL_BASE_URL", "ZEFIRE_BESZEL_BASE_URL"), "/"),
		BeszelPublicURL:               strings.TrimRight(envFirst("http://127.0.0.1:8090", "PEAPOD_BESZEL_PUBLIC_URL", "ZEPHYR_BESZEL_PUBLIC_URL", "ZEFIRE_BESZEL_PUBLIC_URL"), "/"),
		BeszelEmail:                   envFirst("", "PEAPOD_BESZEL_EMAIL", "ZEPHYR_BESZEL_EMAIL", "ZEFIRE_BESZEL_EMAIL"),
		BeszelPassword:                envFirst("", "PEAPOD_BESZEL_PASSWORD", "ZEPHYR_BESZEL_PASSWORD", "ZEFIRE_BESZEL_PASSWORD"),
		DozzleBaseURL:                 strings.TrimRight(envFirst("http://dozzle:8080", "PEAPOD_DOZZLE_BASE_URL", "ZEPHYR_DOZZLE_BASE_URL", "ZEFIRE_DOZZLE_BASE_URL"), "/"),
		DozzlePublicURL:               strings.TrimRight(firstNonEmptyString(envFirst("", "PEAPOD_DOZZLE_PUBLIC_URL", "ZEPHYR_DOZZLE_PUBLIC_URL", "ZEFIRE_DOZZLE_PUBLIC_URL"), env("DOZZLE_PUBLIC_URL", "")), "/"),
		DozzleUsername:                envFirst("", "PEAPOD_DOZZLE_USERNAME", "ZEPHYR_DOZZLE_USERNAME", "ZEFIRE_DOZZLE_USERNAME", "DOZZLE_USERNAME"),
		DozzlePassword:                envFirst("", "PEAPOD_DOZZLE_PASSWORD", "ZEPHYR_DOZZLE_PASSWORD", "ZEFIRE_DOZZLE_PASSWORD", "DOZZLE_PASSWORD"),
		GrafanaPublicURL:              strings.TrimRight(envFirst("", "PEAPOD_GRAFANA_PUBLIC_URL", "ZEPHYR_GRAFANA_PUBLIC_URL", "ZEFIRE_GRAFANA_PUBLIC_URL"), "/"),
		LogStrategy:                   normalizeLogStrategy(envFirst("lightweight", "PEAPOD_LOG_STRATEGY", "ZEPHYR_LOG_STRATEGY", "ZEFIRE_LOG_STRATEGY")),
		DockerLogMaxSize:              fallbackText(env("DOCKER_LOG_MAX_SIZE", ""), "20m"),
		DockerLogMaxFile:              fallbackText(env("DOCKER_LOG_MAX_FILE", ""), "3"),
		AlertWebhookURL:               envFirst("", "PEAPOD_ALERT_WEBHOOK_URL", "ZEPHYR_ALERT_WEBHOOK_URL", "ZEFIRE_ALERT_WEBHOOK_URL"),
		ExternalLinksJSON:             envFirst("", "PEAPOD_LINKS_JSON", "ZEPHYR_LINKS_JSON", "ZEFIRE_LINKS_JSON"),
		MonitorHostsJSON:              envFirst("", "PEAPOD_MONITOR_HOSTS_JSON", "ZEPHYR_MONITOR_HOSTS_JSON", "ZEFIRE_MONITOR_HOSTS_JSON"),
		MonitorSSHKeyPath:             envFirst("/data/ssh/monitor_ed25519", "PEAPOD_MONITOR_SSH_KEY_PATH", "ZEPHYR_MONITOR_SSH_KEY_PATH", "ZEFIRE_MONITOR_SSH_KEY_PATH"),
		MonitorRefreshSeconds:         envIntFirst(20, "PEAPOD_MONITOR_REFRESH_SECONDS", "ZEPHYR_MONITOR_REFRESH_SECONDS", "ZEFIRE_MONITOR_REFRESH_SECONDS"),
		MonitorWarnDisk:               envIntFirst(80, "PEAPOD_MONITOR_WARN_DISK", "ZEPHYR_MONITOR_WARN_DISK", "ZEFIRE_MONITOR_WARN_DISK"),
		MonitorCritDisk:               envIntFirst(90, "PEAPOD_MONITOR_CRIT_DISK", "ZEPHYR_MONITOR_CRIT_DISK", "ZEFIRE_MONITOR_CRIT_DISK"),
		MonitorWarnMemory:             envIntFirst(80, "PEAPOD_MONITOR_WARN_MEMORY", "ZEPHYR_MONITOR_WARN_MEMORY", "ZEFIRE_MONITOR_WARN_MEMORY"),
		AuditPath:                     envFirst("/data/audit.jsonl", "PEAPOD_AUDIT_PATH", "ZEPHYR_AUDIT_PATH", "ZEFIRE_AUDIT_PATH"),
		TasksPath:                     envFirst("/data/tasks.json", "PEAPOD_TASKS_PATH", "ZEPHYR_TASKS_PATH", "ZEFIRE_TASKS_PATH"),
		FrontendDir:                   envFirst("frontend/dist", "PEAPOD_FRONTEND_DIR", "ZEPHYR_FRONTEND_DIR", "ZEFIRE_FRONTEND_DIR"),
	}
	return cfg
}

func loadRuntimeConfigFile(path string) (RuntimeConfigFile, error) {
	if strings.TrimSpace(path) == "" {
		return RuntimeConfigFile{}, os.ErrNotExist
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return RuntimeConfigFile{}, err
	}
	var cfg RuntimeConfigFile
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return RuntimeConfigFile{}, err
	}
	return cfg, nil
}

func saveRuntimeConfigFile(path string, cfg RuntimeConfigFile) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("PEAPOD_CONFIG_PATH is not configured")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0o600)
}

func applyRuntimeConfig(cfg *Config, runtime RuntimeConfigFile) {
	if value := cleanURL(runtime.PublicURL); value != "" {
		cfg.PublicURL = value
	}
	if value := cleanURL(runtime.WoodpeckerServer); value != "" {
		cfg.WoodpeckerServer = value
	}
	if value := cleanURL(runtime.WoodpeckerPublicURL); value != "" {
		cfg.WoodpeckerPublicURL = value
	}
	if value := strings.TrimSpace(runtime.WoodpeckerToken); value != "" {
		cfg.WoodpeckerToken = value
	}
	if value := cleanURL(runtime.BeszelBaseURL); value != "" {
		cfg.BeszelBaseURL = value
	}
	if value := cleanURL(runtime.BeszelPublicURL); value != "" {
		cfg.BeszelPublicURL = value
	}
	if value := strings.TrimSpace(runtime.BeszelEmail); value != "" {
		cfg.BeszelEmail = value
	}
	if value := strings.TrimSpace(runtime.BeszelPassword); value != "" {
		cfg.BeszelPassword = value
	}
	if value := cleanURL(runtime.DozzleBaseURL); value != "" {
		cfg.DozzleBaseURL = value
	}
	cfg.GrafanaPublicURL = cleanURL(runtime.GrafanaPublicURL)
	cfg.DozzlePublicURL = cleanURL(runtime.DozzlePublicURL)
	if value := strings.TrimSpace(runtime.DozzleUsername); value != "" {
		cfg.DozzleUsername = value
	}
	if value := strings.TrimSpace(runtime.DozzlePassword); value != "" {
		cfg.DozzlePassword = value
	}
	if value := normalizeLogStrategy(runtime.LogStrategy); value != "" {
		cfg.LogStrategy = value
	}
	if value := strings.TrimSpace(runtime.DockerLogMaxSize); value != "" {
		cfg.DockerLogMaxSize = value
	}
	if value := strings.TrimSpace(runtime.DockerLogMaxFile); value != "" {
		cfg.DockerLogMaxFile = value
	}
	if value := strings.TrimSpace(runtime.AlertWebhookURL); value != "" {
		cfg.AlertWebhookURL = value
	}
	if runtime.ExternalLinks != nil {
		cfg.ExternalLinksJSON = mustMarshalString(normalizeExternalLinks(runtime.ExternalLinks))
	}
	if runtime.MonitorHosts != nil {
		cfg.MonitorHostsJSON = mustMarshalString(normalizeMonitorHosts(runtime.MonitorHosts, cfg.MonitorSSHKeyPath))
	}
	if runtime.MonitorRefreshSeconds > 0 {
		cfg.MonitorRefreshSeconds = runtime.MonitorRefreshSeconds
	}
	if runtime.MonitorWarnDisk > 0 {
		cfg.MonitorWarnDisk = runtime.MonitorWarnDisk
	}
	if runtime.MonitorCritDisk > 0 {
		cfg.MonitorCritDisk = runtime.MonitorCritDisk
	}
	if runtime.MonitorWarnMemory > 0 {
		cfg.MonitorWarnMemory = runtime.MonitorWarnMemory
	}
}

func runtimeConfigFromInput(input RuntimeConfigInput, current Config, existing RuntimeConfigFile) RuntimeConfigFile {
	cfg := RuntimeConfigFile{
		PublicURL:             cleanURL(input.PublicURL),
		WoodpeckerServer:      cleanURL(input.WoodpeckerServer),
		WoodpeckerPublicURL:   cleanURL(input.WoodpeckerPublicURL),
		BeszelBaseURL:         cleanURL(input.BeszelBaseURL),
		BeszelPublicURL:       cleanURL(input.BeszelPublicURL),
		BeszelEmail:           strings.TrimSpace(input.BeszelEmail),
		DozzleBaseURL:         cleanURL(input.DozzleBaseURL),
		DozzlePublicURL:       cleanURL(input.DozzlePublicURL),
		DozzleUsername:        strings.TrimSpace(input.DozzleUsername),
		GrafanaPublicURL:      cleanURL(input.GrafanaPublicURL),
		LogStrategy:           normalizeLogStrategy(input.LogStrategy),
		DockerLogMaxSize:      strings.TrimSpace(input.DockerLogMaxSize),
		DockerLogMaxFile:      strings.TrimSpace(input.DockerLogMaxFile),
		ExternalLinks:         normalizeExternalLinks(input.ExternalLinks),
		MonitorHosts:          normalizeMonitorHosts(input.MonitorHosts, current.MonitorSSHKeyPath),
		MonitorRefreshSeconds: clampInt(input.MonitorRefreshSeconds, 5, 300, current.MonitorRefreshSeconds),
		MonitorWarnDisk:       clampInt(input.MonitorWarnDisk, 1, 100, current.MonitorWarnDisk),
		MonitorCritDisk:       clampInt(input.MonitorCritDisk, 1, 100, current.MonitorCritDisk),
		MonitorWarnMemory:     clampInt(input.MonitorWarnMemory, 1, 100, current.MonitorWarnMemory),
	}
	cfg.WoodpeckerToken = strings.TrimSpace(input.WoodpeckerToken)
	if cfg.WoodpeckerToken == "" {
		cfg.WoodpeckerToken = existing.WoodpeckerToken
	}
	cfg.BeszelPassword = strings.TrimSpace(input.BeszelPassword)
	if cfg.BeszelPassword == "" {
		cfg.BeszelPassword = existing.BeszelPassword
	}
	cfg.DozzlePassword = strings.TrimSpace(input.DozzlePassword)
	if cfg.DozzlePassword == "" {
		cfg.DozzlePassword = firstNonEmptyString(existing.DozzlePassword, current.DozzlePassword)
	}
	cfg.AlertWebhookURL = strings.TrimSpace(input.AlertWebhookURL)
	if cfg.AlertWebhookURL == "" {
		cfg.AlertWebhookURL = existing.AlertWebhookURL
	}
	if cfg.PublicURL == "" {
		cfg.PublicURL = current.PublicURL
	}
	if cfg.WoodpeckerServer == "" {
		cfg.WoodpeckerServer = current.WoodpeckerServer
	}
	if cfg.WoodpeckerPublicURL == "" {
		cfg.WoodpeckerPublicURL = current.WoodpeckerPublicURL
	}
	if cfg.BeszelBaseURL == "" {
		cfg.BeszelBaseURL = current.BeszelBaseURL
	}
	if cfg.BeszelPublicURL == "" {
		cfg.BeszelPublicURL = current.BeszelPublicURL
	}
	if cfg.DozzleBaseURL == "" {
		cfg.DozzleBaseURL = current.DozzleBaseURL
	}
	if cfg.DozzleUsername == "" {
		cfg.DozzleUsername = current.DozzleUsername
	}
	if cfg.LogStrategy == "" {
		cfg.LogStrategy = current.LogStrategy
	}
	if cfg.LogStrategy == "" {
		cfg.LogStrategy = "lightweight"
	}
	if cfg.DockerLogMaxSize == "" {
		cfg.DockerLogMaxSize = fallbackText(current.DockerLogMaxSize, "20m")
	}
	if cfg.DockerLogMaxFile == "" {
		cfg.DockerLogMaxFile = fallbackText(current.DockerLogMaxFile, "3")
	}
	return cfg
}

func cleanURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func clampInt(value int, minValue int, maxValue int, fallback int) int {
	if value <= 0 {
		value = fallback
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func normalizeLogStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "light", "dozzle", "lightweight":
		return "lightweight"
	case "full", "grafana", "loki", "observability":
		return "observability"
	case "external", "third-party", "third_party":
		return "external"
	default:
		return ""
	}
}

func mustMarshalString(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(payload)
}

func (c Config) validate() error {
	if c.DBDSN == "" && c.Password == "" {
		return errors.New("PEAPOD_PASSWORD is required")
	}
	if c.SessionSecret == "" {
		return errors.New("PEAPOD_SESSION_SECRET is required")
	}
	return nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envFirst(fallback string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return fallback
}

func envIntFirst(fallback int, keys ...string) int {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			if parsed, err := strconv.Atoi(value); err == nil {
				return parsed
			}
		}
	}
	return fallback
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

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if _, ok := a.currentUser(r); ok {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		a.serveFrontend(w, r, loginTemplate)
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, "/login?error=bad_request", http.StatusFound)
			return
		}
		username := r.FormValue("username")
		password := r.FormValue("password")
		user, err := a.authenticate(r.Context(), username, password)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			http.Redirect(w, r, "/login?error=invalid", http.StatusFound)
			return
		}
		a.setSession(w, r, user)
		http.Redirect(w, r, "/", http.StatusFound)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) apiLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, err := a.authenticate(r.Context(), req.Username, req.Password)
	if err != nil {
		time.Sleep(500 * time.Millisecond)
		http.Error(w, "用户名、邮箱或密码不正确", http.StatusUnauthorized)
		return
	}
	a.setSession(w, r, user)
	writeJSON(w, map[string]any{"user": user})
}

func (a *App) apiLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode})
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (a *App) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := a.currentUser(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), authUserContextKey{}, user)))
	}
}

func (a *App) authenticate(ctx context.Context, username, password string) (AuthUser, error) {
	if a.store != nil {
		return a.store.Authenticate(ctx, username, password)
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(a.cfg.Password)) != 1 {
		return AuthUser{}, errors.New("invalid password")
	}
	return AuthUser{Username: "legacy-admin", DisplayName: "管理员", Role: "admin", Active: true, Legacy: true}, nil
}

func (a *App) setSession(w http.ResponseWriter, r *http.Request, user AuthUser) {
	payload := sessionPayload{
		Expires:     time.Now().Add(24 * time.Hour).Unix(),
		UserID:      user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Email:       user.Email,
		Role:        user.Role,
		Legacy:      user.Legacy,
	}
	token := a.signSession(payload)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", MaxAge: 86400, HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode})
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (a *App) currentUser(r *http.Request) (AuthUser, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return AuthUser{}, false
	}
	payload, ok := a.parseSession(cookie.Value)
	if !ok {
		return AuthUser{}, false
	}
	if payload.Expires <= time.Now().Unix() {
		return AuthUser{}, false
	}
	if a.store == nil || payload.Legacy {
		return AuthUser{ID: payload.UserID, Username: payload.Username, DisplayName: payload.DisplayName, Email: payload.Email, Role: payload.Role, Active: true, Legacy: payload.Legacy}, true
	}
	user, err := a.store.GetUser(r.Context(), payload.UserID)
	if err != nil || !user.Active {
		return AuthUser{}, false
	}
	return user, true
}

func (a *App) signSession(payload sessionPayload) string {
	body, _ := json.Marshal(payload)
	encoded := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionSecret))
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *App) parseSession(token string) (sessionPayload, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return sessionPayload{}, false
	}
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionSecret))
	_, _ = mac.Write([]byte(parts[0]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[1])) {
		return sessionPayload{}, false
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return sessionPayload{}, false
	}
	var payload sessionPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return sessionPayload{}, false
	}
	return payload, true
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
	record := AuditRecord{
		Time:      time.Now().Format(time.RFC3339),
		UserID:    user.ID,
		Username:  user.Username,
		RemoteIP:  remoteIP(r),
		TaskID:    task.ID,
		TaskTitle: task.Title,
		RepoID:    task.RepoID,
		Branch:    branch,
		Variables: variables,
		Status:    "ok",
	}
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
	record := AuditRecord{
		Time:      time.Now().Format(time.RFC3339),
		UserID:    user.ID,
		Username:  user.Username,
		RemoteIP:  remoteIP(r),
		TaskID:    "custom-run",
		TaskTitle: "自定义部署",
		RepoID:    req.RepoID,
		Branch:    branch,
		Variables: variables,
		Status:    "ok",
	}
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

func (a *App) setupConfig(w http.ResponseWriter, r *http.Request) {
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, a.setupConfigResponse(time.Now()))
	case http.MethodPost:
		var input RuntimeConfigInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		existing, err := loadRuntimeConfigFile(a.cfg.ConfigPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		runtimeCfg := runtimeConfigFromInput(input, a.cfg, existing)
		if err := validateRuntimeConfig(runtimeCfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := saveRuntimeConfigFile(a.cfg.ConfigPath, runtimeCfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		next := a.cfg
		applyRuntimeConfig(&next, runtimeCfg)
		a.cfg = next
		a.monitor = NewMonitoringService(next, a.client)
		_ = a.writeAudit(AuditRecord{
			Time:      time.Now().Format(time.RFC3339),
			UserID:    user.ID,
			Username:  user.Username,
			RemoteIP:  remoteIP(r),
			TaskID:    "setup-config",
			TaskTitle: "保存接入配置",
			Variables: map[string]string{
				"PEAPOD_PUBLIC_URL":         next.PublicURL,
				"WOODPECKER_PUBLIC_URL":     next.WoodpeckerPublicURL,
				"PEAPOD_BESZEL_PUBLIC_URL":  next.BeszelPublicURL,
				"PEAPOD_DOZZLE_BASE_URL":    next.DozzleBaseURL,
				"PEAPOD_DOZZLE_PUBLIC_URL":  next.DozzlePublicURL,
				"PEAPOD_DOZZLE_USERNAME":    next.DozzleUsername,
				"PEAPOD_GRAFANA_PUBLIC_URL": next.GrafanaPublicURL,
				"PEAPOD_LOG_STRATEGY":       next.LogStrategy,
				"DOCKER_LOG_MAX_SIZE":       next.DockerLogMaxSize,
				"DOCKER_LOG_MAX_FILE":       next.DockerLogMaxFile,
			},
			Status: "ok",
		})
		writeJSON(w, a.setupConfigResponse(time.Now()))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) doctorRun(w http.ResponseWriter, r *http.Request) {
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hosts := parseMonitorHosts(a.cfg)
	verification := deploymentVerificationSummary(a.configuredTasks())
	logStrategy := a.logStrategyStatus()
	checklist := a.setupChecklist(hosts, verification, logStrategy)
	doctor := a.doctorSummary(time.Now(), checklist)
	_ = a.writeAudit(AuditRecord{
		Time:      time.Now().Format(time.RFC3339),
		UserID:    user.ID,
		Username:  user.Username,
		RemoteIP:  remoteIP(r),
		TaskID:    "doctor-run",
		TaskTitle: "运行 Pedpod 体检",
		Variables: map[string]string{"readiness": doctor.Readiness},
		Status:    "ok",
	})
	writeJSON(w, doctor)
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
	cfg, err := a.loadCustomTaskConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
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
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	_ = a.writeAudit(AuditRecord{
		Time:      time.Now().Format(time.RFC3339),
		UserID:    user.ID,
		Username:  user.Username,
		RemoteIP:  remoteIP(r),
		TaskID:    "template-apply",
		TaskTitle: "套用任务模板",
		RepoID:    task.RepoID,
		Branch:    task.Branch,
		Variables: map[string]string{
			"template": template.ID,
			"task":     task.ID,
			"project":  variableValue(task.Variables, "PEAPOD_PROJECT_ID"),
		},
		Status: "ok",
	})
	writeJSON(w, TemplateApplyResponse{Task: task, Config: cfg})
}

func (a *App) woodpeckerRepos(w http.ResponseWriter, r *http.Request) {
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows, err := a.listWoodpeckerRepos()
	errors := []string{}
	if err != nil {
		errors = append(errors, err.Error())
	}
	writeJSON(w, WoodpeckerReposResponse{
		Repos:      rows,
		Configured: a.configuredRepos(),
		Errors:     errors,
	})
}

func (a *App) woodpeckerRepoAction(w http.ResponseWriter, r *http.Request) {
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/woodpecker/repos/"), "/")
	switch action {
	case "lookup":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req WoodpeckerRepoLookupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		repo, err := a.lookupWoodpeckerRepo(req.Owner, req.Name)
		if err != nil {
			writeError(w, http.StatusNotFound, friendlyRepoLookupError(req, err))
			return
		}
		writeJSON(w, map[string]any{"repo": repo})
	case "activate":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req WoodpeckerRepoActivateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		repo, err := a.activateWoodpeckerRepo(req.ForgeRemoteID)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		_ = a.writeAudit(AuditRecord{
			Time:      time.Now().Format(time.RFC3339),
			UserID:    user.ID,
			Username:  user.Username,
			RemoteIP:  remoteIP(r),
			TaskID:    "woodpecker-repo-activate",
			TaskTitle: "启用 Woodpecker 仓库",
			RepoID:    repo.ID,
			Variables: map[string]string{"repo": repo.FullName, "forge_remote_id": repo.ForgeRemoteID},
			Status:    "ok",
		})
		writeJSON(w, map[string]any{"repo": repo})
	case "save":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req WoodpeckerRepoSaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := a.saveConfiguredRepo(req.RepoID, req.RepoName); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = a.writeAudit(AuditRecord{
			Time:      time.Now().Format(time.RFC3339),
			UserID:    user.ID,
			Username:  user.Username,
			RemoteIP:  remoteIP(r),
			TaskID:    "woodpecker-repo-save",
			TaskTitle: "保存 Pedpod 仓库映射",
			RepoID:    req.RepoID,
			Variables: map[string]string{"repo": strings.TrimSpace(req.RepoName)},
			Status:    "ok",
		})
		writeJSON(w, WoodpeckerReposResponse{Repos: []WoodpeckerRepo{}, Configured: a.configuredRepos()})
	default:
		http.NotFound(w, r)
	}
}

func (a *App) customTasks(w http.ResponseWriter, r *http.Request) {
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := a.loadCustomTaskConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, cfg)
	case http.MethodPost:
		var task Task
		if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := normalizeTaskConfig(&task); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if isBuiltinTaskID(task.ID) {
			task.Custom = false
			task.Builtin = true
			task.Overridden = true
		} else {
			task.Custom = true
			task.Builtin = false
			task.Overridden = false
		}
		cfg, err := a.loadCustomTaskConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
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
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"task": task})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) customTaskByID(w http.ResponseWriter, r *http.Request) {
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/config/tasks/"), "/")
	if id == "" {
		http.Error(w, "task id is required", http.StatusBadRequest)
		return
	}
	cfg, err := a.loadCustomTaskConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	next := cfg.Tasks[:0]
	for _, task := range cfg.Tasks {
		if task.ID != id {
			next = append(next, task)
		}
	}
	cfg.Tasks = next
	if err := a.saveCustomTaskConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) userByID(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		http.Error(w, "database auth is not enabled", http.StatusNotFound)
		return
	}
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/users/"), "/")
	if strings.HasSuffix(path, "/password") {
		idText := strings.TrimSuffix(path, "/password")
		id, err := parseID(idText)
		if err != nil {
			http.Error(w, "bad user id", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var input PasswordInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := a.store.SetPassword(r.Context(), id, input.NewPassword); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	id, err := parseID(path)
	if err != nil {
		http.Error(w, "bad user id", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input UserInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	updated, err := a.store.UpdateUser(r.Context(), id, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"user": updated})
}

func (a *App) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		http.Error(w, "database auth is not enabled", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := authUserFromRequest(r)
	var input PasswordInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := a.store.VerifyPassword(r.Context(), user.ID, input.OldPassword); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.store.SetPassword(r.Context(), user.ID, input.NewPassword); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) me(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		http.Error(w, "database auth is not enabled", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := authUserFromRequest(r)
	var input UserInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	input.Role = user.Role
	active := true
	input.Active = &active
	updated, err := a.store.UpdateUser(r.Context(), user.ID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.setSession(w, r, updated)
	writeJSON(w, map[string]any{"user": updated})
}

func (a *App) pipelineAction(w http.ResponseWriter, r *http.Request) {
	user := authUserFromRequest(r)
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/pipelines/"), "/")
	if r.Method == http.MethodGet && strings.HasSuffix(path, "/summary") {
		parts := strings.Split(strings.TrimSuffix(path, "/summary"), "/")
		if len(parts) != 2 {
			http.Error(w, "bad pipeline path", http.StatusBadRequest)
			return
		}
		repoID, err := strconv.Atoi(parts[0])
		if err != nil || repoID <= 0 {
			http.Error(w, "bad repo id", http.StatusBadRequest)
			return
		}
		number, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || number <= 0 {
			http.Error(w, "bad pipeline number", http.StatusBadRequest)
			return
		}
		summary, err := a.pipelineSummary(repoID, number)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, summary)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasSuffix(path, "/cancel") {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.TrimSuffix(path, "/cancel"), "/")
	if len(parts) != 2 {
		http.Error(w, "bad pipeline path", http.StatusBadRequest)
		return
	}
	repoID, err := strconv.Atoi(parts[0])
	if err != nil || repoID <= 0 {
		http.Error(w, "bad repo id", http.StatusBadRequest)
		return
	}
	number, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || number <= 0 {
		http.Error(w, "bad pipeline number", http.StatusBadRequest)
		return
	}
	record := AuditRecord{
		Time:      time.Now().Format(time.RFC3339),
		UserID:    user.ID,
		Username:  user.Username,
		RemoteIP:  remoteIP(r),
		TaskID:    "cancel-pipeline",
		TaskTitle: "取消流水线",
		RepoID:    repoID,
		Pipeline:  number,
		Variables: map[string]string{},
		Status:    "ok",
	}
	if err := a.cancelPipeline(repoID, number); err != nil {
		record.Status = "error"
		record.Error = err.Error()
		_ = a.writeAudit(record)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = a.writeAudit(record)
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) listWoodpeckerRepos() ([]WoodpeckerRepo, error) {
	if strings.TrimSpace(a.cfg.WoodpeckerToken) == "" {
		return nil, errors.New("Woodpecker token 未配置")
	}
	var rows []WoodpeckerRepo
	if err := a.woodpeckerJSON(http.MethodGet, "/api/user/repos?perPage=100", nil, &rows); err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].FullName) < strings.ToLower(rows[j].FullName)
	})
	return rows, nil
}

func (a *App) lookupWoodpeckerRepo(owner string, name string) (WoodpeckerRepo, error) {
	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	if owner == "" || name == "" {
		return WoodpeckerRepo{}, errors.New("请输入 owner 和仓库名")
	}
	var repo WoodpeckerRepo
	path := fmt.Sprintf("/api/repos/lookup/%s/%s", url.PathEscape(owner), url.PathEscape(name))
	if err := a.woodpeckerJSON(http.MethodGet, path, nil, &repo); err != nil {
		return WoodpeckerRepo{}, err
	}
	return repo, nil
}

func (a *App) activateWoodpeckerRepo(forgeRemoteID string) (WoodpeckerRepo, error) {
	forgeRemoteID = strings.TrimSpace(forgeRemoteID)
	if forgeRemoteID == "" {
		return WoodpeckerRepo{}, errors.New("forge_remote_id is required")
	}
	var repo WoodpeckerRepo
	path := "/api/repos?forge_remote_id=" + url.QueryEscape(forgeRemoteID)
	if err := a.woodpeckerJSON(http.MethodPost, path, nil, &repo); err != nil {
		return WoodpeckerRepo{}, err
	}
	return repo, nil
}

func (a *App) woodpeckerJSON(method string, path string, body any, out any) error {
	endpoint := strings.TrimRight(a.cfg.WoodpeckerServer, "/") + path
	var reader io.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+a.cfg.WoodpeckerToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return WoodpeckerRequestError{Operation: method + " " + path, StatusCode: response.StatusCode, Body: strings.TrimSpace(string(payload))}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return err
	}
	return nil
}

func friendlyRepoLookupError(req WoodpeckerRepoLookupRequest, err error) string {
	name := strings.Trim(strings.TrimSpace(req.Owner)+"/"+strings.TrimSpace(req.Name), "/")
	if name == "" {
		name = "这个仓库"
	}
	return fmt.Sprintf("Woodpecker 当前授权看不到 %s。请先确认 GitHub 仓库存在，并在 Woodpecker/GitHub OAuth 授权里允许访问该仓库。原始错误：%v", name, err)
}

func (a *App) createPipeline(repoID int, branch string, variables map[string]string) (Pipeline, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	if err := a.probeWoodpeckerRepo(repoID, branch); err != nil {
		return Pipeline{}, err
	}
	body, _ := json.Marshal(map[string]any{
		"branch":    branch,
		"variables": variables,
	})
	endpoint := fmt.Sprintf("%s/api/repos/%d/pipelines", a.cfg.WoodpeckerServer, repoID)
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Pipeline{}, err
	}
	request.Header.Set("Authorization", "Bearer "+a.cfg.WoodpeckerToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return Pipeline{}, err
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Pipeline{}, WoodpeckerRequestError{
			Operation:  "触发流水线",
			RepoID:     repoID,
			Branch:     branch,
			StatusCode: response.StatusCode,
			Body:       strings.TrimSpace(string(payload)),
		}
	}
	var pipeline Pipeline
	if err := json.Unmarshal(payload, &pipeline); err != nil {
		return Pipeline{}, err
	}
	return pipeline, nil
}

func (a *App) probeWoodpeckerRepo(repoID int, branch string) error {
	endpoint := fmt.Sprintf("%s/api/repos/%d", a.cfg.WoodpeckerServer, repoID)
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+a.cfg.WoodpeckerToken)
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return WoodpeckerRequestError{
			Operation:  "读取仓库",
			RepoID:     repoID,
			Branch:     branch,
			StatusCode: response.StatusCode,
			Body:       strings.TrimSpace(string(payload)),
		}
	}
	return nil
}

func (a *App) cancelPipeline(repoID int, number int64) error {
	endpoint := fmt.Sprintf("%s/api/repos/%d/pipelines/%d/cancel", a.cfg.WoodpeckerServer, repoID, number)
	request, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+a.cfg.WoodpeckerToken)
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("woodpecker %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}

func (a *App) listPipelines(repoID int, limit int) ([]Pipeline, error) {
	endpoint := fmt.Sprintf("%s/api/repos/%d/pipelines?perPage=%d", a.cfg.WoodpeckerServer, repoID, limit)
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+a.cfg.WoodpeckerToken)
	response, err := a.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("woodpecker %d", response.StatusCode)
	}
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	rows, err := decodePipelineRows(payload)
	if err != nil {
		return nil, err
	}
	rows = a.hydratePipelineDetails(repoID, rows, 6)
	for index := range rows {
		rows[index].Variables = sanitizeVariables(rows[index].Variables)
	}
	return rows, nil
}

func (a *App) hydratePipelineDetails(repoID int, rows []Pipeline, limit int) []Pipeline {
	if limit <= 0 || len(rows) == 0 {
		return rows
	}
	if limit > len(rows) {
		limit = len(rows)
	}
	for index := 0; index < limit; index++ {
		if rows[index].Number <= 0 {
			continue
		}
		detail, err := a.pipelineDetail(repoID, rows[index].Number)
		if err != nil {
			continue
		}
		rows[index] = mergePipeline(rows[index], detail)
	}
	return rows
}

func (a *App) pipelineDetail(repoID int, number int64) (Pipeline, error) {
	payload, err := a.pipelineDetailPayload(repoID, number)
	if err != nil {
		return Pipeline{}, err
	}
	return decodePipelinePayload(payload)
}

func (a *App) pipelineDetailPayload(repoID int, number int64) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/api/repos/%d/pipelines/%d", a.cfg.WoodpeckerServer, repoID, number)
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+a.cfg.WoodpeckerToken)
	response, err := a.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("woodpecker %d", response.StatusCode)
	}
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	return payload, nil
}

func (a *App) pipelineSummary(repoID int, number int64) (PipelineSummary, error) {
	payload, err := a.pipelineDetailPayload(repoID, number)
	if err != nil {
		return PipelineSummary{}, err
	}
	pipeline, err := decodePipelinePayload(payload)
	if err != nil {
		return PipelineSummary{}, err
	}
	pipeline.Variables = sanitizeVariables(pipeline.Variables)
	steps := decodePipelineSteps(payload)
	tail, tailErr := a.pipelineFailureLogTail(repoID, number, steps)
	failure := pipelineFailureSummary(pipeline, steps)
	if tailErr != nil && failure == "" {
		failure = tailErr.Error()
	}
	return PipelineSummary{
		Pipeline:       pipeline,
		Steps:          steps,
		FailureSummary: failure,
		LogTail:        tail,
		WoodpeckerURL:  a.pipelineURL(repoID, number),
	}, nil
}

func (a *App) listBranches(repoID int) ([]string, error) {
	endpoint := fmt.Sprintf("%s/api/repos/%d/branches", a.cfg.WoodpeckerServer, repoID)
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+a.cfg.WoodpeckerToken)
	response, err := a.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("woodpecker %d", response.StatusCode)
	}
	var rows []string
	if err := json.NewDecoder(response.Body).Decode(&rows); err != nil {
		return nil, err
	}
	cleaned := make([]string, 0, len(rows))
	seen := map[string]bool{}
	for _, row := range rows {
		branch := strings.TrimSpace(row)
		if branch == "" || seen[branch] {
			continue
		}
		seen[branch] = true
		cleaned = append(cleaned, branch)
	}
	sort.Strings(cleaned)
	return cleaned, nil
}

func decodePipelineRows(payload []byte) ([]Pipeline, error) {
	var rawRows []json.RawMessage
	if err := json.Unmarshal(payload, &rawRows); err != nil {
		return nil, err
	}
	rows := make([]Pipeline, 0, len(rawRows))
	for _, raw := range rawRows {
		row, err := decodePipelinePayload(raw)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func decodePipelinePayload(payload []byte) (Pipeline, error) {
	var pipeline Pipeline
	if err := json.Unmarshal(payload, &pipeline); err != nil {
		return Pipeline{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return pipeline, nil
	}
	variables := cloneMap(pipeline.Variables)
	for _, key := range []string{"variables", "environment", "env", "params", "parameters"} {
		for envKey, envValue := range stringMapFromAny(raw[key]) {
			if _, exists := variables[envKey]; !exists {
				variables[envKey] = envValue
			}
		}
	}
	if deployTo := stringFromAny(raw["deploy_to"]); deployTo != "" {
		pipeline.DeployTo = deployTo
		if variableValue(variables, "DEPLOY_TARGET") == "" {
			variables["DEPLOY_TARGET"] = deployTo
		}
	}
	if len(variables) > 0 {
		pipeline.Variables = variables
	}
	return pipeline, nil
}

func decodePipelineSteps(payload []byte) []PipelineStep {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil
	}
	steps := []PipelineStep{}
	for _, workflowValue := range anySlice(raw["workflows"]) {
		workflow, ok := workflowValue.(map[string]any)
		if !ok {
			continue
		}
		parent := pipelineStepFromMap(workflow)
		if parent.ID > 0 || parent.Name != "" {
			steps = append(steps, parent)
		}
		for _, childValue := range anySlice(workflow["children"]) {
			child, ok := childValue.(map[string]any)
			if !ok {
				continue
			}
			step := pipelineStepFromMap(child)
			if step.ID > 0 || step.Name != "" {
				steps = append(steps, step)
			}
		}
	}
	return steps
}

func pipelineStepFromMap(raw map[string]any) PipelineStep {
	return PipelineStep{
		ID:       int64FromAny(firstRaw(raw, "id", "step_id")),
		PID:      int64FromAny(raw["pid"]),
		PPID:     int64FromAny(raw["ppid"]),
		Name:     firstNonEmptyString(stringFromAny(raw["name"]), stringFromAny(raw["title"])),
		State:    firstNonEmptyString(stringFromAny(raw["state"]), stringFromAny(raw["status"])),
		Error:    stringFromAny(raw["error"]),
		ExitCode: int(int64FromAny(firstRaw(raw, "exit_code", "exitCode"))),
		Started:  int64FromAny(raw["started"]),
		Finished: int64FromAny(raw["finished"]),
		Type:     stringFromAny(raw["type"]),
	}
}

func firstRaw(raw map[string]any, names ...string) any {
	for _, name := range names {
		if value, ok := raw[name]; ok {
			return value
		}
	}
	return nil
}

func int64FromAny(value any) int64 {
	switch v := value.(type) {
	case json.Number:
		parsed, _ := strconv.ParseInt(v.String(), 10, 64)
		return parsed
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return parsed
	default:
		return 0
	}
}

func anySlice(value any) []any {
	switch v := value.(type) {
	case []any:
		return v
	default:
		return nil
	}
}

func pipelineFailureSummary(pipeline Pipeline, steps []PipelineStep) string {
	for _, step := range steps {
		if step.Error != "" {
			return fmt.Sprintf("%s：%s", fallbackText(step.Name, "step"), step.Error)
		}
	}
	for _, step := range steps {
		if step.ExitCode != 0 {
			return fmt.Sprintf("%s：exit code %d", fallbackText(step.Name, "step"), step.ExitCode)
		}
	}
	for _, step := range steps {
		state := strings.ToLower(step.State)
		if state == "failure" || state == "error" {
			return fmt.Sprintf("%s：%s", fallbackText(step.Name, "step"), step.State)
		}
	}
	if pipeline.Status == "failure" || pipeline.Status == "error" {
		return fallbackText(pipeline.Message, "流水线失败，但 Woodpecker 没有返回具体 step 错误")
	}
	return ""
}

func (a *App) pipelineFailureLogTail(repoID int, number int64, steps []PipelineStep) ([]string, error) {
	targets := pipelineLogTargetSteps(steps)
	var lastErr error
	for _, step := range targets {
		if step.ID <= 0 {
			continue
		}
		lines, err := a.pipelineStepLogs(repoID, number, step.ID, 100)
		if err != nil {
			lastErr = err
			continue
		}
		if len(lines) > 0 {
			return lines, nil
		}
	}
	return []string{}, lastErr
}

func pipelineLogTargetSteps(steps []PipelineStep) []PipelineStep {
	failed := []PipelineStep{}
	commands := []PipelineStep{}
	for _, step := range steps {
		state := strings.ToLower(step.State)
		if state == "failure" || state == "error" || step.ExitCode != 0 || step.Error != "" {
			failed = append(failed, step)
		}
		if strings.EqualFold(step.Type, "commands") || step.PPID > 0 {
			commands = append(commands, step)
		}
	}
	if len(failed) > 0 {
		return failed
	}
	if len(commands) > 0 {
		return commands
	}
	return steps
}

type woodpeckerLogEntry struct {
	Time int64   `json:"time"`
	Line int64   `json:"line"`
	Data *string `json:"data"`
	Type int     `json:"type"`
}

func (a *App) pipelineStepLogs(repoID int, number int64, stepID int64, limit int) ([]string, error) {
	endpoint := fmt.Sprintf("%s/api/repos/%d/logs/%d/%d", a.cfg.WoodpeckerServer, repoID, number, stepID)
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+a.cfg.WoodpeckerToken)
	response, err := a.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("logs HTTP %d", response.StatusCode)
	}
	lines, err := decodeWoodpeckerLogLines(payload)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || len(lines) <= limit {
		return lines, nil
	}
	return lines[len(lines)-limit:], nil
}

func decodeWoodpeckerLogLines(payload []byte) ([]string, error) {
	var entries []woodpeckerLogEntry
	if err := json.Unmarshal(payload, &entries); err != nil {
		return nil, err
	}
	lines := []string{}
	for _, entry := range entries {
		if entry.Data == nil {
			continue
		}
		text := strings.TrimRight(decodeWoodpeckerLogData(*entry.Data), "\r\n")
		if text == "" {
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimRight(line, "\r")
			if line == "" {
				continue
			}
			lines = append(lines, maskSensitiveLogLine(line))
		}
	}
	return lines, nil
}

func decodeWoodpeckerLogData(value string) string {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return value
	}
	return string(decoded)
}

func maskSensitiveLogLine(line string) string {
	upper := strings.ToUpper(line)
	for _, marker := range []string{"PASSWORD", "TOKEN", "SECRET", "PRIVATE_KEY", "ACCESS_KEY", "CREDENTIAL"} {
		if strings.Contains(upper, marker+"=") || strings.Contains(upper, marker+":") || strings.Contains(upper, "BEGIN "+marker) {
			return "[已隐藏敏感日志行]"
		}
	}
	return line
}

func mergePipeline(base Pipeline, detail Pipeline) Pipeline {
	if detail.Number > 0 {
		base.Number = detail.Number
	}
	base.Status = fallbackText(detail.Status, base.Status)
	base.Event = fallbackText(detail.Event, base.Event)
	base.Commit = fallbackText(detail.Commit, base.Commit)
	base.Branch = fallbackText(detail.Branch, base.Branch)
	base.Author = fallbackText(detail.Author, base.Author)
	base.Sender = fallbackText(detail.Sender, base.Sender)
	base.DeployTo = fallbackText(detail.DeployTo, base.DeployTo)
	base.Message = fallbackText(detail.Message, base.Message)
	if detail.Created > 0 {
		base.Created = detail.Created
	}
	if detail.Started > 0 {
		base.Started = detail.Started
	}
	if detail.Finished > 0 {
		base.Finished = detail.Finished
	}
	if detail.Updated > 0 {
		base.Updated = detail.Updated
	}
	if len(detail.Variables) > 0 {
		variables := cloneMap(base.Variables)
		for key, value := range detail.Variables {
			variables[key] = value
		}
		base.Variables = variables
	}
	return base
}

func stringMapFromAny(value any) map[string]string {
	out := map[string]string{}
	switch raw := value.(type) {
	case map[string]any:
		for key, value := range raw {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if text := stringFromAny(value); text != "" {
				out[key] = text
			}
		}
	case []any:
		for _, item := range raw {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			key := firstNonEmptyString(
				stringFromAny(row["name"]),
				stringFromAny(row["key"]),
				stringFromAny(row["variable"]),
			)
			value := firstNonEmptyString(
				stringFromAny(row["value"]),
				stringFromAny(row["val"]),
			)
			if key != "" && value != "" {
				out[key] = value
			}
		}
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return strings.TrimSpace(v.String())
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func (a *App) pipelineURL(repoID int, number int64) string {
	base := strings.TrimRight(a.cfg.WoodpeckerPublicURL, "/")
	return fmt.Sprintf("%s/repos/%d/pipeline/%d", base, repoID, number)
}

func (a *App) audit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 80
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = min(parsed, 200)
		}
	}
	records, err := a.listAudit(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for index := range records {
		records[index] = sanitizeAuditRecord(records[index])
	}
	writeJSON(w, AuditListResponse{Records: records})
}

func (a *App) diskDiagnosis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	resp := DiskDiagnosisResponse{CheckedAt: time.Now().Format(time.RFC3339)}
	dfCtx, dfCancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer dfCancel()
	if output, err := exec.CommandContext(dfCtx, "df", "-h", "--output=target,size,used,pcent", "/").CombinedOutput(); err == nil {
		for i, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if i == 0 || strings.TrimSpace(line) == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				percent := 0
				fmt.Sscanf(strings.TrimSuffix(fields[3], "%"), "%d", &percent)
				resp.Filesystems = append(resp.Filesystems, DiskFilesystemInfo{Mount: fields[0], Total: fields[1], Used: fields[2], Percent: percent})
			}
		}
	}
	dockerCtx, dockerCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer dockerCancel()
	if output, err := exec.CommandContext(dockerCtx, "docker", "system", "df", "--format", "{{.Type}}\t{{.TotalCount}}\t{{.Size}}\t{{.Reclaimable}}").CombinedOutput(); err == nil {
		resp.DockerOK = true
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			switch fields[0] {
			case "Images":
				resp.Docker.ImagesTotal, _ = strconv.Atoi(fields[1])
				resp.Docker.ImagesSize = fields[2]
				resp.Docker.ImagesReclaimable = fields[3]
			case "BuildCache", "Build Cache":
				resp.Docker.BuildCacheSize = fields[2]
				resp.Docker.BuildReclaimable = fields[3]
			case "LocalVolumes", "Local Volumes":
				resp.Docker.VolumesSize = fields[2]
				resp.Docker.VolumesReclaimable = fields[3]
			}
		}
	}
	duCtx, duCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer duCancel()
	if output, err := exec.CommandContext(duCtx, "du", "-sh", "/var/lib/docker", "/opt", "/tmp", "/var/log", "/root").CombinedOutput(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				resp.TopDirs = append(resp.TopDirs, DiskUsageItem{Size: fields[0], Path: fields[1], Bytes: uint64(parseHumanBytes(fields[0]))})
			}
		}
	}
	writeJSON(w, resp)
}

func (a *App) diskCleanupPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	resp := DiskCleanupPreviewResponse{Recommendation: "safe"}
	dockerCtx, dockerCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer dockerCancel()
	if output, err := exec.CommandContext(dockerCtx, "docker", "system", "df", "--format", "{{.Type}}\t{{.TotalCount}}\t{{.Size}}\t{{.Reclaimable}}").CombinedOutput(); err == nil {
		resp.DockerOK = true
		var buildReclaimable, imagesReclaimable, volumesReclaimable string
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			switch fields[0] {
			case "Images":
				imagesReclaimable = fields[3]
			case "BuildCache", "Build Cache":
				buildReclaimable = fields[3]
			case "LocalVolumes", "Local Volumes":
				volumesReclaimable = fields[3]
			}
		}
		resp.Levels = []DiskCleanupLevel{
			{Level: "safe", Description: "Docker build cache 清理", Reclaimable: buildReclaimable, Command: "docker builder prune --all --force", Risk: "低，仅清理构建缓存，不影响运行中容器"},
			{Level: "standard", Description: "悬空镜像 + 已停止容器 + build cache", Reclaimable: combineReclaimable(buildReclaimable, imagesReclaimable), Command: "docker system prune -f", Risk: "中，会删除未使用的镜像，可能影响回滚速度"},
			{Level: "deep", Description: "未使用卷 + 未使用镜像 + build cache", Reclaimable: combineReclaimable(buildReclaimable, imagesReclaimable, volumesReclaimable), Command: "docker system prune -af --volumes", Risk: "高，会删除所有未使用的卷和数据"},
		}
	}
	writeJSON(w, resp)
}

func (a *App) diskCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := authUserFromRequest(r)
	if user.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req DiskCleanupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Confirm != "CLEAN" {
		writeError(w, http.StatusBadRequest, "请输入确认词 CLEAN")
		return
	}
	var cmd *exec.Cmd
	level := strings.ToLower(strings.TrimSpace(req.Level))
	switch level {
	case "safe":
		cmd = exec.Command("docker", "builder", "prune", "--all", "--force")
	case "standard":
		cmd = exec.Command("docker", "system", "prune", "-f")
	case "deep":
		cmd = exec.Command("docker", "system", "prune", "-af", "--volumes")
	default:
		writeError(w, http.StatusBadRequest, "无效的清理级别，可选：safe / standard / deep")
		return
	}
	output, err := cmd.CombinedOutput()
	details := strings.TrimSpace(string(output))
	if len(details) > 500 {
		details = details[:500] + "..."
	}
	reclaimed := ""
	if err == nil {
		reclaimed = extractReclaimedSize(string(output))
	}
	_ = a.writeAudit(AuditRecord{
		Time: time.Now().Format(time.RFC3339), UserID: user.ID, Username: user.Username, RemoteIP: remoteIP(r),
		TaskID: "disk-cleanup-" + level, TaskTitle: "磁盘清理",
		Variables: map[string]string{"level": level, "reclaimed": reclaimed},
		Status:    ternaryText(err == nil, "ok", "error"),
		Error:     ternaryText(err != nil, err.Error(), ""),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "清理失败："+err.Error())
		return
	}
	writeJSON(w, DiskCleanupResponse{OK: true, Level: level, Reclaimed: reclaimed, Details: details})
}

func combineReclaimable(values ...string) string {
	total := float64(0)
	for _, v := range values {
		total += parseHumanBytes(v)
	}
	if total <= 0 {
		return "0B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	unitIndex := 0
	size := total
	for size >= 1000 && unitIndex < len(units)-1 {
		size /= 1000
		unitIndex++
	}
	return fmt.Sprintf("%.1f%s", size, units[unitIndex])
}

func extractReclaimedSize(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Reclaimed") || strings.Contains(line, "reclaimed") || strings.Contains(line, "Total reclaimed") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func (a *App) writeAudit(record AuditRecord) error {
	if a.store != nil {
		return a.store.WriteAudit(context.Background(), record)
	}
	if a.cfg.AuditPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(a.cfg.AuditPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(a.cfg.AuditPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line, _ := json.Marshal(record)
	_, err = f.Write(append(line, '\n'))
	return err
}

func (a *App) listAudit(limit int) ([]AuditRecord, error) {
	if limit <= 0 {
		limit = 80
	}
	if a.store != nil {
		return a.store.ListAudit(context.Background(), limit)
	}
	if a.cfg.AuditPath == "" {
		return []AuditRecord{}, nil
	}
	payload, err := os.ReadFile(a.cfg.AuditPath)
	if errors.Is(err, os.ErrNotExist) {
		return []AuditRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	records := []AuditRecord{}
	for _, line := range strings.Split(string(payload), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record AuditRecord
		if err := json.Unmarshal([]byte(line), &record); err == nil {
			records = append(records, record)
		}
	}
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Time > records[j].Time
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func annotatePipelinesWithAudit(pipelines map[int][]Pipeline, records []AuditRecord) {
	byPipeline := map[string]AuditRecord{}
	for _, record := range records {
		if record.RepoID <= 0 || record.Pipeline <= 0 || record.Status != "ok" {
			continue
		}
		key := auditPipelineKey(record.RepoID, record.Pipeline)
		if _, exists := byPipeline[key]; !exists {
			byPipeline[key] = record
		}
	}
	for repoID, rows := range pipelines {
		for index := range rows {
			record, ok := byPipeline[auditPipelineKey(repoID, rows[index].Number)]
			if !ok {
				continue
			}
			rows[index].PedpodTriggeredBy = record.Username
			rows[index].PedpodTriggeredAt = record.Time
			rows[index].PedpodTaskID = record.TaskID
			rows[index].PedpodTaskTitle = record.TaskTitle
		}
		pipelines[repoID] = rows
	}
}

type deploymentTarget struct {
	ID    string
	Name  string
	Group string
}

type deploymentVerifyConfig struct {
	MarkerPath string
	HealthURL  string
	Timeout    time.Duration
}

func deploymentStatuses(tasks []Task, repos map[int]string, pipelines map[int][]Pipeline) []DeploymentStatus {
	taskByID := map[string]Task{}
	targets := map[string]*DeploymentStatus{}
	verifyConfigs := map[string]deploymentVerifyConfig{}
	order := []string{}

	for _, task := range tasks {
		taskByID[task.ID] = task
		target, ok := deploymentTargetFromTask(task)
		if !ok {
			continue
		}
		if cfg := deploymentVerifyConfigFromVariables(task.Variables); cfg.hasChecks() {
			verifyConfigs[target.ID] = mergeDeploymentVerifyConfig(verifyConfigs[target.ID], cfg)
		}
		if _, exists := targets[target.ID]; !exists {
			status := DeploymentStatus{
				ID:               target.ID,
				Name:             target.Name,
				Group:            target.Group,
				RepoID:           task.RepoID,
				RepoName:         fallbackText(task.RepoName, repos[task.RepoID]),
				ConfiguredBranch: fallbackText(task.Branch, "main"),
				LastStatus:       "not_deployed",
				LatestStatus:     "not_deployed",
			}
			targets[target.ID] = &status
			order = append(order, target.ID)
		}
	}

	verifiedByTarget := map[string][]DeploymentStatus{}
	unverifiedByTarget := map[string][]DeploymentStatus{}
	for repoID, rows := range pipelines {
		repoName := repos[repoID]
		for _, pipeline := range rows {
			target, configuredBranch, ok := deploymentTargetFromPipeline(repoID, repoName, pipeline, taskByID, tasks)
			if !ok {
				continue
			}
			status, exists := targets[target.ID]
			if !exists {
				status = &DeploymentStatus{
					ID:               target.ID,
					Name:             target.Name,
					Group:            target.Group,
					RepoID:           repoID,
					RepoName:         repoName,
					ConfiguredBranch: fallbackText(configuredBranch, fallbackText(pipeline.Branch, "main")),
					LastStatus:       "not_deployed",
					LatestStatus:     "not_deployed",
				}
				targets[target.ID] = status
				order = append(order, target.ID)
			}
			if isNewerActivity(pipeline, status.LatestAt) {
				status.LatestAction = deploymentActionText(repoID, repoName, pipeline)
				status.LatestStatus = pipeline.Status
				status.LatestBranch = fallbackText(pipeline.Branch, "-")
				status.LatestCommit = pipeline.Commit
				status.LatestAt = pipelineActivityAt(pipeline)
				status.LatestPipeline = pipeline.Number
				status.LatestTriggeredBy = pipelineActor(pipeline)
			}
			if pipeline.Status == "success" {
				candidate := deploymentStatusFromPipeline(target, repoID, repoName, configuredBranch, pipeline)
				applyDeploymentVerification(&candidate, verifyConfigs[target.ID])
				if candidate.DeployVerified {
					verifiedByTarget[target.ID] = append(verifiedByTarget[target.ID], candidate)
				} else {
					unverifiedByTarget[target.ID] = append(unverifiedByTarget[target.ID], candidate)
				}
			}
		}
	}

	for id, status := range targets {
		verified := verifiedByTarget[id]
		sort.SliceStable(verified, func(i, j int) bool {
			return verified[i].LastDeployedAt > verified[j].LastDeployedAt
		})
		if len(verified) > 0 {
			current := verified[0]
			status.CurrentBranch = current.CurrentBranch
			status.CurrentCommit = current.CurrentCommit
			status.LastAction = current.LastAction
			status.LastStatus = current.LastStatus
			status.LastDeployedAt = current.LastDeployedAt
			status.Pipeline = current.Pipeline
			status.TriggeredBy = current.TriggeredBy
			status.TriggeredAt = current.TriggeredAt
			status.Variables = current.Variables
			status.ActualCommit = current.ActualCommit
			status.HealthURL = current.HealthURL
			status.DeployVerified = current.DeployVerified
			status.DeployDegraded = current.DeployDegraded
			status.DeployVerifyStatus = current.DeployVerifyStatus
			status.DeployVerifyMessage = current.DeployVerifyMessage
			if status.ConfiguredBranch == "" {
				status.ConfiguredBranch = current.ConfiguredBranch
			}
			status.Revisions = deploymentRevisions(verified, 12)
		}
		if len(verified) > 1 {
			previous := verified[1]
			status.PreviousAction = previous.LastAction
			status.PreviousBranch = previous.CurrentBranch
			status.PreviousCommit = previous.CurrentCommit
			status.PreviousDeployedAt = previous.LastDeployedAt
			status.PreviousPipeline = previous.Pipeline
		}
		if status.CurrentCommit == "" {
			unverified := unverifiedByTarget[id]
			sort.SliceStable(unverified, func(i, j int) bool {
				return unverified[i].LastDeployedAt > unverified[j].LastDeployedAt
			})
			if len(unverified) > 0 {
				status.DeployVerified = false
				status.DeployDegraded = unverified[0].DeployDegraded
				status.DeployVerifyStatus = unverified[0].DeployVerifyStatus
				status.DeployVerifyMessage = unverified[0].DeployVerifyMessage
				status.ActualCommit = unverified[0].ActualCommit
				status.HealthURL = unverified[0].HealthURL
			} else {
				applyDeploymentVerification(status, verifyConfigs[id])
			}
		}
	}

	result := make([]DeploymentStatus, 0, len(order))
	for _, id := range order {
		result = append(result, *targets[id])
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Group == result[j].Group {
			return result[i].Name < result[j].Name
		}
		return groupSortKey(result[i].Group) < groupSortKey(result[j].Group)
	})
	return result
}

func deploymentStatusFromPipeline(target deploymentTarget, repoID int, repoName string, configuredBranch string, pipeline Pipeline) DeploymentStatus {
	return DeploymentStatus{
		ID:               target.ID,
		Name:             target.Name,
		Group:            target.Group,
		RepoID:           repoID,
		RepoName:         repoName,
		ConfiguredBranch: fallbackText(configuredBranch, fallbackText(pipeline.Branch, "main")),
		CurrentBranch:    fallbackText(pipeline.Branch, "-"),
		CurrentCommit:    pipeline.Commit,
		LastAction:       deploymentActionText(repoID, repoName, pipeline),
		LastStatus:       pipeline.Status,
		LastDeployedAt:   pipelineFinishedAt(pipeline),
		Pipeline:         pipeline.Number,
		TriggeredBy:      pipelineActor(pipeline),
		TriggeredAt:      pipeline.PedpodTriggeredAt,
		Variables:        sanitizeVariables(pipeline.Variables),
	}
}

func deploymentRevisions(rows []DeploymentStatus, limit int) []DeploymentRevision {
	if limit <= 0 || len(rows) == 0 {
		return nil
	}
	revisions := make([]DeploymentRevision, 0, min(limit, len(rows)))
	seen := map[string]bool{}
	for _, row := range rows {
		commit := strings.TrimSpace(row.CurrentCommit)
		pipeline := row.Pipeline
		key := fmt.Sprintf("%d:%s:%s", pipeline, row.CurrentBranch, commit)
		if seen[key] {
			continue
		}
		seen[key] = true
		revisions = append(revisions, DeploymentRevision{
			Pipeline:    pipeline,
			Branch:      row.CurrentBranch,
			Commit:      commit,
			DeployedAt:  row.LastDeployedAt,
			Action:      row.LastAction,
			Verified:    row.DeployVerified,
			TriggeredBy: row.TriggeredBy,
			TriggeredAt: row.TriggeredAt,
		})
		if len(revisions) >= limit {
			break
		}
	}
	return revisions
}

func deploymentTargetFromPipeline(repoID int, repoName string, pipeline Pipeline, taskByID map[string]Task, tasks []Task) (deploymentTarget, string, bool) {
	if pipeline.PedpodTaskID != "" {
		if task, ok := taskByID[pipeline.PedpodTaskID]; ok {
			target, targetOK := deploymentTargetFromTask(task)
			return target, task.Branch, targetOK
		}
	}
	if task, ok := deploymentTaskFromPipeline(repoID, pipeline, tasks); ok {
		target, targetOK := deploymentTargetFromTask(task)
		return target, task.Branch, targetOK
	}
	action := variableValue(pipeline.Variables, "DEPLOY_ACTION")
	if action == "" && !pipelineHasPedpodProjectMetadata(pipeline.Variables) {
		return deploymentTarget{}, "", false
	}
	if isMaintenanceAction(action) {
		return deploymentTarget{}, "", false
	}
	task := Task{
		ID:        fallbackText(pipeline.PedpodTaskID, fmt.Sprintf("repo-%d-pipeline", repoID)),
		Group:     deploymentGroupFromPipeline(repoName, pipeline),
		Title:     deploymentTitleFromPipeline(repoName, pipeline),
		RepoID:    repoID,
		RepoName:  repoName,
		Branch:    pipeline.Branch,
		Variables: pipeline.Variables,
	}
	target, ok := deploymentTargetFromTask(task)
	return target, task.Branch, ok
}

func pipelineHasPedpodProjectMetadata(variables map[string]string) bool {
	return firstNonEmptyString(
		variableValue(variables, "PEAPOD_PROJECT_ID"),
		variableValue(variables, "ZEPHYR_PROJECT_ID"),
		variableValue(variables, "PROJECT_ID"),
		variableValue(variables, "SERVICE_ID"),
		variableValue(variables, "DEPLOY_SERVICE"),
		variableValue(variables, "APP"),
		variableValue(variables, "PROJECT"),
	) != ""
}

func deploymentTaskFromPipeline(repoID int, pipeline Pipeline, tasks []Task) (Task, bool) {
	bestScore := 0
	var best Task
	for _, task := range tasks {
		if task.RepoID != repoID || task.ExternalURL != "" {
			continue
		}
		if task.Branch != "" && pipeline.Branch != "" && task.Branch != pipeline.Branch {
			continue
		}
		score := deploymentVariableMatchScore(task.Variables, pipeline.Variables)
		if score > bestScore {
			bestScore = score
			best = task
		}
	}
	return best, bestScore > 0
}

func deploymentVariableMatchScore(taskVariables map[string]string, pipelineVariables map[string]string) int {
	if len(taskVariables) == 0 || len(pipelineVariables) == 0 {
		return 0
	}
	score := 0
	for key, taskValue := range taskVariables {
		if isProjectMetadataVariable(key) {
			continue
		}
		taskValue = strings.TrimSpace(taskValue)
		if taskValue == "" {
			continue
		}
		if pipelineValue := variableValue(pipelineVariables, key); pipelineValue != "" && pipelineValue == taskValue {
			score += deploymentVariableWeight(key)
		}
	}
	return score
}

func isProjectMetadataVariable(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	return strings.HasPrefix(upper, "PEAPOD_PROJECT_") || strings.HasPrefix(upper, "ZEPHYR_PROJECT_")
}

func deploymentVariableWeight(key string) int {
	upper := strings.ToUpper(strings.TrimSpace(key))
	switch upper {
	case "DEPLOY_ACTION", "DEPLOY_TARGET":
		return 10
	default:
		return 1
	}
}

func deploymentTargetFromTask(task Task) (deploymentTarget, bool) {
	if task.ExternalURL != "" || task.RepoID <= 0 {
		return deploymentTarget{}, false
	}
	variables := task.Variables
	action := variableValue(variables, "DEPLOY_ACTION")
	if isMaintenanceAction(action) {
		return deploymentTarget{}, false
	}
	groupLabel := meaningfulDeploymentLabel(task.Group)
	titleLabel := meaningfulDeploymentLabel(task.Title)
	projectID := firstNonEmpty(
		variableValue(variables, "PEAPOD_PROJECT_ID"),
		variableValue(variables, "ZEPHYR_PROJECT_ID"),
		variableValue(variables, "PROJECT_ID"),
		variableValue(variables, "SERVICE_ID"),
		variableValue(variables, "DEPLOY_SERVICE"),
		variableValue(variables, "APP"),
		variableValue(variables, "PROJECT"),
		normalizeTaskID(groupLabel),
		normalizeTaskID(titleLabel),
		normalizeTaskID(task.ID),
	)
	name := firstNonEmpty(
		variableValue(variables, "PEAPOD_PROJECT_NAME"),
		variableValue(variables, "ZEPHYR_PROJECT_NAME"),
		variableValue(variables, "PROJECT_NAME"),
		titleLabel,
		groupLabel,
		fallbackText(task.RepoName, fmt.Sprintf("Repo %d", task.RepoID)),
	)
	group := fallbackText(groupLabel, fallbackText(task.RepoName, "部署项目"))
	return deploymentTarget{ID: fmt.Sprintf("repo-%d:%s", task.RepoID, projectID), Name: name, Group: group}, true
}

func deploymentGroupFromPipeline(repoName string, pipeline Pipeline) string {
	group := meaningfulDeploymentLabel(firstNonEmptyString(
		variableValue(pipeline.Variables, "PEAPOD_PROJECT_GROUP"),
		variableValue(pipeline.Variables, "ZEPHYR_PROJECT_GROUP"),
	))
	if group != "" {
		return group
	}
	return fallbackText(repoName, "部署项目")
}

func deploymentTitleFromPipeline(repoName string, pipeline Pipeline) string {
	if title := meaningfulDeploymentLabel(pipeline.PedpodTaskTitle); title != "" {
		return title
	}
	name := meaningfulDeploymentLabel(firstNonEmptyString(
		variableValue(pipeline.Variables, "PEAPOD_PROJECT_NAME"),
		variableValue(pipeline.Variables, "ZEPHYR_PROJECT_NAME"),
	))
	if name != "" {
		return name
	}
	action := variableValue(pipeline.Variables, "DEPLOY_ACTION")
	target := variableValue(pipeline.Variables, "DEPLOY_TARGET")
	if action != "" && target != "" {
		return fmt.Sprintf("%s %s", target, action)
	}
	if target != "" {
		return target
	}
	return fallbackText(repoName, "部署项目")
}

func meaningfulDeploymentLabel(value string) string {
	label := strings.TrimSpace(value)
	switch label {
	case "", "未归类", "默认模块":
		return ""
	default:
		return label
	}
}

func deploymentActionText(repoID int, repoName string, pipeline Pipeline) string {
	if pipeline.PedpodTaskTitle != "" {
		return pipeline.PedpodTaskTitle
	}
	variables := pipeline.Variables
	action := variableValue(variables, "DEPLOY_ACTION")
	if action != "" {
		return "DEPLOY_ACTION=" + action
	}
	if repoName != "" {
		return repoName + " 部署"
	}
	return "部署"
}

func isRollbackTask(task Task) bool {
	action := strings.ToLower(strings.TrimSpace(variableValue(task.Variables, "DEPLOY_ACTION")))
	if action == "rollback" {
		return true
	}
	if strings.TrimSpace(variableValue(task.Variables, "ROLLBACK_VERSION")) != "" {
		return true
	}
	text := strings.ToLower(task.ID + " " + task.Title)
	return strings.Contains(text, "rollback") || strings.Contains(text, "回退") || strings.Contains(text, "回滚")
}

func isAllowedRollbackInput(key string) bool {
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "ROLLBACK_VERSION", "ROLLBACK_COMMIT", "ROLLBACK_BRANCH", "ROLLBACK_PIPELINE", "ROLLBACK_PIPELINE_NUMBER":
		return true
	case "PEAPOD_ROLLBACK_COMMIT", "PEAPOD_ROLLBACK_BRANCH", "PEAPOD_ROLLBACK_PIPELINE":
		return true
	default:
		return false
	}
}

func isMaintenanceAction(action string) bool {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return false
	}
	for _, needle := range []string{"cleanup", "clean", "disk", "ps", "logs", "status", "restart", "reload", "inspect", "observability", "peapod", "zefire", "zephyr", "woodpecker-ui-patch"} {
		if strings.Contains(action, needle) {
			return true
		}
	}
	return false
}

func deploymentVerifyConfigFromVariables(values map[string]string) deploymentVerifyConfig {
	timeoutSeconds := parsePositiveInt(firstNonEmptyString(
		variableValue(values, "PEAPOD_DEPLOY_VERIFY_TIMEOUT_SECONDS"),
		variableValue(values, "ZEPHYR_DEPLOY_VERIFY_TIMEOUT_SECONDS"),
	))
	if timeoutSeconds <= 0 {
		timeoutSeconds = parsePositiveInt(variableValue(values, "DEPLOY_VERIFY_TIMEOUT_SECONDS"))
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 2
	}
	if timeoutSeconds > 15 {
		timeoutSeconds = 15
	}
	return deploymentVerifyConfig{
		MarkerPath: firstNonEmptyString(
			variableValue(values, "PEAPOD_DEPLOY_MARKER_PATH"),
			variableValue(values, "ZEPHYR_DEPLOY_MARKER_PATH"),
			variableValue(values, "DEPLOY_MARKER_PATH"),
		),
		HealthURL: firstNonEmptyString(
			variableValue(values, "PEAPOD_DEPLOY_VERIFY_URL"),
			variableValue(values, "PEAPOD_HEALTH_URL"),
			variableValue(values, "ZEPHYR_DEPLOY_VERIFY_URL"),
			variableValue(values, "ZEPHYR_HEALTH_URL"),
			variableValue(values, "DEPLOY_HEALTH_URL"),
			variableValue(values, "HEALTH_URL"),
		),
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	}
}

func (cfg deploymentVerifyConfig) hasChecks() bool {
	return cfg.MarkerPath != "" || cfg.HealthURL != ""
}

func mergeDeploymentVerifyConfig(current, next deploymentVerifyConfig) deploymentVerifyConfig {
	if next.MarkerPath != "" {
		current.MarkerPath = next.MarkerPath
	}
	if next.HealthURL != "" {
		current.HealthURL = next.HealthURL
	}
	if next.Timeout > 0 {
		current.Timeout = next.Timeout
	}
	return current
}

func applyDeploymentVerification(status *DeploymentStatus, cfg deploymentVerifyConfig) {
	if status == nil {
		return
	}
	status.DeployDegraded = false
	if status.CurrentCommit == "" {
		status.DeployVerified = false
		status.DeployVerifyStatus = "not_deployed"
		status.DeployVerifyMessage = "还没有成功流水线记录"
		return
	}
	if !cfg.hasChecks() {
		status.DeployVerified = false
		status.DeployVerifyStatus = "pipeline_only"
		status.DeployVerifyMessage = "构建成功，部署未验证：尚未配置服务健康或版本落地校验"
		return
	}

	status.HealthURL = cfg.HealthURL
	markerIssues := []string{}
	markerMismatch := ""
	hardIssues := []string{}
	markerChecked := false
	markerOK := false
	healthChecked := false
	healthOK := false

	if cfg.MarkerPath != "" {
		markerChecked = true
		actualCommit, err := readDeploymentMarker(cfg.MarkerPath)
		status.ActualCommit = actualCommit
		if err != nil {
			markerIssues = append(markerIssues, "版本 marker 暂不可读："+err.Error())
			if status.DeployVerifyStatus == "" {
				status.DeployVerifyStatus = "marker_missing"
			}
		} else if actualCommit == "" {
			markerIssues = append(markerIssues, "版本 marker 为空")
			if status.DeployVerifyStatus == "" {
				status.DeployVerifyStatus = "marker_missing"
			}
		} else if !deploymentCommitMatches(actualCommit, status.CurrentCommit) {
			markerMismatch = fmt.Sprintf("实际版本 %s 与流水线版本 %s 不一致", shortCommit(actualCommit), shortCommit(status.CurrentCommit))
			if status.DeployVerifyStatus == "" {
				status.DeployVerifyStatus = "marker_mismatch"
			}
		} else {
			markerOK = true
		}
	}

	if cfg.HealthURL != "" {
		healthChecked = true
		if err := probeDeploymentHealth(cfg.HealthURL, cfg.Timeout); err != nil {
			hardIssues = append(hardIssues, "健康检查失败："+err.Error())
			if status.DeployVerifyStatus == "" || status.DeployVerifyStatus == "marker_missing" {
				status.DeployVerifyStatus = "health_failed"
			}
		} else {
			healthOK = true
		}
	}

	if markerMismatch != "" {
		if healthChecked && healthOK {
			status.DeployVerified = true
			status.DeployDegraded = true
			status.DeployVerifyStatus = "external_marker"
			status.DeployVerifyMessage = "服务健康检查已通过；版本 marker 指向 " + shortCommit(status.ActualCommit) + "，与 Woodpecker 最近成功记录不同，可能是手动或外部部署"
			status.CurrentCommit = status.ActualCommit
			return
		}
		hardIssues = append(hardIssues, markerMismatch)
	}

	if len(hardIssues) > 0 {
		status.DeployVerified = false
		if len(markerIssues) > 0 {
			hardIssues = append(hardIssues, markerIssues...)
		}
		status.DeployVerifyMessage = strings.Join(hardIssues, "；")
		return
	}

	if len(markerIssues) > 0 {
		if healthChecked && healthOK {
			status.DeployVerified = true
			status.DeployDegraded = true
			status.DeployVerifyStatus = "marker_unavailable"
			status.DeployVerifyMessage = "服务健康检查已通过；" + strings.Join(markerIssues, "；")
			return
		}
		status.DeployVerified = false
		status.DeployVerifyStatus = "marker_missing"
		status.DeployVerifyMessage = strings.Join(markerIssues, "；")
		return
	}

	status.DeployVerified = true
	status.DeployVerifyStatus = "verified"
	switch {
	case markerOK && healthOK:
		status.DeployVerifyMessage = "版本 marker 与服务健康检查均已通过"
	case markerChecked && markerOK:
		status.DeployVerifyMessage = "版本 marker 已确认"
	case healthChecked && healthOK:
		status.DeployVerifyMessage = "服务健康检查已通过"
	default:
		status.DeployVerified = false
		status.DeployVerifyStatus = "pipeline_only"
		status.DeployVerifyMessage = "构建成功，部署未验证"
	}
}

func readDeploymentMarker(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(payload))
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], nil
}

func deploymentCommitMatches(actual, expected string) bool {
	actual = strings.ToLower(strings.TrimSpace(actual))
	expected = strings.ToLower(strings.TrimSpace(expected))
	if actual == "" || expected == "" {
		return false
	}
	return strings.HasPrefix(actual, expected) || strings.HasPrefix(expected, actual)
}

func probeDeploymentHealth(rawURL string, timeout time.Duration) error {
	rawURL = strings.TrimSpace(rawURL)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported health URL scheme %q", parsed.Scheme)
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	client := http.Client{Timeout: timeout}
	resp, err := client.Get(rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) <= 8 {
		return commit
	}
	return commit[:8]
}

func parsePositiveInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func variableValue(values map[string]string, key string) string {
	if values == nil {
		return ""
	}
	if value := strings.TrimSpace(values[key]); value != "" {
		return value
	}
	return strings.TrimSpace(values[strings.ToLower(key)])
}

func isNewerDeployment(pipeline Pipeline, current DeploymentStatus) bool {
	return pipelineFinishedAt(pipeline) >= current.LastDeployedAt
}

func isNewerActivity(pipeline Pipeline, currentAt int64) bool {
	return pipelineActivityAt(pipeline) >= currentAt
}

func pipelineActivityAt(pipeline Pipeline) int64 {
	if pipeline.Updated > 0 {
		return pipeline.Updated
	}
	if pipeline.Finished > 0 {
		return pipeline.Finished
	}
	if pipeline.Started > 0 {
		return pipeline.Started
	}
	return pipeline.Created
}

func pipelineFinishedAt(pipeline Pipeline) int64 {
	if pipeline.Finished > 0 {
		return pipeline.Finished
	}
	if pipeline.Started > 0 {
		return pipeline.Started
	}
	return pipeline.Created
}

func pipelineActor(pipeline Pipeline) string {
	if pipeline.PedpodTriggeredBy != "" {
		return pipeline.PedpodTriggeredBy
	}
	if pipeline.Author != "" {
		return pipeline.Author
	}
	return pipeline.Sender
}

func groupSortKey(group string) string {
	return strings.ToLower(strings.TrimSpace(group))
}

func auditPipelineKey(repoID int, number int64) string {
	return fmt.Sprintf("%d:%d", repoID, number)
}

func sanitizeAuditRecord(record AuditRecord) AuditRecord {
	record.RemoteIP = ""
	record.Variables = sanitizeVariables(record.Variables)
	return record
}

func sanitizeVariables(values map[string]string) map[string]string {
	cleaned := map[string]string{}
	for key, value := range values {
		if isSensitiveAuditKey(key) {
			cleaned[key] = "******"
		} else {
			cleaned[key] = value
		}
	}
	return cleaned
}

func isSensitiveAuditKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	for _, needle := range []string{"PASSWORD", "TOKEN", "SECRET", "KEY", "PRIVATE", "CREDENTIAL", "ACCESS"} {
		if strings.Contains(upper, needle) {
			return true
		}
	}
	return false
}

func (a *App) authMode() string {
	if a.store != nil {
		return "db"
	}
	return "legacy"
}

func healthStatus(ok bool, okMessage string, fallbackMessage string) map[string]string {
	if ok {
		return map[string]string{"status": "ok", "message": okMessage}
	}
	return map[string]string{"status": "warning", "message": fallbackMessage}
}

func (a *App) setupConfigResponse(now time.Time) SetupConfigResponse {
	hosts := parseMonitorHosts(a.cfg)
	config := RuntimeConfigInput{
		PublicURL:             a.cfg.PublicURL,
		WoodpeckerServer:      a.cfg.WoodpeckerServer,
		WoodpeckerPublicURL:   a.cfg.WoodpeckerPublicURL,
		WoodpeckerToken:       "",
		BeszelBaseURL:         a.cfg.BeszelBaseURL,
		BeszelPublicURL:       a.cfg.BeszelPublicURL,
		BeszelEmail:           a.cfg.BeszelEmail,
		BeszelPassword:        "",
		DozzleBaseURL:         a.cfg.DozzleBaseURL,
		DozzlePublicURL:       a.cfg.DozzlePublicURL,
		DozzleUsername:        a.cfg.DozzleUsername,
		DozzlePassword:        "",
		GrafanaPublicURL:      a.cfg.GrafanaPublicURL,
		LogStrategy:           normalizeLogStrategy(a.cfg.LogStrategy),
		DockerLogMaxSize:      fallbackText(a.cfg.DockerLogMaxSize, "20m"),
		DockerLogMaxFile:      fallbackText(a.cfg.DockerLogMaxFile, "3"),
		AlertWebhookURL:       "",
		ExternalLinks:         a.extraExternalLinks(),
		MonitorHosts:          hosts,
		MonitorRefreshSeconds: a.cfg.MonitorRefreshSeconds,
		MonitorWarnDisk:       a.cfg.MonitorWarnDisk,
		MonitorCritDisk:       a.cfg.MonitorCritDisk,
		MonitorWarnMemory:     a.cfg.MonitorWarnMemory,
	}
	verification := deploymentVerificationSummary(a.configuredTasks())
	logStrategy := a.logStrategyStatus()
	checklist := a.setupChecklist(hosts, verification, logStrategy)
	doctor := a.doctorSummary(time.Now(), checklist)
	return SetupConfigResponse{
		Config: config,
		Secrets: map[string]bool{
			"woodpecker_token": strings.TrimSpace(a.cfg.WoodpeckerToken) != "",
			"beszel_password":  strings.TrimSpace(a.cfg.BeszelPassword) != "",
			"dozzle_password":  strings.TrimSpace(a.cfg.DozzlePassword) != "",
			"session_secret":   strings.TrimSpace(a.cfg.SessionSecret) != "",
			"database_dsn":     strings.TrimSpace(a.cfg.DBDSN) != "",
			"alert_webhook":    strings.TrimSpace(a.cfg.AlertWebhookURL) != "",
		},
		Readiness:                     setupReadiness(checklist),
		Status:                        a.setupStatus(hosts),
		Checklist:                     checklist,
		DeploymentVerificationSummary: verification,
		LogStrategy:                   logStrategy,
		Onboarding:                    onboardingProgress(checklist),
		Doctor:                        doctor,
		Commands:                      a.setupCommands(hosts),
		Docs:                          setupDocLinks(),
		UpdatedAt:                     now.Format(time.RFC3339),
	}
}

func onboardingProgress(checklist []SetupChecklistItem) OnboardingProgress {
	progress := OnboardingProgress{TotalCount: len(checklist)}
	for _, item := range checklist {
		switch item.Status {
		case "ok", "optional":
			progress.ReadyCount++
		case "error", "critical":
			progress.BlockedCount++
		case "warning", "unknown":
			progress.WarningCount++
		}
		if progress.NextAction == "" && (item.Status == "error" || item.Status == "critical" || item.Status == "warning") {
			progress.NextAction = item.Title
			if item.Fix != "" {
				progress.NextAction = item.Title + "：" + item.Fix
			}
		}
	}
	if progress.TotalCount > 0 {
		progress.Percent = int(float64(progress.ReadyCount) / float64(progress.TotalCount) * 100)
	}
	if progress.NextAction == "" && progress.WarningCount > 0 {
		for _, item := range checklist {
			if item.Status == "unknown" {
				progress.NextAction = item.Title + "：" + fallbackText(item.Fix, item.Message)
				break
			}
		}
	}
	if progress.NextAction == "" {
		progress.NextAction = "核心接入已完成，可以开始配置仓库和部署任务。"
	}
	return progress
}

func (a *App) doctorSummary(now time.Time, checklist []SetupChecklistItem) DoctorSummary {
	checks := make([]DoctorCheck, 0, len(checklist)+6)
	for _, item := range checklist {
		checks = append(checks, DoctorCheck{
			ID:          item.ID,
			Title:       item.Title,
			Status:      item.Status,
			Severity:    fallbackText(item.Severity, item.Status),
			Message:     item.Message,
			Fix:         item.Fix,
			ActionLabel: item.ActionLabel,
			ActionURL:   item.ActionURL,
		})
	}
	checks = append(checks, a.localDoctorChecks()...)
	return DoctorSummary{
		Readiness: doctorReadiness(checks),
		Checks:    checks,
		UpdatedAt: now.Format(time.RFC3339),
	}
}

func doctorReadiness(checks []DoctorCheck) string {
	hasWarning := false
	for _, check := range checks {
		switch check.Severity {
		case "error", "critical":
			return "blocked"
		case "warning":
			hasWarning = true
		}
	}
	if hasWarning {
		return "warning"
	}
	return "ready"
}

func (a *App) localDoctorChecks() []DoctorCheck {
	checks := []DoctorCheck{}
	add := func(check DoctorCheck) {
		if check.Severity == "" {
			check.Severity = check.Status
		}
		checks = append(checks, check)
	}
	add(commandDoctorCheck("docker", "Docker Engine", []string{"docker", "--version"}, "安装 Docker，并确认当前用户可以访问 Docker。"))
	add(commandDoctorCheck("docker-compose", "Docker Compose", []string{"docker", "compose", "version"}, "安装 Docker Compose plugin。"))
	add(fileDoctorCheck("env-file", ".env 文件", ".env", "运行 scripts/bootstrap.sh 生成 .env，再补充公开地址和密钥。"))
	add(fileDoctorCheck("tasks-file", "任务配置文件", a.cfg.TasksPath, "进入配置中心使用任务模板，或准备 data/peapod/tasks.json。"))
	add(diskUsageDoctorCheck("disk-usage", "磁盘使用率"))
	add(dockerDiskDoctorCheck("docker-disk", "Docker 磁盘空间"))
	if a.store == nil {
		add(DoctorCheck{
			ID:       "database-auth",
			Title:    "团队账号数据库",
			Status:   "warning",
			Severity: "warning",
			Message:  "当前没有启用数据库账号体系，只能用共享密码或旧兼容模式。",
			Fix:      "配置 PEAPOD_DB_DSN，启用成员账号、审计和接入配置保存。",
		})
	} else {
		add(DoctorCheck{ID: "database-auth", Title: "团队账号数据库", Status: "ok", Severity: "ok", Message: "数据库账号体系已启用。"})
	}
	return checks
}

func commandDoctorCheck(id string, title string, args []string, fix string) DoctorCheck {
	if len(args) == 0 {
		return DoctorCheck{ID: id, Title: title, Status: "warning", Severity: "warning", Message: "检查命令未配置。", Fix: fix}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	message := strings.TrimSpace(string(output))
	if len(message) > 180 {
		message = message[:180] + "..."
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			message = "检查超时"
		}
		if message == "" {
			message = err.Error()
		}
		return DoctorCheck{ID: id, Title: title, Status: "error", Severity: "error", Message: message, Fix: fix}
	}
	return DoctorCheck{ID: id, Title: title, Status: "ok", Severity: "ok", Message: fallbackText(message, "可用")}
}

func fileDoctorCheck(id string, title string, path string, fix string) DoctorCheck {
	path = strings.TrimSpace(path)
	if path == "" {
		return DoctorCheck{ID: id, Title: title, Status: "warning", Severity: "warning", Message: "路径未配置。", Fix: fix}
	}
	if _, err := os.Stat(path); err != nil {
		severity := "warning"
		if id == "env-file" {
			severity = "error"
		}
		return DoctorCheck{ID: id, Title: title, Status: severity, Severity: severity, Message: "未找到 " + path, Fix: fix}
	}
	return DoctorCheck{ID: id, Title: title, Status: "ok", Severity: "ok", Message: path + " 已存在。"}
}

func diskUsageDoctorCheck(id string, title string) DoctorCheck {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "df", "-P", "/")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return DoctorCheck{ID: id, Title: title, Status: "warning", Severity: "warning", Message: "无法读取磁盘信息", Fix: "检查 df 命令是否可用"}
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return DoctorCheck{ID: id, Title: title, Status: "warning", Severity: "warning", Message: "无法解析磁盘信息", Fix: "检查 df 命令输出"}
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 5 {
		return DoctorCheck{ID: id, Title: title, Status: "warning", Severity: "warning", Message: "无法解析磁盘信息", Fix: "检查 df 命令输出"}
	}
	percentStr := strings.TrimSuffix(fields[4], "%")
	percent, _ := strconv.Atoi(percentStr)
	message := fmt.Sprintf("根分区使用率 %d%%", percent)
	if percent >= 95 {
		return DoctorCheck{ID: id, Title: title, Status: "error", Severity: "error", Message: message + "，磁盘即将耗尽", Fix: "立即执行磁盘清理，或扩容磁盘"}
	} else if percent >= 90 {
		return DoctorCheck{ID: id, Title: title, Status: "error", Severity: "error", Message: message + "，需要尽快清理", Fix: "执行磁盘清理，或扩容磁盘"}
	} else if percent >= 80 {
		return DoctorCheck{ID: id, Title: title, Status: "warning", Severity: "warning", Message: message + "，建议关注", Fix: "关注磁盘增长趋势，必要时清理"}
	}
	return DoctorCheck{ID: id, Title: title, Status: "ok", Severity: "ok", Message: message}
}

func dockerDiskDoctorCheck(id string, title string) DoctorCheck {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "system", "df", "--format", "{{.Type}}\t{{.TotalCount}}\t{{.Size}}\t{{.Reclaimable}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return DoctorCheck{ID: id, Title: title, Status: "warning", Severity: "warning", Message: "无法读取 Docker 磁盘信息（容器内可能无 Docker socket）", Fix: "确认 Docker 可用，或通过 SSH 监控查看"}
	}
	totalReclaimable := float64(0)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			reclaimable := parseHumanBytes(fields[3])
			totalReclaimable += reclaimable
		}
	}
	reclaimGB := uint64(totalReclaimable / (1024 * 1024 * 1024))
	message := fmt.Sprintf("Docker 可回收约 %d GB", reclaimGB)
	if reclaimGB > 20 {
		return DoctorCheck{ID: id, Title: title, Status: "warning", Severity: "warning", Message: message, Fix: "运行 docker system prune 或通过 Peapod 磁盘清理任务回收空间"}
	}
	return DoctorCheck{ID: id, Title: title, Status: "ok", Severity: "ok", Message: message}
}

func (a *App) setupStatus(hosts []MonitorHostConfig) []SetupStatusItem {
	items := []SetupStatusItem{
		{
			ID:          "peapod",
			Title:       "Pedpod 入口",
			Status:      setupStatusFromBool(a.cfg.PublicURL != ""),
			Message:     fallbackText(a.cfg.PublicURL, "未配置公开访问地址"),
			ActionLabel: "打开 Pedpod",
			ActionURL:   a.cfg.PublicURL,
		},
		{
			ID:          "auth",
			Title:       "账号体系",
			Status:      setupStatusFromBool(a.store != nil),
			Message:     ternaryText(a.store != nil, "数据库账号模式已启用", "当前是共享密码模式，建议配置数据库后再给团队使用"),
			ActionLabel: "",
		},
		{
			ID:          "woodpecker",
			Title:       "Woodpecker",
			Status:      setupStatusFromBool(a.cfg.WoodpeckerServer != "" && a.cfg.WoodpeckerPublicURL != "" && a.cfg.WoodpeckerToken != ""),
			Message:     setupWoodpeckerMessage(a.cfg),
			ActionLabel: "打开 Woodpecker",
			ActionURL:   a.cfg.WoodpeckerPublicURL,
		},
		{
			ID:          "beszel",
			Title:       "Beszel",
			Status:      setupStatusFromBool(a.cfg.BeszelBaseURL != "" && a.cfg.BeszelPublicURL != ""),
			Message:     setupBeszelMessage(a.cfg),
			ActionLabel: "打开 Beszel",
			ActionURL:   a.cfg.BeszelPublicURL,
		},
		{
			ID:          "dozzle",
			Title:       "Dozzle 轻量日志",
			Status:      setupStatusFromBool(a.cfg.DozzleBaseURL != "" || a.cfg.DozzlePublicURL != ""),
			Message:     fallbackText(firstNonEmptyString(a.cfg.DozzlePublicURL, a.cfg.DozzleBaseURL), "未配置 Dozzle；轻量模式下建议启用"),
			ActionLabel: "打开 Dozzle",
			ActionURL:   a.cfg.DozzlePublicURL,
		},
		{
			ID:          "grafana",
			Title:       "Grafana / Loki",
			Status:      ternaryText(a.cfg.GrafanaPublicURL != "", "ok", "optional"),
			Message:     fallbackText(a.cfg.GrafanaPublicURL, "未配置 Grafana 入口；完整历史日志/指标模式再启用"),
			ActionLabel: "打开 Grafana",
			ActionURL:   a.cfg.GrafanaPublicURL,
		},
		{
			ID:      "hosts",
			Title:   "被管机器",
			Status:  setupStatusFromBool(len(hosts) > 0),
			Message: fmt.Sprintf("已配置 %d 台机器；业务机只需要 agent 和 SSH key，不需要运行 Pedpod", len(hosts)),
		},
		{
			ID:      "tasks",
			Title:   "部署任务",
			Status:  setupStatusFromBool(len(a.configuredTasks()) > 0),
			Message: fmt.Sprintf("已加载 %d 个任务/入口，可在部署任务页维护 Woodpecker 参数", len(a.configuredTasks())),
		},
	}
	return items
}

func (a *App) setupChecklist(hosts []MonitorHostConfig, verification DeploymentVerificationSummary, logStrategy LogStrategyStatus) []SetupChecklistItem {
	items := []SetupChecklistItem{}
	add := func(item SetupChecklistItem) {
		if item.Severity == "" {
			item.Severity = item.Status
		}
		items = append(items, item)
	}
	add(a.urlChecklistItem("peapod-url", "Pedpod 公开地址", a.cfg.PublicURL, true, "配置 PEAPOD_PUBLIC_URL，并确认反向代理可访问。"))
	add(a.urlChecklistItem("woodpecker-url", "Woodpecker 公开入口", a.cfg.WoodpeckerPublicURL, true, "配置 WOODPECKER_PUBLIC_URL，并确认 ci 域名反代到 Woodpecker。"))
	add(SetupChecklistItem{
		ID:          "woodpecker-token",
		Title:       "Woodpecker API token",
		Status:      ternaryText(strings.TrimSpace(a.cfg.WoodpeckerToken) != "", "ok", "error"),
		Severity:    ternaryText(strings.TrimSpace(a.cfg.WoodpeckerToken) != "", "ok", "error"),
		Message:     ternaryText(strings.TrimSpace(a.cfg.WoodpeckerToken) != "", "已配置，Pedpod 可以触发流水线。", "未配置，Pedpod 无法触发或取消流水线。"),
		Fix:         "在 Woodpecker 创建用户 token 后填入配置中心。",
		ActionLabel: "打开 Woodpecker",
		ActionURL:   a.cfg.WoodpeckerPublicURL,
	})
	add(SetupChecklistItem{
		ID:          "woodpecker-oauth",
		Title:       "GitHub OAuth / 仓库 Trusted",
		Status:      "unknown",
		Severity:    "warning",
		Message:     "Woodpecker 的 GitHub OAuth、仓库启用和 Trusted 权限需要在 Woodpecker 内确认。",
		Fix:         "进入 Woodpecker，确认仓库已启用；部署类仓库需要 Trusted/Secrets/Volumes 权限。",
		ActionLabel: "去确认",
		ActionURL:   a.cfg.WoodpeckerPublicURL,
	})
	add(a.urlChecklistItem("beszel-url", "Beszel 资源监控", a.cfg.BeszelPublicURL, len(hosts) > 0, "配置 Beszel 公开入口，或保留 SSH 只读兜底。"))
	add(a.urlChecklistItem("dozzle-url", "Dozzle 轻量日志", a.cfg.DozzlePublicURL, logStrategy.Mode == "lightweight", "轻量日志模式需要配置 Dozzle 入口。"))
	add(SetupChecklistItem{
		ID:          "dozzle-mcp",
		Title:       "Dozzle MCP",
		Status:      ternaryText(logStrategy.DozzleMCPReady, "ok", ternaryText(logStrategy.Mode == "lightweight", "warning", "optional")),
		Severity:    ternaryText(logStrategy.DozzleMCPReady, "ok", ternaryText(logStrategy.Mode == "lightweight", "warning", "ok")),
		Message:     fallbackText(logStrategy.DozzleMCPMessage, "用于 Pedpod 内置日志查询的只读接口。"),
		Fix:         "设置 PEAPOD_DOZZLE_BASE_URL，并给 Dozzle 配置 DOZZLE_ENABLE_MCP=true。",
		ActionLabel: "打开 Dozzle",
		ActionURL:   a.cfg.DozzlePublicURL,
	})
	add(a.urlChecklistItem("grafana-url", "Grafana / Loki 完整观测", a.cfg.GrafanaPublicURL, logStrategy.Mode == "observability", "完整观测模式需要配置 Grafana 入口。"))
	publicKeyReady := strings.TrimSpace(readMonitorPublicKey(a.cfg.MonitorSSHKeyPath)) != ""
	add(SetupChecklistItem{
		ID:       "monitor-ssh-key",
		Title:    "只读监控 SSH key",
		Status:   ternaryText(publicKeyReady, "ok", "warning"),
		Severity: ternaryText(publicKeyReady, "ok", "warning"),
		Message:  ternaryText(publicKeyReady, fmt.Sprintf("公钥已准备；已配置 %d 台被管机器。", len(hosts)), "未找到监控公钥；SSH 兜底监控不可用。"),
		Fix:      "在 PEAPOD_MONITOR_SSH_KEY_PATH 对应位置放置专用只读 key，并把 .pub 写入被管机器。",
	})
	add(SetupChecklistItem{
		ID:       "monitor-hosts",
		Title:    "被管机器",
		Status:   ternaryText(len(hosts) > 0, "ok", "warning"),
		Severity: ternaryText(len(hosts) > 0, "ok", "warning"),
		Message:  fmt.Sprintf("已配置 %d 台机器。业务机不需要运行 Pedpod，只需要监控 agent 或 SSH 兜底。", len(hosts)),
		Fix:      "在配置中心添加 production / staging / operations / service 机器。",
	})
	verifyStatus := "ok"
	verifySeverity := "ok"
	verifyMessage := fmt.Sprintf("部署任务 %d 个，已配置验证 %d 个。", verification.TaskCount, verification.ConfiguredCount)
	if verification.MissingCount > 0 {
		verifyStatus = "error"
		verifySeverity = "error"
		verifyMessage = fmt.Sprintf("%d 个部署任务缺少 marker/healthz，不能作为可信部署入口。", verification.MissingCount)
	}
	add(SetupChecklistItem{
		ID:       "deployment-verification",
		Title:    "部署可信验证",
		Status:   verifyStatus,
		Severity: verifySeverity,
		Message:  verifyMessage,
		Fix:      "给部署/回退/release 任务补充 PEAPOD_DEPLOY_MARKER_PATH 或 PEAPOD_DEPLOY_VERIFY_URL。",
	})
	add(SetupChecklistItem{
		ID:          "log-strategy",
		Title:       "日志策略",
		Status:      logStrategyChecklistStatus(logStrategy),
		Severity:    logStrategyChecklistSeverity(logStrategy),
		Message:     fmt.Sprintf("%s；Docker 日志保留 %s。", logStrategy.Message, logStrategy.DockerRetention),
		Fix:         "轻量模式配置 Dozzle；完整观测模式配置 Grafana/Loki；外部模式配置第三方日志入口。",
		ActionLabel: ternaryText(logStrategy.Mode == "observability", "打开 Grafana", "打开 Dozzle"),
		ActionURL:   firstNonEmptyString(logStrategy.GrafanaPublicURL, logStrategy.DozzlePublicURL),
	})
	return items
}

func (a *App) urlChecklistItem(id string, title string, rawURL string, required bool, fix string) SetupChecklistItem {
	rawURL = cleanURL(rawURL)
	if rawURL == "" {
		status := "optional"
		severity := "ok"
		message := "未配置，可按需补充。"
		if required {
			status = "warning"
			severity = "warning"
			message = "未配置。"
		}
		return SetupChecklistItem{ID: id, Title: title, Status: status, Severity: severity, Message: message, Fix: fix}
	}
	if err := probePublicURL(rawURL, 800*time.Millisecond); err != nil {
		return SetupChecklistItem{
			ID:          id,
			Title:       title,
			Status:      "warning",
			Severity:    "warning",
			Message:     "已配置，但轻量探测失败：" + err.Error(),
			Fix:         fix,
			ActionLabel: "打开",
			ActionURL:   rawURL,
		}
	}
	return SetupChecklistItem{ID: id, Title: title, Status: "ok", Severity: "ok", Message: "已配置且可访问。", ActionLabel: "打开", ActionURL: rawURL}
}

func probePublicURL(rawURL string, timeout time.Duration) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("URL 格式不正确")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("不支持 %s", parsed.Scheme)
	}
	client := http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodHead, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		req, err = http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		resp, err = client.Do(req)
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		return nil
	}
	return fmt.Errorf("HTTP %d", resp.StatusCode)
}

func setupReadiness(items []SetupChecklistItem) string {
	hasWarning := false
	for _, item := range items {
		switch item.Severity {
		case "error", "critical":
			return "blocked"
		case "warning":
			hasWarning = true
		}
	}
	if hasWarning {
		return "warning"
	}
	return "ready"
}

func logStrategyChecklistStatus(status LogStrategyStatus) string {
	switch status.Mode {
	case "lightweight":
		if status.DozzlePublicURL == "" {
			return "warning"
		}
	case "observability":
		if status.GrafanaPublicURL == "" {
			return "warning"
		}
	}
	return "ok"
}

func logStrategyChecklistSeverity(status LogStrategyStatus) string {
	if logStrategyChecklistStatus(status) == "warning" {
		return "warning"
	}
	return "ok"
}

func deploymentVerificationSummary(tasks []Task) DeploymentVerificationSummary {
	summary := DeploymentVerificationSummary{MissingTasks: []string{}}
	for _, task := range tasks {
		if !deploymentTaskRequiresVerification(task) {
			continue
		}
		summary.TaskCount++
		if taskHasDeploymentVerification(task) {
			summary.ConfiguredCount++
			continue
		}
		summary.MissingCount++
		summary.MissingTasks = append(summary.MissingTasks, fallbackText(task.Title, task.ID))
	}
	sort.Strings(summary.MissingTasks)
	return summary
}

func (a *App) logStrategyStatus() LogStrategyStatus {
	mode := normalizeLogStrategy(a.cfg.LogStrategy)
	if mode == "" {
		mode = "lightweight"
	}
	maxSize := fallbackText(strings.TrimSpace(a.cfg.DockerLogMaxSize), "20m")
	maxFile := fallbackText(strings.TrimSpace(a.cfg.DockerLogMaxFile), "3")
	status := LogStrategyStatus{
		Mode:              mode,
		DozzleBaseURL:     a.cfg.DozzleBaseURL,
		DozzlePublicURL:   a.cfg.DozzlePublicURL,
		GrafanaPublicURL:  a.cfg.GrafanaPublicURL,
		DockerLogMaxSize:  maxSize,
		DockerLogMaxFile:  maxFile,
		DockerRetention:   fmt.Sprintf("%s × %s", maxSize, maxFile),
		AlertWebhookReady: strings.TrimSpace(a.cfg.AlertWebhookURL) != "",
	}
	status.DozzleMCPReady, status.DozzleMCPMessage = a.probeDozzleMCP(900 * time.Millisecond)
	switch mode {
	case "observability":
		status.Label = "完整观测 Grafana/Loki"
		status.Message = "跨机器历史检索、指标、告警和排障"
	case "external":
		status.Label = "外部日志平台"
		status.Message = "日志由外部平台保存，Pedpod 只保留入口和策略说明"
	default:
		status.Label = "轻量模式 Dozzle"
		status.Message = "查看 Docker 已保留日志并实时跟随"
	}
	return status
}

func (a *App) setupCommands(hosts []MonitorHostConfig) []SetupCommand {
	publicKey := strings.TrimSpace(readMonitorPublicKey(a.cfg.MonitorSSHKeyPath))
	if publicKey == "" {
		publicKey = "ssh-ed25519 AAAA... peapod-monitor"
	}
	firstHost := "your-host"
	if len(hosts) > 0 {
		firstHost = fallbackText(hosts[0].SSHHost, hosts[0].Name)
	}
	return []SetupCommand{
		{
			ID:          "install-peapod",
			Title:       "安装 Pedpod 运维机",
			Description: "在运维/构建机 clone 仓库后执行。默认启动轻量栈，不强制 Grafana/Loki。",
			Command: strings.TrimSpace(`git clone https://github.com/tangfire/peapod.git peapod
cd peapod
scripts/install.sh`),
		},
		{
			ID:          "host-preflight",
			Title:       "被管机器一键准备",
			Description: "在每台业务机上执行，完成基础信息检查、可选 Docker 安装、监控用户和 Pedpod 只读公钥写入。",
			Command:     fmt.Sprintf(`curl -fsSL https://raw.githubusercontent.com/tangfire/peapod/main/scripts/managed-host.sh | PEAPOD_MONITOR_PUBLIC_KEY='%s' PEAPOD_MANAGED_USER=peapod-monitor INSTALL_DOCKER=1 sh`, publicKey),
		},
		{
			ID:          "monitor-key",
			Title:       "写入 Pedpod 只读监控 SSH key",
			Description: "在被管机器的 SSH 用户下执行。这个 key 用于资源兜底读取，不进入前端。",
			Command: fmt.Sprintf(`mkdir -p ~/.ssh
chmod 700 ~/.ssh
grep -qxF '%s' ~/.ssh/authorized_keys 2>/dev/null || echo '%s' >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys`, publicKey, publicKey),
		},
		{
			ID:          "beszel-agent",
			Title:       "接入 Beszel agent",
			Description: "优先在 Beszel 页面创建系统并复制官方 agent 命令；Pedpod 负责展示接入状态和跳转。",
			Command:     fmt.Sprintf("# 打开 %s，在 Systems 里新增 %s，然后复制 Beszel 给出的 agent 命令到目标机器执行。", fallbackText(a.cfg.BeszelPublicURL, "Beszel"), firstHost),
		},
		{
			ID:          "logs-agent",
			Title:       "接入日志采集 agent",
			Description: "轻量模式先用 Dozzle 看 Docker 已保留日志并实时跟随；需要跨机器历史检索时，业务机再跑采集端推到运维机 Loki。",
			Command: strings.TrimSpace(`# 推荐使用 Grafana Alloy / Promtail / Vector
# 采集：Docker logs、Caddy/Nginx logs、应用结构化日志
# 推送：中心 Loki
# 完成后在 Grafana 里按 host / project / container 查询。`),
		},
		{
			ID:          "backup",
			Title:       "备份 Pedpod",
			Description: "升级或迁移前执行。默认备份配置、任务、审计和数据库 dump，不把 SSH 私钥打进备份包。",
			Command:     "scripts/backup.sh",
		},
		{
			ID:          "upgrade",
			Title:       "升级 Pedpod",
			Description: "先体检、自动备份，再拉取更新、构建并验证健康检查。",
			Command:     "scripts/upgrade.sh",
		},
	}
}

func setupDocLinks() []SetupDocLink {
	return []SetupDocLink{
		{Title: "运维架构", Description: "Pedpod、Woodpecker、Beszel、Dozzle、Grafana/Loki 和业务机的关系。", Path: "docs/ops-architecture.md"},
		{Title: "组件方案", Description: "如何选择轻量方案或完整观测方案。", Path: "docs/component-profiles.md"},
		{Title: "迁移 Runbook", Description: "把 Pedpod 迁到专用运维/构建机的步骤和验收项。", Path: "docs/migration-runbook.md"},
	}
}

func validateRuntimeConfig(cfg RuntimeConfigFile) error {
	for label, value := range map[string]string{
		"Pedpod URL":           cfg.PublicURL,
		"Woodpecker Server":    cfg.WoodpeckerServer,
		"Woodpecker PublicURL": cfg.WoodpeckerPublicURL,
		"Beszel BaseURL":       cfg.BeszelBaseURL,
		"Beszel PublicURL":     cfg.BeszelPublicURL,
		"Dozzle BaseURL":       cfg.DozzleBaseURL,
		"Dozzle PublicURL":     cfg.DozzlePublicURL,
		"Grafana PublicURL":    cfg.GrafanaPublicURL,
		"Alert Webhook URL":    cfg.AlertWebhookURL,
	} {
		if value != "" && !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			return fmt.Errorf("%s 必须以 http:// 或 https:// 开头", label)
		}
	}
	if normalizeLogStrategy(cfg.LogStrategy) == "" {
		return errors.New("日志策略只支持 lightweight / observability / external")
	}
	if strings.TrimSpace(cfg.DockerLogMaxSize) == "" || strings.TrimSpace(cfg.DockerLogMaxFile) == "" {
		return errors.New("Docker 日志保留参数不能为空")
	}
	if cfg.MonitorCritDisk < cfg.MonitorWarnDisk {
		return errors.New("磁盘严重阈值不能小于提醒阈值")
	}
	return nil
}

func setupStatusFromBool(ok bool) string {
	if ok {
		return "ok"
	}
	return "warning"
}

func setupWoodpeckerMessage(cfg Config) string {
	missing := []string{}
	if cfg.WoodpeckerServer == "" {
		missing = append(missing, "内部地址")
	}
	if cfg.WoodpeckerPublicURL == "" {
		missing = append(missing, "公开入口")
	}
	if cfg.WoodpeckerToken == "" {
		missing = append(missing, "API token")
	}
	if len(missing) > 0 {
		return "缺少：" + strings.Join(missing, "、")
	}
	return fmt.Sprintf("内部 %s，入口 %s", cfg.WoodpeckerServer, cfg.WoodpeckerPublicURL)
}

func setupBeszelMessage(cfg Config) string {
	missing := []string{}
	if cfg.BeszelBaseURL == "" {
		missing = append(missing, "内部地址")
	}
	if cfg.BeszelPublicURL == "" {
		missing = append(missing, "公开入口")
	}
	if cfg.BeszelEmail == "" || cfg.BeszelPassword == "" {
		missing = append(missing, "API 登录账号")
	}
	if len(missing) > 0 {
		return "缺少：" + strings.Join(missing, "、")
	}
	return fmt.Sprintf("内部 %s，入口 %s", cfg.BeszelBaseURL, cfg.BeszelPublicURL)
}

func readMonitorPublicKey(privateKeyPath string) string {
	path := strings.TrimSpace(privateKeyPath)
	if path == "" {
		return ""
	}
	for _, candidate := range []string{path + ".pub", strings.TrimSuffix(path, filepath.Ext(path)) + ".pub"} {
		payload, err := os.ReadFile(candidate)
		if err == nil {
			return strings.TrimSpace(string(payload))
		}
	}
	return ""
}

func ternaryText(ok bool, yes string, no string) string {
	if ok {
		return yes
	}
	return no
}

func taskWithAccessDefaults(task Task) Task {
	if len(task.AllowedRoles) == 0 && taskRequiresAdmin(task) {
		task.AllowedRoles = []string{"admin"}
	}
	return taskWithVerificationGuard(task)
}

func taskWithVerificationGuard(task Task) Task {
	if deploymentTaskRequiresVerification(task) && !taskHasDeploymentVerification(task) {
		task.Disabled = true
		task.DisabledReason = "部署任务缺少版本 marker 或 healthz 验证配置"
	}
	return task
}

func taskRequiresAdmin(task Task) bool {
	if task.Risk == "danger" {
		return true
	}
	action := variableValue(task.Variables, "DEPLOY_ACTION")
	target := variableValue(task.Variables, "DEPLOY_TARGET")
	if strings.Contains(strings.ToLower(action), "production") || strings.Contains(strings.ToLower(action), "observability") || strings.Contains(strings.ToLower(action), "peapod") || strings.Contains(strings.ToLower(action), "zephyr") || strings.Contains(strings.ToLower(action), "zefire") || target == "production" || target == "prod" {
		return true
	}
	return false
}

func canRunTask(user AuthUser, task Task) bool {
	if task.Disabled {
		return false
	}
	roles := taskWithAccessDefaults(task).AllowedRoles
	if len(roles) == 0 {
		return true
	}
	for _, role := range roles {
		if strings.EqualFold(strings.TrimSpace(role), user.Role) {
			return true
		}
	}
	return false
}

func taskForbiddenMessage(task Task) string {
	if task.Disabled && task.DisabledReason != "" {
		return task.DisabledReason
	}
	if taskRequiresAdmin(task) {
		return "这个动作会影响生产环境，只允许管理员执行"
	}
	return "当前账号没有权限执行这个动作"
}

func deploymentTaskRequiresVerification(task Task) bool {
	if task.ExternalURL != "" || task.RepoID <= 0 {
		return false
	}
	action := strings.ToLower(strings.TrimSpace(variableValue(task.Variables, "DEPLOY_ACTION")))
	switch action {
	case "deploy", "rollback", "release":
		return true
	}
	if isMaintenanceAction(action) {
		return false
	}
	return strings.TrimSpace(firstNonEmptyString(
		variableValue(task.Variables, "PEAPOD_PROJECT_ID"),
		variableValue(task.Variables, "ZEPHYR_PROJECT_ID"),
	)) != ""
}

func taskHasDeploymentVerification(task Task) bool {
	return deploymentVerifyConfigFromVariables(task.Variables).hasChecks()
}

type ExternalLinkConfig struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Group       string `json:"group"`
}

func (a *App) configuredLinks() map[string]string {
	links := map[string]string{}
	addLink := func(key string, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			links[key] = value
		}
	}
	addLink("peapod", a.cfg.PublicURL)
	addLink("woodpecker", a.cfg.WoodpeckerPublicURL)
	addLink("grafana", a.cfg.GrafanaPublicURL)
	addLink("beszel", a.cfg.BeszelPublicURL)
	addLink("dozzle", a.cfg.DozzlePublicURL)
	for _, link := range a.extraExternalLinks() {
		id := normalizeTaskID(link.ID)
		if id == "" {
			id = normalizeTaskID(link.Title)
		}
		if id != "" && link.URL != "" {
			links[id] = link.URL
		}
	}
	return links
}

func (a *App) externalLinkTasks() []Task {
	links := []Task{}
	add := func(id string, title string, description string, url string) {
		url = strings.TrimSpace(url)
		if url == "" {
			return
		}
		links = append(links, Task{
			ID:          id,
			Group:       "基础设施入口",
			Title:       title,
			Description: description,
			Risk:        "link",
			Disabled:    true,
			ExternalURL: url,
			Builtin:     true,
		})
	}
	add("peapod-open", "打开 Pedpod", "回到运维驾驶舱入口。", a.cfg.PublicURL)
	add("woodpecker-open", "打开 Woodpecker", "查看完整流水线、日志和仓库配置。", a.cfg.WoodpeckerPublicURL)
	add("dozzle-open", "打开 Dozzle", "轻量查看本机 Docker 已保留日志并实时跟随，不落地集中日志库。", a.cfg.DozzlePublicURL)
	add("grafana-open", "打开 Grafana", "查看日志、指标、链路和仪表盘。", a.cfg.GrafanaPublicURL)
	add("beszel-open", "打开 Beszel", "查看机器资源、磁盘、Docker 容器和资源曲线。", a.cfg.BeszelPublicURL)
	for _, link := range a.extraExternalLinks() {
		id := normalizeTaskID(link.ID)
		if id == "" {
			id = normalizeTaskID(link.Title)
		}
		if id == "" || strings.TrimSpace(link.URL) == "" {
			continue
		}
		links = append(links, Task{
			ID:          id,
			Group:       fallbackText(strings.TrimSpace(link.Group), "基础设施入口"),
			Title:       fallbackText(strings.TrimSpace(link.Title), id),
			Description: strings.TrimSpace(link.Description),
			Risk:        "link",
			Disabled:    true,
			ExternalURL: strings.TrimSpace(link.URL),
			Builtin:     true,
		})
	}
	return links
}

func (a *App) extraExternalLinks() []ExternalLinkConfig {
	raw := strings.TrimSpace(a.cfg.ExternalLinksJSON)
	if raw == "" {
		return nil
	}
	var rows []ExternalLinkConfig
	if err := json.Unmarshal([]byte(raw), &rows); err == nil {
		return rows
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		zap.L().Warn("parse PEAPOD_LINKS_JSON failed", zap.String("event", "peapod_links_parse_failed"), zap.Error(err))
		return nil
	}
	rows = make([]ExternalLinkConfig, 0, len(values))
	for key, url := range values {
		rows = append(rows, ExternalLinkConfig{ID: key, Title: key, URL: url})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

func normalizeExternalLinks(rows []ExternalLinkConfig) []ExternalLinkConfig {
	out := []ExternalLinkConfig{}
	seen := map[string]bool{}
	for _, row := range rows {
		row.ID = normalizeTaskID(row.ID)
		row.Title = strings.TrimSpace(row.Title)
		row.URL = cleanURL(row.URL)
		row.Description = strings.TrimSpace(row.Description)
		row.Group = strings.TrimSpace(row.Group)
		if row.ID == "" {
			row.ID = normalizeTaskID(row.Title)
		}
		if row.ID == "" || row.URL == "" || seen[row.ID] {
			continue
		}
		if row.Title == "" {
			row.Title = row.ID
		}
		seen[row.ID] = true
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group == out[j].Group {
			return out[i].Title < out[j].Title
		}
		return out[i].Group < out[j].Group
	})
	return out
}

func (a *App) configuredTasks() []Task {
	baseTasks := append([]Task{}, tasks...)
	baseTasks = append(baseTasks, a.externalLinkTasks()...)
	out := make([]Task, 0, len(baseTasks))
	indexByID := map[string]int{}
	for _, task := range baseTasks {
		task.Builtin = true
		task.Custom = false
		task.Overridden = false
		task = taskWithAccessDefaults(task)
		indexByID[task.ID] = len(out)
		out = append(out, task)
	}
	custom, err := a.loadCustomTaskConfig()
	if err != nil {
		zap.L().Warn("load custom tasks failed", zap.String("event", "custom_tasks_load_failed"), zap.Error(err))
		return out
	}
	for _, task := range custom.Tasks {
		task.ID = strings.TrimSpace(task.ID)
		task.Title = strings.TrimSpace(task.Title)
		if task.ID == "" || task.Title == "" || (task.RepoID <= 0 && strings.TrimSpace(task.ExternalURL) == "") {
			continue
		}
		if task.Group == "" {
			task.Group = "自定义任务"
		}
		if task.Branch == "" {
			task.Branch = "main"
		}
		if task.Risk == "" {
			task.Risk = "normal"
		}
		if task.RepoName == "" {
			task.RepoName = custom.Repos[task.RepoID]
		}
		if index, exists := indexByID[task.ID]; exists {
			task.Builtin = true
			task.Custom = false
			task.Overridden = true
			task = taskWithAccessDefaults(task)
			out[index] = task
			continue
		}
		task.Builtin = false
		task.Custom = true
		task.Overridden = false
		task = taskWithAccessDefaults(task)
		indexByID[task.ID] = len(out)
		out = append(out, task)
	}
	return out
}

func (a *App) configuredRepos() map[int]string {
	out := map[int]string{}
	for id, name := range repos {
		out[id] = name
	}
	custom, err := a.loadCustomTaskConfig()
	if err != nil {
		return out
	}
	for id, name := range custom.Repos {
		name = strings.TrimSpace(name)
		if id > 0 && name != "" {
			out[id] = name
		}
	}
	for _, task := range custom.Tasks {
		if task.RepoID <= 0 {
			continue
		}
		name := strings.TrimSpace(task.RepoName)
		if name == "" {
			name = strings.TrimSpace(custom.Repos[task.RepoID])
		}
		if name == "" {
			name = fmt.Sprintf("Repo %d", task.RepoID)
		}
		out[task.RepoID] = name
	}
	return out
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
