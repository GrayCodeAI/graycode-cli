package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
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
	reader  *bufio.Scanner
	pending map[int]chan json.RawMessage // response channels keyed by request ID
	pendMu  sync.Mutex
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
	ID      int         `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

const defaultCallTimeout = 30 * time.Second

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

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB buffer

	s := &Server{
		Name:    name,
		Command: command,
		Args:    args,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		reader:  scanner,
		pending: make(map[int]chan json.RawMessage),
	}

	// Start background reader to dispatch responses and notifications
	go s.readLoop()

	// Initialize
	_, err = s.callWithTimeout(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "hawk", "version": "0.2.0"},
	})
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("mcp: initialize: %w", err)
	}

	// Send initialized notification
	s.notify("notifications/initialized", nil)

	return s, nil
}

// readLoop reads lines from stdout and dispatches to pending request channels.
func (s *Server) readLoop() {
	for s.reader.Scan() {
		line := s.reader.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg jsonrpcResponse
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		// If it has an ID, it's a response to a request
		if msg.ID != 0 {
			s.pendMu.Lock()
			ch, ok := s.pending[msg.ID]
			if ok {
				delete(s.pending, msg.ID)
			}
			s.pendMu.Unlock()
			if ok {
				if msg.Error != nil {
					ch <- nil // signal error via nil
				} else {
					ch <- msg.Result
				}
				close(ch)
			}
			continue
		}
		// Otherwise it's a notification — ignore for now
	}
	// Scanner done — close all pending channels
	s.pendMu.Lock()
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
	}
	s.pendMu.Unlock()
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
	return s.callWithTimeout(context.Background(), method, params)
}

func (s *Server) callWithTimeout(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()

	// Register pending response channel
	ch := make(chan json.RawMessage, 1)
	s.pendMu.Lock()
	s.pending[id] = ch
	s.pendMu.Unlock()

	req := jsonrpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	data, _ := json.Marshal(req)
	data = append(data, '\n')

	s.mu.Lock()
	_, err := s.stdin.Write(data)
	s.mu.Unlock()
	if err != nil {
		s.pendMu.Lock()
		delete(s.pending, id)
		s.pendMu.Unlock()
		return nil, fmt.Errorf("write: %w", err)
	}

	// Wait for response with timeout
	timeout := defaultCallTimeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	select {
	case result, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("mcp: connection closed")
		}
		if result == nil {
			return nil, fmt.Errorf("mcp: server returned error")
		}
		return result, nil
	case <-time.After(timeout):
		s.pendMu.Lock()
		delete(s.pending, id)
		s.pendMu.Unlock()
		return nil, fmt.Errorf("mcp: call %s timed out after %s", method, timeout)
	case <-ctx.Done():
		s.pendMu.Lock()
		delete(s.pending, id)
		s.pendMu.Unlock()
		return nil, ctx.Err()
	}
}

func (s *Server) notify(method string, params interface{}) {
	req := jsonrpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	data, _ := json.Marshal(req)
	data = append(data, '\n')
	s.mu.Lock()
	s.stdin.Write(data)
	s.mu.Unlock()
}
