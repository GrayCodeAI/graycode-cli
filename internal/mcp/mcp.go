package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// clientVersion is the hawk version reported in MCP `initialize` clientInfo.
// It is wired at startup by main.go from the canonical hawk version (the
// VERSION file at the repo root, injected via ldflags). The "dev" default
// applies only to local builds without ldflags.
var clientVersion = "dev"

// SetClientVersion lets main.go propagate the canonical hawk version into
// this package without creating an import cycle with cmd.
func SetClientVersion(v string) { clientVersion = v }

// Server represents a connected MCP server.
type Server struct {
	Name       string
	Command    string
	Args       []string
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	mu         sync.Mutex
	nextID     int
	reader     *bufio.Scanner
	pending    map[int]chan json.RawMessage // response channels keyed by request ID
	pendErrors map[int]string               // error details keyed by request ID
	pendMu     sync.Mutex
	closeOnce  sync.Once
	closeErr   error
	// dead is set once the stdout reader stops (oversized response, server
	// crash, …). New calls fail fast instead of hanging until the timeout,
	// and the child process is killed so it cannot linger (M7).
	dead     atomic.Bool
	deadOnce sync.Once
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
	cmd := exec.CommandContext(ctx, command, args...) // #nosec G204 -- command comes from the configured MCP server definition
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}
	if startErr := cmd.Start(); startErr != nil {
		return nil, fmt.Errorf("mcp: start %s: %w", command, startErr)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB buffer

	s := &Server{
		Name:       name,
		Command:    command,
		Args:       args,
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdout,
		reader:     scanner,
		pending:    make(map[int]chan json.RawMessage),
		pendErrors: make(map[int]string),
	}

	// Start background reader to dispatch responses and notifications
	go s.readLoop()

	// Initialize
	_, err = s.callWithTimeout(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "hawk", "version": clientVersion},
	})
	if err != nil {
		_ = cmd.Process.Kill()
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
					// Store error details so the caller can include them
					// in the returned error instead of a generic message.
					s.pendMu.Lock()
					s.pendErrors[msg.ID] = fmt.Sprintf("code %d: %s", msg.Error.Code, msg.Error.Message)
					s.pendMu.Unlock()
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
	// Scanner done — log the cause if it was an error (e.g., oversized
	// response exceeding the 1MB buffer), then close all pending channels.
	var cause string
	if err := s.reader.Err(); err != nil {
		cause = err.Error()
		slog.Warn("mcp: stdout reader stopped", "server", s.Name, "error", err)
	}
	// The connection is permanently broken: kill the child so it cannot
	// linger in the background, and mark the server dead so new calls fail
	// fast instead of hanging until the call timeout (M7). Close() is safe
	// to race: it closes stdin and waits; killing first makes Wait return
	// immediately.
	s.deadOnce.Do(func() {
		s.dead.Store(true)
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
	})
	if cause != "" {
		slog.Warn("mcp: server connection lost", "server", s.Name, "cause", cause)
	}
	s.pendMu.Lock()
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
		// Clean up pendErrors only for requests that will never be
		// answered. Entries for already-signaled requests (no longer in
		// s.pending) are left for the caller to reap.
		delete(s.pendErrors, id)
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
func (s *Server) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	result, err := s.callWithTimeout(ctx, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}
	return parseToolCallResult(result)
}

// parseToolCallResult decodes an MCP tools/call result into flattened text.
//
// It honors the spec's isError flag: when the remote tool reports a failure
// (isError:true), the content carries the failure detail and this returns it as
// a Go error so the agent loop surfaces it to the model — which can then
// self-correct — instead of mistaking a failure for a successful result.
// hawk's own MCP server sets this flag (internal/mcp/server.go), so the client
// must read it for symmetry. An undecodable result falls back to the raw bytes.
func parseToolCallResult(result json.RawMessage) (string, error) {
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
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
	if resp.IsError {
		if text == "" {
			text = "remote MCP tool reported an error"
		}
		return text, fmt.Errorf("MCP tool error: %s", text)
	}
	return text, nil
}

// Close shuts down the MCP server, killing the child process if it doesn't exit
// within 5 seconds of stdin being closed.
//
// It is idempotent and safe to call concurrently: Connect's initialize-failure
// path and a manager's later cleanup may both reach Close, and closing stdin or
// calling cmd.Wait twice races. The sync.Once ensures the shutdown runs exactly
// once and every caller observes the same result. Killing the child causes
// stdout to EOF, which unblocks and terminates the readLoop goroutine.
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		_ = s.stdin.Close()
		done := make(chan error, 1)
		go func() { done <- s.cmd.Wait() }()
		select {
		case err := <-done:
			s.closeErr = err
		case <-time.After(5 * time.Second):
			_ = s.cmd.Process.Kill()
			s.closeErr = <-done
		}
	})
	return s.closeErr
}

func (s *Server) call(method string, params interface{}) (json.RawMessage, error) {
	return s.callWithTimeout(context.Background(), method, params)
}

func (s *Server) callWithTimeout(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	// Fail fast once the reader has stopped (oversized response, crash):
	// a request registered now would never be answered and would hang
	// until the call timeout (M7).
	if s.dead.Load() {
		return nil, fmt.Errorf("mcp: connection closed (server %s is no longer running)", s.Name)
	}

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
		delete(s.pendErrors, id)
		s.pendMu.Unlock()
		return nil, fmt.Errorf("write: %w", err)
	}

	// Wait for response with timeout
	timeout := defaultCallTimeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	// Use time.NewTimer + Stop instead of time.After to avoid leaking
	// the timer in the runtime when the response arrives or ctx is
	// cancelled before the timeout fires.
	timer := time.NewTimer(timeout)
	select {
	case result, ok := <-ch:
		timer.Stop()
		if !ok {
			return nil, fmt.Errorf("mcp: connection closed")
		}
		if result == nil {
			// Include the server's error code and message if available,
			// instead of a generic "server returned error" with no detail.
			s.pendMu.Lock()
			errMsg := s.pendErrors[id]
			delete(s.pendErrors, id)
			s.pendMu.Unlock()
			if errMsg != "" {
				return nil, fmt.Errorf("mcp: server error: %s", errMsg)
			}
			return nil, fmt.Errorf("mcp: server returned error")
		}
		return result, nil
	case <-timer.C:
		s.pendMu.Lock()
		delete(s.pending, id)
		delete(s.pendErrors, id)
		s.pendMu.Unlock()
		return nil, fmt.Errorf("mcp: call %s timed out after %s", method, timeout)
	case <-ctx.Done():
		timer.Stop()
		s.pendMu.Lock()
		delete(s.pending, id)
		delete(s.pendErrors, id)
		s.pendMu.Unlock()
		return nil, ctx.Err()
	}
}

func (s *Server) notify(method string, params interface{}) {
	req := jsonrpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	data, _ := json.Marshal(req)
	data = append(data, '\n')
	s.mu.Lock()
	_, _ = s.stdin.Write(data)
	s.mu.Unlock()
}
