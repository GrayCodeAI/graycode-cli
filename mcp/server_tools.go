package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolExecutor is a function that executes a named tool with JSON input.
// This avoids importing the tool package (which already imports mcp).
type ToolExecutor func(ctx context.Context, name string, input json.RawMessage) (string, error)

// RegisterDefaultTools registers hawk's standard capabilities as MCP tools.
// If executor is non-nil, tools that delegate to hawk's tool registry will
// use it for execution; otherwise those tools return a not-configured error.
func RegisterDefaultTools(server *MCPServer, executor ToolExecutor) {
	server.RegisterTool(hawkChatTool(executor))
	server.RegisterTool(hawkSearchTool(executor))
	server.RegisterTool(hawkMemoryRecallTool(executor))
	server.RegisterTool(hawkMemoryStoreTool(executor))
	server.RegisterTool(hawkReviewTool(executor))
	server.RegisterTool(hawkScanTool(executor))
	server.RegisterTool(hawkCompressTool(executor))
}

// hawkChatTool sends a prompt to hawk and returns the response.
func hawkChatTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name:        "hawk_chat",
		Description: "Send a prompt to the hawk AI coding agent and receive a response.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{
					"type":        "string",
					"description": "The prompt or question to send to hawk.",
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

// hawkSearchTool searches across hawk sessions.
func hawkSearchTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name:        "hawk_search",
		Description: "Search across hawk sessions and conversation history.",
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

// hawkMemoryRecallTool recalls information from yaad memory.
func hawkMemoryRecallTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name:        "hawk_memory_recall",
		Description: "Recall stored information from hawk's persistent memory (yaad).",
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

// hawkMemoryStoreTool stores information to yaad memory.
func hawkMemoryStoreTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name:        "hawk_memory_store",
		Description: "Store information in hawk's persistent memory (yaad) for future recall.",
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

// hawkReviewTool triggers a code review via sight.
func hawkReviewTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name:        "hawk_review",
		Description: "Trigger a code review using hawk's sight module. Analyzes code for quality, style, and potential issues.",
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

// hawkScanTool triggers a security scan via inspect.
func hawkScanTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name:        "hawk_scan",
		Description: "Trigger a security scan using hawk's inspect module. Identifies vulnerabilities and security issues.",
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

// hawkCompressTool compresses text via tok.
func hawkCompressTool(executor ToolExecutor) MCPToolHandler {
	return MCPToolHandler{
		Name:        "hawk_compress",
		Description: "Compress text using hawk's tok module to reduce token usage while preserving meaning.",
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
