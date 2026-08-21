package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// ErrClientClosed is returned when an operation is attempted on a closed ACP client.
var ErrClientClosed = errors.New("acp: client is closed")

// PermissionRequest represents a server-initiated tool execution approval request.
type PermissionRequest struct {
	SessionID string          `json:"sessionId"`
	ToolName  string          `json:"toolName"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Summary   string          `json:"summary,omitempty"`
}

// PromptResult represents the outcome of an ACP session/prompt execution.
type PromptResult struct {
	Status string `json:"status"`
	Output string `json:"output,omitempty"`
}

// ClientOptions configures the ACP client.
type ClientOptions struct {
	Timeout             time.Duration
	Env                 []string
	OnUpdate            func(sessionID string, update json.RawMessage)
	OnPermissionRequest func(req PermissionRequest) (bool, error)
}

// ClientOption is a functional option for configuring an ACP client.
type ClientOption func(*ClientOptions)

// WithTimeout sets the initialize handshake timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(o *ClientOptions) {
		o.Timeout = d
	}
}

// WithEnv sets additional environment variables for the child process.
func WithEnv(env []string) ClientOption {
	return func(o *ClientOptions) {
		o.Env = env
	}
}

// WithOnUpdate sets a callback for server-streamed session/update notifications.
func WithOnUpdate(fn func(sessionID string, update json.RawMessage)) ClientOption {
	return func(o *ClientOptions) {
		o.OnUpdate = fn
	}
}

// WithOnPermissionRequest sets a callback for handling server-initiated tool permissions.
func WithOnPermissionRequest(fn func(req PermissionRequest) (bool, error)) ClientOption {
	return func(o *ClientOptions) {
		o.OnPermissionRequest = fn
	}
}

// Client communicates with an Agent Client Protocol (ACP) peer.
type Client struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	w         io.Writer
	writeMu   sync.Mutex
	pendMu    sync.Mutex
	pending   map[string]chan rpcMessage
	nextReqID int

	opts     ClientOptions
	closed   bool
	closeMu  sync.Mutex
	handlers sync.WaitGroup
}

// Start launches an external ACP process and performs the initialize handshake.
func Start(ctx context.Context, command string, args []string, opts ...ClientOption) (*Client, error) {
	var opt ClientOptions
	opt.Timeout = 10 * time.Second
	for _, o := range opts {
		o(&opt)
	}

	cmd := exec.Command(command, args...)
	setCmdProcessGroup(cmd)
	if len(opt.Env) > 0 {
		cmd.Env = append(cmd.Environ(), opt.Env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("acp: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("acp: start command: %w", err)
	}

	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		w:       stdin,
		pending: make(map[string]chan rpcMessage),
		opts:    opt,
	}

	// Start read loop
	c.handlers.Add(1)
	go func() {
		defer c.handlers.Done()
		c.readLoop(stdout)
	}()

	// Initialize handshake with timeout
	initCtx, cancel := context.WithTimeout(ctx, opt.Timeout)
	defer cancel()

	if err := c.initialize(initCtx); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("acp: handshake failed: %w", err)
	}

	return c, nil
}

// Connect wraps an existing io.Reader/io.Writer pair as an ACP client without spawning a process.
func Connect(ctx context.Context, r io.Reader, w io.Writer, opts ...ClientOption) (*Client, error) {
	var opt ClientOptions
	opt.Timeout = 10 * time.Second
	for _, o := range opts {
		o(&opt)
	}

	c := &Client{
		w:       w,
		pending: make(map[string]chan rpcMessage),
		opts:    opt,
	}

	c.handlers.Add(1)
	go func() {
		defer c.handlers.Done()
		c.readLoop(r)
	}()

	initCtx, cancel := context.WithTimeout(ctx, opt.Timeout)
	defer cancel()

	if err := c.initialize(initCtx); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("acp: handshake failed: %w", err)
	}

	return c, nil
}

func (c *Client) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": ProtocolVersion,
		"clientCapabilities": map[string]any{
			"tools": map[string]any{
				"approval": true,
			},
		},
	}
	res, err := c.call(ctx, "initialize", params)
	if err != nil {
		return err
	}
	if res.Error != nil {
		return fmt.Errorf("rpc error (%d): %s", res.Error.Code, res.Error.Message)
	}
	return nil
}

// NewSession creates a new remote ACP session.
func (c *Client) NewSession(ctx context.Context, cwd string) (string, error) {
	params := map[string]any{
		"cwd": cwd,
	}
	res, err := c.call(ctx, "session/new", params)
	if err != nil {
		return "", err
	}
	if res.Error != nil {
		return "", fmt.Errorf("rpc error (%d): %s", res.Error.Code, res.Error.Message)
	}

	var out struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(res.Result, &out); err != nil {
		return "", fmt.Errorf("unmarshal session/new result: %w", err)
	}
	if out.SessionID == "" {
		return "", errors.New("empty sessionId in session/new response")
	}
	return out.SessionID, nil
}

// LoadSessionResult contains the response from loading a persisted session.
type LoadSessionResult struct {
	SessionID    string `json:"sessionId"`
	Model        string `json:"model,omitempty"`
	MessageCount int    `json:"messageCount"`
	Status       string `json:"status"`
}

// SessionSummary represents a session entry returned by ListSessions.
type SessionSummary struct {
	ID        string `json:"id"`
	Preview   string `json:"preview,omitempty"`
	CWD       string `json:"cwd,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// LoadSession opens an existing persisted session on the ACP server.
func (c *Client) LoadSession(ctx context.Context, sessionID string) (*LoadSessionResult, error) {
	params := map[string]any{
		"sessionId": sessionID,
	}
	res, err := c.call(ctx, "session/load", params)
	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, fmt.Errorf("rpc error (%d): %s", res.Error.Code, res.Error.Message)
	}

	var out LoadSessionResult
	if err := json.Unmarshal(res.Result, &out); err != nil {
		return nil, fmt.Errorf("unmarshal session/load result: %w", err)
	}
	return &out, nil
}

