package composio

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/mcp"
)

func TestNewMCPBridge(t *testing.T) {
	t.Parallel()
	provider := NewComposioProvider("test-key")
	server := mcp.NewMCPServer(mcp.ServerInfo{
		Name:    "test",
		Version: "0.0.1",
	})

	bridge := NewMCPBridge(provider, server)
	if bridge == nil {
		t.Fatal("expected non-nil bridge")
	}
}

func TestMCPBridgeRegisterTools(t *testing.T) {
	t.Parallel()
	provider := NewComposioProvider("test-key")
	provider.RegisterTool(&ComposioTool{
		Name:         "github_issues",
		Description:  "List GitHub issues",
		Scope:        ScopeReadOnly,
		AuthRequired: false,
		Params:       map[string]interface{}{"repo_id": "string"},
		Tags:         []string{"github"},
		Category:     "github",
	})
	provider.RegisterTool(&ComposioTool{
		Name:         "slack_messages",
		Description:  "Send Slack messages",
		Scope:        ScopeWrite,
		AuthRequired: false,
		Params:       map[string]interface{}{},
		Tags:         []string{"slack"},
		Category:     "slack",
	})

	server := mcp.NewMCPServer(mcp.ServerInfo{
		Name:    "test",
		Version: "0.0.1",
	})
	bridge := NewMCPBridge(provider, server)

	count := bridge.RegisterTools()
	if count != 2 {
		t.Errorf("expected 2 tools registered, got %d", count)
	}
}

func TestMCPBridgeRegisterToolsEmpty(t *testing.T) {
	t.Parallel()
	provider := NewComposioProvider("test-key")
	server := mcp.NewMCPServer(mcp.ServerInfo{
		Name:    "test",
		Version: "0.0.1",
	})
	bridge := NewMCPBridge(provider, server)

	count := bridge.RegisterTools()
	if count != 0 {
		t.Errorf("expected 0 tools registered, got %d", count)
	}
}

func TestMCPBridgeRegisterAll(t *testing.T) {
	t.Parallel()
	provider := NewComposioProvider("test-key")
	provider.RegisterTool(&ComposioTool{
		Name:        "tool1",
		Description: "First tool",
		Scope:       ScopeReadOnly,
		Params:      map[string]interface{}{},
	})
	provider.RegisterTool(&ComposioTool{
		Name:        "tool2",
		Description: "Second tool",
		Scope:       ScopeReadOnly,
		Params:      map[string]interface{}{},
	})

	server := mcp.NewMCPServer(mcp.ServerInfo{
		Name:    "test",
		Version: "0.0.1",
	})
	bridge := NewMCPBridge(provider, server)

	total := bridge.RegisterAll()
	// 2 composio tools + 1 search tool + 1 credential tool = 4
	if total != 4 {
		t.Errorf("expected 4 total tools registered, got %d", total)
	}
}

func TestMCPBridgeRegisterAllEmpty(t *testing.T) {
	t.Parallel()
	provider := NewComposioProvider("test-key")
	server := mcp.NewMCPServer(mcp.ServerInfo{
		Name:    "test",
		Version: "0.0.1",
	})
	bridge := NewMCPBridge(provider, server)

	total := bridge.RegisterAll()
	// 0 composio tools + 1 search tool + 1 credential tool = 2
	if total != 2 {
		t.Errorf("expected 2 total tools registered, got %d", total)
	}
}

func TestMCPBridgeWrapTool(t *testing.T) {
	t.Parallel()
	provider := NewComposioProvider("test-key")
	server := mcp.NewMCPServer(mcp.ServerInfo{
		Name:    "test",
		Version: "0.0.1",
	})
	bridge := NewMCPBridge(provider, server)

	tool := &ComposioTool{
		Name:         "test_tool",
		Description:  "A test tool",
		Scope:        ScopeReadOnly,
		AuthRequired: false,
		Params:       map[string]interface{}{"key": "string"},
	}

	handler := bridge.wrapTool(tool)
	if handler.Name != "test_tool" {
		t.Errorf("expected name 'test_tool', got %q", handler.Name)
	}
	if handler.Description != "A test tool" {
		t.Errorf("expected description 'A test tool', got %q", handler.Description)
	}
	if handler.InputSchema == nil {
		t.Error("expected non-nil input schema")
	}
}

func TestMCPBridgeToolExecution(t *testing.T) {
	t.Parallel()
	provider := NewComposioProvider("test-key")
	provider.RegisterTool(&ComposioTool{
		Name:         "test_tool",
		Description:  "A test tool",
		Scope:        ScopeReadOnly,
		AuthRequired: false,
		Params:       map[string]interface{}{},
	})

	server := mcp.NewMCPServer(mcp.ServerInfo{
		Name:    "test",
		Version: "0.0.1",
	})
	bridge := NewMCPBridge(provider, server)

	// Wrap the tool and execute it
	tool := &ComposioTool{
		Name:         "test_tool",
		Description:  "A test tool",
		Scope:        ScopeReadOnly,
		AuthRequired: false,
		Params:       map[string]interface{}{},
	}
	handler := bridge.wrapTool(tool)

	params := json.RawMessage(`{"key": "value"}`)
	result, err := handler.Handler(context.Background(), params)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !strings.Contains(result, "success") {
		t.Error("expected result to contain 'success'")
	}
	if !strings.Contains(result, "test_tool") {
		t.Error("expected result to contain tool name")
	}
}

