package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInitializeHandshake(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})
	server.RegisterTool(MCPToolHandler{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: map[string]interface{}{"type": "object"},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			return "ok", nil
		},
	})

	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}` + "\n"
	resp := sendRequest(t, server, req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if resp.ID == nil {
		t.Fatal("response ID should not be nil")
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result to be a map, got %T", resp.Result)
	}

	// Check protocol version
	if pv, ok := result["protocolVersion"].(string); !ok || pv != "2025-03-26" {
		t.Errorf("expected protocolVersion 2025-03-26, got %v", result["protocolVersion"])
	}

	// Check server info
	si, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("expected serverInfo in result")
	}
	if si["name"] != "hawk" {
		t.Errorf("expected server name hawk, got %v", si["name"])
	}
	if si["version"] != "1.0.0" {
		t.Errorf("expected server version 1.0.0, got %v", si["version"])
	}

	// Check capabilities
	caps, ok := result["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("expected capabilities in result")
	}
	if _, ok := caps["tools"]; !ok {
		t.Error("expected tools capability")
	}
}

func TestInitializedNotification(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})

	// initialized is a notification (has no response)
	req := `{"jsonrpc":"2.0","method":"initialized"}` + "\n"
	out := runServer(t, server, req)

	// Should produce no output for a notification
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected no response for initialized notification, got: %s", out)
	}
}

func TestToolsList(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})
	server.RegisterTool(MCPToolHandler{
		Name:        "alpha",
		Description: "Alpha tool",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"msg": map[string]interface{}{"type": "string"},
			},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			return "alpha", nil
		},
	})
	server.RegisterTool(MCPToolHandler{
		Name:        "beta",
		Description: "Beta tool",
		InputSchema: map[string]interface{}{"type": "object"},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			return "beta", nil
		},
	})

	req := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n"
	resp := sendRequest(t, server, req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %T", resp.Result)
	}

	toolsRaw, ok := result["tools"]
	if !ok {
		t.Fatal("expected tools key in result")
	}

	tools, ok := toolsRaw.([]interface{})
	if !ok {
		t.Fatalf("expected tools to be array, got %T", toolsRaw)
	}

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	// Verify tool schemas have required fields
	for _, raw := range tools {
		toolMap, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected tool to be map, got %T", raw)
		}
		if toolMap["name"] == nil {
			t.Error("tool missing name")
		}
		if toolMap["description"] == nil {
			t.Error("tool missing description")
		}
		if toolMap["inputSchema"] == nil {
			t.Error("tool missing inputSchema")
		}
	}
}

func TestToolExecution(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})
	server.RegisterTool(MCPToolHandler{
		Name:        "echo",
		Description: "Echoes input",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": map[string]interface{}{"type": "string"},
			},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			var input struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return "", err
			}
			return "echo: " + input.Message, nil
		},
	})

	req := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"message":"hello"}}}` + "\n"
	resp := sendRequest(t, server, req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %T", resp.Result)
	}

	content, ok := result["content"].([]interface{})
	if !ok {
		t.Fatalf("expected content array, got %T", result["content"])
	}
	if len(content) == 0 {
		t.Fatal("expected at least one content item")
	}

	item, ok := content[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected content item to be map, got %T", content[0])
	}
	if item["type"] != "text" {
		t.Errorf("expected type text, got %v", item["type"])
	}
	if item["text"] != "echo: hello" {
		t.Errorf("expected 'echo: hello', got %v", item["text"])
	}
}

func TestToolExecutionError(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})
	server.RegisterTool(MCPToolHandler{
		Name:        "failing",
		Description: "Always fails",
		InputSchema: map[string]interface{}{"type": "object"},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			return "", fmt.Errorf("something went wrong")
		},
	})

	req := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"failing","arguments":{}}}` + "\n"
	resp := sendRequest(t, server, req)

	// Tool errors are returned as isError content, not RPC errors.
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %+v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %T", resp.Result)
	}
	if result["isError"] != true {
		t.Error("expected isError: true")
	}

	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("expected content with error message")
	}
	item := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if !strings.Contains(text, "something went wrong") {
		t.Errorf("expected error message in content, got: %s", text)
	}
}

func TestUnknownMethod(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})

	req := `{"jsonrpc":"2.0","id":5,"method":"nonexistent/method"}` + "\n"
	resp := sendRequest(t, server, req)

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != errCodeMethodNotFound {
		t.Errorf("expected error code %d, got %d", errCodeMethodNotFound, resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "nonexistent/method") {
		t.Errorf("expected method name in error message, got: %s", resp.Error.Message)
	}
}

func TestUnknownTool(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})

	req := `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"does_not_exist","arguments":{}}}` + "\n"
	resp := sendRequest(t, server, req)

	if resp.Error == nil {
		t.Fatal("expected error for unknown tool")
	}
	if resp.Error.Code != errCodeMethodNotFound {
		t.Errorf("expected error code %d, got %d", errCodeMethodNotFound, resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "does_not_exist") {
		t.Errorf("expected tool name in error message, got: %s", resp.Error.Message)
	}
}

func TestInvalidParams(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})
	server.RegisterTool(MCPToolHandler{
		Name:        "tool1",
		Description: "A tool",
		InputSchema: map[string]interface{}{"type": "object"},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			return "ok", nil
		},
	})

	// Invalid JSON in params
	req := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":"not-valid-object"}` + "\n"
	resp := sendRequest(t, server, req)

	if resp.Error == nil {
		t.Fatal("expected error for invalid params")
	}
	if resp.Error.Code != errCodeInvalidParams {
		t.Errorf("expected error code %d, got %d", errCodeInvalidParams, resp.Error.Code)
	}
}

