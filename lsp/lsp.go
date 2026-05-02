package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Client represents an LSP client connection.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	reader *bufio.Reader // single persistent reader
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
	Method  string          `json:"method,omitempty"`
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

	c := &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		reader: bufio.NewReader(stdout), // single reader for the connection lifetime
	}
	m.servers[name] = c

	// Send initialize request with correct processId
	_, _ = c.Request("initialize", map[string]interface{}{
		"processId":    os.Getpid(),
		"rootUri":      "file://.",
		"capabilities": map[string]interface{}{},
	})

	// Send initialized notification per LSP spec
	c.Notify("initialized", struct{}{})

	return nil
}

// Stop stops an LSP server gracefully.
func (m *ServerManager) Stop(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.servers[name]
	if !ok {
		return nil
	}
	delete(m.servers, name)

	// Graceful shutdown per LSP spec
	_, _ = c.Request("shutdown", nil)
	c.Notify("exit", nil)

	_ = c.stdin.Close()
	_ = c.cmd.Process.Kill()
	return nil
}

// Notify sends a notification (no response expected).
func (c *Client) Notify(method string, params interface{}) {
	type notification struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}
	data, err := json.Marshal(notification{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(data))
	c.stdin.Write(data)
}

// Request sends a request to an LSP server.
func (c *Client) Request(method string, params interface{}) (*Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.id++
	id := c.id

	req := Request{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	if _, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return nil, err
	}
	if _, err := c.stdin.Write(data); err != nil {
		return nil, err
	}

	// Read response using the persistent reader, skipping notifications
	for {
		var contentLength int
		for {
			line, err := c.reader.ReadString('\n')
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
		if _, err := io.ReadFull(c.reader, respData); err != nil {
			return nil, err
		}

		var resp Response
		if err := json.Unmarshal(respData, &resp); err != nil {
			return nil, err
		}

		// Skip server-initiated notifications (no ID)
		if resp.ID == 0 && resp.Method != "" {
			continue
		}

		return &resp, nil
	}
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
