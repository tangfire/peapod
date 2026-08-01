package main

import "testing"

func TestParseKeyValue(t *testing.T) {
	key, value, err := parseKeyValue("DEPLOY_ACTION=test-deploy")
	if err != nil {
		t.Fatalf("parseKeyValue returned error: %v", err)
	}
	if key != "DEPLOY_ACTION" || value != "test-deploy" {
		t.Fatalf("parseKeyValue = %q, %q", key, value)
	}
	if _, _, err := parseKeyValue("missing-equals"); err == nil {
		t.Fatal("parseKeyValue accepted missing equals")
	}
}

func TestTerminalPipelineStatus(t *testing.T) {
	for _, status := range []string{"success", "failure", "error", "killed", "skipped"} {
		if !terminalPipelineStatus(status) {
			t.Fatalf("%s should be terminal", status)
		}
	}
	for _, status := range []string{"pending", "running", ""} {
		if terminalPipelineStatus(status) {
			t.Fatalf("%s should not be terminal", status)
		}
	}
}

func TestDeploymentTargetIDPrefersProjectMetadata(t *testing.T) {
	got := deploymentTargetID(task{
		ID:     "xzm-test-deploy",
		Group:  "写书猫",
		Title:  "部署测试环境",
		RepoID: 7,
		Variables: map[string]string{
			"PEAPOD_PROJECT_ID": "novelcat-test",
		},
	})
	if got != "repo-7:novelcat-test" {
		t.Fatalf("deploymentTargetID = %q", got)
	}
}

func TestDeploymentStatusReady(t *testing.T) {
	for _, status := range []deploymentStatus{
		{DeployVerified: true},
		{DeployVerifyStatus: "health_only"},
		{DeployVerifyStatus: "marker_unavailable"},
		{DeployVerifyStatus: "external_marker"},
	} {
		if !deploymentStatusReady(status) {
			t.Fatalf("status %+v should be ready", status)
		}
	}
	for _, status := range []deploymentStatus{
		{DeployVerifyStatus: "pipeline_only"},
		{DeployVerifyStatus: "marker_missing"},
		{DeployVerifyStatus: "health_failed"},
	} {
		if deploymentStatusReady(status) {
			t.Fatalf("status %+v should not be ready", status)
		}
	}
}