// ListSessions queries the ACP server for available sessions.
func (c *Client) ListSessions(ctx context.Context) ([]SessionSummary, error) {
	res, err := c.call(ctx, "session/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, fmt.Errorf("rpc error (%d): %s", res.Error.Code, res.Error.Message)
	}

	var out struct {
		Sessions []SessionSummary `json:"sessions"`
	}
	if err := json.Unmarshal(res.Result, &out); err != nil {
		return nil, fmt.Errorf("unmarshal session/list result: %w", err)
	}
	return out.Sessions, nil
}

// Prompt submits a prompt to an active ACP session and awaits the response.
func (c *Client) Prompt(ctx context.Context, sessionID, prompt string) (*PromptResult, error) {
	params := map[string]any{
		"sessionId": sessionID,
		"prompt":    prompt,
	}
	res, err := c.call(ctx, "session/prompt", params)
	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, fmt.Errorf("rpc error (%d): %s", res.Error.Code, res.Error.Message)
	}

	var out PromptResult
	if len(res.Result) > 0 {
		_ = json.Unmarshal(res.Result, &out)
	}
	return &out, nil
}

// Cancel requests cancellation of an in-flight prompt via session/cancel notification.
func (c *Client) Cancel(_ context.Context, sessionID string) error {
	params := map[string]any{
		"sessionId": sessionID,
	}
	return c.notify("session/cancel", params)
}

func (c *Client) notify(method string, params any) error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return ErrClientClosed
	}
	c.closeMu.Unlock()

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}

	msg := rpcMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsBytes,
	}
	return c.send(msg)
}

