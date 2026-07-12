package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type MonitoringService struct {
	cfg    Config
	client *http.Client
	hosts  []MonitorHostConfig

	mu         sync.Mutex
	cache      MonitoringSummary
	cacheUntil time.Time
}

type MonitorHostConfig struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Role            string     `json:"role"`
	SSHHost         string     `json:"ssh_host"`
	Address         string     `json:"address"`
	SSHUser         string     `json:"ssh_user"`
	SSHKeyPath      string     `json:"ssh_key_path"`
	BeszelNames     []string   `json:"beszel_names"`
	Containers      []string   `json:"containers"`
	ContainerGroups [][]string `json:"container_groups"`
	CleanupTaskID   string     `json:"cleanup_task_id"`
}

type MonitoringSummary struct {
	Hosts          []MonitoringHost      `json:"hosts"`
	Containers     []MonitoringContainer `json:"containers"`
	Alerts         []MonitoringAlert     `json:"alerts"`
	Links          map[string]string     `json:"links"`
	CheckedAt      string                `json:"checked_at"`
	Source         string                `json:"source"`
	DegradedReason string                `json:"degraded_reason,omitempty"`
}

type MonitoringHost struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Role             string  `json:"role"`
	Status           string  `json:"status"`
	Source           string  `json:"source"`
	CPUPercent       float64 `json:"cpu_percent,omitempty"`
	MemoryUsedBytes  uint64  `json:"memory_used_bytes,omitempty"`
	MemoryTotalBytes uint64  `json:"memory_total_bytes,omitempty"`
	MemoryPercent    float64 `json:"memory_percent,omitempty"`
	DiskUsedBytes    uint64  `json:"disk_used_bytes,omitempty"`
	DiskTotalBytes   uint64  `json:"disk_total_bytes,omitempty"`
	DiskPercent      float64 `json:"disk_percent,omitempty"`
	Load1            float64 `json:"load_1,omitempty"`
	Load5            float64 `json:"load_5,omitempty"`
	Load15           float64 `json:"load_15,omitempty"`
	NetworkBytes     uint64  `json:"network_bytes_per_second,omitempty"`
	UptimeSeconds    uint64  `json:"uptime_seconds,omitempty"`
	Uptime           string  `json:"uptime,omitempty"`
	Message          string  `json:"message,omitempty"`
	CleanupTaskID          string          `json:"cleanup_task_id,omitempty"`
	DockerReclaimableBytes uint64          `json:"docker_reclaimable_bytes,omitempty"`
	DiskBreakdown          []DiskUsageItem `json:"disk_breakdown,omitempty"`
	CheckedAt              string          `json:"checked_at,omitempty"`
}

type DiskUsageItem struct {
	Path  string `json:"path"`
	Size  string `json:"size"`
	Bytes uint64 `json:"bytes"`
}

type MonitoringContainer struct {
	HostID        string  `json:"host_id"`
	HostName      string  `json:"host_name"`
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	CPUPercent    float64 `json:"cpu_percent,omitempty"`
	MemoryUsage   string  `json:"memory_usage,omitempty"`
	MemoryPercent float64 `json:"memory_percent,omitempty"`
	Configured    bool    `json:"configured"`
	Message       string  `json:"message,omitempty"`
}

type MonitoringAlert struct {
	Level       string `json:"level"`
	HostID      string `json:"host_id,omitempty"`
	HostName    string `json:"host_name,omitempty"`
	Metric      string `json:"metric,omitempty"`
	Title       string `json:"title"`
	Message     string `json:"message"`
	ActionURL   string `json:"action_url,omitempty"`
	ActionLabel string `json:"action_label,omitempty"`
}

type beszelAuthResponse struct {
	Token string `json:"token"`
}

type beszelListResponse struct {
	Items []map[string]any `json:"items"`
}

func NewMonitoringService(cfg Config, client *http.Client) *MonitoringService {
	return &MonitoringService{
		cfg:    cfg,
		client: client,
		hosts:  parseMonitorHosts(cfg),
	}
}

