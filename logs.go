package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	maxLogContainers       = 10
	maxLogLines            = 1000
	defaultLogTail         = 200
	defaultLogSinceMinutes = 15
	logQueryTimeout        = 15 * time.Second
)

type LogSummaryResponse struct {
	Mode             string         `json:"mode"`
	Label            string         `json:"label"`
	Message          string         `json:"message"`
	Source           string         `json:"source"`
	DozzlePublicURL  string         `json:"dozzle_public_url,omitempty"`
	GrafanaPublicURL string         `json:"grafana_public_url,omitempty"`
	DozzleMCPReady   bool           `json:"dozzle_mcp_ready"`
	DozzleMCPMessage string         `json:"dozzle_mcp_message,omitempty"`
	DockerLogMaxSize string         `json:"docker_log_max_size"`
	DockerLogMaxFile string         `json:"docker_log_max_file"`
	DockerRetention  string         `json:"docker_retention"`
	ContainerCount   int            `json:"container_count"`
	HostCount        int            `json:"host_count"`
	Limits           LogQueryLimits `json:"limits"`
	CheckedAt        string         `json:"checked_at"`
	DegradedReason   string         `json:"degraded_reason,omitempty"`
}

type LogQueryLimits struct {
	MaxLines      int `json:"max_lines"`
	MaxContainers int `json:"max_containers"`
	Timeout       int `json:"timeout_seconds"`
}

type LogContainersResponse struct {
	Containers     []LogContainer `json:"containers"`
	Source         string         `json:"source"`
	CheckedAt      string         `json:"checked_at"`
	DegradedReason string         `json:"degraded_reason,omitempty"`
}

type LogContainer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Image    string `json:"image,omitempty"`
	State    string `json:"state,omitempty"`
	Health   string `json:"health,omitempty"`
	Host     string `json:"host"`
	HostName string `json:"host_name,omitempty"`
	Group    string `json:"group,omitempty"`
	Created  string `json:"created,omitempty"`
	Source   string `json:"source"`
}

type LogQueryRequest struct {
	Hosts        []string `json:"hosts"`
	Containers   []string `json:"containers"`
	Keyword      string   `json:"keyword"`
	Level        string   `json:"level"`
	SinceMinutes int      `json:"since_minutes"`
	Tail         int      `json:"tail"`
	Stream       string   `json:"stream"`
}

type LogQueryResponse struct {
	Lines          []LogLine      `json:"lines"`
	Source         string         `json:"source"`
	Containers     []LogContainer `json:"containers"`
	CheckedAt      string         `json:"checked_at"`
	DegradedReason string         `json:"degraded_reason,omitempty"`
}

type LogLine struct {
	Timestamp     string `json:"timestamp,omitempty"`
	Level         string `json:"level,omitempty"`
	Stream        string `json:"stream,omitempty"`
	Type          string `json:"type,omitempty"`
	Message       string `json:"message"`
	Host          string `json:"host"`
	HostName      string `json:"host_name,omitempty"`
	ContainerID   string `json:"container_id"`
	ContainerName string `json:"container_name"`
}

type dozzleContainerRaw struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	State   string            `json:"state"`
	Health  string            `json:"health"`
	Host    string            `json:"host"`
	Created string            `json:"created"`
	Labels  map[string]string `json:"labels"`
	Group   string            `json:"group"`
}

type mcpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpRPCError    `json:"error,omitempty"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError,omitempty"`
}

type dozzleMCPClient struct {
	baseURL     string
	endpoint    string
	client      *http.Client
	sessionID   string
	nextID      int
	initialized bool
}

var (
	sensitiveInlinePattern = regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key|authorization|cookie|private[_-]?key|access[_-]?key|credential)\b\s*[:=]\s*("[^"]*"|'[^']*'|[^,\s;]+)`)
	bearerPattern          = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
)