func (c *Client) call(ctx context.Context, method string, params any) (rpcMessage, error) {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return rpcMessage{}, ErrClientClosed
	}
	c.closeMu.Unlock()

	c.pendMu.Lock()
	c.nextReqID++
	reqID := strconv.Itoa(c.nextReqID)
	respCh := make(chan rpcMessage, 1)
	c.pending[reqID] = respCh
	c.pendMu.Unlock()

	defer func() {
		c.pendMu.Lock()
		delete(c.pending, reqID)
		c.pendMu.Unlock()
	}()

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return rpcMessage{}, fmt.Errorf("marshal params: %w", err)
	}

	rawID := json.RawMessage(strconv.Quote(reqID))
	msg := rpcMessage{
		JSONRPC: "2.0",
		ID:      rawID,
		Method:  method,
		Params:  paramsBytes,
	}

	if err := c.send(msg); err != nil {
		return rpcMessage{}, err
	}

	select {
	case <-ctx.Done():
		return rpcMessage{}, ctx.Err()
	case resp, ok := <-respCh:
		if !ok {
			return rpcMessage{}, ErrClientClosed
		}
		return resp, nil
	}
}

func (c *Client) send(msg rpcMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal rpcMessage: %w", err)
	}
	data = append(data, '\n')

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.w == nil {
		return ErrClientClosed
	}
	_, err = c.w.Write(data)
	return err
}

func (c *Client) readLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg rpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		// 1. Response to a client-initiated request
		if len(msg.ID) > 0 && msg.Method == "" {
			var idStr string
			if err := json.Unmarshal(msg.ID, &idStr); err != nil {
				// Might be numeric
				var idInt int
				if errInt := json.Unmarshal(msg.ID, &idInt); errInt == nil {
					idStr = strconv.Itoa(idInt)
				}
			}

			c.pendMu.Lock()
			if ch, ok := c.pending[idStr]; ok {
				select {
				case ch <- msg:
				default:
				}
			}
			c.pendMu.Unlock()
			continue
		}

		// 2. Server-initiated notification or request
		switch msg.Method {
		case "session/update":
			if c.opts.OnUpdate != nil {
				var p struct {
					SessionID string          `json:"sessionId"`
					Update    json.RawMessage `json:"update"`
				}
				if err := json.Unmarshal(msg.Params, &p); err == nil {
					c.opts.OnUpdate(p.SessionID, p.Update)
				}
			}

		case "session/request_permission":
			c.handlers.Add(1)
			go func(req rpcMessage) {
				defer c.handlers.Done()
				c.handlePermissionRequest(req)
			}(msg)
		}
	}

	// EOF / error: close pending requests
	c.closeMu.Lock()
	c.closed = true
	c.closeMu.Unlock()

	c.pendMu.Lock()
	for _, ch := range c.pending {
		close(ch)
	}
	c.pending = make(map[string]chan rpcMessage)
	c.pendMu.Unlock()
}

func (c *Client) handlePermissionRequest(msg rpcMessage) {
	var p PermissionRequest
	_ = json.Unmarshal(msg.Params, &p)

	allowed := true
	if c.opts.OnPermissionRequest != nil {
		var err error
		allowed, err = c.opts.OnPermissionRequest(p)
		if err != nil {
			allowed = false
		}
	}

	optionID := "deny"
	if allowed {
		optionID = "allow"
	}
	resPayload, _ := json.Marshal(map[string]any{
		"outcome": map[string]string{
			"outcome":  "selected",
			"optionId": optionID,
		},
	})

	_ = c.send(rpcMessage{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result:  resPayload,
	})
}

// Close disposes the client, closing streams and terminating the child process tree if started.
func (c *Client) Close() error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return nil
	}
	c.closed = true
	c.closeMu.Unlock()

	if c.stdin != nil {
		_ = c.stdin.Close()
	}

	if c.cmd != nil && c.cmd.Process != nil {
		_ = killProcessGroup(c.cmd.Process)
		_ = c.cmd.Wait()
	}

	c.pendMu.Lock()
	for _, ch := range c.pending {
		close(ch)
	}
	c.pending = make(map[string]chan rpcMessage)
	c.pendMu.Unlock()

	return nil
}