func (a *App) monitoringSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.monitor == nil {
		writeJSON(w, MonitoringSummary{
			Hosts:          []MonitoringHost{},
			Containers:     []MonitoringContainer{},
			Alerts:         []MonitoringAlert{{Level: "warning", Title: "监控未启用", Message: "Peapod monitoring service is not configured"}},
			Links:          a.monitoringLinks(),
			CheckedAt:      time.Now().Format(time.RFC3339),
			Source:         "degraded",
			DegradedReason: "monitoring service is not initialized",
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()
	writeJSON(w, a.monitor.Summary(ctx))
}

func (a *App) monitoringLinks() map[string]string {
	return a.configuredLinks()
}

func (m *MonitoringService) Summary(ctx context.Context) MonitoringSummary {
	now := time.Now()
	m.mu.Lock()
	if now.Before(m.cacheUntil) && m.cache.CheckedAt != "" {
		cached := m.cache
		m.mu.Unlock()
		return cached
	}
	m.mu.Unlock()

	summary := m.collect(ctx, now)

	m.mu.Lock()
	m.cache = summary
	m.cacheUntil = now.Add(time.Duration(maxInt(m.cfg.MonitorRefreshSeconds, 5)) * time.Second)
	m.mu.Unlock()
	return summary
}

func (m *MonitoringService) collect(ctx context.Context, now time.Time) MonitoringSummary {
	hosts := make([]MonitoringHost, 0, len(m.hosts))
	hostIndex := map[string]int{}
	for _, cfg := range m.hosts {
		host := MonitoringHost{
			ID:            cfg.ID,
			Name:          cfg.Name,
			Role:          cfg.Role,
			Status:        "unknown",
			Source:        "configured",
			CleanupTaskID: cfg.CleanupTaskID,
			CheckedAt:     now.Format(time.RFC3339),
		}
		hostIndex[cfg.ID] = len(hosts)
		hosts = append(hosts, host)
	}

	beszelMatched, beszelErr := m.enrichFromBeszel(ctx, hosts, hostIndex)
	containers := []MonitoringContainer{}
	sshErrors := []string{}
	sshSuccess := 0
	for _, cfg := range m.hosts {
		if strings.TrimSpace(firstNonEmpty(cfg.SSHHost, cfg.Address)) == "" {
			continue
		}
		host, rows, err := m.collectSSH(ctx, cfg)
		if err != nil {
			sshErrors = append(sshErrors, fmt.Sprintf("%s：%v", cfg.Name, err))
			continue
		}
		sshSuccess++
		if index, ok := hostIndex[cfg.ID]; ok {
			mergeSSHHost(&hosts[index], host)
			for i := range rows {
				rows[i].HostID = cfg.ID
				rows[i].HostName = hosts[index].Name
			}
		}
		containers = append(containers, rows...)
	}

	source := "degraded"
	reasons := []string{}
	beszelActive := 0
	sshFallbackActive := 0
	for _, host := range hosts {
		switch host.Source {
		case "beszel":
			beszelActive++
		case "ssh_fallback":
			sshFallbackActive++
		}
	}
	if beszelActive > 0 {
		source = "beszel"
	} else if sshFallbackActive > 0 || sshSuccess > 0 {
		source = "ssh_fallback"
	}
	if beszelErr != nil {
		reasons = append(reasons, "Beszel："+beszelErr.Error())
	}
	if len(sshErrors) > 0 {
		reasons = append(reasons, "SSH："+strings.Join(sshErrors, "；"))
	}
	alerts := buildMonitoringAlerts(hosts, containers, m.cfg)
	if beszelMatched == 0 && sshSuccess == 0 && len(alerts) == 0 {
		alerts = append(alerts, MonitoringAlert{Level: "critical", Title: "监控不可用", Message: "Beszel 和 SSH 只读兜底都没有返回可用数据"})
	}
	sort.SliceStable(containers, func(i, j int) bool {
		if containers[i].HostID == containers[j].HostID {
			return containers[i].Name < containers[j].Name
		}
		return containers[i].HostID < containers[j].HostID
	})
	sort.SliceStable(alerts, func(i, j int) bool {
		return alertRank(alerts[i].Level) > alertRank(alerts[j].Level)
	})

	return MonitoringSummary{
		Hosts:      hosts,
		Containers: containers,
		Alerts:     alerts,
		Links: map[string]string{
			"peapod":     m.cfg.PublicURL,
			"woodpecker": m.cfg.WoodpeckerPublicURL,
			"grafana":    m.cfg.GrafanaPublicURL,
			"beszel":     m.cfg.BeszelPublicURL,
		},
		CheckedAt:      now.Format(time.RFC3339),
		Source:         source,
		DegradedReason: strings.Join(reasons, "；"),
	}
}

func (m *MonitoringService) enrichFromBeszel(ctx context.Context, hosts []MonitoringHost, hostIndex map[string]int) (int, error) {
	if m.cfg.BeszelBaseURL == "" {
		return 0, errors.New("PEAPOD_BESZEL_BASE_URL 未配置")
	}
	if m.cfg.BeszelEmail == "" || m.cfg.BeszelPassword == "" {
		return 0, errors.New("PEAPOD_BESZEL_EMAIL / PEAPOD_BESZEL_PASSWORD 未配置")
	}
	token, err := m.beszelToken(ctx)
	if err != nil {
		return 0, err
	}
	items, err := m.beszelSystems(ctx, token)
	if err != nil {
		return 0, err
	}
	matched := 0
	for _, item := range items {
		cfg, ok := m.matchBeszelHost(item)
		if !ok {
			continue
		}
		index, ok := hostIndex[cfg.ID]
		if !ok {
			continue
		}
		enrichHostFromBeszel(&hosts[index], item)
		matched++
	}
	if matched == 0 {
		return 0, errors.New("Beszel 没有返回匹配的系统记录")
	}
	return matched, nil
}

func (m *MonitoringService) beszelToken(ctx context.Context) (string, error) {
	payload, _ := json.Marshal(map[string]string{"identity": m.cfg.BeszelEmail, "password": m.cfg.BeszelPassword})
	var lastErr error
	for _, collection := range []string{"users", "_superusers"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.BeszelBaseURL+"/api/collections/"+collection+"/auth-with-password", strings.NewReader(string(payload)))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := m.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("%s auth HTTP %d", collection, resp.StatusCode)
			continue
		}
		var parsed beszelAuthResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			lastErr = err
			continue
		}
		if parsed.Token != "" {
			return parsed.Token, nil
		}
		lastErr = errors.New("Beszel auth response missing token")
	}
	if lastErr == nil {
		lastErr = errors.New("Beszel auth failed")
	}
	return "", lastErr
}

