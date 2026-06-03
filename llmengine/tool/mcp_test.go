package tool

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewMCPToolsListsAndCallsRemoteTool(t *testing.T) {
	var toolCallArguments map[string]any
	const sessionID = "test-session"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			t.Fatalf("path = %q, want /mcp", r.URL.Path)
		}

		var req struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", sessionID)
			writeMCPEventResult(t, w, req.ID, map[string]any{
				"protocolVersion": mcpProtocolVersion,
				"capabilities":    map[string]any{},
				"serverInfo": map[string]string{
					"name":    "test-mcp",
					"version": "0.1.0",
				},
			})
		case "notifications/initialized":
			assertSessionID(t, r, sessionID)
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			assertSessionID(t, r, sessionID)
			writeMCPEventResult(t, w, req.ID, map[string]any{
				"tools": []map[string]any{{
					"name":        "remote_time",
					"description": "Returns the current time.",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"timezone": map[string]string{"type": "string"},
						},
					},
				}},
			})
		case "tools/call":
			assertSessionID(t, r, sessionID)
			var params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				t.Fatalf("decode tools/call params: %v", err)
			}
			if params.Name != "remote_time" {
				t.Fatalf("tool name = %q, want remote_time", params.Name)
			}
			toolCallArguments = params.Arguments
			writeMCPEventResult(t, w, req.ID, map[string]any{
				"content": []map[string]string{{
					"type": "text",
					"text": "Current time: 2026-06-03T12:00:00+09:00",
				}},
			})
		default:
			t.Fatalf("unexpected method %q", req.Method)
		}
	}))
	defer server.Close()

	tools, err := NewMCPTools(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("NewMCPTools returned error: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tool count = %d, want 1", len(tools))
	}

	result, err := tools[0].Call(context.Background(), "Asia/Tokyo")
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if result != "Current time: 2026-06-03T12:00:00+09:00" {
		t.Fatalf("result = %q", result)
	}
	if toolCallArguments["timezone"] != "Asia/Tokyo" {
		t.Fatalf("arguments = %#v, want timezone argument", toolCallArguments)
	}
}

func TestReadMCPEventStreamReturnsAfterFirstEventWithoutEOF(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()

	go func() {
		_, _ = writer.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n"))
	}()

	payload, err := readMCPEventStream(reader)
	if err != nil {
		t.Fatalf("readMCPEventStream returned error: %v", err)
	}
	if string(payload) != `{"jsonrpc":"2.0","id":1,"result":{}}` {
		t.Fatalf("payload = %q", payload)
	}
}

func assertSessionID(t *testing.T, r *http.Request, want string) {
	t.Helper()
	if got := r.Header.Get("Mcp-Session-Id"); got != want {
		t.Fatalf("Mcp-Session-Id = %q, want %q", got, want)
	}
}

func writeMCPEventResult(t *testing.T, w http.ResponseWriter, id any, result any) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	if _, err := w.Write([]byte("event: message\ndata: " + string(payload) + "\n\n")); err != nil {
		t.Fatalf("write response: %v", err)
	}
}
