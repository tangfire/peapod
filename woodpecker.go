package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

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
		_ = a.writeAudit(buildAuditRecord(user, r, "woodpecker-repo-activate", "启用 Woodpecker 仓库", repo.ID, "", 0, map[string]string{"repo": repo.FullName, "forge_remote_id": repo.ForgeRemoteID}))
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
		_ = a.writeAudit(buildAuditRecord(user, r, "woodpecker-repo-save", "保存 Pedpod 仓库映射", req.RepoID, "", 0, map[string]string{"repo": strings.TrimSpace(req.RepoName)}))
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
		_, err := a.upsertTaskIntoConfig(task)
		if err != nil {
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
	record := buildAuditRecord(user, r, "cancel-pipeline", "取消流水线", repoID, "", number, map[string]string{})
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