func (m *MonitoringService) beszelSystems(ctx context.Context, token string) ([]map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.cfg.BeszelBaseURL+"/api/collections/systems/records?perPage=100", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("systems HTTP %d", resp.StatusCode)
	}
	var parsed beszelListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return parsed.Items, nil
}

func (m *MonitoringService) matchBeszelHost(item map[string]any) (MonitorHostConfig, bool) {
	candidates := []string{}
	for _, key := range []string{"id", "name", "host", "hostname", "system_name"} {
		if value, ok := directString(item, key); ok {
			candidates = append(candidates, normalizeMonitorName(value))
			continue
		}
		if value, ok := lookupString(item, key); ok {
			candidates = append(candidates, normalizeMonitorName(value))
		}
	}
	for _, cfg := range m.hosts {
		names := append([]string{cfg.ID, cfg.Name}, cfg.BeszelNames...)
		for _, name := range names {
			needle := normalizeMonitorName(name)
			for _, candidate := range candidates {
				if needle != "" && candidate != "" && needle == candidate {
					return cfg, true
				}
			}
		}
	}
	for _, cfg := range m.hosts {
		names := append([]string{cfg.ID, cfg.Name}, cfg.BeszelNames...)
		for _, name := range names {
			needle := normalizeMonitorName(name)
			if !allowFuzzyBeszelMatch(needle) {
				continue
			}
			for _, candidate := range candidates {
				if candidate != "" && (strings.Contains(candidate, needle) || strings.Contains(needle, candidate)) {
					return cfg, true
				}
			}
		}
	}
	return MonitorHostConfig{}, false
}

func allowFuzzyBeszelMatch(name string) bool {
	if len(name) < 10 {
		return false
	}
	switch name {
	case "prod", "production", "builder", "local", "localhost":
		return false
	case "生产机", "构建机", "本机":
		return false
	}
	return true
}

func enrichHostFromBeszel(host *MonitoringHost, item map[string]any) {
	host.Source = "beszel"
	host.Status = "ok"
	if status, ok := lookupStringAny(item, []string{"status", "state"}); ok && status != "" {
		host.Status = normalizeMonitorStatus(status)
	}
	if cpu, ok := lookupNumberAny(item, []string{"cpu", "cpu_percent", "cpu_pct", "cpuusage"}); ok {
		host.CPUPercent = normalizePercent(cpu)
	}
	if pct, ok := lookupNumberAny(item, []string{"memory_percent", "mem_percent", "mem_pct", "memory_pct", "ram_percent", "mp"}); ok {
		host.MemoryPercent = normalizePercent(pct)
	}
	if used, ok := lookupNumberAny(item, []string{"memory_used", "mem_used", "used_memory", "ram_used"}); ok {
		host.MemoryUsedBytes = normalizeBytes(used)
	}
	if total, ok := lookupNumberAny(item, []string{"memory_total", "mem_total", "total_memory", "ram_total"}); ok {
		host.MemoryTotalBytes = normalizeBytes(total)
	}
	if host.MemoryPercent == 0 && host.MemoryTotalBytes > 0 {
		host.MemoryPercent = roundPercent(float64(host.MemoryUsedBytes) / float64(host.MemoryTotalBytes) * 100)
	}
	if pct, ok := lookupNumberAny(item, []string{"disk_percent", "disk_pct", "disk_usage", "filesystem_percent", "dp"}); ok {
		host.DiskPercent = normalizePercent(pct)
	}
	if used, ok := lookupNumberAny(item, []string{"disk_used", "used_disk", "filesystem_used"}); ok {
		host.DiskUsedBytes = normalizeBytes(used)
	}
	if total, ok := lookupNumberAny(item, []string{"disk_total", "total_disk", "filesystem_total"}); ok {
		host.DiskTotalBytes = normalizeBytes(total)
	}
	if host.DiskPercent == 0 && host.DiskTotalBytes > 0 {
		host.DiskPercent = roundPercent(float64(host.DiskUsedBytes) / float64(host.DiskTotalBytes) * 100)
	}
	if uptime, ok := lookupStringAny(item, []string{"uptime", "uptime_text"}); ok {
		host.Uptime = uptime
	}
	if uptime, ok := lookupNumberAny(item, []string{"u", "uptime_seconds"}); ok {
		host.UptimeSeconds = uint64(uptime)
		if host.Uptime == "" {
			host.Uptime = formatUptimeSeconds(host.UptimeSeconds)
		}
	}
	if load, ok := lookupNumberArrayAny(item, []string{"la", "load", "load_average"}); ok {
		if len(load) > 0 {
			host.Load1 = roundPercent(load[0])
		}
		if len(load) > 1 {
			host.Load5 = roundPercent(load[1])
		}
		if len(load) > 2 {
			host.Load15 = roundPercent(load[2])
		}
	}
	if network, ok := lookupNumberAny(item, []string{"bb", "network_bytes_per_second", "network"}); ok {
		host.NetworkBytes = normalizeBytes(network)
	}
	host.Message = "Beszel 状态已同步"
}

