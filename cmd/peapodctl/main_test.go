package main

import (
	"flag"
	"testing"
	"time"
)

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

func TestNormalizeLocalWoodpeckerServerHostGateway(t *testing.T) {
	got := normalizeLocalWoodpeckerServer("http://host.docker.internal:8000")
	if got != "http://127.0.0.1:8000" {
		t.Fatalf("normalizeLocalWoodpeckerServer = %q", got)
	}
}

func TestBindLocalSourceBranchUsesConfiguredDeploymentTask(t *testing.T) {
	variables := map[string]string{
		"DEPLOY_ACTION":             "test-deploy",
		"PEAPOD_DEPLOY_VERIFY_URL":  "https://test.example/api/health",
		"PEAPOD_DEPLOY_MARKER_PATH": "/opt/app/.deploy/current-source-sha",
		"PEAPOD_REQUESTED_BRANCH":   "old",
		"SOURCE_BRANCH":             "old",
	}
	bindLocalSourceBranch(task{Variables: variables}, variables, "dev")
	if variables["SOURCE_BRANCH"] != "dev" {
		t.Fatalf("SOURCE_BRANCH = %q", variables["SOURCE_BRANCH"])
	}
	if variables["PEAPOD_REQUESTED_BRANCH"] != "dev" {
		t.Fatalf("PEAPOD_REQUESTED_BRANCH = %q", variables["PEAPOD_REQUESTED_BRANCH"])
	}
}

func TestParseInterspersedFlagsAllowsTaskBeforeFlags(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var branch string
	var timeout time.Duration
	var wait bool
	fs.StringVar(&branch, "branch", "", "")
	fs.DurationVar(&timeout, "timeout", 0, "")
	fs.BoolVar(&wait, "wait", false, "")

	if err := parseInterspersedFlags(fs, []string{"xzm-test-deploy", "--branch", "dev", "--timeout", "60m", "--wait"}); err != nil {
		t.Fatalf("parseInterspersedFlags returned error: %v", err)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "xzm-test-deploy" {
		t.Fatalf("positional args = %v", fs.Args())
	}
	if branch != "dev" || timeout != 60*time.Minute || !wait {
		t.Fatalf("parsed flags = branch=%q timeout=%s wait=%v", branch, timeout, wait)
	}
}

func TestParseInterspersedFlagsKeepsInlineValues(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var branch string
	fs.StringVar(&branch, "branch", "", "")

	if err := parseInterspersedFlags(fs, []string{"--branch=dev", "xzm-test-deploy"}); err != nil {
		t.Fatalf("parseInterspersedFlags returned error: %v", err)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "xzm-test-deploy" || branch != "dev" {
		t.Fatalf("args/flags = args=%v branch=%q", fs.Args(), branch)
	}
}
