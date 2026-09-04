package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolExecutor is a function that executes a named tool with JSON input.
// This avoids importing the tool package (which already imports mcp).
type ToolExecutor func(ctx context.Context, name string, input json.RawMessage) (string, error)

// boolPtr returns a pointer to b, for the pointer-typed MCP annotation hints.
func boolPtr(b bool) *bool { return &b }

// readOnlyAnnotations marks a tool that only reads/inspects and never mutates
// the workspace, so a client can run it without prompting.
func readOnlyAnnotations(title string) *ToolAnnotations {
	return &ToolAnnotations{Title: title, ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false)}
}

// RegisterDefaultTools registers graycode's standard capabilities as MCP tools.
// If executor is non-nil, tools that delegate to graycode's tool registry will
// use it for execution; otherwise those tools return a not-configured error.
func RegisterDefaultTools(server *MCPServer, executor ToolExecutor) {
	server.RegisterTool(graycodeChatTool(executor))
	server.RegisterTool(graycodeSearchTool(executor))
	server.RegisterTool(graycodeMemoryRecallTool(executor))
	server.RegisterTool(graycodeMemoryStoreTool(executor))
	server.RegisterTool(graycodeReviewTool(executor))
	server.RegisterTool(graycodeScanTool(executor))
	server.RegisterTool(graycodeCompressTool(executor))
}

// graycodeChatTool sends a prompt to graycode and returns the response.
func graycodeChatTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name: "graycode_chat",
		Description: "Send a prompt to the graycode AI coding agent and receive a response. " +
			"WARNING: this runs an autonomous agent that may execute shell commands and modify files.",
		Annotations: &ToolAnnotations{
			Title:           "Run graycode agent",
			ReadOnlyHint:    boolPtr(false),
			DestructiveHint: boolPtr(true),
			OpenWorldHint:   boolPtr(true),
		},
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{
					"type":        "string",
					"description": "The prompt or question to send to graycode.",
				},
			},
			"required": []string{"prompt"},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			var input struct {
				Prompt string `json:"prompt"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return "", fmt.Errorf("invalid input: %w", err)
			}
			if input.Prompt == "" {
				return "", fmt.Errorf("prompt is required")
			}
			return delegateToExecutor(ctx, executor, "agent", params)
		},
	}
}

// graycodeSearchTool searches across graycode sessions.
func graycodeSearchTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name:        "graycode_search",
		Description: "Search across graycode sessions and conversation history.",
		Annotations: readOnlyAnnotations("Search graycode sessions"),
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "The search query.",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of results to return.",
				},
			},
			"required": []string{"query"},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			var input struct {
				Query string `json:"query"`
				Limit int    `json:"limit"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return "", fmt.Errorf("invalid input: %w", err)
			}
			if input.Query == "" {
				return "", fmt.Errorf("query is required")
			}
			return delegateToExecutor(ctx, executor, "code_search", params)
		},
	}
}

// graycodeMemoryRecallTool recalls information from harrier memory.
func graycodeMemoryRecallTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name:        "graycode_memory_recall",
		Description: "Recall stored information from graycode's persistent memory (harrier).",
		Annotations: readOnlyAnnotations("Recall from graycode memory"),
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "The memory recall query.",
				},
				"namespace": map[string]interface{}{
					"type":        "string",
					"description": "Optional namespace to search within.",
				},
			},
			"required": []string{"query"},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			var input struct {
				Query     string `json:"query"`
				Namespace string `json:"namespace"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return "", fmt.Errorf("invalid input: %w", err)
			}
			if input.Query == "" {
				return "", fmt.Errorf("query is required")
			}
			return delegateToExecutor(ctx, executor, "core_memory", params)
		},
	}
}

// graycodeMemoryStoreTool stores information to harrier memory.
func graycodeMemoryStoreTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name:        "graycode_memory_store",
		Description: "Store information in graycode's persistent memory (harrier) for future recall.",
		Annotations: &ToolAnnotations{
			Title:           "Store in graycode memory",
			ReadOnlyHint:    boolPtr(false),
			DestructiveHint: boolPtr(false), // additive write, does not destroy existing data
		},
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"key": map[string]interface{}{
					"type":        "string",
					"description": "A key or label for the memory entry.",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The content to store.",
				},
				"namespace": map[string]interface{}{
					"type":        "string",
					"description": "Optional namespace for organization.",
				},
			},
			"required": []string{"key", "content"},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			var input struct {
				Key       string `json:"key"`
				Content   string `json:"content"`
				Namespace string `json:"namespace"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return "", fmt.Errorf("invalid input: %w", err)
			}
			if input.Key == "" || input.Content == "" {
				return "", fmt.Errorf("key and content are required")
			}
			return delegateToExecutor(ctx, executor, "core_memory", params)
		},
	}
}

// graycodeReviewTool triggers a code review via kestrel.
func graycodeReviewTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name:        "graycode_review",
		Description: "Trigger a code review using graycode's kestrel module. Analyzes code for quality, style, and potential issues.",
		Annotations: readOnlyAnnotations("Review code (kestrel)"),
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "File or directory path to review.",
				},
				"diff": map[string]interface{}{
					"type":        "string",
					"description": "Optional diff content to review instead of a path.",
				},
			},
			"required": []string{},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			var input struct {
				Path string `json:"path"`
				Diff string `json:"diff"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return "", fmt.Errorf("invalid input: %w", err)
			}
			if input.Path == "" && input.Diff == "" {
				return "", fmt.Errorf("either path or diff is required")
			}
			return delegateToExecutor(ctx, executor, "code_review", params)
		},
	}
}

// graycodeScanTool triggers a security scan via merlin.
func graycodeScanTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name:        "graycode_scan",
		Description: "Trigger a security scan using graycode's merlin module. Identifies vulnerabilities and security issues.",
		Annotations: readOnlyAnnotations("Security scan (merlin)"),
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "File or directory path to scan.",
				},
				"severity": map[string]interface{}{
					"type":        "string",
					"description": "Minimum severity level to report (low, medium, high, critical).",
					"enum":        []string{"low", "medium", "high", "critical"},
				},
			},
			"required": []string{"path"},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			var input struct {
				Path     string `json:"path"`
				Severity string `json:"severity"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return "", fmt.Errorf("invalid input: %w", err)
			}
			if input.Path == "" {
				return "", fmt.Errorf("path is required")
			}
			return delegateToExecutor(ctx, executor, "security_scan", params)
		},
	}
}

// graycodeCompressTool compresses text via shrike.
func graycodeCompressTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name:        "graycode_compress",
		Description: "Compress text using graycode's shrike module to reduce token usage while preserving meaning.",
		Annotations: readOnlyAnnotations("Compress text (shrike)"),
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"text": map[string]interface{}{
					"type":        "string",
					"description": "The text to compress.",
				},
				"ratio": map[string]interface{}{
					"type":        "number",
					"description": "Target compression ratio (0.0-1.0). Lower means more compression.",
				},
			},
			"required": []string{"text"},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			var input struct {
				Text  string  `json:"text"`
				Ratio float64 `json:"ratio"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return "", fmt.Errorf("invalid input: %w", err)
			}
			if input.Text == "" {
				return "", fmt.Errorf("text is required")
			}
			return delegateToExecutor(ctx, executor, "compress", params)
		},
	}
}

// delegateToExecutor executes a tool through the provided executor function.
func delegateToExecutor(ctx context.Context, executor ToolExecutor, name string, params json.RawMessage) (string, error) {
	if executor == nil {
		return "", fmt.Errorf("tool executor not configured")
	}
	return executor(ctx, name, params)
}