func (m *MonitoringService) collectSSH(ctx context.Context, cfg MonitorHostConfig) (MonitoringHost, []MonitoringContainer, error) {
	keyPath := strings.TrimSpace(cfg.SSHKeyPath)
	if keyPath == "" {
		keyPath = m.cfg.MonitorSSHKeyPath
	}
	if keyPath == "" {
		return MonitoringHost{}, nil, errors.New("SSH key 未配置")
	}
	address := strings.TrimSpace(cfg.SSHHost)
	if address == "" {
		address = strings.TrimSpace(cfg.Address)
	}
	if address == "" {
		return MonitoringHost{}, nil, errors.New("SSH host 未配置")
	}
	if _, _, err := net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(address, "22")
	}
	user := strings.TrimSpace(cfg.SSHUser)
	if user == "" {
		user = "codex"
	}
	payload, err := os.ReadFile(keyPath)
	if err != nil {
		return MonitoringHost{}, nil, fmt.Errorf("读取 SSH key 失败")
	}
	signer, err := ssh.ParsePrivateKey(payload)
	if err != nil {
		return MonitoringHost{}, nil, fmt.Errorf("解析 SSH key 失败")
	}
	clientConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         7 * time.Second,
	}
	client, err := ssh.Dial("tcp", address, clientConfig)
	if err != nil {
		return MonitoringHost{}, nil, fmt.Errorf("连接失败")
	}
	defer client.Close()
	output, err := runSSHSession(ctx, client, monitorSSHScript)
	if err != nil {
		return MonitoringHost{}, nil, err
	}
	host, containers := parseSSHMonitorOutput(output, cfg)
	return host, containers, nil
}

func runSSHSession(ctx context.Context, client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建 session 失败")
	}
	defer session.Close()
	done := make(chan struct {
		out []byte
		err error
	}, 1)
	go func() {
		out, err := session.CombinedOutput(command)
		done <- struct {
			out []byte
			err error
		}{out: out, err: err}
	}()
	select {
	case <-ctx.Done():
		_ = client.Close()
		return "", errors.New("SSH 采集超时")
	case result := <-done:
		if result.err != nil {
			return string(result.out), fmt.Errorf("SSH 只读命令失败")
		}
		return string(result.out), nil
	}
}

const monitorSSHScript = `set +e
printf "__CPU__\n"
cpu1="$(grep '^cpu ' /proc/stat)"
sleep 0.3
cpu2="$(grep '^cpu ' /proc/stat)"
awk -v a="$cpu1" -v b="$cpu2" 'BEGIN {
  n1=split(a, x, " "); n2=split(b, y, " ");
  idle1=x[5]+x[6]; idle2=y[5]+y[6];
  total1=0; total2=0;
  for (i=2;i<=n1;i++) { total1+=x[i]; }
  for (i=2;i<=n2;i++) { total2+=y[i]; }
  dt=total2-total1; di=idle2-idle1;
  if (dt > 0) printf "%.1f\n", (1 - di/dt) * 100; else print "";
}'
printf "__LOAD__\n"
cat /proc/loadavg 2>/dev/null
printf "__DF__\n"
df -P / 2>/dev/null | tail -n 1
printf "__FREE__\n"
free -b 2>/dev/null | awk '/^Mem:/ {print $2" "$3" "$7}'
printf "__UPTIME__\n"
uptime -p 2>/dev/null || uptime 2>/dev/null
printf "__PS__\n"
docker ps --format '{{.Names}}	{{.Status}}' 2>/dev/null
printf "__STATS__\n"
docker stats --no-stream --format '{{.Name}}	{{.CPUPerc}}	{{.MemUsage}}' 2>/dev/null
printf "__DOCKER_DF__\n"
docker system df 2>/dev/null
printf "__DU__\n"
du -sh /var/lib/docker /opt /tmp /var/log /root 2>/dev/null
exit 0
`

