package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// RegisterDefaultTools
// ---------------------------------------------------------------------------

func TestRegisterDefaultTools_RegistersAllSevenTools(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	RegisterDefaultTools(server, nil)

	// Send tools/list
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"
	resp := sendRequest(t, server, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	result := resp.Result.(map[string]interface{})
	tools := result["tools"].([]interface{})

	expectedNames := map[string]bool{
		"graycode_chat":          false,
		"graycode_search":        false,
		"graycode_memory_recall": false,
		"graycode_memory_store":  false,
		"graycode_review":        false,
		"graycode_scan":          false,
		"graycode_compress":      false,
	}

	for _, raw := range tools {
		tm := raw.(map[string]interface{})
		name := tm["name"].(string)
		if _, ok := expectedNames[name]; ok {
			expectedNames[name] = true
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("tool %q not registered", name)
		}
	}
	if len(tools) != 7 {
		t.Errorf("expected 7 tools, got %d", len(tools))
	}
}

// ---------------------------------------------------------------------------
// graycode_chat tool
// ---------------------------------------------------------------------------

func TestGraycodeChatTool_ValidPrompt(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	executor := func(ctx context.Context, name string, params json.RawMessage) (string, error) {
		if name != "agent" {
			return "", fmt.Errorf("expected agent, got %s", name)
		}
		return "chat response", nil
	}
	RegisterDefaultTools(server, executor)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_chat","arguments":{"prompt":"hello"}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertNoError(t, resp)
	assertResponseContains(t, resp, "chat response")
}

func TestGraycodeChatTool_EmptyPrompt(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	RegisterDefaultTools(server, nil)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_chat","arguments":{"prompt":""}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertIsError(t, resp)
}

func TestGraycodeChatTool_MissingPrompt(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	RegisterDefaultTools(server, nil)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_chat","arguments":{}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertIsError(t, resp)
}

// ---------------------------------------------------------------------------
// graycode_search tool
// ---------------------------------------------------------------------------

func TestGraycodeSearchTool_ValidQuery(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	executor := func(ctx context.Context, name string, params json.RawMessage) (string, error) {
		if name != "code_search" {
			return "", fmt.Errorf("expected code_search, got %s", name)
		}
		return "search results", nil
	}
	RegisterDefaultTools(server, executor)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_search","arguments":{"query":"auth middleware"}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertNoError(t, resp)
	assertResponseContains(t, resp, "search results")
}

func TestGraycodeSearchTool_EmptyQuery(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	RegisterDefaultTools(server, nil)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_search","arguments":{"query":""}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertIsError(t, resp)
}

// ---------------------------------------------------------------------------
// graycode_memory_recall tool
// ---------------------------------------------------------------------------

func TestGraycodeMemoryRecallTool_ValidQuery(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	executor := func(ctx context.Context, name string, params json.RawMessage) (string, error) {
		if name != "core_memory" {
			return "", fmt.Errorf("expected core_memory, got %s", name)
		}
		return "recalled memory", nil
	}
	RegisterDefaultTools(server, executor)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_memory_recall","arguments":{"query":"last decision"}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertNoError(t, resp)
	assertResponseContains(t, resp, "recalled memory")
}

func TestGraycodeMemoryRecallTool_WithNamespace(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	executor := func(ctx context.Context, name string, params json.RawMessage) (string, error) {
		return "namespaced result", nil
	}
	RegisterDefaultTools(server, executor)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_memory_recall","arguments":{"query":"test","namespace":"project"}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertNoError(t, resp)
	assertResponseContains(t, resp, "namespaced result")
}

func TestGraycodeMemoryRecallTool_EmptyQuery(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	RegisterDefaultTools(server, nil)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_memory_recall","arguments":{"query":""}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertIsError(t, resp)
}

// ---------------------------------------------------------------------------
// graycode_memory_store tool
// ---------------------------------------------------------------------------

func TestGraycodeMemoryStoreTool_ValidInput(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	executor := func(ctx context.Context, name string, params json.RawMessage) (string, error) {
		return "stored", nil
	}
	RegisterDefaultTools(server, executor)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_memory_store","arguments":{"key":"decision-1","content":"use Go for backend"}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertNoError(t, resp)
	assertResponseContains(t, resp, "stored")
}

func TestGraycodeMemoryStoreTool_MissingKey(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	RegisterDefaultTools(server, nil)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_memory_store","arguments":{"content":"some content"}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertIsError(t, resp)
}

func TestGraycodeMemoryStoreTool_MissingContent(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	RegisterDefaultTools(server, nil)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_memory_store","arguments":{"key":"some-key"}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertIsError(t, resp)
}

// ---------------------------------------------------------------------------
// graycode_review tool
// ---------------------------------------------------------------------------

func TestGraycodeReviewTool_WithPath(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	executor := func(ctx context.Context, name string, params json.RawMessage) (string, error) {
		if name != "code_review" {
			return "", fmt.Errorf("expected code_review, got %s", name)
		}
		return "review result", nil
	}
	RegisterDefaultTools(server, executor)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_review","arguments":{"path":"internal/mcp/server.go"}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertNoError(t, resp)
	assertResponseContains(t, resp, "review result")
}

func TestGraycodeReviewTool_WithDiff(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	executor := func(ctx context.Context, name string, params json.RawMessage) (string, error) {
		return "diff review", nil
	}
	RegisterDefaultTools(server, executor)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_review","arguments":{"diff":"+func New() {}"}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertNoError(t, resp)
	assertResponseContains(t, resp, "diff review")
}

func TestGraycodeReviewTool_NeitherPathNorDiff(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	RegisterDefaultTools(server, nil)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_review","arguments":{}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertIsError(t, resp)
}

// ---------------------------------------------------------------------------
// graycode_scan tool
// ---------------------------------------------------------------------------

func TestGraycodeScanTool_ValidPath(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	executor := func(ctx context.Context, name string, params json.RawMessage) (string, error) {
		if name != "security_scan" {
			return "", fmt.Errorf("expected security_scan, got %s", name)
		}
		return "scan result", nil
	}
	RegisterDefaultTools(server, executor)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_scan","arguments":{"path":"."}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertNoError(t, resp)
	assertResponseContains(t, resp, "scan result")
}

func TestGraycodeScanTool_MissingPath(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	RegisterDefaultTools(server, nil)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_scan","arguments":{}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertIsError(t, resp)
}

func TestGraycodeScanTool_WithSeverity(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	executor := func(ctx context.Context, name string, params json.RawMessage) (string, error) {
		var input struct {
			Path     string `json:"path"`
			Severity string `json:"severity"`
		}
		_ = json.Unmarshal(params, &input)
		if input.Severity != "high" {
			return "", fmt.Errorf("expected severity high, got %s", input.Severity)
		}
		return "high severity scan", nil
	}
	RegisterDefaultTools(server, executor)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_scan","arguments":{"path":".","severity":"high"}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertNoError(t, resp)
	assertResponseContains(t, resp, "high severity scan")
}

// ---------------------------------------------------------------------------
// graycode_compress tool
// ---------------------------------------------------------------------------

func TestGraycodeCompressTool_ValidText(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	executor := func(ctx context.Context, name string, params json.RawMessage) (string, error) {
		if name != "compress" {
			return "", fmt.Errorf("expected compress, got %s", name)
		}
		return "compressed text", nil
	}
	RegisterDefaultTools(server, executor)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_compress","arguments":{"text":"long text to compress"}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertNoError(t, resp)
	assertResponseContains(t, resp, "compressed text")
}

func TestGraycodeCompressTool_EmptyText(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	RegisterDefaultTools(server, nil)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_compress","arguments":{"text":""}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertIsError(t, resp)
}

func TestGraycodeCompressTool_WithRatio(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	executor := func(ctx context.Context, name string, params json.RawMessage) (string, error) {
		var input struct {
			Text  string  `json:"text"`
			Ratio float64 `json:"ratio"`
		}
		_ = json.Unmarshal(params, &input)
		if input.Ratio != 0.5 {
			return "", fmt.Errorf("expected ratio 0.5, got %f", input.Ratio)
		}
		return "ratio compressed", nil
	}
	RegisterDefaultTools(server, executor)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"graycode_compress","arguments":{"text":"some text","ratio":0.5}}}` + "\n"
	resp := sendRequest(t, server, req)
	assertNoError(t, resp)
	assertResponseContains(t, resp, "ratio compressed")
}

// ---------------------------------------------------------------------------
// delegateToExecutor
// ---------------------------------------------------------------------------

func TestDelegateToExecutor_NilExecutor(t *testing.T) {
	_, err := delegateToExecutor(context.Background(), nil, "test", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error with nil executor")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' in error, got: %s", err.Error())
	}
}

func TestDelegateToExecutor_CallsExecutor(t *testing.T) {
	executor := func(ctx context.Context, name string, params json.RawMessage) (string, error) {
		return "executed:" + name, nil
	}
	result, err := delegateToExecutor(context.Background(), executor, "my_tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if result != "executed:my_tool" {
		t.Errorf("expected 'executed:my_tool', got %q", result)
	}
}

func TestDelegateToExecutor_ExecutorError(t *testing.T) {
	executor := func(ctx context.Context, name string, params json.RawMessage) (string, error) {
		return "", fmt.Errorf("executor failed")
	}
	_, err := delegateToExecutor(context.Background(), executor, "failing", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error from executor")
	}
}

// ---------------------------------------------------------------------------
// Tool schema validation
// ---------------------------------------------------------------------------

func TestToolSchemas_HaveRequiredFields(t *testing.T) {
	server := NewMCPServer(ServerInfo{Name: "graycode", Version: "test"})
	RegisterDefaultTools(server, nil)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"
	resp := sendRequest(t, server, req)

	result := resp.Result.(map[string]interface{})
	tools := result["tools"].([]interface{})

	for _, raw := range tools {
		tm := raw.(map[string]interface{})
		name := tm["name"].(string)

		if tm["description"] == nil || tm["description"] == "" {
			t.Errorf("tool %q has empty description", name)
		}
		schema, ok := tm["inputSchema"].(map[string]interface{})
		if !ok {
			t.Errorf("tool %q has no inputSchema", name)
			continue
		}
		if schema["type"] != "object" {
			t.Errorf("tool %q inputSchema type = %v, want object", name, schema["type"])
		}
		if schema["properties"] == nil {
			t.Errorf("tool %q inputSchema has no properties", name)
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertNoError(t *testing.T, resp JSONRPCResponse) {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %+v", resp.Error)
	}
}

func assertIsError(t *testing.T, resp JSONRPCResponse) {
	t.Helper()
	if resp.Error != nil {
		return // RPC error is also acceptable
	}
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("expected result map")
	}
	if result["isError"] != true {
		t.Errorf("expected isError=true in result, got: %v", result)
	}
}

func assertResponseContains(t *testing.T, resp JSONRPCResponse, substr string) {
	t.Helper()
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %T", resp.Result)
	}
	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("expected content array")
	}
	item := content[0].(map[string]interface{})
	text := item["text"].(string)
	if !strings.Contains(text, substr) {
		t.Errorf("expected response to contain %q, got %q", substr, text)
	}
}
