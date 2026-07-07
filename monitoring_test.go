package main

import "testing"

func TestParseSSHMonitorOutputResolvesBlueGreenPeer(t *testing.T) {
	output := "__PS__\nnovel-factory-api-green\tUp 10 minutes (healthy)\n__STATS__\nnovel-factory-api-green\t1.1%\t19MiB / 3.5GiB\n"
	cfg := MonitorHostConfig{
		ID:         "production",
		Name:       "生产机",
		Containers: []string{"novel-factory-api-blue"},
	}

	_, rows := parseSSHMonitorOutput(output, cfg)
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].Name != "novel-factory-api-green" {
		t.Fatalf("row name = %q, want green peer", rows[0].Name)
	}
	if rows[0].Status == "missing" {
		t.Fatalf("blue-green peer should not be reported missing: %+v", rows[0])
	}
}

func TestParseSSHMonitorOutputBlueGreenGroupMissingOnlyWhenAllCandidatesMissing(t *testing.T) {
	output := "__PS__\nnovel-factory-api-green\tUp 10 minutes (healthy)\n__STATS__\nnovel-factory-api-green\t1.1%\t19MiB / 3.5GiB\n"
	cfg := MonitorHostConfig{
		ID:   "production",
		Name: "生产机",
		ContainerGroups: [][]string{
			{"novel-factory-api-blue", "novel-factory-api-green"},
			{"novel-factory-web-blue", "novel-factory-web-green"},
		},
	}

	_, rows := parseSSHMonitorOutput(output, cfg)
	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want active api plus missing web group", len(rows))
	}
	if rows[0].Name != "novel-factory-api-green" || rows[0].Status == "missing" {
		t.Fatalf("first row should be active green api: %+v", rows[0])
	}
	if rows[1].Name != "novel-factory-web (blue/green)" || rows[1].Status != "missing" {
		t.Fatalf("second row should be missing logical web group: %+v", rows[1])
	}
}

func TestMatchBeszelHostDoesNotUseGenericBuilderAlias(t *testing.T) {
	service := &MonitoringService{hosts: []MonitorHostConfig{
		{
			ID:          "estar-production",
			Name:        "e站生产机 / 边缘入口",
			BeszelNames: []string{"xiezuomao-builder", "e站生产机", "159.75.153.53"},
		},
		{
			ID:          "ops-test-builder",
			Name:        "测试/构建机",
			BeszelNames: []string{"peapod-ops-builder", "peapod", "111.230.32.26"},
		},
	}}

	cfg, ok := service.matchBeszelHost(map[string]any{
		"name": "peapod-ops-builder",
		"host": "111.230.32.26",
	})
	if !ok {
		t.Fatal("expected peapod host to match")
	}
	if cfg.ID != "ops-test-builder" {
		t.Fatalf("matched host = %q, want ops-test-builder", cfg.ID)
	}
}

func TestFormatUptimeSecondsAvoidsZeroMinutes(t *testing.T) {
	cases := []struct {
		name    string
		seconds uint64
		want    string
	}{
		{name: "unknown", seconds: 0, want: ""},
		{name: "just started", seconds: 12, want: "刚启动"},
		{name: "one minute", seconds: 60, want: "1 分钟"},
		{name: "hour", seconds: 3660, want: "1 小时 1 分钟"},
		{name: "day", seconds: 90000, want: "1 天 1 小时"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatUptimeSeconds(tt.seconds); got != tt.want {
				t.Fatalf("formatUptimeSeconds(%d) = %q, want %q", tt.seconds, got, tt.want)
			}
		})
	}
}
