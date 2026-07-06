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
