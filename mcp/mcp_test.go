package mcp

import (
	"strings"
	"testing"
)

func TestToolName(t *testing.T) {
	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
	}
	if tool.Name != "test_tool" {
		t.Fatalf("expected name 'test_tool', got %q", tool.Name)
	}
}

func TestResource(t *testing.T) {
	r := Resource{
		URI:         "file:///test.txt",
		Name:        "test.txt",
		MimeType:    "text/plain",
		Description: "A test file",
	}
	if r.URI != "file:///test.txt" {
		t.Fatalf("expected URI 'file:///test.txt', got %q", r.URI)
	}
}

func TestParseToolName(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantTool string
	}{
		{"mcp__server1__tool1", "server1", "tool1"},
		{"mcp_server1_tool1", "server1", "tool1"},
		{"mcp__server__my_tool", "server", "my_tool"},
	}

	for _, tt := range tests {
		server, tool := parseToolName(tt.input)
		if server != tt.wantName || tool != tt.wantTool {
			t.Errorf("parseToolName(%q) = (%q, %q), want (%q, %q)",
				tt.input, server, tool, tt.wantName, tt.wantTool)
		}
	}
}

func TestFormatToolName(t *testing.T) {
	name := formatToolName("server1", "tool1")
	if name != "mcp__server1__tool1" {
		t.Fatalf("expected 'mcp__server1__tool1', got %q", name)
	}
}

func TestParseResourceURI(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantURI  string
	}{
		{"mcp://server1/file.txt", "server1", "file.txt"},
		{"mcp://server1/dir/file.txt", "server1", "dir/file.txt"},
	}

	for _, tt := range tests {
		server, uri := parseResourceURI(tt.input)
		if server != tt.wantName || uri != tt.wantURI {
			t.Errorf("parseResourceURI(%q) = (%q, %q), want (%q, %q)",
				tt.input, server, uri, tt.wantName, tt.wantURI)
		}
	}
}

// parseToolName parses an MCP tool name.
func parseToolName(name string) (server, tool string) {
	// Handle both mcp__server__tool and mcp_server_tool formats
	name = strings.TrimPrefix(name, "mcp")
	name = strings.TrimPrefix(name, "__")
	name = strings.TrimPrefix(name, "_")
	parts := strings.SplitN(name, "__", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	parts = strings.SplitN(name, "_", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", name
}

func formatToolName(server, tool string) string {
	return "mcp__" + server + "__" + tool
}

func parseResourceURI(uri string) (server, resource string) {
	uri = strings.TrimPrefix(uri, "mcp://")
	parts := strings.SplitN(uri, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}
