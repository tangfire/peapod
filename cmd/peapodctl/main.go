package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
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

const (
	defaultBaseURL      = "http://127.0.0.1:8095"
	sessionCookieName   = "peapod_session"
	defaultPollInterval = 5 * time.Second
	defaultWaitTimeout  = 45 * time.Minute
)

type commonOptions struct {
	baseURL     string
	username    string
	password    string
	sessionFile string
	insecureTLS bool
}

type client struct {
	baseURL     string
	username    string
	password    string
	sessionFile string
	session     sessionCache
	httpClient  *http.Client
}

type sessionCache struct {
	BaseURL     string    `json:"base_url"`
	CookieName  string    `json:"cookie_name"`
	CookieValue string    `json:"cookie_value"`
	SavedAt     time.Time `json:"saved_at"`
}

type apiErrorPayload struct {
	Error   string   `json:"error"`
	Details []string `json:"details,omitempty"`
}

type apiError struct {
	Status  int
	Message string
	Details []string
}

func (e apiError) Error() string {
	if len(e.Details) == 0 {
		return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("HTTP %d: %s (%s)", e.Status, e.Message, strings.Join(e.Details, "；"))
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type stateResponse struct {
	Tasks              []task             `json:"tasks"`
	Pipelines          map[int][]pipeline `json:"pipelines"`
	DeploymentStatuses []deploymentStatus `json:"deployment_statuses"`
	Repos              map[int]string     `json:"repos"`
	Now                string             `json:"now"`
}

type task struct {
	ID          string            `json:"id"`
	Group       string            `json:"group"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	RepoID      int               `json:"repo_id"`
	RepoName    string            `json:"repo_name,omitempty"`
	Branch      string            `json:"branch"`
	Variables   map[string]string `json:"variables"`
	Risk        string            `json:"risk"`
	ConfirmText string            `json:"confirm_text,omitempty"`
	Disabled    bool              `json:"disabled,omitempty"`
}

type runRequest struct {
	Inputs map[string]string `json:"inputs"`
	Branch string            `json:"branch"`
}

type runResponse struct {
	OK          bool     `json:"ok"`
	Task        task     `json:"task"`
	Pipeline    pipeline `json:"pipeline"`
	Woodpecker  string   `json:"woodpecker_url"`
	TriggeredAt string   `json:"triggered_at"`
}

type pipeline struct {
	Number    int64             `json:"number"`
	Status    string            `json:"status"`
	Commit    string            `json:"commit"`
	Branch    string            `json:"branch"`
	Created   int64             `json:"created"`
	Started   int64             `json:"started"`
	Finished  int64             `json:"finished"`
	Message   string            `json:"message"`
	Variables map[string]string `json:"variables,omitempty"`
	RepoID    int               `json:"repo_id,omitempty"`
	RepoName  string            `json:"repo_name,omitempty"`
}

type pipelineStep struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	State    string `json:"state"`
	Error    string `json:"error,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Started  int64  `json:"started,omitempty"`
	Finished int64  `json:"finished,omitempty"`
}

type pipelineSummary struct {
	Pipeline       pipeline       `json:"pipeline"`
	Steps          []pipelineStep `json:"steps"`
	FailureSummary string         `json:"failure_summary,omitempty"`
	LogTail        []string       `json:"log_tail"`
	WoodpeckerURL  string         `json:"woodpecker_url"`
}

type deploymentStatus struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Group               string            `json:"group"`
	RepoID              int               `json:"repo_id"`
	RepoName            string            `json:"repo_name"`
	ConfiguredBranch    string            `json:"configured_branch"`
	CurrentBranch       string            `json:"current_branch"`
	CurrentCommit       string            `json:"current_commit"`
	LastAction          string            `json:"last_action"`
	LastStatus          string            `json:"last_status"`
	LastDeployedAt      int64             `json:"last_deployed_at"`
	Pipeline            int64             `json:"pipeline"`
	Variables           map[string]string `json:"variables,omitempty"`
	DeployVerified      bool              `json:"deploy_verified"`
	DeployDegraded      bool              `json:"deploy_degraded,omitempty"`
	DeployVerifyStatus  string            `json:"deploy_verify_status,omitempty"`
	DeployVerifyMessage string            `json:"deploy_verify_message,omitempty"`
	ActualCommit        string            `json:"actual_commit,omitempty"`
	HealthURL           string            `json:"health_url,omitempty"`
	LatestStatus        string            `json:"latest_status,omitempty"`
	LatestCommit        string            `json:"latest_commit,omitempty"`
	LatestPipeline      int64             `json:"latest_pipeline,omitempty"`
}

type keyValueFlags map[string]string

func (f *keyValueFlags) String() string {
	if f == nil || len(*f) == 0 {
		return ""
	}
	keys := make([]string, 0, len(*f))
	for key := range *f {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+(*f)[key])
	}
	return strings.Join(parts, ",")
}

