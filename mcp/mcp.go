package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Server represents a connected MCP server.
type Server struct {
	Name    string
	Command string
	Args    []string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	mu      sync.Mutex
	nextID  int
}

// Tool is a tool exposed by an MCP server.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// Resource is a resource exposed by an MCP server.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	MimeType    string `json:"mimeType,omitempty"`
	Description string `json:"description,omitempty"`
}

type jsonrpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Connect starts an MCP server process via stdio transport.
func Connect(ctx context.Context, name, command string, args ...string) (*Server, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start %s: %w", command, err)
	}

	s := &Server{Name: name, Command: command, Args: args, cmd: cmd, stdin: stdin, stdout: stdout}

	// Initialize
	_, err = s.call("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "hawk", "version": "0.0.1"},
	})
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("mcp: initialize: %w", err)
	}

	// Send initialized notification
	s.notify("notifications/initialized", nil)

	return s, nil
}

// ListTools returns tools available on this MCP server.
func (s *Server) ListTools() ([]Tool, error) {
	result, err := s.call("tools/list", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, err
	}
	return resp.Tools, nil
}

// ListResources returns resources available on this MCP server.
func (s *Server) ListResources() ([]Resource, error) {
	result, err := s.call("resources/list", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Resources []Resource `json:"resources"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, err
	}
	return resp.Resources, nil
}

// ReadResource reads a resource from this MCP server.
func (s *Server) ReadResource(uri string) (string, error) {
	result, err := s.call("resources/read", map[string]interface{}{"uri": uri})
	if err != nil {
		return "", err
	}
	var resp struct {
		Contents []struct {
			URI      string `json:"uri"`
			MimeType string `json:"mimeType,omitempty"`
			Text     string `json:"text,omitempty"`
			Blob     string `json:"blob,omitempty"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return string(result), nil
	}
	var out string
	for _, c := range resp.Contents {
		if c.Text != "" {
			out += c.Text
		} else if c.Blob != "" {
			out += fmt.Sprintf("[blob resource %s, mime=%s, base64 bytes=%d]", c.URI, c.MimeType, len(c.Blob))
		}
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
	}
	return strings.TrimRight(out, "\n"), nil
}

// CallTool invokes a tool on the MCP server.
func (s *Server) CallTool(name string, args map[string]interface{}) (string, error) {
	result, err := s.call("tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return string(result), nil
	}
	var text string
	for _, c := range resp.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}
	return text, nil
}

// Close shuts down the MCP server.
func (s *Server) Close() error {
	s.stdin.Close()
	return s.cmd.Wait()
}

func (s *Server) call(method string, params interface{}) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	req := jsonrpcRequest{JSONRPC: "2.0", ID: s.nextID, Method: method, Params: params}
	data, _ := json.Marshal(req)
	data = append(data, '\n')

	if _, err := s.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	// Read response line
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1)
	for {
		n, err := s.stdout.Read(tmp)
		if err != nil {
			return nil, fmt.Errorf("read: %w", err)
		}
		if n > 0 {
			if tmp[0] == '\n' {
				break
			}
			buf = append(buf, tmp[0])
		}
	}

	var resp jsonrpcResponse
	if err := json.Unmarshal(buf, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

func (s *Server) notify(method string, params interface{}) {
	req := jsonrpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	data, _ := json.Marshal(req)
	data = append(data, '\n')
	s.stdin.Write(data)
}
