package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

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
	bindSelectedSourceBranch(task, variables, branch)
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
	responseTask.Variables = variables
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
	customTask := Task{ID: "custom-run", Group: "高级触发", Title: "高级触发", RepoID: req.RepoID, RepoName: strings.TrimSpace(req.RepoName), Branch: branch, Variables: variables}
	bindSelectedSourceBranch(customTask, variables, branch)
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
		Task:        customTask,
		Pipeline:    pipeline,
		Woodpecker:  a.pipelineURL(req.RepoID, pipeline.Number),
		TriggeredAt: time.Now().Format(time.RFC3339),
	})
}

func bindSelectedSourceBranch(task Task, variables map[string]string, branch string) {
	branch = strings.TrimSpace(branch)
	if branch == "" || variables == nil || !deploymentTaskRequiresVerification(task) {
		return
	}
	for key := range variables {
		if strings.EqualFold(strings.TrimSpace(key), "SOURCE_BRANCH") {
			delete(variables, key)
		}
	}
	variables["SOURCE_BRANCH"] = branch
	variables["PEAPOD_REQUESTED_BRANCH"] = branch
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
