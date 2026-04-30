package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/GrayCodeAI/hawk/mcp"
)

var connectedMCPServers = struct {
	sync.RWMutex
	servers map[string]*mcp.Server
}{servers: make(map[string]*mcp.Server)}

// MCPTool wraps an MCP server tool as a hawk tool.
type MCPTool struct {
	server      *mcp.Server
	toolName    string
	aliases     []string
	remoteName  string
	description string
	schema      map[string]interface{}
}

func NewMCPTool(server *mcp.Server, t mcp.Tool) *MCPTool {
	tsName := fmt.Sprintf("mcp__%s__%s", normalizeNameForMCP(server.Name), normalizeNameForMCP(t.Name))
	legacyName := fmt.Sprintf("mcp_%s_%s", server.Name, t.Name)
	return &MCPTool{
		server:      server,
		toolName:    tsName,
		aliases:     []string{legacyName},
		remoteName:  t.Name,
		description: fmt.Sprintf("[MCP:%s] %s", server.Name, t.Description),
		schema:      t.InputSchema,
	}
}

func (m *MCPTool) Name() string                       { return m.toolName }
func (m *MCPTool) Aliases() []string                  { return m.aliases }
func (m *MCPTool) Description() string                { return m.description }
func (m *MCPTool) Parameters() map[string]interface{} { return m.schema }

func (m *MCPTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	return m.server.CallTool(m.remoteName, args)
}

func normalizeNameForMCP(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r)
		case r >= '0' && r <= '9':
			out = append(out, r)
		case r == '_' || r == '-':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

// LoadMCPTools connects to an MCP server and returns hawk tools for all its tools.
func LoadMCPTools(ctx context.Context, name, command string, args ...string) ([]Tool, error) {
	server, err := mcp.Connect(ctx, name, command, args...)
	if err != nil {
		return nil, err
	}
	registerMCPServer(server)
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

func registerMCPServer(server *mcp.Server) {
	connectedMCPServers.Lock()
	defer connectedMCPServers.Unlock()
	connectedMCPServers.servers[server.Name] = server
}

func listMCPServers() []*mcp.Server {
	connectedMCPServers.RLock()
	defer connectedMCPServers.RUnlock()
	servers := make([]*mcp.Server, 0, len(connectedMCPServers.servers))
	for _, server := range connectedMCPServers.servers {
		servers = append(servers, server)
	}
	return servers
}

func getMCPServer(name string) (*mcp.Server, bool) {
	connectedMCPServers.RLock()
	defer connectedMCPServers.RUnlock()
	server, ok := connectedMCPServers.servers[name]
	return server, ok
}
