package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseDozzleLogTextMasksSensitiveData(t *testing.T) {
	container := LogContainer{ID: "abc123", Name: "api", Host: "local", HostName: "local"}
	text := strings.Join([]string{
		`{"timestamp":"2026-07-07T08:00:00Z","level":"error","stream":"stderr","type":"single","message":"request failed Authorization: Bearer secret-token"}`,
		`{"timestamp":"2026-07-07T08:00:01Z","level":"info","stream":"stdout","type":"single","message":"TOKEN=secret-token"}`,
	}, "\n")
	lines := parseDozzleLogText(text, container)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if strings.Contains(lines[0].Message, "secret-token") {
		t.Fatalf("bearer token leaked: %q", lines[0].Message)
	}
	if lines[1].Message != "[已隐藏敏感日志行]" {
		t.Fatalf("expected sensitive line to be hidden, got %q", lines[1].Message)
	}
}

func TestMCPJSONPayloadParsesSSE(t *testing.T) {
	body := []byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"ok\":true}}\n\n")
	payload := mcpJSONPayload(body, "text/event-stream")
	var response mcpRPCResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("payload did not parse: %v", err)
	}
	if response.Result == nil {
		t.Fatal("expected result")
	}
}

func TestDozzleHostUUIDUsesConfiguredDisplayName(t *testing.T) {
	app := &App{cfg: Config{
		MonitorHostsJSON: `[{"id":"ops","name":"运维机","beszel_names":["ops"]}]`,
	}}
	containers := app.dozzleRawContainersToLogContainers([]dozzleContainerRaw{
		{ID: "abc", Name: "/peapod", Host: "c6c100c7-2f10-4ba7-97d0-d800f76f882f", State: "running"},
	})
	if len(containers) != 1 {
		t.Fatalf("expected one container, got %d", len(containers))
	}
	if containers[0].HostName != "运维机" {
		t.Fatalf("expected configured display name, got %q", containers[0].HostName)
	}
	if containers[0].Host == containers[0].HostName {
		t.Fatalf("raw host id should remain separate from display name")
	}
}

func TestDozzleSingleOpaqueHostPrefersOperationsHost(t *testing.T) {
	app := &App{cfg: Config{
		MonitorHostsJSON: `[
			{"id":"production","name":"写书猫生产机","role":"production"},
			{"id":"ops","name":"Peapod 运维/测试构建机","role":"operations"}
		]`,
	}}
	containers := app.dozzleRawContainersToLogContainers([]dozzleContainerRaw{
		{ID: "abc", Name: "/peapod", Host: "c6c100c7-2f10-4ba7-97d0-d800f76f882f", State: "running"},
	})
	if containers[0].HostName != "Peapod 运维/测试构建机" {
		t.Fatalf("expected operations host display name, got %q", containers[0].HostName)
	}
}

func TestMergeLogContainersKeepsRemoteHostsWhenDozzleIsHealthy(t *testing.T) {
	dozzle := []LogContainer{
		{ID: "abc", Name: "peapod", Host: "opaque-host-id", HostName: "Peapod 运维/测试构建机", State: "running", Source: "dozzle_mcp"},
	}
	monitoring := []LogContainer{
		{ID: "peapod", Name: "peapod", Host: "ops-test-builder", HostName: "Peapod 运维/测试构建机", State: "running", Source: "monitoring_fallback"},
		{ID: "novel-factory-api-green", Name: "novel-factory-api-green", Host: "novel-production", HostName: "写书猫生产机", State: "running", Source: "monitoring_fallback"},
	}

	merged := mergeLogContainers(dozzle, monitoring)
	if len(merged) != 2 {
		t.Fatalf("merged len = %d, want 2: %+v", len(merged), merged)
	}
	if !hasNonDozzleLogContainers(merged) {
		t.Fatalf("expected merged containers to include remote fallback source: %+v", merged)
	}
	for _, item := range merged {
		if item.Name == "peapod" && item.Source != "dozzle_mcp" {
			t.Fatalf("local duplicate should keep Dozzle source, got %+v", item)
		}
	}
}

func TestSelectLogContainersMatchesRemoteSelector(t *testing.T) {
	containers := []LogContainer{
		{ID: "abc", Name: "peapod", Host: "ops-test-builder", HostName: "Peapod 运维/测试构建机", State: "running", Source: "dozzle_mcp"},
		{ID: "novel-factory-api-green", Name: "novel-factory-api-green", Host: "novel-production", HostName: "写书猫生产机", State: "running", Source: "monitoring_fallback"},
	}
	selected := selectLogContainers(containers, LogQueryRequest{Containers: []string{"novel-production|novel-factory-api-green"}})
	if len(selected) != 1 || selected[0].Host != "novel-production" {
		t.Fatalf("unexpected selected containers: %+v", selected)
	}
}

func TestDozzleMCPClientCallToolText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/mcp" {
			http.NotFound(w, r)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("bad request: %v", err)
		}
		method := request["method"]
		if method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		id := request["id"]
		w.Header().Set("Content-Type", "application/json")
		if method == "initialize" {
			w.Header().Set("Mcp-Session-Id", "test-session")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{}})
			return
		}
		if method == "tools/call" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"content": []map[string]string{{"type": "text", "text": "[]"}},
				},
			})
			return
		}
		http.Error(w, "unexpected method", http.StatusBadRequest)
	}))
	defer server.Close()

	text, err := newDozzleMCPClient(server.URL, "", "", server.Client()).callToolText(context.Background(), "list_containers", map[string]any{})
	if err != nil {
		t.Fatalf("callToolText failed: %v", err)
	}
	if text != "[]" {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestDozzleMCPClientAuthenticatesBeforeMCP(t *testing.T) {
	var sawLogin bool
	var sawCookie bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/token" {
			if err := r.ParseMultipartForm(1024); err != nil {
				t.Fatalf("bad multipart form: %v", err)
			}
			if r.FormValue("username") != "ops" || r.FormValue("password") != "secret" {
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}
			sawLogin = true
			http.SetCookie(w, &http.Cookie{Name: "jwt", Value: "test-token", Path: "/", HttpOnly: true})
			_, _ = w.Write([]byte("OK"))
			return
		}
		if r.URL.Path != "/api/mcp" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Cookie") == "jwt=test-token" {
			sawCookie = true
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("bad request: %v", err)
		}
		method := request["method"]
		if method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		id := request["id"]
		w.Header().Set("Content-Type", "application/json")
		if method == "initialize" {
			w.Header().Set("Mcp-Session-Id", "test-session")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{}})
			return
		}
		if method == "tools/call" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"content": []map[string]string{{"type": "text", "text": "[]"}},
				},
			})
			return
		}
		http.Error(w, "unexpected method", http.StatusBadRequest)
	}))
	defer server.Close()

	_, err := newDozzleMCPClient(server.URL, "ops", "secret", server.Client()).callToolText(context.Background(), "list_containers", map[string]any{})
	if err != nil {
		t.Fatalf("callToolText failed: %v", err)
	}
	if !sawLogin {
		t.Fatal("expected login request before MCP")
	}
	if !sawCookie {
		t.Fatal("expected MCP request to include auth cookie")
	}
}