func parseSSHMonitorOutput(output string, cfg MonitorHostConfig) (MonitoringHost, []MonitoringContainer) {
	host := MonitoringHost{
		ID:            cfg.ID,
		Name:          cfg.Name,
		Role:          cfg.Role,
		Status:        "ok",
		Source:        "ssh",
		CleanupTaskID: cfg.CleanupTaskID,
		Message:       "SSH 只读状态已同步",
		CheckedAt:     time.Now().Format(time.RFC3339),
	}
	ps := map[string]string{}
	stats := map[string]MonitoringContainer{}
	section := ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "__") && strings.HasSuffix(line, "__") {
			section = line
			continue
		}
		switch section {
		case "__CPU__":
			if value, err := strconv.ParseFloat(strings.TrimSuffix(line, "%"), 64); err == nil {
				host.CPUPercent = roundPercent(value)
			}
		case "__DF__":
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				total, _ := strconv.ParseUint(fields[1], 10, 64)
				used, _ := strconv.ParseUint(fields[2], 10, 64)
				host.DiskTotalBytes = total * 1024
				host.DiskUsedBytes = used * 1024
				host.DiskPercent = parsePercent(fields[4])
			}
		case "__FREE__":
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				total, _ := strconv.ParseUint(fields[0], 10, 64)
				used, _ := strconv.ParseUint(fields[1], 10, 64)
				host.MemoryTotalBytes = total
				host.MemoryUsedBytes = used
				if total > 0 {
					host.MemoryPercent = roundPercent(float64(used) / float64(total) * 100)
				}
			}
		case "__LOAD__":
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				host.Load1, _ = strconv.ParseFloat(fields[0], 64)
				host.Load5, _ = strconv.ParseFloat(fields[1], 64)
				host.Load15, _ = strconv.ParseFloat(fields[2], 64)
			}
		case "__UPTIME__":
			if host.Uptime == "" {
				host.Uptime = strings.TrimPrefix(line, "up ")
			}
		case "__PS__":
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) == 2 {
				ps[parts[0]] = parts[1]
			}
		case "__STATS__":
			parts := strings.Split(line, "\t")
			if len(parts) >= 3 {
				stats[parts[0]] = MonitoringContainer{
					Name:          parts[0],
					CPUPercent:    parsePercent(parts[1]),
					MemoryUsage:   parts[2],
					MemoryPercent: parseDockerMemoryPercent(parts[2]),
				}
			}
		case "__DOCKER_DF__":
			if strings.HasPrefix(line, "Build Cache") {
				fields := strings.Fields(line)
				if len(fields) >= 4 {
					host.DockerReclaimableBytes = uint64(parseHumanBytes(fields[3]))
				}
			}
		case "__DU__":
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				item := DiskUsageItem{
					Size: fields[0],
					Path: fields[1],
				}
				item.Bytes = uint64(parseHumanBytes(fields[0]))
				host.DiskBreakdown = append(host.DiskBreakdown, item)
			}
		}
	}
	names := cfg.Containers
	if len(names) == 0 && len(cfg.ContainerGroups) == 0 {
		for name := range ps {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	containers := make([]MonitoringContainer, 0, len(names)+len(cfg.ContainerGroups))
	added := map[string]bool{}
	for _, name := range names {
		if name == "" {
			continue
		}
		appendConfiguredContainer(&containers, added, cfg, name, ps, stats)
	}
	for _, group := range cfg.ContainerGroups {
		appendConfiguredContainerGroup(&containers, added, cfg, group, ps, stats)
	}
	return host, containers
}

func appendConfiguredContainer(rows *[]MonitoringContainer, added map[string]bool, cfg MonitorHostConfig, configuredName string, ps map[string]string, stats map[string]MonitoringContainer) {
	name := configuredName
	if _, ok := ps[name]; !ok {
		if peer := blueGreenPeerName(name); peer != "" {
			if _, peerOK := ps[peer]; peerOK {
				name = peer
			}
		}
	}
	if added[name] {
		return
	}
	row := monitoringContainerRow(cfg, name, ps, stats)
	row.Configured = true
	if row.Status == "missing" {
		row.Message = "未在 docker ps 中找到"
	}
	added[name] = true
	*rows = append(*rows, row)
}

func appendConfiguredContainerGroup(rows *[]MonitoringContainer, added map[string]bool, cfg MonitorHostConfig, group []string, ps map[string]string, stats map[string]MonitoringContainer) {
	candidates := normalizeContainerGroup(group)
	if len(candidates) == 0 {
		return
	}
	found := false
	for _, name := range candidates {
		if _, ok := ps[name]; !ok {
			continue
		}
		if added[name] {
			found = true
			continue
		}
		row := monitoringContainerRow(cfg, name, ps, stats)
		row.Configured = true
		added[name] = true
		*rows = append(*rows, row)
		found = true
	}
	if found {
		return
	}
	groupName := blueGreenGroupName(candidates)
	if added[groupName] {
		return
	}
	added[groupName] = true
	*rows = append(*rows, MonitoringContainer{
		HostID:     cfg.ID,
		HostName:   cfg.Name,
		Name:       groupName,
		Status:     "missing",
		Configured: true,
		Message:    "蓝绿槽位都未在 docker ps 中找到",
	})
}

func monitoringContainerRow(cfg MonitorHostConfig, name string, ps map[string]string, stats map[string]MonitoringContainer) MonitoringContainer {
	row := MonitoringContainer{HostID: cfg.ID, HostName: cfg.Name, Name: name, Status: "missing", Configured: true}
	if status, ok := ps[name]; ok {
		row.Status = status
	}
	if stat, ok := stats[name]; ok {
		row.CPUPercent = stat.CPUPercent
		row.MemoryUsage = stat.MemoryUsage
		row.MemoryPercent = stat.MemoryPercent
	}
	return row
}

func normalizeContainerGroup(group []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, name := range group {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func blueGreenPeerName(name string) string {
	switch {
	case strings.HasSuffix(name, "-blue"):
		return strings.TrimSuffix(name, "-blue") + "-green"
	case strings.HasSuffix(name, "-green"):
		return strings.TrimSuffix(name, "-green") + "-blue"
	default:
		return ""
	}
}

func blueGreenGroupName(group []string) string {
	if len(group) == 0 {
		return "蓝绿容器组"
	}
	base := ""
	colors := []string{}
	seenColors := map[string]bool{}
	for _, name := range group {
		color := ""
		prefix := name
		if strings.HasSuffix(name, "-blue") {
			prefix = strings.TrimSuffix(name, "-blue")
			color = "blue"
		} else if strings.HasSuffix(name, "-green") {
			prefix = strings.TrimSuffix(name, "-green")
			color = "green"
		}
		if base == "" {
			base = prefix
		} else if base != prefix {
			base = strings.Join(group, " / ")
			break
		}
		if color != "" && !seenColors[color] {
			seenColors[color] = true
			colors = append(colors, color)
		}
	}
	if len(colors) == 0 || strings.Contains(base, " / ") {
		return base
	}
	sort.Strings(colors)
	return fmt.Sprintf("%s (%s)", base, strings.Join(colors, "/"))
}

func mergeSSHHost(host *MonitoringHost, sshHost MonitoringHost) {
	if host.Source != "beszel" || host.CPUPercent == 0 {
		host.CPUPercent = sshHost.CPUPercent
	}
	if host.Source != "beszel" || host.MemoryPercent == 0 {
		host.MemoryUsedBytes = sshHost.MemoryUsedBytes
		host.MemoryTotalBytes = sshHost.MemoryTotalBytes
		host.MemoryPercent = sshHost.MemoryPercent
	}
	if host.Source != "beszel" || host.DiskPercent == 0 {
		host.DiskUsedBytes = sshHost.DiskUsedBytes
		host.DiskTotalBytes = sshHost.DiskTotalBytes
		host.DiskPercent = sshHost.DiskPercent
	}
	if host.Uptime == "" {
		host.Uptime = sshHost.Uptime
	}
	if host.UptimeSeconds == 0 {
		host.UptimeSeconds = sshHost.UptimeSeconds
	}
	if host.Load1 == 0 && host.Load5 == 0 && host.Load15 == 0 {
		host.Load1 = sshHost.Load1
		host.Load5 = sshHost.Load5
		host.Load15 = sshHost.Load15
	}
	if host.NetworkBytes == 0 {
		host.NetworkBytes = sshHost.NetworkBytes
	}
	if host.DockerReclaimableBytes == 0 {
		host.DockerReclaimableBytes = sshHost.DockerReclaimableBytes
	}
	if len(host.DiskBreakdown) == 0 {
		host.DiskBreakdown = sshHost.DiskBreakdown
	}
	if host.Source != "beszel" {
		host.Source = "ssh_fallback"
		host.Status = sshHost.Status
		host.Message = sshHost.Message
	} else if !monitoringStatusHealthy(host.Status) && monitoringStatusHealthy(sshHost.Status) {
		host.Source = "ssh_fallback"
		host.Status = sshHost.Status
		host.Message = "Beszel 记录不可用，已使用 SSH 兜底"
	}
	host.CheckedAt = sshHost.CheckedAt
}

func monitoringStatusHealthy(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "ok", "up", "online", "running", "healthy":
		return true
	default:
		return false
	}
}

func buildMonitoringAlerts(hosts []MonitoringHost, containers []MonitoringContainer, cfg Config) []MonitoringAlert {
	alerts := []MonitoringAlert{}
	warnDisk := float64(maxInt(cfg.MonitorWarnDisk, 80))
	critDisk := float64(maxInt(cfg.MonitorCritDisk, 90))
	warnMemory := float64(maxInt(cfg.MonitorWarnMemory, 80))
	for _, host := range hosts {
		if host.DiskPercent >= critDisk {
			alerts = append(alerts, MonitoringAlert{Level: "critical", HostID: host.ID, HostName: host.Name, Metric: "disk", Title: "磁盘接近满载", Message: fmt.Sprintf("%s 磁盘 %.1f%%，需要尽快清理", host.Name, host.DiskPercent), ActionLabel: "诊断磁盘", ActionURL: "/monitoring"})
		} else if host.DiskPercent >= warnDisk {
			alerts = append(alerts, MonitoringAlert{Level: "warning", HostID: host.ID, HostName: host.Name, Metric: "disk", Title: "磁盘偏高", Message: fmt.Sprintf("%s 磁盘 %.1f%%，建议关注", host.Name, host.DiskPercent), ActionLabel: "诊断磁盘", ActionURL: "/monitoring"})
		}
		if host.MemoryPercent >= warnMemory {
			alerts = append(alerts, MonitoringAlert{Level: "warning", HostID: host.ID, HostName: host.Name, Metric: "memory", Title: "内存偏高", Message: fmt.Sprintf("%s 内存 %.1f%%", host.Name, host.MemoryPercent)})
		}
		if host.Status != "" && host.Status != "ok" && host.Status != "up" && host.Status != "online" && host.Status != "unknown" {
			alerts = append(alerts, MonitoringAlert{Level: "warning", HostID: host.ID, HostName: host.Name, Metric: "host", Title: "机器状态异常", Message: fmt.Sprintf("%s 状态：%s", host.Name, host.Status)})
		}
		if host.DockerReclaimableBytes > 10*1024*1024*1024 {
			reclaimGB := host.DockerReclaimableBytes / (1024 * 1024 * 1024)
			alerts = append(alerts, MonitoringAlert{Level: "info", HostID: host.ID, HostName: host.Name, Metric: "disk", Title: "Docker 可回收空间较大", Message: fmt.Sprintf("%s 可回收约 %d GB Docker 空间", host.Name, reclaimGB), ActionLabel: "诊断磁盘", ActionURL: "/monitoring"})
		}
	}
	for _, row := range containers {
		status := strings.ToLower(row.Status)
		if status == "missing" || strings.Contains(status, "exited") || strings.Contains(status, "dead") || strings.Contains(status, "restarting") {
			alerts = append(alerts, MonitoringAlert{Level: "critical", HostID: row.HostID, HostName: row.HostName, Metric: "container", Title: "核心容器异常", Message: fmt.Sprintf("%s / %s：%s", row.HostName, row.Name, row.Status)})
		}
	}
	return alerts
}

func parseMonitorHosts(cfg Config) []MonitorHostConfig {
	if strings.TrimSpace(cfg.MonitorHostsJSON) != "" {
		var hosts []MonitorHostConfig
		if err := json.Unmarshal([]byte(cfg.MonitorHostsJSON), &hosts); err == nil {
			return normalizeMonitorHosts(hosts, cfg.MonitorSSHKeyPath)
		}
		logMonitoringConfigError("PEAPOD_MONITOR_HOSTS_JSON 解析失败，使用默认监控主机")
	}
	return normalizeMonitorHosts(defaultMonitorHosts(), cfg.MonitorSSHKeyPath)
}

func defaultMonitorHosts() []MonitorHostConfig {
	return []MonitorHostConfig{
		{
			ID:          "local",
			Name:        "本机",
			Role:        "infra",
			BeszelNames: []string{"local", "localhost", "本机", "peapod", "zephyr"},
			Containers: []string{
				"peapod",
				"woodpecker-server",
				"woodpecker-agent",
				"beszel",
				"grafana",
				"loki",
				"prometheus",
				"tempo",
				"mysql",
				"postgres",
			},
		},
	}
}

func normalizeMonitorHosts(hosts []MonitorHostConfig, defaultKeyPath string) []MonitorHostConfig {
	out := []MonitorHostConfig{}
	for _, host := range hosts {
		host.ID = normalizeMonitorName(host.ID)
		host.Name = strings.TrimSpace(host.Name)
		host.Role = strings.TrimSpace(host.Role)
		host.SSHHost = strings.TrimSpace(firstNonEmpty(host.SSHHost, host.Address))
		host.SSHUser = strings.TrimSpace(firstNonEmpty(host.SSHUser, "codex"))
		host.SSHKeyPath = strings.TrimSpace(firstNonEmpty(host.SSHKeyPath, defaultKeyPath))
		host.CleanupTaskID = strings.TrimSpace(host.CleanupTaskID)
		host.Containers = normalizeContainerGroup(host.Containers)
		groups := make([][]string, 0, len(host.ContainerGroups))
		for _, group := range host.ContainerGroups {
			if normalized := normalizeContainerGroup(group); len(normalized) > 0 {
				groups = append(groups, normalized)
			}
		}
		host.ContainerGroups = groups
		if host.ID == "" {
			host.ID = normalizeMonitorName(host.Name)
		}
		if host.ID == "" {
			continue
		}
		if host.Name == "" {
			host.Name = host.ID
		}
		out = append(out, host)
	}
	return out
}

func logMonitoringConfigError(message string) {
	fmt.Fprintln(os.Stderr, message)
}

func lookupString(value any, key string) (string, bool) {
	return lookupStringAny(value, []string{key})
}

func directString(values map[string]any, key string) (string, bool) {
	value, ok := values[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	return text, text != ""
}

func lookupStringAny(value any, names []string) (string, bool) {
	if parsed, ok := parseJSONString(value); ok {
		value = parsed
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if monitorKeyMatches(key, names) {
				if s, ok := child.(string); ok {
					return strings.TrimSpace(s), true
				}
			}
		}
		for _, child := range typed {
			if s, ok := lookupStringAny(child, names); ok {
				return s, true
			}
		}
	case []any:
		for _, child := range typed {
			if s, ok := lookupStringAny(child, names); ok {
				return s, true
			}
		}
	case string:
		return strings.TrimSpace(typed), false
	}
	return "", false
}

func lookupNumberAny(value any, names []string) (float64, bool) {
	if parsed, ok := parseJSONString(value); ok {
		value = parsed
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if monitorKeyMatches(key, names) {
				if number, ok := asFloat(child); ok {
					return number, true
				}
			}
		}
		for _, child := range typed {
			if number, ok := lookupNumberAny(child, names); ok {
				return number, true
			}
		}
	case []any:
		for _, child := range typed {
			if number, ok := lookupNumberAny(child, names); ok {
				return number, true
			}
		}
	}
	return 0, false
}

func lookupNumberArrayAny(value any, names []string) ([]float64, bool) {
	if parsed, ok := parseJSONString(value); ok {
		value = parsed
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if monitorKeyMatches(key, names) {
				if numbers, ok := asFloatArray(child); ok {
					return numbers, true
				}
			}
		}
		for _, child := range typed {
			if numbers, ok := lookupNumberArrayAny(child, names); ok {
				return numbers, true
			}
		}
	case []any:
		if numbers, ok := asFloatArray(typed); ok {
			return numbers, true
		}
		for _, child := range typed {
			if numbers, ok := lookupNumberArrayAny(child, names); ok {
				return numbers, true
			}
		}
	}
	return nil, false
}