func (a *App) logsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	containers, source, degraded := a.listLogContainers(ctx)
	logStrategy := a.logStrategyStatus()
	writeJSON(w, LogSummaryResponse{
		Mode:             logStrategy.Mode,
		Label:            logStrategy.Label,
		Message:          logStrategy.Message,
		Source:           source,
		DozzlePublicURL:  logStrategy.DozzlePublicURL,
		GrafanaPublicURL: logStrategy.GrafanaPublicURL,
		DozzleMCPReady:   logStrategy.DozzleMCPReady,
		DozzleMCPMessage: logStrategy.DozzleMCPMessage,
		DockerLogMaxSize: logStrategy.DockerLogMaxSize,
		DockerLogMaxFile: logStrategy.DockerLogMaxFile,
		DockerRetention:  logStrategy.DockerRetention,
		ContainerCount:   len(containers),
		HostCount:        countLogHosts(containers),
		Limits:           defaultLogLimits(),
		CheckedAt:        time.Now().Format(time.RFC3339),
		DegradedReason:   degraded,
	})
}

func (a *App) logsContainers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	containers, source, degraded := a.listLogContainers(ctx)
	writeJSON(w, LogContainersResponse{
		Containers:     containers,
		Source:         source,
		CheckedAt:      time.Now().Format(time.RFC3339),
		DegradedReason: degraded,
	})
}

func (a *App) logsQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input LogQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	input = normalizeLogQuery(input)
	ctx, cancel := context.WithTimeout(r.Context(), logQueryTimeout)
	defer cancel()

	lines, containers, degraded, err := a.queryDozzleLogs(ctx, input)
	source := "dozzle_mcp"
	if err != nil {
		fallbackLines, fallbackContainers, fallbackErr := a.querySSHFallbackLogs(ctx, input)
		if fallbackErr != nil {
			writeError(w, http.StatusBadGateway, "日志查询不可用", err.Error(), fallbackErr.Error())
			return
		}
		lines = fallbackLines
		containers = fallbackContainers
		source = "ssh_fallback"
		degraded = "Dozzle MCP 不可用，已使用 SSH 只读 docker logs 兜底：" + err.Error()
	}
	lines = filterAndLimitLogLines(lines, input)
	writeJSON(w, LogQueryResponse{
		Lines:          lines,
		Source:         source,
		Containers:     containers,
		CheckedAt:      time.Now().Format(time.RFC3339),
		DegradedReason: degraded,
	})
}

