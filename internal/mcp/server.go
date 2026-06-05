package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// rateLimiter implements a simple sliding-window rate limiter.
type rateLimiter struct {
	mu       sync.Mutex
	window   time.Duration
	maxCalls int
	calls    []time.Time
}

func newRateLimiter(window time.Duration, maxCalls int) *rateLimiter {
	return &rateLimiter{window: window, maxCalls: maxCalls}
}

func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-rl.window)
	// Remove expired entries
	i := 0
	for i < len(rl.calls) && rl.calls[i].Before(cutoff) {
		i++
	}
	rl.calls = rl.calls[i:]
	if len(rl.calls) >= rl.maxCalls {
		return false
	}
	rl.calls = append(rl.calls, now)
	return true
}

// MCPServer exposes hawk's capabilities as an MCP server that external
// clients (IDEs, agents, CLI tools) can connect to via JSON-RPC 2.0 over stdio.
type MCPServer struct {
	tools  map[string]MCPToolHandler
	mu     sync.RWMutex
	info   ServerInfo
	limits *rateLimiter
}

// ServerInfo identifies the MCP server to connecting clients.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPToolHandler defines a tool exposed by the MCP server.
type MCPToolHandler struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	Handler     func(ctx context.Context, params json.RawMessage) (string, error)
}

// JSON-RPC 2.0 request from a client.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSON-RPC 2.0 response to a client.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Standard JSON-RPC 2.0 error codes.
const (
	errCodeParseError     = -32700
	errCodeInvalidRequest = -32600
	errCodeMethodNotFound = -32601
	errCodeInvalidParams  = -32602
	errCodeInternal       = -32603
)

// NewMCPServer creates a new MCP server with the given identity.
func NewMCPServer(info ServerInfo) *MCPServer {
	return &MCPServer{
		tools:  make(map[string]MCPToolHandler),
		info:   info,
		limits: newRateLimiter(time.Second, 100), // 100 tool calls per second
	}
}

// RegisterTool adds a tool to the server. If a tool with the same name
// already exists, it is replaced.
func (s *MCPServer) RegisterTool(handler MCPToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[handler.Name] = handler
}

// ServeStdio runs the MCP server on stdin/stdout, reading JSON-RPC requests
// line-by-line and writing responses. It blocks until ctx is cancelled or
// stdin reaches EOF.
//
// Security: stdio is inherently local — stdin/stdout cannot be reached over
// the network. If an HTTP/SSE endpoint is added in the future, it MUST
// default to binding on 127.0.0.1 (localhost) only and accept an explicit
// flag to bind elsewhere.
func (s *MCPServer) ServeStdio(ctx context.Context) error {
	return s.Serve(ctx, os.Stdin, os.Stdout)
}

// Serve runs the MCP server reading from r and writing responses to w.
// This is the testable core of ServeStdio.
func (s *MCPServer) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB max line

	var writeMu sync.Mutex

	for {
		// Check for context cancellation before blocking on read.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !scanner.Scan() {
			// EOF or error.
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("mcp server: read error: %w", err)
			}
			return nil // clean EOF
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			resp := &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &RPCError{Code: errCodeParseError, Message: "Parse error"},
			}
			writeMu.Lock()
			_ = writeJSON(w, resp)
			writeMu.Unlock()
			continue
		}

		// Notifications (no ID) don't require a response.
		resp := s.handleRequest(ctx, &req)
		if resp == nil {
			continue
		}

		writeMu.Lock()
		if err := writeJSON(w, resp); err != nil {
			writeMu.Unlock()
			return fmt.Errorf("mcp server: write error: %w", err)
		}
		writeMu.Unlock()
	}
}

// handleRequest dispatches a JSON-RPC request to the appropriate handler.
func (s *MCPServer) handleRequest(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "initialized":
		// Notification — no response needed.
		return nil
	case "ping":
		return s.handlePing(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/read":
		return s.handleResourcesRead(req)
	case "resources/subscribe":
		return s.handleResourcesSubscribe(req)
	case "prompts/list":
		return s.handlePromptsList(req)
	case "prompts/get":
		return s.handlePromptsGet(req)
	default:
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: errCodeMethodNotFound, Message: fmt.Sprintf("Method not found: %s", req.Method)},
		}
	}
}

// handleInitialize responds to the MCP initialize handshake.
func (s *MCPServer) handleInitialize(req *JSONRPCRequest) *JSONRPCResponse {
	result := map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities": map[string]interface{}{
			"tools":     map[string]interface{}{},
			"resources": map[string]interface{}{"subscribe": true},
			"prompts":   map[string]interface{}{},
		},
		"serverInfo": s.info,
	}
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

// handlePing responds with an empty result (pong).
func (s *MCPServer) handlePing(req *JSONRPCRequest) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{},
	}
}

// handleToolsList returns all registered tool schemas.
func (s *MCPServer) handleToolsList(req *JSONRPCRequest) *JSONRPCResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type toolSchema struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		InputSchema map[string]interface{} `json:"inputSchema"`
	}

	tools := make([]toolSchema, 0, len(s.tools))
	for _, t := range s.tools {
		tools = append(tools, toolSchema{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"tools": tools},
	}
}

