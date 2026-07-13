package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
		MonitorAutoCleanupLevel:       envFirst("", "PEAPOD_MONITOR_AUTO_CLEANUP_LEVEL", "ZEPHYR_MONITOR_AUTO_CLEANUP_LEVEL", "ZEFIRE_MONITOR_AUTO_CLEANUP_LEVEL"),
		MonitorAutoCleanupDisk:        envIntFirst(0, "PEAPOD_MONITOR_AUTO_CLEANUP_DISK", "ZEPHYR_MONITOR_AUTO_CLEANUP_DISK", "ZEFIRE_MONITOR_AUTO_CLEANUP_DISK"),
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
	if runtime.MonitorAutoCleanupLevel != "" {
		cfg.MonitorAutoCleanupLevel = runtime.MonitorAutoCleanupLevel
	}
	if runtime.MonitorAutoCleanupDisk > 0 {
		cfg.MonitorAutoCleanupDisk = runtime.MonitorAutoCleanupDisk
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
		MonitorAutoCleanupLevel: strings.TrimSpace(input.MonitorAutoCleanupLevel),
		MonitorAutoCleanupDisk:  clampInt(input.MonitorAutoCleanupDisk, 0, 100, current.MonitorAutoCleanupDisk),
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