func (a *App) probeDozzleMCP(timeout time.Duration) (bool, string) {
	if strings.TrimSpace(a.cfg.DozzleBaseURL) == "" {
		return false, "未配置 PEAPOD_DOZZLE_BASE_URL"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := newDozzleMCPClient(a.cfg.DozzleBaseURL, a.client).callToolText(ctx, "list_containers", map[string]any{"state": "running"})
	if err != nil {
		return false, "MCP 不可用：" + err.Error()
	}
	return true, "MCP 已启用，Peapod 可以查询 Docker 已保留日志。"
}

func defaultLogLimits() LogQueryLimits {
	return LogQueryLimits{MaxLines: maxLogLines, MaxContainers: maxLogContainers, Timeout: int(logQueryTimeout.Seconds())}
}

func normalizeLogQuery(input LogQueryRequest) LogQueryRequest {
	input.Keyword = strings.TrimSpace(input.Keyword)
	input.Level = strings.ToLower(strings.TrimSpace(input.Level))
	input.Stream = strings.ToLower(strings.TrimSpace(input.Stream))
	if input.Stream != "stdout" && input.Stream != "stderr" {
		input.Stream = "all"
	}
	if input.SinceMinutes <= 0 {
		input.SinceMinutes = defaultLogSinceMinutes
	}
	if input.SinceMinutes > 24*60 {
		input.SinceMinutes = 24 * 60
	}
	if input.Tail <= 0 {
		input.Tail = defaultLogTail
	}
	if input.Tail > maxLogLines {
		input.Tail = maxLogLines
	}
	input.Hosts = cleanStringList(input.Hosts)
	input.Containers = cleanStringList(input.Containers)
	return input
}

func cleanStringList(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (a *App) listLogContainers(ctx context.Context) ([]LogContainer, string, string) {
	containers, err := a.listDozzleContainers(ctx)
	if err == nil {
		return containers, "dozzle_mcp", ""
	}
	fallback := a.monitoringLogContainers(ctx)
	if len(fallback) > 0 {
		return fallback, "monitoring_fallback", "Dozzle MCP 不可用，已退回监控容器列表：" + err.Error()
	}
	return []LogContainer{}, "degraded", "Dozzle MCP 不可用，且没有可用的监控容器列表：" + err.Error()
}

func (a *App) listDozzleContainers(ctx context.Context) ([]LogContainer, error) {
	text, err := newDozzleMCPClient(a.cfg.DozzleBaseURL, a.client).callToolText(ctx, "list_containers", map[string]any{})
	if err != nil {
		return nil, err
	}
	var rows []dozzleContainerRaw
	if err := json.Unmarshal([]byte(text), &rows); err != nil {
		return nil, fmt.Errorf("Dozzle list_containers 返回无法解析")
	}
	containers := make([]LogContainer, 0, len(rows))
	for _, row := range rows {
		host := strings.TrimSpace(row.Host)
		containers = append(containers, LogContainer{
			ID:       strings.TrimSpace(row.ID),
			Name:     strings.TrimPrefix(strings.TrimSpace(row.Name), "/"),
			Image:    strings.TrimSpace(row.Image),
			State:    strings.TrimSpace(row.State),
			Health:   strings.TrimSpace(row.Health),
			Host:     host,
			HostName: host,
			Group:    strings.TrimSpace(row.Group),
			Created:  strings.TrimSpace(row.Created),
			Source:   "dozzle_mcp",
		})
	}
	sortLogContainers(containers)
	return containers, nil
}

func (a *App) monitoringLogContainers(ctx context.Context) []LogContainer {
	rows := []LogContainer{}
	if a.monitor != nil {
		summary := a.monitor.Summary(ctx)
		for _, item := range summary.Containers {
			rows = append(rows, LogContainer{
				ID:       item.Name,
				Name:     item.Name,
				State:    item.Status,
				Host:     item.HostID,
				HostName: item.HostName,
				Source:   "monitoring_fallback",
			})
		}
	}
	if len(rows) == 0 {
		for _, host := range parseMonitorHosts(a.cfg) {
			for _, name := range host.Containers {
				rows = append(rows, LogContainer{
					ID:       name,
					Name:     name,
					State:    "configured",
					Host:     host.ID,
					HostName: host.Name,
					Source:   "configured",
				})
			}
		}
	}
	sortLogContainers(rows)
	return rows
}

func sortLogContainers(containers []LogContainer) {
	sort.SliceStable(containers, func(i, j int) bool {
		if containers[i].Host == containers[j].Host {
			return strings.ToLower(containers[i].Name) < strings.ToLower(containers[j].Name)
		}
		return strings.ToLower(containers[i].Host) < strings.ToLower(containers[j].Host)
	})
}

func countLogHosts(containers []LogContainer) int {
	seen := map[string]bool{}
	for _, item := range containers {
		if item.Host != "" {
			seen[item.Host] = true
		}
	}
	return len(seen)
}

func (a *App) queryDozzleLogs(ctx context.Context, input LogQueryRequest) ([]LogLine, []LogContainer, string, error) {
	client := newDozzleMCPClient(a.cfg.DozzleBaseURL, a.client)
	containers, err := a.listDozzleContainersWithClient(ctx, client)
	if err != nil {
		return nil, nil, "", err
	}
	selected := selectLogContainers(containers, input)
	if len(selected) == 0 {
		return []LogLine{}, selected, "", nil
	}
	lines := []LogLine{}
	errorsByContainer := []string{}
	for _, item := range selected {
		args := map[string]any{
			"host":          item.Host,
			"container_id":  item.ID,
			"since_minutes": input.SinceMinutes,
			"stream":        input.Stream,
		}
		text, err := client.callToolText(ctx, "get_container_logs", args)
		if err != nil {
			errorsByContainer = append(errorsByContainer, item.Name+"："+err.Error())
			continue
		}
		lines = append(lines, parseDozzleLogText(text, item)...)
	}
	degraded := ""
	if len(errorsByContainer) > 0 {
		degraded = strings.Join(errorsByContainer, "；")
	}
	if len(lines) == 0 && len(errorsByContainer) > 0 {
		return nil, selected, degraded, errors.New(degraded)
	}
	return lines, selected, degraded, nil
}

func (a *App) listDozzleContainersWithClient(ctx context.Context, client *dozzleMCPClient) ([]LogContainer, error) {
	text, err := client.callToolText(ctx, "list_containers", map[string]any{})
	if err != nil {
		return nil, err
	}
	var rows []dozzleContainerRaw
	if err := json.Unmarshal([]byte(text), &rows); err != nil {
		return nil, fmt.Errorf("Dozzle list_containers 返回无法解析")
	}
	out := make([]LogContainer, 0, len(rows))
	for _, row := range rows {
		host := strings.TrimSpace(row.Host)
		out = append(out, LogContainer{
			ID:       strings.TrimSpace(row.ID),
			Name:     strings.TrimPrefix(strings.TrimSpace(row.Name), "/"),
			Image:    strings.TrimSpace(row.Image),
			State:    strings.TrimSpace(row.State),
			Health:   strings.TrimSpace(row.Health),
			Host:     host,
			HostName: host,
			Group:    strings.TrimSpace(row.Group),
			Created:  strings.TrimSpace(row.Created),
			Source:   "dozzle_mcp",
		})
	}
	sortLogContainers(out)
	return out, nil
}

func selectLogContainers(containers []LogContainer, input LogQueryRequest) []LogContainer {
	hostSet := map[string]bool{}
	for _, host := range input.Hosts {
		hostSet[strings.ToLower(host)] = true
	}
	containerSet := map[string]bool{}
	for _, key := range input.Containers {
		containerSet[strings.ToLower(key)] = true
	}
	selected := []LogContainer{}
	for _, item := range containers {
		if len(hostSet) > 0 && !hostSet[strings.ToLower(item.Host)] && !hostSet[strings.ToLower(item.HostName)] {
			continue
		}
		if len(containerSet) > 0 && !matchesLogContainerSelector(item, containerSet) {
			continue
		}
		selected = append(selected, item)
		if len(selected) >= maxLogContainers {
			break
		}
	}
	if len(selected) == 0 && len(containerSet) == 0 {
		for _, item := range containers {
			if len(hostSet) > 0 && !hostSet[strings.ToLower(item.Host)] && !hostSet[strings.ToLower(item.HostName)] {
				continue
			}
			if strings.EqualFold(item.State, "running") || strings.Contains(strings.ToLower(item.State), "up") {
				selected = append(selected, item)
			}
			if len(selected) >= 3 {
				break
			}
		}
	}
	return selected
}

func matchesLogContainerSelector(item LogContainer, selectors map[string]bool) bool {
	keys := []string{
		item.ID,
		item.Name,
		item.Host + "|" + item.ID,
		item.Host + "|" + item.Name,
		item.HostName + "|" + item.ID,
		item.HostName + "|" + item.Name,
	}
	for _, key := range keys {
		if selectors[strings.ToLower(strings.TrimSpace(key))] {
			return true
		}
	}
	return false
}

func parseDozzleLogText(text string, container LogContainer) []LogLine {
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "(no logs") {
		return []LogLine{}
	}
	lines := []LogLine{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var entry struct {
			Timestamp string `json:"timestamp"`
			Level     string `json:"level"`
			Stream    string `json:"stream"`
			Type      string `json:"type"`
			Message   any    `json:"message"`
		}
		if err := json.Unmarshal([]byte(raw), &entry); err == nil {
			lines = append(lines, LogLine{
				Timestamp:     entry.Timestamp,
				Level:         strings.ToLower(entry.Level),
				Stream:        entry.Stream,
				Type:          entry.Type,
				Message:       maskSensitiveLogText(messageToLogText(entry.Message)),
				Host:          container.Host,
				HostName:      container.HostName,
				ContainerID:   container.ID,
				ContainerName: container.Name,
			})
			continue
		}
		lines = append(lines, LogLine{
			Message:       maskSensitiveLogText(raw),
			Host:          container.Host,
			HostName:      container.HostName,
			ContainerID:   container.ID,
			ContainerName: container.Name,
		})
	}
	return lines
}

func messageToLogText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, messageToLogText(item))
		}
		return strings.Join(parts, "\n")
	default:
		payload, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(payload)
	}
}