func parseJSONString(value any) (any, bool) {
	text, ok := value.(string)
	if !ok {
		return nil, false
	}
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		return nil, false
	}
	var parsed any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, false
	}
	return parsed, true
}

func asFloatArray(value any) ([]float64, bool) {
	rows, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := []float64{}
	for _, item := range rows {
		number, ok := asFloat(item)
		if !ok {
			continue
		}
		out = append(out, number)
	}
	return out, len(out) > 0
}

func asFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		number, err := typed.Float64()
		return number, err == nil
	case string:
		number, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(typed), "%"), 64)
		return number, err == nil
	default:
		return 0, false
	}
}

func monitorKeyMatches(key string, names []string) bool {
	cleanKey := normalizeMonitorKey(key)
	for _, name := range names {
		cleanName := normalizeMonitorKey(name)
		if cleanName != "" && (cleanKey == cleanName || strings.Contains(cleanKey, cleanName)) {
			return true
		}
	}
	return false
}

func normalizeMonitorStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "up", "online", "ok", "healthy", "active":
		return "ok"
	case "":
		return "unknown"
	default:
		return status
	}
}

func normalizeMonitorName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

func normalizeMonitorKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "", "-", "", " ", "", ".", "")
	return replacer.Replace(value)
}

func normalizePercent(value float64) float64 {
	if value > 0 && value <= 1 {
		value *= 100
	}
	return roundPercent(value)
}

