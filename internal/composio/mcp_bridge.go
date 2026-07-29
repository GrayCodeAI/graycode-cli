// Package composio provides integration between hawk's MCP server and the
// Composio tool platform. This file adds the MCP bridge that registers
// composio tools as MCP tools in hawk's MCP server.
package composio

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GrayCodeAI/hawk/internal/mcp"
)

// MCPBridge connects a ComposioProvider to hawk's MCP server, registering
// composio tools as MCP tools and providing tool search.
type MCPBridge struct {
	provider *ComposioProvider
	server   *mcp.MCPServer
}

// NewMCPBridge creates a bridge between a composio provider and an MCP server.
// The MCP server will receive composio tools registered via RegisterTools().
func NewMCPBridge(provider *ComposioProvider, server *mcp.MCPServer) *MCPBridge {
	return &MCPBridge{
		provider: provider,
		server:   server,
	}
}

// RegisterTools registers all composio tools as MCP tools on the server.
// Each composio tool becomes an MCP tool with its name, description, and
// input schema. Tool execution is proxied to the composio provider.
func (b *MCPBridge) RegisterTools() int {
	count := 0
	for _, tool := range b.provider.ListTools() {
		handler := b.wrapTool(tool)
		b.server.RegisterTool(handler)
		count++
	}
	return count
}

// RegisterSearchTool registers a composio tool search MCP tool.
// This allows MCP clients to search the composio tool catalog.
func (b *MCPBridge) RegisterSearchTool() {
	handler := mcp.MCPToolHandler{
		Name:        "composio_search_tools",
		Description: "Search the composio tool catalog by name, description, or tags",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query (empty returns all tools)",
				},
			},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			var req struct {
				Query string `json:"query"`
			}
			if len(params) > 0 {
				if err := json.Unmarshal(params, &req); err != nil {
					return "", fmt.Errorf("parse params: %w", err)
				}
			}

			results := b.provider.SearchTools(req.Query)
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
			data, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				return "", fmt.Errorf("marshal results: %w", err)
			}
			return string(data), nil
		},
	}
	b.server.RegisterTool(handler)
}

// RegisterCredentialTool registers a composio credential management MCP tool.
// This allows MCP clients to list and manage composio credentials.
func (b *MCPBridge) RegisterCredentialTool() {
	handler := mcp.MCPToolHandler{
		Name:        "composio_credentials",
		Description: "List composio credentials for connected services",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			creds := b.provider.Credentials().List()
			items := make([]map[string]interface{}, 0, len(creds))
			for _, c := range creds {
				items = append(items, map[string]interface{}{
					"id":           c.ID,
					"service_name": c.ServiceName,
					"type":         c.Type,
					"scope":        c.Scope,
					"expires_at":   c.ExpiresAt.Format("2006-01-02T15:04:05Z"),
					"expired":      c.IsExpired(),
				})
			}

			output := map[string]interface{}{
				"credentials": items,
				"count":       len(items),
			}
			data, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				return "", fmt.Errorf("marshal credentials: %w", err)
			}
			return string(data), nil
		},
	}
	b.server.RegisterTool(handler)
}

// wrapTool converts a ComposioTool into an MCPToolHandler.
func (b *MCPBridge) wrapTool(tool *ComposioTool) mcp.MCPToolHandler {
	// Build input schema from the tool's params
	schema := map[string]interface{}{
		"type":       "object",
		"properties": tool.Params,
	}
	if schema["properties"] == nil {
		schema["properties"] = map[string]interface{}{}
	}

	return mcp.MCPToolHandler{
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: schema,
		Annotations: b.toolAnnotations(tool),
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			result, err := b.provider.ExecuteTool(ctx, tool.Name, params)
			if err != nil {
				return "", err
			}
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return "", fmt.Errorf("marshal result: %w", err)
			}
			return string(data), nil
		},
	}
}

// toolAnnotations converts composio tool scope to MCP tool annotations.
func (b *MCPBridge) toolAnnotations(tool *ComposioTool) *mcp.ToolAnnotations {
	annotations := &mcp.ToolAnnotations{}

	switch tool.Scope {
	case ScopeReadOnly:
		ro := true
		annotations.ReadOnlyHint = &ro
	case ScopeWrite:
		ro := false
		annotations.ReadOnlyHint = &ro
		dh := true
		annotations.DestructiveHint = &dh
	case ScopeAction:
		ro := false
		annotations.ReadOnlyHint = &ro
		ih := true
		annotations.IdempotentHint = &ih
	}

	return annotations
}

// RegisterAll registers all composio tools plus search and credential
// management tools on the MCP server. Returns the total number of tools
// registered.
func (b *MCPBridge) RegisterAll() int {
	count := b.RegisterTools()
	b.RegisterSearchTool()
	b.RegisterCredentialTool()
	return count + 2 // +2 for search and credential tools
}
