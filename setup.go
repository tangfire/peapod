package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

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
		MonitorAutoCleanupLevel: a.cfg.MonitorAutoCleanupLevel,
		MonitorAutoCleanupDisk:  a.cfg.MonitorAutoCleanupDisk,
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
		return DoctorCheck{ID: id, Title: title, Status: "warning", Severity: "warning", Message: message, Fix: "运行 docker system prune 或通过 Pedpod 磁盘清理任务回收空间"}
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
	_ = a.writeAudit(buildAuditRecord(user, r, "doctor-run", "运行 Pedpod 体检", 0, "", 0, map[string]string{"readiness": doctor.Readiness}))
	writeJSON(w, doctor)
}

