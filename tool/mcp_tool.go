package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GrayCodeAI/hawk/mcp"
)

// MCPTool wraps an MCP server tool as a hawk tool.
type MCPTool struct {
	server      *mcp.Server
	toolName    string
	description string
	schema      map[string]interface{}
}

func NewMCPTool(server *mcp.Server, t mcp.Tool) *MCPTool {
	return &MCPTool{
		server:      server,
		toolName:    fmt.Sprintf("mcp_%s_%s", server.Name, t.Name),
		description: fmt.Sprintf("[MCP:%s] %s", server.Name, t.Description),
		schema:      t.InputSchema,
	}
}

func (m *MCPTool) Name() string                       { return m.toolName }
func (m *MCPTool) Description() string                { return m.description }
func (m *MCPTool) Parameters() map[string]interface{} { return m.schema }

func (m *MCPTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	// Extract the original tool name (strip mcp_servername_ prefix)
	origName := m.toolName[len("mcp_"+m.server.Name+"_"):]
	return m.server.CallTool(origName, args)
}

// LoadMCPTools connects to an MCP server and returns hawk tools for all its tools.
func LoadMCPTools(ctx context.Context, name, command string, args ...string) ([]Tool, error) {
	server, err := mcp.Connect(ctx, name, command, args...)
	if err != nil {
		return nil, err
	}
	mcpTools, err := server.ListTools()
	if err != nil {
		server.Close()
		return nil, err
	}
	var tools []Tool
	for _, t := range mcpTools {
		tools = append(tools, NewMCPTool(server, t))
	}
	return tools, nil
}
