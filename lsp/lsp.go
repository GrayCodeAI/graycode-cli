package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Client represents an LSP client connection.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	mu     sync.Mutex
	id     int
}

// Request is an LSP request.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Response is an LSP response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// ResponseError is an LSP error.
type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ServerManager manages LSP server connections.
type ServerManager struct {
	mu      sync.RWMutex
	servers map[string]*Client
}

// NewServerManager creates a new LSP server manager.
func NewServerManager() *ServerManager {
	return &ServerManager{servers: make(map[string]*Client)}
}

// Start starts an LSP server.
func (m *ServerManager) Start(name, command string, args ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.servers[name]; ok {
		return nil // already running
	}

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	client := &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}
	m.servers[name] = client

	// Send initialize request
	_, _ = client.Request("initialize", map[string]interface{}{
		"processId":    command,
		"rootUri":      "file://.",
		"capabilities": map[string]interface{}{},
	})

	return nil
}

// Stop stops an LSP server.
func (m *ServerManager) Stop(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, ok := m.servers[name]
	if !ok {
		return nil
	}
	delete(m.servers, name)

	_ = client.stdin.Close()
	_ = client.cmd.Process.Kill()
	return nil
}

// Request sends a request to an LSP server.
func (c *Client) Request(method string, params interface{}) (*Response, error) {
	c.mu.Lock()
	c.id++
	id := c.id
	c.mu.Unlock()

	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return nil, err
	}
	if _, err := c.stdin.Write(data); err != nil {
		return nil, err
	}

	// Read response
	reader := bufio.NewReader(c.stdout)
	var contentLength int
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			fmt.Sscanf(line, "Content-Length: %d", &contentLength)
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("no content length")
	}

	respData := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, respData); err != nil {
		return nil, err
	}

	var resp Response
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// List returns all running servers.
func (m *ServerManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []string
	for name := range m.servers {
		out = append(out, name)
	}
	return out
}

// IsRunning checks if a server is running.
func (m *ServerManager) IsRunning(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.servers[name]
	return ok
}