func (f *keyValueFlags) Set(value string) error {
	key, parsed, err := parseKeyValue(value)
	if err != nil {
		return err
	}
	if *f == nil {
		*f = map[string]string{}
	}
	(*f)[key] = parsed
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "peapodctl:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stdout)
		return nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	case "login":
		return commandLogin(args[1:])
	case "tasks":
		return commandTasks(args[1:])
	case "status":
		return commandStatus(args[1:])
	case "run":
		return commandRun(args[1:], false)
	case "deploy":
		return commandRun(args[1:], true)
	case "local-run":
		return commandLocalRun(args[1:], false)
	case "local-deploy":
		return commandLocalRun(args[1:], true)
	case "summary":
		return commandSummary(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  peapodctl login [flags]
  peapodctl tasks [flags]
  peapodctl status [task-id] [flags]
  peapodctl run <task-id> [--branch dev] [--input KEY=VALUE] [--wait] [flags]
  peapodctl deploy <task-id> [--branch dev] [--input KEY=VALUE] [flags]
  peapodctl local-run <task-id> [--dir /opt/peapod] [--branch dev] [--wait]
  peapodctl local-deploy <task-id> [--dir /opt/peapod] [--branch dev]
  peapodctl summary <repo-id> <pipeline-number> [flags]

Common flags:
  --url            Peapod base URL (env PEAPOD_URL, default http://127.0.0.1:8095)
  --username       Login username/email (env PEAPOD_USERNAME, default admin when password is set)
  --password       Login password (env PEAPOD_PASSWORD)
  --session-file   Session cache path (env PEAPOD_SESSION_FILE)
  --insecure-tls   Skip TLS certificate verification

Examples:
  PEAPOD_URL=https://deploy.novelcat.cloud peapodctl tasks
  PEAPOD_URL=https://deploy.novelcat.cloud PEAPOD_PASSWORD=... peapodctl login
  peapodctl deploy xzm-test-deploy --branch dev --timeout 45m
  peapodctl local-deploy xzm-test-deploy --dir /opt/peapod --branch dev --timeout 45m
  peapodctl run peapod-deploy --branch main --wait --timeout 30m`)
}

func commandLogin(args []string) error {
	opts := defaultCommonOptions()
	fs := newFlagSet("login", &opts)
	if err := parseInterspersedFlags(fs, args); err != nil {
		return err
	}
	c, err := newClient(opts)
	if err != nil {
		return err
	}
	if err := c.login(context.Background()); err != nil {
		return err
	}
	fmt.Printf("logged in to %s; session cached at %s\n", c.baseURL, c.sessionFile)
	return nil
}

func commandTasks(args []string) error {
	opts := defaultCommonOptions()
	var jsonOutput bool
	var groupFilter string
	fs := newFlagSet("tasks", &opts)
	fs.BoolVar(&jsonOutput, "json", false, "print raw JSON")
	fs.StringVar(&groupFilter, "group", "", "filter by task group")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return err
	}
	c, err := newClient(opts)
	if err != nil {
		return err
	}
	state, err := c.state(context.Background())
	if err != nil {
		return err
	}
	rows := state.Tasks
	if groupFilter != "" {
		filtered := rows[:0]
		for _, row := range rows {
			if strings.Contains(strings.ToLower(row.Group), strings.ToLower(groupFilter)) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	if jsonOutput {
		return printJSON(rows)
	}
	if len(rows) == 0 {
		fmt.Println("no tasks")
		return nil
	}
	fmt.Printf("%-32s  %-12s  %-28s  %-8s  %s\n", "ID", "BRANCH", "GROUP", "RISK", "TITLE")
	for _, row := range rows {
		disabled := ""
		if row.Disabled {
			disabled = " disabled"
		}
		fmt.Printf("%-32s  %-12s  %-28s  %-8s  %s%s\n", row.ID, fallback(row.Branch, "main"), truncate(row.Group, 28), fallback(row.Risk, "-"), row.Title, disabled)
	}
	return nil
}

func commandStatus(args []string) error {
	opts := defaultCommonOptions()
	var jsonOutput bool
	fs := newFlagSet("status", &opts)
	fs.BoolVar(&jsonOutput, "json", false, "print raw JSON")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return errors.New("status accepts at most one task id")
	}
	c, err := newClient(opts)
	if err != nil {
		return err
	}
	state, err := c.state(context.Background())
	if err != nil {
		return err
	}
	if fs.NArg() == 0 {
		if jsonOutput {
			return printJSON(state.DeploymentStatuses)
		}
		printDeploymentStatuses(state.DeploymentStatuses)
		return nil
	}
	taskID := fs.Arg(0)
	row, ok := findTask(state.Tasks, taskID)
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}
	targetID := deploymentTargetID(row)
	matches := []deploymentStatus{}
	for _, status := range state.DeploymentStatuses {
		if targetID != "" && status.ID == targetID {
			matches = append(matches, status)
		}
	}
	if jsonOutput {
		return printJSON(matches)
	}
	if len(matches) == 0 {
		fmt.Printf("task %s has no deployment status yet\n", taskID)
		return nil
	}
	printDeploymentStatuses(matches)
	return nil
}

func commandRun(args []string, deployMode bool) error {
	opts := defaultCommonOptions()
	var inputs keyValueFlags
	var branch string
	var wait bool
	var jsonOutput bool
	var timeout time.Duration
	var pollInterval time.Duration
	var noVerify bool
	var confirm string
	fs := newFlagSet("run", &opts)
	if deployMode {
		fs = newFlagSet("deploy", &opts)
		wait = true
	}
	fs.StringVar(&branch, "branch", "", "source branch")
	fs.Var(&inputs, "input", "task input as KEY=VALUE; repeatable")
	fs.BoolVar(&wait, "wait", wait, "wait for pipeline completion")
	fs.DurationVar(&timeout, "timeout", defaultWaitTimeout, "wait timeout")
	fs.DurationVar(&pollInterval, "poll", defaultPollInterval, "poll interval")
	fs.BoolVar(&jsonOutput, "json", false, "print raw JSON")
	fs.BoolVar(&noVerify, "no-verify", false, "for deploy: do not require Peapod deployment verification")
	fs.StringVar(&confirm, "confirm", "", "required confirmation text for guarded tasks")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("task id is required")
	}
	taskID := fs.Arg(0)
	c, err := newClient(opts)
	if err != nil {
		return err
	}
	state, err := c.state(context.Background())
	if err != nil {
		return err
	}
	taskRow, ok := findTask(state.Tasks, taskID)
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}
	if taskRow.ConfirmText != "" && strings.TrimSpace(confirm) != taskRow.ConfirmText {
		return fmt.Errorf("task %q requires --confirm %q", taskID, taskRow.ConfirmText)
	}
	response, err := c.runTask(context.Background(), taskID, branch, inputs)
	if err != nil {
		return err
	}
	if jsonOutput && !wait {
		return printJSON(response)
	}
	if !jsonOutput {
		fmt.Printf("triggered %s -> pipeline #%d (%s)\n", response.Task.ID, response.Pipeline.Number, fallback(response.Woodpecker, "Woodpecker URL unavailable"))
	}
	if !wait {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	summary, err := c.waitPipeline(ctx, response.Task.RepoID, response.Pipeline.Number, pollInterval)
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(summary)
	}
	if summary.Pipeline.Status != "success" {
		printPipelineFailure(summary)
		return fmt.Errorf("pipeline #%d finished with status %s", response.Pipeline.Number, summary.Pipeline.Status)
	}
	fmt.Printf("pipeline #%d succeeded (%s)\n", response.Pipeline.Number, shortCommit(summary.Pipeline.Commit))
	if deployMode && !noVerify {
		status, err := c.waitDeploymentVerified(ctx, response.Task, summary.Pipeline, pollInterval)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(status)
		}
		suffix := fallback(status.DeployVerifyMessage, "verified")
		if status.DeployDegraded && !status.DeployVerified {
			suffix = "degraded · " + suffix
		}
		fmt.Printf("deployment ready: %s %s (%s)\n", status.Name, shortCommit(fallback(status.ActualCommit, status.CurrentCommit)), suffix)
	}
	return nil
}

func commandLocalRun(args []string, deployMode bool) error {
	var inputs keyValueFlags
	var branch string
	var dir string
	var server string
	var token string
	var wait bool
	var jsonOutput bool
	var timeout time.Duration
	var pollInterval time.Duration
	var noVerify bool
	var confirm string
	fs := flag.NewFlagSet("local-run", flag.ContinueOnError)
	if deployMode {
		fs = flag.NewFlagSet("local-deploy", flag.ContinueOnError)
		wait = true
	}
	fs.SetOutput(os.Stderr)
	fs.StringVar(&dir, "dir", firstNonEmpty(os.Getenv("PEAPOD_DIR"), "/opt/peapod"), "Peapod deploy directory")
	fs.StringVar(&branch, "branch", "", "source branch")
	fs.Var(&inputs, "input", "task input as KEY=VALUE; repeatable")
	fs.StringVar(&server, "woodpecker-server", os.Getenv("WOODPECKER_SERVER"), "Woodpecker API URL")
	fs.StringVar(&token, "woodpecker-token", os.Getenv("WOODPECKER_TOKEN"), "Woodpecker API token")
	fs.BoolVar(&wait, "wait", wait, "wait for pipeline completion")
	fs.DurationVar(&timeout, "timeout", defaultWaitTimeout, "wait timeout")
	fs.DurationVar(&pollInterval, "poll", defaultPollInterval, "poll interval")
	fs.BoolVar(&jsonOutput, "json", false, "print raw JSON")
	fs.BoolVar(&noVerify, "no-verify", false, "for local-deploy: do not check marker/health")
	fs.StringVar(&confirm, "confirm", "", "required confirmation text for guarded tasks")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("task id is required")
	}
	taskID := fs.Arg(0)
	local, err := loadLocalPeapod(dir)
	if err != nil {
		return err
	}
	taskRow, ok := findTask(local.Tasks, taskID)
	if !ok {
		return fmt.Errorf("task %q not found in %s", taskID, local.TasksPath)
	}
	if taskRow.Disabled {
		return fmt.Errorf("task %q is disabled", taskID)
	}
	if taskRow.ConfirmText != "" && strings.TrimSpace(confirm) != taskRow.ConfirmText {
		return fmt.Errorf("task %q requires --confirm %q", taskID, taskRow.ConfirmText)
	}
	if branch = strings.TrimSpace(branch); branch == "" {
		branch = firstNonEmpty(taskRow.Branch, "main")
	}
	if server = strings.TrimSpace(server); server == "" {
		server = local.WoodpeckerServer
	}
	if token = strings.TrimSpace(token); token == "" {
		token = local.WoodpeckerToken
	}
	if server == "" {
		return errors.New("Woodpecker server is missing; set --woodpecker-server or configure Peapod")
	}
	if token == "" {
		return errors.New("Woodpecker token is missing; set --woodpecker-token or configure Peapod")
	}
	server = normalizeLocalWoodpeckerServer(server)
	wc, err := newWoodpeckerClient(server, token)
	if err != nil {
		return err
	}
	variables := cloneStringMap(taskRow.Variables)
	for key, value := range inputs {
		variables[key] = value
	}
	bindLocalSourceBranch(taskRow, variables, branch)
	pipeline, err := wc.createPipeline(context.Background(), taskRow.RepoID, branch, variables)
	if err != nil {
		return err
	}
	pipeline.RepoID = taskRow.RepoID
	pipeline.RepoName = firstNonEmpty(taskRow.RepoName, local.Repos[taskRow.RepoID])
	if jsonOutput && !wait {
		return printJSON(pipeline)
	}
	if !jsonOutput {
		fmt.Printf("triggered local task %s -> pipeline #%d (%s branch=%s)\n", taskRow.ID, pipeline.Number, server, branch)
	}
	if !wait {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	summary, err := wc.waitPipeline(ctx, taskRow.RepoID, pipeline.Number, pollInterval)
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(summary)
	}
	if summary.Pipeline.Status != "success" {
		printPipelineFailure(summary)
		return fmt.Errorf("pipeline #%d finished with status %s", pipeline.Number, summary.Pipeline.Status)
	}
	fmt.Printf("pipeline #%d succeeded (%s)\n", pipeline.Number, shortCommit(summary.Pipeline.Commit))
	if deployMode && !noVerify {
		if err := waitLocalDeploymentReady(ctx, taskRow, summary.Pipeline, pollInterval); err != nil {
			return err
		}
	}
	return nil
}

func commandSummary(args []string) error {
	opts := defaultCommonOptions()
	var jsonOutput bool
	fs := newFlagSet("summary", &opts)
	fs.BoolVar(&jsonOutput, "json", false, "print raw JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("summary requires <repo-id> <pipeline-number>")
	}
	repoID, err := strconv.Atoi(fs.Arg(0))
	if err != nil || repoID <= 0 {
		return errors.New("repo-id must be a positive integer")
	}
	number, err := strconv.ParseInt(fs.Arg(1), 10, 64)
	if err != nil || number <= 0 {
		return errors.New("pipeline-number must be a positive integer")
	}
	c, err := newClient(opts)
	if err != nil {
		return err
	}
	summary, err := c.pipelineSummary(context.Background(), repoID, number)
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(summary)
	}
	printPipelineSummary(summary)
	return nil
}

type localPeapod struct {
	Dir              string
	TasksPath        string
	Tasks            []task
	Repos            map[int]string
	WoodpeckerServer string
	WoodpeckerToken  string
}

type localTaskConfig struct {
	Repos map[int]string `json:"repos,omitempty"`
	Tasks []task         `json:"tasks"`
}

type runtimeConfigFile struct {
	WoodpeckerServer string `json:"woodpecker_server,omitempty"`
	WoodpeckerToken  string `json:"woodpecker_token,omitempty"`
}

type woodpeckerClient struct {
	server     string
	token      string
	httpClient *http.Client
}

func loadLocalPeapod(dir string) (localPeapod, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "/opt/peapod"
	}
	envValues := map[string]string{}
	if values, err := readEnvFile(filepath.Join(dir, ".env")); err == nil {
		envValues = values
	}
	taskPath := firstExistingFile(
		envValues["PEAPOD_TASKS_PATH"],
		filepath.Join(dir, "data", "peapod", "tasks.json"),
		filepath.Join(dir, "data", "tasks.json"),
	)
	if taskPath == "" {
		return localPeapod{}, fmt.Errorf("tasks file not found under %s", dir)
	}
	var taskConfig localTaskConfig
	if err := readJSONFile(taskPath, &taskConfig); err != nil {
		return localPeapod{}, err
	}
	runtimeConfig := runtimeConfigFile{}
	_ = readJSONFile(firstExistingFile(
		envValues["PEAPOD_CONFIG_PATH"],
		filepath.Join(dir, "data", "peapod", "config.json"),
		filepath.Join(dir, "data", "config.json"),
	), &runtimeConfig)
	return localPeapod{
		Dir:              dir,
		TasksPath:        taskPath,
		Tasks:            taskConfig.Tasks,
		Repos:            taskConfig.Repos,
		WoodpeckerServer: firstNonEmpty(runtimeConfig.WoodpeckerServer, envValues["WOODPECKER_SERVER"], "http://127.0.0.1:8000"),
		WoodpeckerToken:  firstNonEmpty(runtimeConfig.WoodpeckerToken, envValues["WOODPECKER_TOKEN"]),
	}, nil
}

func readJSONFile(path string, out any) error {
	if strings.TrimSpace(path) == "" {
		return os.ErrNotExist
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func readEnvFile(path string) (map[string]string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(payload), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		values[key] = value
	}
	return values, nil
}

func firstExistingFile(paths ...string) string {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) && !strings.Contains(path, string(filepath.Separator)) {
			continue
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func normalizeLocalWoodpeckerServer(raw string) string {
	normalized, err := normalizeBaseURL(raw)
	if err != nil {
		return strings.TrimRight(strings.TrimSpace(raw), "/")
	}
	parsed, err := url.Parse(normalized)
	if err == nil && strings.EqualFold(parsed.Hostname(), "host.docker.internal") {
		port := parsed.Port()
		if port == "" {
			port = "8000"
		}
		parsed.Host = "127.0.0.1:" + port
		return strings.TrimRight(parsed.String(), "/")
	}
	return normalized
}

func newWoodpeckerClient(server, token string) (*woodpeckerClient, error) {
	server, err := normalizeBaseURL(server)
	if err != nil {
		return nil, err
	}
	return &woodpeckerClient{
		server:     server,
		token:      strings.TrimSpace(token),
		httpClient: &http.Client{Timeout: 3 * time.Minute},
	}, nil
}

func (c *woodpeckerClient) createPipeline(ctx context.Context, repoID int, branch string, variables map[string]string) (pipeline, error) {
	body := map[string]any{
		"branch":    branch,
		"variables": variables,
	}
	var out pipeline
	path := fmt.Sprintf("/api/repos/%d/pipelines", repoID)
	err := c.do(ctx, http.MethodPost, path, body, &out)
	return out, err
}

func (c *woodpeckerClient) pipeline(ctx context.Context, repoID int, number int64) (pipeline, error) {
	var out pipeline
	path := fmt.Sprintf("/api/repos/%d/pipelines/%d", repoID, number)
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	out.RepoID = repoID
	return out, err
}

func (c *woodpeckerClient) waitPipeline(ctx context.Context, repoID int, number int64, poll time.Duration) (pipelineSummary, error) {
	if poll <= 0 {
		poll = defaultPollInterval
	}
	var lastStatus string
	for {
		row, err := c.pipeline(ctx, repoID, number)
		if err != nil {
			return pipelineSummary{}, err
		}
		status := strings.ToLower(strings.TrimSpace(row.Status))
		if status != lastStatus {
			fmt.Printf("pipeline #%d status: %s\n", number, fallback(status, "unknown"))
			lastStatus = status
		}
		if terminalPipelineStatus(status) {
			return pipelineSummary{Pipeline: row, WoodpeckerURL: c.pipelineURL(repoID, number)}, nil
		}
		select {
		case <-ctx.Done():
			return pipelineSummary{}, ctx.Err()
		case <-time.After(poll):
		}
	}
}

func (c *woodpeckerClient) pipelineURL(repoID int, number int64) string {
	return fmt.Sprintf("%s/repos/%d/pipeline/%d", strings.TrimRight(c.server, "/"), repoID, number)
}

func (c *woodpeckerClient) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.server+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseAPIError(resp.StatusCode, payload)
	}
	if out == nil || len(strings.TrimSpace(string(payload))) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("decode Woodpecker response %s %s: %w", method, path, err)
	}
	return nil
}

func bindLocalSourceBranch(t task, variables map[string]string, branch string) {
	if variables == nil || strings.TrimSpace(branch) == "" || !localDeploymentTask(t) {
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

func localDeploymentTask(t task) bool {
	return variableValue(t.Variables, "PEAPOD_DEPLOY_MARKER_PATH") != "" ||
		variableValue(t.Variables, "PEAPOD_DEPLOY_VERIFY_URL") != "" ||
		variableValue(t.Variables, "PEAPOD_HEALTH_URL") != ""
}

func waitLocalDeploymentReady(ctx context.Context, t task, p pipeline, poll time.Duration) error {
	markerPath := variableValue(t.Variables, "PEAPOD_DEPLOY_MARKER_PATH")
	healthURL := firstNonEmpty(variableValue(t.Variables, "PEAPOD_DEPLOY_VERIFY_URL"), variableValue(t.Variables, "PEAPOD_HEALTH_URL"))
	if markerPath == "" && healthURL == "" {
		return errors.New("local deployment verification needs PEAPOD_DEPLOY_MARKER_PATH or PEAPOD_DEPLOY_VERIFY_URL")
	}
	if poll <= 0 {
		poll = defaultPollInterval
	}
	for {
		markerOK, markerMessage := localMarkerOK(markerPath, p.Commit)
		healthOK, healthMessage := localHealthOK(ctx, healthURL)
		ready := true
		if markerPath != "" && !markerOK {
			ready = false
		}
		if healthURL != "" && !healthOK {
			ready = false
		}
		if ready {
			parts := []string{}
			if markerPath != "" {
				parts = append(parts, "marker "+markerMessage)
			}
			if healthURL != "" {
				parts = append(parts, "health "+healthMessage)
			}
			fmt.Printf("deployment ready: %s\n", strings.Join(parts, "；"))
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("deployment verification timeout: marker=%s health=%s", markerMessage, healthMessage)
		case <-time.After(poll):
		}
	}
}

func localMarkerOK(path, commit string) (bool, string) {
	if path == "" {
		return true, "not configured"
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return false, err.Error()
	}
	actual := strings.TrimSpace(string(payload))
	if actual == "" {
		return false, "empty marker"
	}
	if deploymentCommitMatches(actual, commit) {
		return true, shortCommit(actual)
	}
	return false, fmt.Sprintf("actual %s != pipeline %s", shortCommit(actual), shortCommit(commit))
}

func localHealthOK(ctx context.Context, healthURL string) (bool, string) {
	if healthURL == "" {
		return true, "not configured"
	}
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, healthURL, nil)
	if err != nil {
		return false, err.Error()
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return true, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

func deploymentCommitMatches(actual, expected string) bool {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	if actual == "" || expected == "" {
		return false
	}
	return strings.HasPrefix(actual, expected) || strings.HasPrefix(expected, actual)
}

func defaultCommonOptions() commonOptions {
	return commonOptions{
		baseURL:     firstNonEmpty(os.Getenv("PEAPOD_URL"), defaultBaseURL),
		username:    os.Getenv("PEAPOD_USERNAME"),
		password:    os.Getenv("PEAPOD_PASSWORD"),
		sessionFile: firstNonEmpty(os.Getenv("PEAPOD_SESSION_FILE"), defaultSessionFile()),
	}
}

func newFlagSet(name string, opts *commonOptions) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&opts.baseURL, "url", opts.baseURL, "Peapod base URL")
	fs.StringVar(&opts.username, "username", opts.username, "Peapod username/email")
	fs.StringVar(&opts.password, "password", opts.password, "Peapod password")
	fs.StringVar(&opts.sessionFile, "session-file", opts.sessionFile, "session cache path")
	fs.BoolVar(&opts.insecureTLS, "insecure-tls", opts.insecureTLS, "skip TLS certificate verification")
	return fs
}

func newClient(opts commonOptions) (*client, error) {
	base, err := normalizeBaseURL(opts.baseURL)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if opts.insecureTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	c := &client{
		baseURL:     base,
		username:    strings.TrimSpace(opts.username),
		password:    opts.password,
		sessionFile: opts.sessionFile,
		httpClient:  &http.Client{Timeout: 60 * time.Second, Transport: transport},
	}
	if c.username == "" && c.password != "" {
		c.username = "admin"
	}
	_ = c.loadSession()
	return c, nil
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultBaseURL
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid Peapod URL %q", raw)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (c *client) loadSession() error {
	if c.sessionFile == "" {
		return nil
	}
	payload, err := os.ReadFile(c.sessionFile)
	if err != nil {
		return err
	}
	var cached sessionCache
	if err := json.Unmarshal(payload, &cached); err != nil {
		return err
	}
	if cached.BaseURL != c.baseURL || cached.CookieName != sessionCookieName || cached.CookieValue == "" {
		return nil
	}
	c.session = cached
	return nil
}

func (c *client) saveSession() error {
	if c.sessionFile == "" || c.session.CookieValue == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.sessionFile), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(c.session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.sessionFile, payload, 0o600)
}

func (c *client) login(ctx context.Context) error {
	if c.password == "" {
		return errors.New("not logged in; set PEAPOD_PASSWORD or pass --password")
	}
	username := c.username
	if username == "" {
		username = "admin"
	}
	var out map[string]any
	if err := c.do(ctx, http.MethodPost, "/api/login", loginRequest{Username: username, Password: c.password}, &out, false); err != nil {
		return err
	}
	if c.session.CookieValue == "" {
		return errors.New("login succeeded but Peapod did not return a session cookie")
	}
	if err := c.saveSession(); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func (c *client) state(ctx context.Context) (stateResponse, error) {
	var out stateResponse
	err := c.do(ctx, http.MethodGet, "/api/state", nil, &out, true)
	return out, err
}

func (c *client) runTask(ctx context.Context, taskID, branch string, inputs map[string]string) (runResponse, error) {
	var out runResponse
	path := "/api/tasks/" + url.PathEscape(taskID) + "/run"
	body := runRequest{Inputs: inputs, Branch: strings.TrimSpace(branch)}
	err := c.do(ctx, http.MethodPost, path, body, &out, true)
	if out.Task.RepoID > 0 {
		out.Pipeline.RepoID = out.Task.RepoID
		out.Pipeline.RepoName = out.Task.RepoName
	}
	return out, err
}

func (c *client) pipelineSummary(ctx context.Context, repoID int, number int64) (pipelineSummary, error) {
	var out pipelineSummary
	path := fmt.Sprintf("/api/pipelines/%d/%d/summary", repoID, number)
	err := c.do(ctx, http.MethodGet, path, nil, &out, true)
	if out.Pipeline.RepoID == 0 {
		out.Pipeline.RepoID = repoID
	}
	return out, err
}

func (c *client) waitPipeline(ctx context.Context, repoID int, number int64, poll time.Duration) (pipelineSummary, error) {
	if poll <= 0 {
		poll = defaultPollInterval
	}
	var lastStatus string
	for {
		summary, err := c.pipelineSummary(ctx, repoID, number)
		if err != nil {
			return pipelineSummary{}, err
		}
		status := strings.ToLower(strings.TrimSpace(summary.Pipeline.Status))
		if status != lastStatus {
			fmt.Printf("pipeline #%d status: %s\n", number, fallback(status, "unknown"))
			lastStatus = status
		}
		if terminalPipelineStatus(status) {
			return summary, nil
		}
		select {
		case <-ctx.Done():
			return pipelineSummary{}, ctx.Err()
		case <-time.After(poll):
		}
	}
}

func (c *client) waitDeploymentVerified(ctx context.Context, t task, p pipeline, poll time.Duration) (deploymentStatus, error) {
	targetID := deploymentTargetID(t)
	if targetID == "" {
		return deploymentStatus{}, errors.New("cannot determine deployment target; task has no repo/project metadata")
	}
	for {
		state, err := c.state(ctx)
		if err != nil {
			return deploymentStatus{}, err
		}
		status, ok := findDeploymentStatus(state.DeploymentStatuses, targetID)
		if ok {
			if deploymentStatusReady(status) && deploymentStatusMatchesPipeline(status, p) {
				return status, nil
			}
			if deploymentStatusSeesPipeline(status, p) && terminalDeploymentFailure(status) {
				return deploymentStatus{}, fmt.Errorf("deployment verification failed: %s", fallback(status.DeployVerifyMessage, status.DeployVerifyStatus))
			}
		}
		select {
		case <-ctx.Done():
			if ok {
				return deploymentStatus{}, fmt.Errorf("deployment verification timeout: %s", fallback(status.DeployVerifyMessage, status.DeployVerifyStatus))
			}
			return deploymentStatus{}, ctx.Err()
		case <-time.After(poll):
		}
	}
}

func (c *client) do(ctx context.Context, method, path string, body any, out any, retryAuth bool) error {
	err := c.doOnce(ctx, method, path, body, out)
	if err == nil {
		return nil
	}
	var apiErr apiError
	if !retryAuth || !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		return err
	}
	if loginErr := c.login(ctx); loginErr != nil {
		return loginErr
	}
	return c.doOnce(ctx, method, path, body, out)
}

func (c *client) doOnce(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.session.CookieValue != "" {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: c.session.CookieValue})
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	c.captureSession(response)
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return parseAPIError(response.StatusCode, payload)
	}
	if out == nil || len(strings.TrimSpace(string(payload))) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("decode response from %s %s: %w", method, path, err)
	}
	return nil
}

func (c *client) captureSession(response *http.Response) {
	for _, cookie := range response.Cookies() {
		if cookie.Name != sessionCookieName || cookie.Value == "" {
			continue
		}
		c.session = sessionCache{
			BaseURL:     c.baseURL,
			CookieName:  sessionCookieName,
			CookieValue: cookie.Value,
			SavedAt:     time.Now(),
		}
	}
}

func parseAPIError(status int, payload []byte) error {
	text := strings.TrimSpace(string(payload))
	var parsed apiErrorPayload
	if text != "" && json.Unmarshal(payload, &parsed) == nil && (parsed.Error != "" || len(parsed.Details) > 0) {
		return apiError{Status: status, Message: fallback(parsed.Error, http.StatusText(status)), Details: parsed.Details}
	}
	return apiError{Status: status, Message: fallback(text, http.StatusText(status))}
}

func parseKeyValue(value string) (string, string, error) {
	key, parsed, ok := strings.Cut(value, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return "", "", fmt.Errorf("expected KEY=VALUE, got %q", value)
	}
	return key, parsed, nil
}

func parseInterspersedFlags(fs *flag.FlagSet, args []string) error {
	if len(args) == 0 {
		return fs.Parse(args)
	}
	reordered := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if isFlagArgument(arg) {
			reordered = append(reordered, arg)
			if flagNeedsSeparateValue(fs, arg) && i+1 < len(args) {
				reordered = append(reordered, args[i+1])
				i++
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	reordered = append(reordered, positionals...)
	return fs.Parse(reordered)
}

func isFlagArgument(arg string) bool {
	return strings.HasPrefix(arg, "-") && arg != "-"
}

func flagNeedsSeparateValue(fs *flag.FlagSet, arg string) bool {
	if strings.Contains(arg, "=") {
		return false
	}
	name := strings.TrimLeft(arg, "-")
	if name == "" {
		return false
	}
	registered := fs.Lookup(name)
	if registered == nil {
		return false
	}
	if boolFlag, ok := registered.Value.(interface{ IsBoolFlag() bool }); ok && boolFlag.IsBoolFlag() {
		return false
	}
	return true
}

func terminalPipelineStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "failure", "error", "killed", "skipped":
		return true
	default:
		return false
	}
}

func terminalDeploymentFailure(status deploymentStatus) bool {
	switch strings.ToLower(strings.TrimSpace(status.DeployVerifyStatus)) {
	case "health_failed", "marker_mismatch", "pipeline_only", "not_deployed":
		return true
	default:
		return false
	}
}

func deploymentStatusReady(status deploymentStatus) bool {
	if status.DeployVerified {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(status.DeployVerifyStatus)) {
	case "health_only", "marker_unavailable", "external_marker":
		return true
	default:
		return false
	}
}

func deploymentStatusMatchesPipeline(status deploymentStatus, p pipeline) bool {
	if status.Pipeline == p.Number || status.LatestPipeline == p.Number {
		return true
	}
	return shortCommit(status.CurrentCommit) != "" && shortCommit(status.CurrentCommit) == shortCommit(p.Commit)
}

func deploymentStatusSeesPipeline(status deploymentStatus, p pipeline) bool {
	return status.Pipeline == p.Number || status.LatestPipeline == p.Number || status.LatestCommit == p.Commit
}

func deploymentTargetID(t task) string {
	if t.RepoID <= 0 {
		return ""
	}
	projectID := firstNonEmpty(
		variableValue(t.Variables, "PEAPOD_PROJECT_ID"),
		variableValue(t.Variables, "ZEPHYR_PROJECT_ID"),
		variableValue(t.Variables, "PROJECT_ID"),
		variableValue(t.Variables, "SERVICE_ID"),
		variableValue(t.Variables, "DEPLOY_SERVICE"),
		variableValue(t.Variables, "APP"),
		variableValue(t.Variables, "PROJECT"),
		normalizeID(t.Group),
		normalizeID(t.Title),
		normalizeID(t.ID),
	)
	if projectID == "" {
		return ""
	}
	return fmt.Sprintf("repo-%d:%s", t.RepoID, projectID)
}

func findDeploymentStatus(rows []deploymentStatus, id string) (deploymentStatus, bool) {
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return deploymentStatus{}, false
}

func findTask(rows []task, id string) (task, bool) {
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return task{}, false
}

func variableValue(values map[string]string, key string) string {
	if len(values) == 0 {
		return ""
	}
	for currentKey, value := range values {
		if strings.EqualFold(strings.TrimSpace(currentKey), key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneStringMap(values map[string]string) map[string]string {
	cloned := map[string]string{}
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func normalizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func printPipelineSummary(summary pipelineSummary) {
	p := summary.Pipeline
	fmt.Printf("pipeline #%d %s branch=%s commit=%s\n", p.Number, fallback(p.Status, "unknown"), fallback(p.Branch, "-"), shortCommit(p.Commit))
	for _, step := range summary.Steps {
		line := fmt.Sprintf("  - %s: %s", step.Name, fallback(step.State, "-"))
		if step.ExitCode != 0 {
			line += fmt.Sprintf(" exit=%d", step.ExitCode)
		}
		if step.Error != "" {
			line += " error=" + step.Error
		}
		fmt.Println(line)
	}
	if summary.FailureSummary != "" {
		fmt.Println("failure:", summary.FailureSummary)
	}
	if summary.WoodpeckerURL != "" {
		fmt.Println("url:", summary.WoodpeckerURL)
	}
}

func printPipelineFailure(summary pipelineSummary) {
	printPipelineSummary(summary)
	if len(summary.LogTail) == 0 {
		return
	}
	fmt.Println("log tail:")
	start := 0
	if len(summary.LogTail) > 30 {
		start = len(summary.LogTail) - 30
	}
	for _, line := range summary.LogTail[start:] {
		fmt.Println(line)
	}
}

func printDeploymentStatuses(rows []deploymentStatus) {
	if len(rows) == 0 {
		fmt.Println("no deployment statuses")
		return
	}
	fmt.Printf("%-34s  %-10s  %-12s  %-10s  %s\n", "ID", "VERIFY", "PIPELINE", "COMMIT", "NAME")
	for _, row := range rows {
		verify := row.DeployVerifyStatus
		if row.DeployVerified && verify == "" {
			verify = "verified"
		}
		fmt.Printf("%-34s  %-10s  %-12s  %-10s  %s\n", row.ID, fallback(verify, "-"), fmt.Sprintf("#%d", row.Pipeline), shortCommit(fallback(row.ActualCommit, row.CurrentCommit)), row.Name)
		if row.DeployVerifyMessage != "" {
			fmt.Printf("  %s\n", row.DeployVerifyMessage)
		}
	}
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func defaultSessionFile() string {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "peapodctl", "session.json")
	}
	if dir, err := os.UserHomeDir(); err == nil && dir != "" {
		return filepath.Join(dir, ".peapodctl", "session.json")
	}
	return filepath.Join(os.TempDir(), "peapodctl-session.json")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func fallback(value, fallbackValue string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallbackValue)
}

func shortCommit(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func truncate(value string, max int) string {
	if max <= 0 || len([]rune(value)) <= max {
		return value
	}
	runes := []rune(value)
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}