func normalizeBytes(value float64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func parsePercent(value string) float64 {
	value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
	if value == "" {
		return 0
	}
	parsed, _ := strconv.ParseFloat(value, 64)
	return roundPercent(parsed)
}

func parseDockerMemoryPercent(value string) float64 {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0
	}
	used := parseHumanBytes(parts[0])
	total := parseHumanBytes(parts[1])
	if total <= 0 {
		return 0
	}
	return roundPercent(float64(used) / float64(total) * 100)
}

func parseHumanBytes(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	fields := strings.Fields(value)
	if len(fields) > 1 {
		value = fields[0] + fields[1]
	}
	index := 0
	for index < len(value) && ((value[index] >= '0' && value[index] <= '9') || value[index] == '.') {
		index++
	}
	number, _ := strconv.ParseFloat(value[:index], 64)
	unit := strings.ToLower(strings.TrimSpace(value[index:]))
	multipliers := map[string]float64{
		"b":   1,
		"kb":  1000,
		"kib": 1024,
		"mb":  1000 * 1000,
		"mib": 1024 * 1024,
		"gb":  1000 * 1000 * 1000,
		"gib": 1024 * 1024 * 1024,
	}
	if multiplier, ok := multipliers[unit]; ok {
		return number * multiplier
	}
	return number
}

func roundPercent(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}

func formatUptimeSeconds(seconds uint64) string {
	if seconds == 0 {
		return ""
	}
	if seconds < 60 {
		return "刚启动"
	}
	days := seconds / 86400
	seconds %= 86400
	hours := seconds / 3600
	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%d 天 %d 小时", days, hours)
		}
		return fmt.Sprintf("%d 天", days)
	}
	minutes := (seconds % 3600) / 60
	if hours > 0 {
		return fmt.Sprintf("%d 小时 %d 分钟", hours, minutes)
	}
	return fmt.Sprintf("%d 分钟", minutes)
}

func alertRank(level string) int {
	switch level {
	case "critical":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