func TestMCPBridgeToolExecutionNotFound(t *testing.T) {
	t.Parallel()
	provider := NewComposioProvider("test-key")
	server := mcp.NewMCPServer(mcp.ServerInfo{
		Name:    "test",
		Version: "0.0.1",
	})
	bridge := NewMCPBridge(provider, server)

	tool := &ComposioTool{
		Name:         "missing_tool",
		Description:  "A tool not in the provider",
		Scope:        ScopeReadOnly,
		AuthRequired: false,
		Params:       map[string]interface{}{},
	}
	handler := bridge.wrapTool(tool)

	_, err := handler.Handler(context.Background(), nil)
	if err == nil {
		t.Error("expected error for tool not in provider")
	}
}

func TestMCPBridgeToolAnnotations(t *testing.T) {
	t.Parallel()
	provider := NewComposioProvider("test-key")
	server := mcp.NewMCPServer(mcp.ServerInfo{
		Name:    "test",
		Version: "0.0.1",
	})
	bridge := NewMCPBridge(provider, server)

	// Test read-only annotation
	tool := &ComposioTool{
		Name:   "read_tool",
		Scope:  ScopeReadOnly,
		Params: map[string]interface{}{},
	}
	annotations := bridge.toolAnnotations(tool)
	if annotations.ReadOnlyHint == nil || !*annotations.ReadOnlyHint {
		t.Error("expected ReadOnlyHint to be true for read-only tool")
	}

	// Test write annotation
	tool = &ComposioTool{
		Name:   "write_tool",
		Scope:  ScopeWrite,
		Params: map[string]interface{}{},
	}
	annotations = bridge.toolAnnotations(tool)
	if annotations.ReadOnlyHint == nil || *annotations.ReadOnlyHint {
		t.Error("expected ReadOnlyHint to be false for write tool")
	}
	if annotations.DestructiveHint == nil || !*annotations.DestructiveHint {
		t.Error("expected DestructiveHint to be true for write tool")
	}

	// Test action annotation
	tool = &ComposioTool{
		Name:   "action_tool",
		Scope:  ScopeAction,
		Params: map[string]interface{}{},
	}
	annotations = bridge.toolAnnotations(tool)
	if annotations.ReadOnlyHint == nil || *annotations.ReadOnlyHint {
		t.Error("expected ReadOnlyHint to be false for action tool")
	}
	if annotations.IdempotentHint == nil || !*annotations.IdempotentHint {
		t.Error("expected IdempotentHint to be true for action tool")
	}
}

func TestMCPBridgeSearchHandler(t *testing.T) {
	t.Parallel()
	provider := NewComposioProvider("test-key")
	provider.RegisterTool(&ComposioTool{
		Name:        "github_issues",
		Description: "List GitHub issues",
		Scope:       ScopeReadOnly,
		Params:      map[string]interface{}{},
		Tags:        []string{"github", "issues"},
		Category:    "github",
	})

	server := mcp.NewMCPServer(mcp.ServerInfo{
		Name:    "test",
		Version: "0.0.1",
	})
	bridge := NewMCPBridge(provider, server)

	// Build the search handler manually (same as RegisterSearchTool does)
	searchHandler := mcp.MCPToolHandler{
		Name:        "composio_search_tools",
		Description: "Search the composio tool catalog",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query",
				},
			},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			var req struct {
				Query string `json:"query"`
			}
			if len(params) > 0 {
				if err := json.Unmarshal(params, &req); err != nil {
					return "", err
				}
			}

			results := bridge.provider.SearchTools(req.Query)
			tools := make([]map[string]interface{}, 0, len(results))
			for _, t := range results {
				tools = append(tools, map[string]interface{}{
					"name":          t.Name,
					"description":   t.Description,
					"scope":         string(t.Scope),
					"auth_required": t.AuthRequired,
					"tags":          t.Tags,
					"category":      t.Category,
				})
			}

			output := map[string]interface{}{
				"tools": tools,
				"count": len(tools),
			}
			data, _ := json.MarshalIndent(output, "", "  ")
			return string(data), nil
		},
	}

	// Execute search with query
	params := json.RawMessage(`{"query": "github"}`)
	result, err := searchHandler.Handler(context.Background(), params)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	count, ok := output["count"].(float64)
	if !ok {
		t.Fatal("expected count field in result")
	}
	if count != 1 {
		t.Errorf("expected 1 tool found, got %f", count)
	}

	// Verify tool details
	tools, ok := output["tools"].([]interface{})
	if !ok {
		t.Fatal("expected tools array in result")
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	toolMap, ok := tools[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected tool to be a map")
	}
	if toolMap["name"] != "github_issues" {
		t.Errorf("expected name 'github_issues', got %v", toolMap["name"])
	}
}

func TestMCPBridgeCredentialHandler(t *testing.T) {
	t.Parallel()
	provider := NewComposioProvider("test-key")
	provider.Credentials().Store(&Credential{
		ID:          "cred-1",
		ServiceName: "github",
		Type:        "oauth",
		Value:       "token123",
	})

	server := mcp.NewMCPServer(mcp.ServerInfo{
		Name:    "test",
		Version: "0.0.1",
	})
	bridge := NewMCPBridge(provider, server)

	// Build the credential handler manually
	credHandler := mcp.MCPToolHandler{
		Name:        "composio_credentials",
		Description: "List composio credentials",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			creds := bridge.provider.Credentials().List()
			items := make([]map[string]interface{}, 0, len(creds))
			for _, c := range creds {
				items = append(items, map[string]interface{}{
					"id":           c.ID,
					"service_name": c.ServiceName,
					"type":         c.Type,
					"scope":        c.Scope,
					"expired":      c.IsExpired(),
				})
			}

			output := map[string]interface{}{
				"credentials": items,
				"count":       len(items),
			}
			data, _ := json.MarshalIndent(output, "", "  ")
			return string(data), nil
		},
	}

	result, err := credHandler.Handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	count, ok := output["count"].(float64)
	if !ok {
		t.Fatal("expected count field in result")
	}
	if count != 1 {
		t.Errorf("expected 1 credential, got %f", count)
	}
}
