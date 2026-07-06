package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestDecodePipelineStepsFromWoodpeckerWorkflow(t *testing.T) {
	payload := []byte(`{
		"number": 42,
		"status": "failure",
		"workflows": [{
			"id": 10,
			"name": "deploy",
			"state": "failure",
			"children": [{
				"id": 99,
				"pid": 2,
				"ppid": 1,
				"name": "deploy",
				"state": "failure",
				"exit_code": 1,
				"type": "commands"
			}]
		}]
	}`)

	steps := decodePipelineSteps(payload)
	if len(steps) != 2 {
		t.Fatalf("steps len = %d, want workflow plus child", len(steps))
	}
	if steps[1].ID != 99 || steps[1].ExitCode != 1 || steps[1].Type != "commands" {
		t.Fatalf("unexpected child step: %+v", steps[1])
	}
	summary := pipelineFailureSummary(Pipeline{Status: "failure"}, steps)
	if !strings.Contains(summary, "exit code 1") {
		t.Fatalf("failure summary = %q, want exit code", summary)
	}
}

func TestDecodeWoodpeckerLogLinesBase64AndMaskSensitive(t *testing.T) {
	encodedNormal := base64.StdEncoding.EncodeToString([]byte("building image\n"))
	encodedSecret := base64.StdEncoding.EncodeToString([]byte("PASSWORD=super-secret\n"))
	payload := []byte(`[{"data":"` + encodedNormal + `"},{"data":"` + encodedSecret + `"},{"data":null}]`)

	lines, err := decodeWoodpeckerLogLines(payload)
	if err != nil {
		t.Fatalf("decodeWoodpeckerLogLines returned error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("lines len = %d, want 2", len(lines))
	}
	if lines[0] != "building image" {
		t.Fatalf("line 0 = %q", lines[0])
	}
	if lines[1] != "[已隐藏敏感日志行]" {
		t.Fatalf("secret line was not masked: %q", lines[1])
	}
}

func TestDecodeWoodpeckerLogLinesRejectsHTML(t *testing.T) {
	if _, err := decodeWoodpeckerLogLines([]byte("<!doctype html>")); err == nil {
		t.Fatal("expected JSON decode error for HTML payload")
	}
}
