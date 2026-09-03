package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GrayCodeAI/graycode-cli/internal/mcp"
)

type ListMcpResourcesTool struct{}

func (ListMcpResourcesTool) Name() string { return "ListMcpResourcesTool" }
func (ListMcpResourcesTool) Aliases() []string {
	return []string{"list_mcp_resources", "listMcpResources"}
}

func (ListMcpResourcesTool) Description() string {
	return "List resources exposed by connected MCP servers. Optionally filter by server name."
}

func (ListMcpResourcesTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"server": map[string]interface{}{"type": "string", "description": "Optional MCP server name to filter resources by"},
		},
	}
}

func (ListMcpResourcesTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Server string `json:"server"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
	}

	type resourceOut struct {
		URI         string `json:"uri"`
		Name        string `json:"name"`
		MimeType    string `json:"mimeType,omitempty"`
		Description string `json:"description,omitempty"`
		Server      string `json:"server"`
	}
	var out []resourceOut
	servers := listMCPServers()
	if p.Server != "" {
		server, ok := getMCPServer(p.Server)
		if !ok {
			return "", fmt.Errorf("MCP server %q not found", p.Server)
		}
		servers = map[string]mcpClient{p.Server: server}
	}
	for name, server := range servers {
		// Resource listing is currently stdio-only: mcp.HTTPServer/WSServer
		// don't implement it. Remote-transport servers are silently skipped
		// here rather than erroring, same as if they'd exposed zero resources.
		stdioServer, ok := server.(*mcp.Server)
		if !ok {
			continue
		}
		resources, err := stdioServer.ListResources()
		if err != nil {
			continue
		}
		for _, r := range resources {
			out = append(out, resourceOut{URI: r.URI, Name: r.Name, MimeType: r.MimeType, Description: r.Description, Server: name})
		}
	}
	if len(out) == 0 {
		return "No MCP resources found.", nil
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	return string(data), nil
}

type ReadMcpResourceTool struct{}

func (ReadMcpResourceTool) Name() string { return "ReadMcpResourceTool" }
func (ReadMcpResourceTool) Aliases() []string {
	return []string{"read_mcp_resource", "readMcpResource"}
}

func (ReadMcpResourceTool) Description() string {
	return "Read a resource exposed by a connected MCP server."
}

func (ReadMcpResourceTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"server": map[string]interface{}{"type": "string", "description": "MCP server name"},
			"uri":    map[string]interface{}{"type": "string", "description": "Resource URI"},
		},
		"required": []string{"server", "uri"},
	}
}

func (ReadMcpResourceTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Server string `json:"server"`
		URI    string `json:"uri"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Server == "" || p.URI == "" {
		return "", fmt.Errorf("server and uri are required")
	}
	server, ok := getMCPServer(p.Server)
	if !ok {
		return "", fmt.Errorf("MCP server %q not found", p.Server)
	}
	stdioServer, ok := server.(*mcp.Server)
	if !ok {
		return "", fmt.Errorf("MCP server %q does not support resource reads (remote transports don't implement this yet)", p.Server)
	}
	return stdioServer.ReadResource(p.URI)
}