func filterAndLimitLogLines(lines []LogLine, input LogQueryRequest) []LogLine {
	keyword := strings.ToLower(input.Keyword)
	level := strings.ToLower(input.Level)
	out := make([]LogLine, 0, len(lines))
	for _, line := range lines {
		if keyword != "" && !strings.Contains(strings.ToLower(line.Message), keyword) && !strings.Contains(strings.ToLower(line.ContainerName), keyword) {
			continue
		}
		if level != "" && level != "all" && !strings.Contains(strings.ToLower(line.Level), level) && !strings.Contains(strings.ToLower(line.Message), level) {
			continue
		}
		out = append(out, line)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, leftOK := parseLogTimestamp(out[i].Timestamp)
		right, rightOK := parseLogTimestamp(out[j].Timestamp)
		if leftOK && rightOK {
			return left.Before(right)
		}
		if leftOK != rightOK {
			return leftOK
		}
		if out[i].Host == out[j].Host {
			return out[i].ContainerName < out[j].ContainerName
		}
		return out[i].Host < out[j].Host
	})
	limit := input.Tail
	if limit <= 0 || limit > maxLogLines {
		limit = maxLogLines
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func parseLogTimestamp(value string) (time.Time, bool) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000000Z07:00"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func maskSensitiveLogText(text string) string {
	if maskSensitiveLogLine(text) == "[已隐藏敏感日志行]" {
		return "[已隐藏敏感日志行]"
	}
	text = bearerPattern.ReplaceAllString(text, "Bearer ******")
	text = sensitiveInlinePattern.ReplaceAllStringFunc(text, func(match string) string {
		if index := strings.IndexAny(match, ":="); index >= 0 {
			return strings.TrimSpace(match[:index+1]) + " ******"
		}
		return "[已隐藏敏感片段]"
	})
	return text
}

