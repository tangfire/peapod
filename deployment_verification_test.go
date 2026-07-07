package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestApplyDeploymentVerificationMarkerAndHealthOK(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "current-source-sha")
	if err := os.WriteFile(marker, []byte("abcdef1234567890\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	status := &DeploymentStatus{CurrentCommit: "abcdef12"}
	applyDeploymentVerification(status, deploymentVerifyConfig{MarkerPath: marker, HealthURL: server.URL, Timeout: time.Second})

	if !status.DeployVerified {
		t.Fatalf("DeployVerified = false, message=%q", status.DeployVerifyMessage)
	}
	if status.DeployVerifyStatus != "verified" {
		t.Fatalf("DeployVerifyStatus = %q", status.DeployVerifyStatus)
	}
	if status.ActualCommit != "abcdef1234567890" {
		t.Fatalf("ActualCommit = %q", status.ActualCommit)
	}
}

func TestApplyDeploymentVerificationDetectsMarkerMismatch(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "current-source-sha")
	if err := os.WriteFile(marker, []byte("bbbbbbbb\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	status := &DeploymentStatus{CurrentCommit: "aaaaaaaa"}
	applyDeploymentVerification(status, deploymentVerifyConfig{MarkerPath: marker})

	if status.DeployVerified {
		t.Fatal("DeployVerified = true, want false")
	}
	if status.DeployVerifyStatus != "marker_mismatch" {
		t.Fatalf("DeployVerifyStatus = %q", status.DeployVerifyStatus)
	}
	if !strings.Contains(status.DeployVerifyMessage, "实际版本") {
		t.Fatalf("message = %q", status.DeployVerifyMessage)
	}
}

func TestApplyDeploymentVerificationPipelineOnly(t *testing.T) {
	status := &DeploymentStatus{CurrentCommit: "aaaaaaaa"}
	applyDeploymentVerification(status, deploymentVerifyConfig{})

	if status.DeployVerified {
		t.Fatal("DeployVerified = true, want false")
	}
	if status.DeployVerifyStatus != "pipeline_only" {
		t.Fatalf("DeployVerifyStatus = %q", status.DeployVerifyStatus)
	}
	if !strings.Contains(status.DeployVerifyMessage, "构建成功，部署未验证") {
		t.Fatalf("message = %q", status.DeployVerifyMessage)
	}
}

func TestApplyDeploymentVerificationHealthOKWithMissingMarkerIsDegraded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	status := &DeploymentStatus{CurrentCommit: "aaaaaaaa"}
	applyDeploymentVerification(status, deploymentVerifyConfig{MarkerPath: filepath.Join(t.TempDir(), "missing-sha"), HealthURL: server.URL, Timeout: time.Second})

	if !status.DeployVerified {
		t.Fatalf("DeployVerified = false, message=%q", status.DeployVerifyMessage)
	}
	if !status.DeployDegraded {
		t.Fatal("DeployDegraded = false, want true")
	}
	if status.DeployVerifyStatus != "marker_unavailable" {
		t.Fatalf("DeployVerifyStatus = %q", status.DeployVerifyStatus)
	}
	if !strings.Contains(status.DeployVerifyMessage, "服务健康检查已通过") {
		t.Fatalf("message = %q", status.DeployVerifyMessage)
	}
}

func TestDeploymentStatusesOnlyVerifiedPipelineBecomesCurrent(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "current-source-sha")
	if err := os.WriteFile(marker, []byte("verified123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	task := Task{
		ID:     "deploy-app",
		Title:  "部署应用",
		Group:  "业务服务",
		RepoID: 7,
		Branch: "main",
		Variables: map[string]string{
			"DEPLOY_ACTION":             "deploy",
			"PEAPOD_PROJECT_ID":         "app",
			"PEAPOD_DEPLOY_MARKER_PATH": marker,
		},
	}
	pipelines := map[int][]Pipeline{7: {
		{Number: 2, Status: "success", Branch: "main", Commit: "unverified999", Finished: 200, Variables: task.Variables},
		{Number: 1, Status: "success", Branch: "main", Commit: "verified123", Finished: 100, Variables: task.Variables},
	}}

	rows := deploymentStatuses([]Task{task}, map[int]string{7: "app"}, pipelines)
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].CurrentCommit != "verified123" {
		t.Fatalf("CurrentCommit = %q, want verified commit", rows[0].CurrentCommit)
	}
	if rows[0].Pipeline != 1 {
		t.Fatalf("Pipeline = %d, want verified pipeline #1", rows[0].Pipeline)
	}
	if rows[0].LatestCommit != "unverified999" {
		t.Fatalf("LatestCommit = %q, want latest unverified commit", rows[0].LatestCommit)
	}
}