// handleToolsCall executes a registered tool.
func (s *MCPServer) handleToolsCall(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	// Rate limit tool calls to prevent runaway execution.
	if !s.limits.allow() {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: errCodeInternal, Message: "Rate limit exceeded: too many tool calls"},
		}
	}
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: errCodeInvalidParams, Message: "Invalid params: " + err.Error()},
		}
	}

	s.mu.RLock()
	handler, ok := s.tools[params.Name]
	s.mu.RUnlock()

	if !ok {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: errCodeMethodNotFound, Message: fmt.Sprintf("Unknown tool: %s", params.Name)},
		}
	}

	result, err := handler.Handler(ctx, params.Arguments)
	if err != nil {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": fmt.Sprintf("Error: %s", err.Error())},
				},
				"isError": true,
			},
		}
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": result},
			},
		},
	}
}

// handleResourcesList returns available resources (workspace files).
func (s *MCPServer) handleResourcesList(req *JSONRPCRequest) *JSONRPCResponse {
	resources := []map[string]interface{}{
		{
			"uri":         "hawk://workspace",
			"name":        "workspace",
			"description": "Current workspace directory listing",
			"mimeType":    "text/plain",
		},
		{
			"uri":         "hawk://session",
			"name":        "session",
			"description": "Current session messages",
			"mimeType":    "application/json",
		},
	}
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"resources": resources},
	}
}

// handleResourcesRead returns resource content.
func (s *MCPServer) handleResourcesRead(req *JSONRPCRequest) *JSONRPCResponse {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: errCodeInvalidParams, Message: "Invalid params: " + err.Error()},
		}
	}

	var content string
	switch params.URI {
	case "hawk://workspace":
		entries, err := os.ReadDir(".")
		if err != nil {
			content = "Error reading workspace: " + err.Error()
		} else {
			var names []string
			for _, e := range entries {
				if e.IsDir() {
					names = append(names, e.Name()+"/")
				} else {
					names = append(names, e.Name())
				}
			}
			content = "Workspace files:\n" + strings.Join(names, "\n")
		}
	case "hawk://session":
		content = "{}" // placeholder
	default:
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: errCodeInvalidParams, Message: fmt.Sprintf("Unknown resource: %s", params.URI)},
		}
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"contents": []map[string]interface{}{
				{"uri": params.URI, "text": content},
			},
		},
	}
}

// handleResourcesSubscribe handles resource subscription requests.
func (s *MCPServer) handleResourcesSubscribe(req *JSONRPCRequest) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{},
	}
}

// handlePromptsList returns available prompt templates.
func (s *MCPServer) handlePromptsList(req *JSONRPCRequest) *JSONRPCResponse {
	prompts := []map[string]interface{}{
		{
			"name":        "review",
			"description": "Review code changes in the workspace",
			"arguments": []map[string]interface{}{
				{"name": "scope", "description": "What to review (e.g., 'staged changes', 'last commit')", "required": false},
			},
		},
		{
			"name":        "explain",
			"description": "Explain code or a concept",
			"arguments": []map[string]interface{}{
				{"name": "target", "description": "What to explain (file path, function name, concept)", "required": true},
			},
		},
		{
			"name":        "refactor",
			"description": "Suggest refactoring improvements",
			"arguments": []map[string]interface{}{
				{"name": "file", "description": "File to refactor", "required": true},
			},
		},
	}
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"prompts": prompts},
	}
}

// handlePromptsGet returns a prompt template with arguments filled in.
func (s *MCPServer) handlePromptsGet(req *JSONRPCRequest) *JSONRPCResponse {
	var params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: errCodeInvalidParams, Message: "Invalid params: " + err.Error()},
		}
	}

	var messages []map[string]interface{}
	switch params.Name {
	case "review":
		scope := params.Arguments["scope"]
		if scope == "" {
			scope = "staged changes"
		}
		messages = append(messages, map[string]interface{}{
			"role":    "user",
			"content": map[string]interface{}{"type": "text", "text": fmt.Sprintf("Please review the %s in the workspace. Focus on correctness, security, and style.", scope)},
		})
	case "explain":
		target := params.Arguments["target"]
		messages = append(messages, map[string]interface{}{
			"role":    "user",
			"content": map[string]interface{}{"type": "text", "text": fmt.Sprintf("Explain %s in detail. Include what it does, how it works, and why it's structured that way.", target)},
		})
	case "refactor":
		file := params.Arguments["file"]
		messages = append(messages, map[string]interface{}{
			"role":    "user",
			"content": map[string]interface{}{"type": "text", "text": fmt.Sprintf("Suggest refactoring improvements for %s. Focus on readability, performance, and maintainability.", file)},
		})
	default:
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: errCodeMethodNotFound, Message: fmt.Sprintf("Unknown prompt: %s", params.Name)},
		}
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"messages": messages},
	}
}

// writeJSON marshals v as JSON and writes it to w followed by a newline.
func writeJSON(w io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}
