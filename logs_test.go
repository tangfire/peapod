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

	text, err := newDozzleMCPClient(server.URL, server.Client()).callToolText(context.Background(), "list_containers", map[string]any{})
	if err != nil {
		t.Fatalf("callToolText failed: %v", err)
	}
	if text != "[]" {
		t.Fatalf("unexpected text: %q", text)
	}
}