func TestNormalizeTaskConfigRequiresDeploymentVerification(t *testing.T) {
	task := Task{
		ID:     "deploy-app",
		Title:  "部署应用",
		RepoID: 7,
		Branch: "main",
		Variables: map[string]string{
			"DEPLOY_ACTION":     "deploy",
			"PEAPOD_PROJECT_ID": "app",
		},
	}
	if err := normalizeTaskConfig(&task); err == nil {
		t.Fatal("normalizeTaskConfig returned nil, want verification error")
	}
	task.Variables["PEAPOD_DEPLOY_VERIFY_URL"] = "http://127.0.0.1:8080/healthz"
	if err := normalizeTaskConfig(&task); err != nil {
		t.Fatalf("normalizeTaskConfig with healthz returned error: %v", err)
	}
}

func TestMaintenanceTaskDoesNotRequireDeploymentVerification(t *testing.T) {
	task := Task{
		ID:     "cleanup",
		Title:  "清理磁盘",
		RepoID: 7,
		Variables: map[string]string{
			"DEPLOY_ACTION": "cleanup",
		},
	}
	if err := normalizeTaskConfig(&task); err != nil {
		t.Fatalf("normalizeTaskConfig maintenance returned error: %v", err)
	}
}

func TestBuildTaskFromTemplateAddsVerificationDefaults(t *testing.T) {
	template, ok := findTaskTemplate("docker-compose-service")
	if !ok {
		t.Fatal("docker-compose-service template not found")
	}
	task, err := buildTaskFromTemplate(template, TemplateApplyRequest{
		RepoID:      12,
		RepoName:    "owner/app",
		Branch:      "main",
		ProjectID:   "app",
		ProjectName: "应用服务",
		Environment: "production",
	})
	if err != nil {
		t.Fatalf("buildTaskFromTemplate returned error: %v", err)
	}
	if task.Variables["PEAPOD_PROJECT_ID"] != "app" {
		t.Fatalf("PEAPOD_PROJECT_ID = %q", task.Variables["PEAPOD_PROJECT_ID"])
	}
	if task.Variables["PEAPOD_PROJECT_ENV"] != "production" {
		t.Fatalf("PEAPOD_PROJECT_ENV = %q", task.Variables["PEAPOD_PROJECT_ENV"])
	}
	if task.Variables["PEAPOD_DEPLOY_MARKER_PATH"] == "" {
		t.Fatal("template did not add default marker path")
	}
	if !taskHasDeploymentVerification(task) {
		t.Fatal("template task should have deployment verification")
	}
}

func TestDoctorReadinessTreatsWarningAsNotReady(t *testing.T) {
	readiness := doctorReadiness([]DoctorCheck{
		{ID: "docker", Severity: "ok"},
		{ID: "woodpecker-oauth", Severity: "warning"},
	})
	if readiness != "warning" {
		t.Fatalf("readiness = %q, want warning", readiness)
	}
	readiness = doctorReadiness([]DoctorCheck{
		{ID: "docker", Severity: "ok"},
		{ID: "token", Severity: "error"},
	})
	if readiness != "blocked" {
		t.Fatalf("readiness = %q, want blocked", readiness)
	}
}
