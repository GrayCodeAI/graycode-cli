package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GrayCodeAI/hawk/mcp"
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
		servers = []*mcp.Server{server}
	}
	for _, server := range servers {
		resources, err := server.ListResources()
		if err != nil {
			continue
		}
		for _, r := range resources {
			out = append(out, resourceOut{URI: r.URI, Name: r.Name, MimeType: r.MimeType, Description: r.Description, Server: server.Name})
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
	return server.ReadResource(p.URI)
}
