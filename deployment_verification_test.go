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

	if status.DeployVerified {
		t.Fatalf("DeployVerified = true, want false: message=%q", status.DeployVerifyMessage)
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

func TestApplyDeploymentVerificationHealthOnlyDoesNotClaimVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	status := &DeploymentStatus{CurrentCommit: "aaaaaaaa"}
	applyDeploymentVerification(status, deploymentVerifyConfig{HealthURL: server.URL, Timeout: time.Second})

	if status.DeployVerified {
		t.Fatal("health-only verification must not claim a deployed commit")
	}
	if status.DeployVerifyStatus != "health_only" {
		t.Fatalf("DeployVerifyStatus = %q", status.DeployVerifyStatus)
	}
}

func TestDeploymentSourceBranchPrefersBoundSourceBranch(t *testing.T) {
	pipeline := Pipeline{
		Branch: "main",
		Variables: map[string]string{
			"SOURCE_BRANCH": "dev",
		},
	}
	if got := deploymentSourceBranch(pipeline); got != "dev" {
		t.Fatalf("deploymentSourceBranch = %q, want dev", got)
	}
}

func TestBindSelectedSourceBranchOverridesStaleTaskVariable(t *testing.T) {
	variables := map[string]string{
		"DEPLOY_ACTION":            "test-deploy",
		"PEAPOD_PROJECT_ID":        "novel-test",
		"PEAPOD_DEPLOY_VERIFY_URL": "https://test.example/healthz",
		"source_branch":            "dev",
	}
	task := Task{RepoID: 1, Variables: variables}

	bindSelectedSourceBranch(task, variables, "main")

	if got := variables["SOURCE_BRANCH"]; got != "main" {
		t.Fatalf("SOURCE_BRANCH = %q, want main", got)
	}
	if _, exists := variables["source_branch"]; exists {
		t.Fatal("stale case-insensitive source_branch was not removed")
	}
	if got := variables["PEAPOD_REQUESTED_BRANCH"]; got != "main" {
		t.Fatalf("PEAPOD_REQUESTED_BRANCH = %q, want main", got)
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

func TestDeploymentStatusesHealthOnlyKeepsVersionUnconfirmed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	task := Task{
		ID:     "deploy-app",
		Title:  "部署应用",
		RepoID: 7,
		Branch: "main",
		Variables: map[string]string{
			"DEPLOY_ACTION":            "test-deploy",
			"PEAPOD_PROJECT_ID":        "app-test",
			"PEAPOD_DEPLOY_VERIFY_URL": server.URL,
		},
	}
	pipelines := map[int][]Pipeline{7: {
		{Number: 2, Status: "success", Branch: "main", Commit: "unverified999", Finished: 200, Variables: task.Variables},
	}}

	rows := deploymentStatuses([]Task{task}, map[int]string{7: "app"}, pipelines)
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].CurrentCommit != "" || rows[0].CurrentBranch != "" {
		t.Fatalf("health-only status claimed current version: branch=%q commit=%q", rows[0].CurrentBranch, rows[0].CurrentCommit)
	}
	if rows[0].LatestCommit != "unverified999" || rows[0].DeployVerifyStatus != "health_only" {
		t.Fatalf("latest status = %#v", rows[0])
	}
}

func TestDeploymentStatusesUseRollbackTargetCommit(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "current-source-sha")
	if err := os.WriteFile(marker, []byte("abcdef1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	task := Task{
		ID:     "rollback-app",
		Title:  "回滚应用",
		Group:  "业务服务",
		RepoID: 7,
		Branch: "main",
		Variables: map[string]string{
			"DEPLOY_ACTION":             "rollback",
			"PEAPOD_PROJECT_ID":         "app",
			"PEAPOD_DEPLOY_MARKER_PATH": marker,
		},
	}
	variables := map[string]string{
		"DEPLOY_ACTION":             "rollback",
		"PEAPOD_PROJECT_ID":         "app",
		"PEAPOD_DEPLOY_MARKER_PATH": marker,
		"ROLLBACK_COMMIT":           "abcdef1",
	}
	pipelines := map[int][]Pipeline{7: {
		{Number: 9, Status: "success", Branch: "main", Commit: "current9", Finished: 900, Variables: variables},
	}}

	rows := deploymentStatuses([]Task{task}, map[int]string{7: "app"}, pipelines)
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].CurrentCommit != "abcdef1" {
		t.Fatalf("CurrentCommit = %q, want rollback target", rows[0].CurrentCommit)
	}
	if rows[0].LatestCommit != "abcdef1" {
		t.Fatalf("LatestCommit = %q, want rollback target", rows[0].LatestCommit)
	}
	if !rows[0].DeployVerified {
		t.Fatalf("rollback target should verify against marker: status=%s message=%s", rows[0].DeployVerifyStatus, rows[0].DeployVerifyMessage)
	}
}

