package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

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
	successHistoryByTarget := map[string][]DeploymentStatus{}
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
				status.LatestBranch = fallbackText(deploymentSourceBranch(pipeline), "-")
				status.LatestCommit = deploymentCommitFromPipeline(pipeline)
				status.LatestAt = pipelineActivityAt(pipeline)
				status.LatestPipeline = pipeline.Number
				status.LatestTriggeredBy = pipelineActor(pipeline)
			}
			if pipeline.Status == "success" {
				historyCandidate := deploymentStatusFromPipeline(target, repoID, repoName, configuredBranch, pipeline)
				successHistoryByTarget[target.ID] = append(successHistoryByTarget[target.ID], historyCandidate)
				candidate := historyCandidate
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
		successHistory := successHistoryByTarget[id]
		sort.SliceStable(successHistory, func(i, j int) bool {
			return successHistory[i].LastDeployedAt > successHistory[j].LastDeployedAt
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
		}
		if len(successHistory) > 0 {
			status.Revisions = deploymentRevisions(successHistory, 12)
		}
		if previous, ok := previousDeploymentRevision(successHistory, status.CurrentCommit, status.Pipeline); ok {
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
		CurrentBranch:    fallbackText(deploymentSourceBranch(pipeline), "-"),
		CurrentCommit:    deploymentCommitFromPipeline(pipeline),
		LastAction:       deploymentActionText(repoID, repoName, pipeline),
		LastStatus:       pipeline.Status,
		LastDeployedAt:   pipelineFinishedAt(pipeline),
		Pipeline:         pipeline.Number,
		TriggeredBy:      pipelineActor(pipeline),
		TriggeredAt:      pipeline.PedpodTriggeredAt,
		Variables:        sanitizeVariables(pipeline.Variables),
	}
}

func deploymentCommitFromPipeline(pipeline Pipeline) string {
	variables := pipeline.Variables
	if isRollbackPipeline(pipeline) {
		for _, key := range []string{
			"PEAPOD_ROLLBACK_COMMIT",
			"ROLLBACK_COMMIT",
			"ROLLBACK_VERSION",
			"ROLLBACK_SHA",
			"TARGET_SHA",
			"TARGET_COMMIT",
			"DEPLOY_COMMIT",
		} {
			if commit := normalizeDeploymentCommit(variableValue(variables, key)); commit != "" {
				return commit
			}
		}
		if commit := commitFromImageTag(variableValue(variables, "IMAGE_TAG")); commit != "" {
			return commit
		}
	}
	if commit := normalizeDeploymentCommit(variableValue(variables, "PEAPOD_DEPLOY_COMMIT")); commit != "" {
		return commit
	}
	return strings.TrimSpace(pipeline.Commit)
}

func deploymentSourceBranch(pipeline Pipeline) string {
	return firstNonEmptyString(
		variableValue(pipeline.Variables, "SOURCE_BRANCH"),
		variableValue(pipeline.Variables, "PEAPOD_REQUESTED_BRANCH"),
		pipeline.Branch,
	)
}

func isRollbackPipeline(pipeline Pipeline) bool {
	variables := pipeline.Variables
	action := strings.ToLower(strings.TrimSpace(variableValue(variables, "DEPLOY_ACTION")))
	if action == "rollback" {
		return true
	}
	if truthyVariable(variableValue(variables, "ROLLBACK_TO_PREVIOUS")) {
		return true
	}
	for _, key := range []string{"ROLLBACK_COMMIT", "ROLLBACK_VERSION", "PEAPOD_ROLLBACK_COMMIT"} {
		if strings.TrimSpace(variableValue(variables, key)) != "" {
			return true
		}
	}
	return false
}

func truthyVariable(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func normalizeDeploymentCommit(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "refs/heads/")
	value = strings.TrimPrefix(value, "origin/")
	value = strings.Trim(value, `"'`)
	if looksLikeCommit(value) {
		return value
	}
	return ""
}

func commitFromImageTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	for _, part := range strings.FieldsFunc(tag, func(r rune) bool {
		return r == '-' || r == '_' || r == ':' || r == '/'
	}) {
		if commit := normalizeDeploymentCommit(part); commit != "" && containsHexLetter(commit) {
			return commit
		}
	}
	return ""
}

func looksLikeCommit(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 7 || len(value) > 40 {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func containsHexLetter(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			return true
		}
	}
	return false
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

func previousDeploymentRevision(rows []DeploymentStatus, currentCommit string, currentPipeline int64) (DeploymentStatus, bool) {
	currentCommit = strings.TrimSpace(currentCommit)
	for _, row := range rows {
		if currentPipeline > 0 && row.Pipeline == currentPipeline {
			continue
		}
		commit := strings.TrimSpace(row.CurrentCommit)
		if currentCommit != "" && commit != "" && deploymentCommitMatches(currentCommit, commit) {
			continue
		}
		return row, true
	}
	return DeploymentStatus{}, false
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
	sourceBranch := deploymentSourceBranch(pipeline)
	for _, task := range tasks {
		if task.RepoID != repoID || task.ExternalURL != "" {
			continue
		}
		if task.Branch != "" && sourceBranch != "" && task.Branch != sourceBranch {
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
			status.DeployVerified = false
			status.DeployDegraded = true
			status.DeployVerifyStatus = "marker_unavailable"
			status.DeployVerifyMessage = "服务健康检查已通过，但无法确认本次部署版本；" + strings.Join(markerIssues, "；")
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
		status.DeployVerified = false
		status.DeployDegraded = true
		status.DeployVerifyStatus = "health_only"
		status.DeployVerifyMessage = "服务健康检查已通过，但没有版本 marker，无法确认本次部署的分支和 commit"
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