func TestParseError(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})

	// Send garbage that isn't valid JSON
	out := runServer(t, server, "this is not json\n")

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("failed to parse response: %v (raw: %s)", err, out)
	}
	if resp.Error == nil {
		t.Fatal("expected parse error")
	}
	if resp.Error.Code != errCodeParseError {
		t.Errorf("expected error code %d, got %d", errCodeParseError, resp.Error.Code)
	}
}

func TestPing(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})

	req := `{"jsonrpc":"2.0","id":8,"method":"ping"}` + "\n"
	resp := sendRequest(t, server, req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	// Result should be an empty object
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %T", resp.Result)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for ping, got %v", result)
	}
}

func TestConcurrentRequests(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})
	server.RegisterTool(MCPToolHandler{
		Name:        "slow",
		Description: "Simulates slow work",
		InputSchema: map[string]interface{}{"type": "object"},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			time.Sleep(10 * time.Millisecond)
			return "done", nil
		},
	})

	// Build multiple requests
	const n = 20
	var lines strings.Builder
	for i := 1; i <= n; i++ {
		lines.WriteString(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"slow","arguments":{}}}`, i))
		lines.WriteString("\n")
	}

	out := runServer(t, server, lines.String())

	// Parse all responses
	responses := strings.Split(strings.TrimSpace(out), "\n")
	if len(responses) != n {
		t.Fatalf("expected %d responses, got %d", n, len(responses))
	}

	ids := make(map[float64]bool)
	for _, line := range responses {
		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if resp.Error != nil {
			t.Errorf("unexpected error in response: %+v", resp.Error)
		}
		// Track IDs to ensure all were answered
		if id, ok := resp.ID.(float64); ok {
			ids[id] = true
		}
	}

	for i := 1; i <= n; i++ {
		if !ids[float64(i)] {
			t.Errorf("missing response for request ID %d", i)
		}
	}
}

func TestRegisterToolOverwrite(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})

	server.RegisterTool(MCPToolHandler{
		Name:        "dup",
		Description: "First version",
		InputSchema: map[string]interface{}{"type": "object"},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			return "v1", nil
		},
	})
	server.RegisterTool(MCPToolHandler{
		Name:        "dup",
		Description: "Second version",
		InputSchema: map[string]interface{}{"type": "object"},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			return "v2", nil
		},
	})

	// Call should use the second registration
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"dup","arguments":{}}}` + "\n"
	resp := sendRequest(t, server, req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result := resp.Result.(map[string]interface{})
	content := result["content"].([]interface{})
	item := content[0].(map[string]interface{})
	if item["text"] != "v2" {
		t.Errorf("expected v2, got %v", item["text"])
	}
}

func TestConcurrentRegisterAndCall(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})
	server.RegisterTool(MCPToolHandler{
		Name:        "base",
		Description: "Base tool",
		InputSchema: map[string]interface{}{"type": "object"},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			return "base", nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// Concurrently register tools while calling existing ones.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			server.RegisterTool(MCPToolHandler{
				Name:        fmt.Sprintf("tool_%d", i),
				Description: fmt.Sprintf("Tool %d", i),
				InputSchema: map[string]interface{}{"type": "object"},
				Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
					return fmt.Sprintf("result_%d", i), nil
				},
			})
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := &JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      float64(100),
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name":"base","arguments":{}}`),
			}
			resp := server.handleRequest(ctx, req)
			if resp.Error != nil {
				t.Errorf("unexpected error: %+v", resp.Error)
			}
		}()
	}

	wg.Wait()
}

func TestMultipleRequestsInSequence(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "hawk", Version: "1.0.0"})
	server.RegisterTool(MCPToolHandler{
		Name:        "greet",
		Description: "Greets",
		InputSchema: map[string]interface{}{"type": "object"},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			return "hello", nil
		},
	})

	// Send initialize, then initialized notification, then tools/list, then tools/call
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`,
		`{"jsonrpc":"2.0","method":"initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"greet","arguments":{}}}`,
	}, "\n") + "\n"

	out := runServer(t, server, input)
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// Should have 3 responses (initialized is a notification, no response)
	if len(lines) != 3 {
		t.Fatalf("expected 3 responses, got %d: %v", len(lines), lines)
	}

	// Verify order by ID
	for i, line := range lines {
		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("response %d parse error: %v", i, err)
		}
		expectedID := float64(i + 1)
		if id, ok := resp.ID.(float64); !ok || id != expectedID {
			t.Errorf("response %d: expected ID %v, got %v", i, expectedID, resp.ID)
		}
	}
}

// --- Test helpers ---

// sendRequest sends a single request to the server and returns the parsed response.
func sendRequest(t *testing.T, server *MCPServer, req string) JSONRPCResponse {
	t.Helper()
	out := runServer(t, server, req)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("no response from server for request: %s", req)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatalf("failed to parse response: %v (raw: %s)", err, lines[0])
	}
	return resp
}

// runServer runs the server with the given input and returns all output.
func runServer(t *testing.T, server *MCPServer, input string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := strings.NewReader(input)
	var buf bytes.Buffer

	// Use a pipe to allow the server to detect EOF properly.
	pr, pw := io.Pipe()
	go func() {
		io.Copy(pw, r)
		pw.Close()
	}()

	err := server.Serve(ctx, pr, &buf)
	if err != nil && err != context.DeadlineExceeded {
		// EOF is expected
		if !strings.Contains(err.Error(), "context") {
			t.Logf("server returned: %v", err)
		}
	}

	return buf.String()
}