func TestDeploymentRevisionsKeepOriginalPipelineCommitsWhenMarkerIsCurrent(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "current-source-sha")
	if err := os.WriteFile(marker, []byte("cbe6587c\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	baseVariables := map[string]string{
		"PEAPOD_PROJECT_ID":            "app",
		"PEAPOD_DEPLOY_MARKER_PATH":    marker,
		"PEAPOD_DEPLOY_VERIFY_URL":     server.URL,
		"PEAPOD_PROJECT_NAME":          "App",
		"PEAPOD_PROJECT_GROUP":         "Products",
		"PEAPOD_DEPLOY_VERIFY_TIMEOUT": "1",
	}
	deployVariables := cloneMap(baseVariables)
	deployVariables["DEPLOY_ACTION"] = "deploy"
	rollbackVariables := cloneMap(baseVariables)
	rollbackVariables["DEPLOY_ACTION"] = "rollback"
	rollbackVariables["ROLLBACK_COMMIT"] = "cbe6587c"

	task := Task{
		ID:        "deploy-app",
		Title:     "Deploy App",
		Group:     "Products",
		RepoID:    7,
		RepoName:  "app",
		Branch:    "main",
		Variables: deployVariables,
	}
	pipelines := map[int][]Pipeline{7: {
		{Number: 797, Status: "success", Branch: "main", Commit: "runnerhead", Finished: 797, Variables: rollbackVariables},
		{Number: 796, Status: "success", Branch: "main", Commit: "cbe6587c", Finished: 796, Variables: deployVariables},
		{Number: 792, Status: "success", Branch: "main", Commit: "347e07bc", Finished: 792, Variables: deployVariables},
		{Number: 790, Status: "success", Branch: "main", Commit: "04e00186", Finished: 790, Variables: deployVariables},
	}}

	rows := deploymentStatuses([]Task{task}, map[int]string{7: "app"}, pipelines)
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	status := rows[0]
	if status.CurrentCommit != "cbe6587c" {
		t.Fatalf("CurrentCommit = %q, want current marker commit", status.CurrentCommit)
	}
	if len(status.Revisions) < 4 {
		t.Fatalf("revisions len = %d, want at least 4: %#v", len(status.Revisions), status.Revisions)
	}
	commitsByPipeline := map[int64]string{}
	for _, revision := range status.Revisions {
		commitsByPipeline[revision.Pipeline] = revision.Commit
	}
	if commitsByPipeline[792] != "347e07bc" {
		t.Fatalf("pipeline #792 revision commit = %q, want original commit 347e07bc", commitsByPipeline[792])
	}
	if commitsByPipeline[790] != "04e00186" {
		t.Fatalf("pipeline #790 revision commit = %q, want original commit 04e00186", commitsByPipeline[790])
	}
	if status.PreviousPipeline != 792 || status.PreviousCommit != "347e07bc" {
		t.Fatalf("previous = #%d %q, want #792 347e07bc", status.PreviousPipeline, status.PreviousCommit)
	}
}

func TestDeploymentCommitFromRollbackImageTagUsesLeadingCommit(t *testing.T) {
	pipeline := Pipeline{
		Commit: "current9",
		Variables: map[string]string{
			"DEPLOY_ACTION": "rollback",
			"IMAGE_TAG":     "abcdef1-contentabc123",
		},
	}
	if got := deploymentCommitFromPipeline(pipeline); got != "abcdef1" {
		t.Fatalf("deploymentCommitFromPipeline = %q, want image tag leading commit", got)
	}
}

func TestDeploymentStatusesIgnoreTargetOnlyPipelineNoise(t *testing.T) {
	pipelines := map[int][]Pipeline{3: {
		{Number: 1, Status: "success", Branch: "main", Commit: "aaaaaaaa", Finished: 100, Variables: map[string]string{"DEPLOY_TARGET": "production"}},
	}}

	rows := deploymentStatuses(nil, map[int]string{3: "router"}, pipelines)
	if len(rows) != 0 {
		t.Fatalf("rows len = %d, want 0: %#v", len(rows), rows)
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