func (a *App) querySSHFallbackLogs(ctx context.Context, input LogQueryRequest) ([]LogLine, []LogContainer, error) {
	containers := selectLogContainers(a.monitoringLogContainers(ctx), input)
	if len(containers) == 0 {
		return nil, nil, errors.New("没有可用于 SSH 日志兜底的容器")
	}
	hostConfigs := map[string]MonitorHostConfig{}
	for _, host := range parseMonitorHosts(a.cfg) {
		hostConfigs[strings.ToLower(host.ID)] = host
		hostConfigs[strings.ToLower(host.Name)] = host
	}
	lines := []LogLine{}
	errorsByContainer := []string{}
	for _, item := range containers {
		cfg, ok := hostConfigs[strings.ToLower(item.Host)]
		if !ok {
			cfg, ok = hostConfigs[strings.ToLower(item.HostName)]
		}
		if !ok {
			errorsByContainer = append(errorsByContainer, item.Name+"：没有匹配的 SSH 主机配置")
			continue
		}
		out, err := runMonitorSSHCommand(ctx, a.cfg, cfg, dockerLogsCommand(item.Name, input))
		if err != nil {
			errorsByContainer = append(errorsByContainer, item.Name+"："+err.Error())
			continue
		}
		lines = append(lines, parseSSHDockerLogs(out, item)...)
	}
	if len(lines) == 0 && len(errorsByContainer) > 0 {
		return nil, containers, errors.New(strings.Join(errorsByContainer, "；"))
	}
	return lines, containers, nil
}

func dockerLogsCommand(container string, input LogQueryRequest) string {
	tail := input.Tail
	if tail <= 0 || tail > maxLogLines {
		tail = defaultLogTail
	}
	since := input.SinceMinutes
	if since <= 0 {
		since = defaultLogSinceMinutes
	}
	return fmt.Sprintf("docker logs --timestamps --tail %d --since %dm %s 2>&1", tail, since, shellQuote(container))
}

func parseSSHDockerLogs(output string, container LogContainer) []LogLine {
	lines := []LogLine{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		timestamp := ""
		message := raw
		if fields := strings.Fields(raw); len(fields) > 1 {
			if _, ok := parseLogTimestamp(fields[0]); ok {
				timestamp = fields[0]
				message = strings.TrimSpace(strings.TrimPrefix(raw, fields[0]))
			}
		}
		lines = append(lines, LogLine{
			Timestamp:     timestamp,
			Message:       maskSensitiveLogText(message),
			Host:          container.Host,
			HostName:      container.HostName,
			ContainerID:   container.ID,
			ContainerName: container.Name,
		})
	}
	return lines
}

func runMonitorSSHCommand(ctx context.Context, cfg Config, host MonitorHostConfig, command string) (string, error) {
	keyPath := strings.TrimSpace(host.SSHKeyPath)
	if keyPath == "" {
		keyPath = cfg.MonitorSSHKeyPath
	}
	if keyPath == "" {
		return "", errors.New("SSH key 未配置")
	}
	address := strings.TrimSpace(host.SSHHost)
	if address == "" {
		address = strings.TrimSpace(host.Address)
	}
	if address == "" {
		return "", errors.New("SSH host 未配置")
	}
	if _, _, err := net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(address, "22")
	}
	user := strings.TrimSpace(host.SSHUser)
	if user == "" {
		user = "codex"
	}
	payload, err := os.ReadFile(keyPath)
	if err != nil {
		return "", errors.New("读取 SSH key 失败")
	}
	signer, err := ssh.ParsePrivateKey(payload)
	if err != nil {
		return "", errors.New("解析 SSH key 失败")
	}
	clientConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         7 * time.Second,
	}
	client, err := ssh.Dial("tcp", address, clientConfig)
	if err != nil {
		return "", errors.New("连接失败")
	}
	defer client.Close()
	return runSSHSession(ctx, client, command)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func newDozzleMCPClient(baseURL string, client *http.Client) *dozzleMCPClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	endpoint := baseURL
	if !strings.HasSuffix(endpoint, "/api/mcp") {
		endpoint += "/api/mcp"
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &dozzleMCPClient{baseURL: baseURL, endpoint: endpoint, client: client}
}

func (c *dozzleMCPClient) callToolText(ctx context.Context, name string, args map[string]any) (string, error) {
	if err := c.initialize(ctx); err != nil {
		return "", err
	}
	raw, err := c.rpc(ctx, "tools/call", map[string]any{"name": name, "arguments": args}, true)
	if err != nil {
		return "", err
	}
	var result mcpToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("MCP tool result 无法解析")
	}
	text := mcpTextContent(result)
	if result.IsError {
		return "", errors.New(fallbackText(text, "Dozzle MCP tool 返回错误"))
	}
	return text, nil
}

func (c *dozzleMCPClient) initialize(ctx context.Context) error {
	if c.initialized {
		return nil
	}
	if strings.TrimSpace(c.endpoint) == "/api/mcp" || strings.TrimSpace(c.baseURL) == "" {
		return errors.New("Dozzle BaseURL 未配置")
	}
	_, err := c.rpc(ctx, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "peapod", "version": "v0.1.0"},
	}, true)
	if err != nil {
		return err
	}
	_, _ = c.rpc(ctx, "notifications/initialized", map[string]any{}, false)
	c.initialized = true
	return nil
}

func (c *dozzleMCPClient) rpc(ctx context.Context, method string, params any, expectResponse bool) (json.RawMessage, error) {
	c.nextID++
	request := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	if expectResponse {
		request["id"] = c.nextID
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if value := resp.Header.Get("Mcp-Session-Id"); value != "" {
		c.sessionID = value
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if !expectResponse && (resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusOK) {
		return nil, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("Dozzle MCP 端点不存在，请确认 DOZZLE_ENABLE_MCP=true")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Dozzle MCP HTTP %d：%s", resp.StatusCode, shortBody(body))
	}
	data := mcpJSONPayload(body, resp.Header.Get("Content-Type"))
	var rpc mcpRPCResponse
	if err := json.Unmarshal(data, &rpc); err != nil {
		return nil, fmt.Errorf("MCP 响应无法解析")
	}
	if rpc.Error != nil {
		return nil, fmt.Errorf("MCP %d：%s", rpc.Error.Code, rpc.Error.Message)
	}
	return rpc.Result, nil
}

func mcpTextContent(result mcpToolResult) string {
	parts := []string{}
	for _, item := range result.Content {
		if item.Text != "" {
			parts = append(parts, item.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func mcpJSONPayload(body []byte, contentType string) []byte {
	if !strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return body
	}
	blocks := strings.Split(string(body), "\n\n")
	for _, block := range blocks {
		lines := []string{}
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		joined := strings.TrimSpace(strings.Join(lines, "\n"))
		if strings.HasPrefix(joined, "{") {
			return []byte(joined)
		}
	}
	return body
}

func shortBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "empty response"
	}
	if len(text) > 180 {
		return text[:180] + "..."
	}
	return text
}
