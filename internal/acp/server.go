// Package acp implements an Agent Client Protocol (ACP) server for hawk, exposing
// the agent over newline-delimited JSON-RPC 2.0 on stdio so editors (e.g. Zed)
// can drive it. It mirrors the framing of internal/mcp and the session-driving
// pattern of internal/daemon.
//
// Scope (first cut): the core agent-side methods (initialize, session/new,
// session/prompt, session/cancel), streamed session/update notifications, and
// client-routed tool approvals via session/request_permission. File reads/writes
// use hawk's local tools; client fs routing is intentionally out of scope.
package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// ProtocolVersion is the ACP protocol version this server implements.
const ProtocolVersion = 1

// SessionFactory creates a fresh, configured engine session for a new ACP session.
type SessionFactory func() (*engine.Session, error)

// Standard JSON-RPC 2.0 error codes.
const (
	errCodeParseError     = -32700
	errCodeInvalidRequest = -32600
	errCodeMethodNotFound = -32601
	errCodeInvalidParams  = -32602
	errCodeInternal       = -32603
)

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Server is an ACP server bound to a single stdio peer.
type Server struct {
	factory SessionFactory

	mu       sync.Mutex
	sessions map[string]*acpSession
	seq      int

	writeMu sync.Mutex
	w       io.Writer

	handlers sync.WaitGroup

	pendMu    sync.Mutex
	pending   map[int]chan rpcMessage
	nextReqID int
}

type acpSession struct {
	sess   *engine.Session
	cancel context.CancelFunc
}

// NewServer creates an ACP server that builds sessions via factory.
func NewServer(factory SessionFactory) *Server {
	return &Server{
		factory:  factory,
		sessions: make(map[string]*acpSession),
		pending:  make(map[int]chan rpcMessage),
	}
}

// ServeStdio runs the server on stdin/stdout until ctx is cancelled or EOF.
func (s *Server) ServeStdio(ctx context.Context) error {
	return s.Serve(ctx, os.Stdin, os.Stdout)
}

// Serve runs the ACP server reading from r and writing to w. Each inbound
// request is dispatched on its own goroutine so the read loop stays free to
// receive responses to server-initiated requests (e.g. permission prompts)
// while a prompt is streaming.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	s.w = w
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)

	for {
		select {
		case <-ctx.Done():
			s.handlers.Wait()
			return ctx.Err()
		default:
		}
		if !scanner.Scan() {
			s.handlers.Wait() // let in-flight handlers finish writing
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("acp: read error: %w", err)
			}
			return nil // clean EOF
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg rpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			s.writeError(nil, errCodeParseError, "parse error")
			continue
		}

		// A message with no method but with a result/error is a response to a
		// server-initiated request (e.g. session/request_permission).
		if msg.Method == "" && (msg.Result != nil || msg.Error != nil) {
			s.routeResponse(msg)
			continue
		}

		// session/prompt is long-running and may issue server->client permission
		// requests mid-stream, so it runs on its own goroutine to keep the read
		// loop free to receive those responses. Other methods are quick and run
		// inline, which also preserves request ordering (e.g. session/new before
		// a session/prompt that references it).
		if msg.Method == "session/prompt" {
			s.handlers.Add(1)
			go func(m rpcMessage) {
				defer s.handlers.Done()
				s.handle(ctx, m)
			}(msg)
		} else {
			s.handle(ctx, msg)
		}
	}
}

func (s *Server) handle(ctx context.Context, msg rpcMessage) {
	switch msg.Method {
	case "initialize":
		s.reply(msg.ID, map[string]any{
			"protocolVersion": ProtocolVersion,
			"agentCapabilities": map[string]any{
				"loadSession": false,
				"promptCapabilities": map[string]any{
					"image": false,
					"audio": false,
				},
			},
		})
	case "session/new":
		s.handleSessionNew(msg)
	case "session/prompt":
		s.handlePrompt(ctx, msg)
	case "session/cancel":
		s.handleCancel(msg)
	default:
		if len(msg.ID) > 0 {
			s.writeError(msg.ID, errCodeMethodNotFound, "method not found: "+msg.Method)
		}
	}
}

func (s *Server) handleSessionNew(msg rpcMessage) {
	sess, err := s.factory()
	if err != nil {
		s.writeError(msg.ID, errCodeInternal, "session creation failed: "+err.Error())
		return
	}

	s.mu.Lock()
	s.seq++
	id := fmt.Sprintf("sess_%d", s.seq)
	s.sessions[id] = &acpSession{sess: sess}
	s.mu.Unlock()

	// Route tool-permission prompts to the client for this session.
	sess.SetPermissionFn(s.permissionFnFor(id))

	s.reply(msg.ID, map[string]any{"sessionId": id})
}

type promptParams struct {
	SessionID string `json:"sessionId"`
	Prompt    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"prompt"`
}

func (s *Server) handlePrompt(ctx context.Context, msg rpcMessage) {
	var p promptParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		s.writeError(msg.ID, errCodeInvalidParams, "invalid params")
		return
	}
	s.mu.Lock()
	as, ok := s.sessions[p.SessionID]
	s.mu.Unlock()
	if !ok {
		s.writeError(msg.ID, errCodeInvalidParams, "unknown sessionId")
		return
	}

	var text string
	for _, b := range p.Prompt {
		if b.Type == "text" {
			text += b.Text
		}
	}

	turnCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	as.cancel = cancel
	s.mu.Unlock()
	defer cancel()

	as.sess.AddUser(text)
	events, err := as.sess.Stream(turnCtx)
	if err != nil {
		s.writeError(msg.ID, errCodeInternal, "stream failed: "+err.Error())
		return
	}

	stopReason := "end_turn"
	for ev := range events {
		switch ev.Type {
		case "content":
			s.sessionUpdate(p.SessionID, map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": ev.Content},
			})
		case "thinking":
			s.sessionUpdate(p.SessionID, map[string]any{
				"sessionUpdate": "agent_thought_chunk",
				"content":       map[string]any{"type": "text", "text": ev.Content},
			})
		case "tool_use":
			s.sessionUpdate(p.SessionID, map[string]any{
				"sessionUpdate": "tool_call",
				"toolCallId":    ev.ToolID,
				"title":         ev.ToolName,
				"status":        "in_progress",
			})
		case "tool_result":
			s.sessionUpdate(p.SessionID, map[string]any{
				"sessionUpdate": "tool_call_update",
				"toolCallId":    ev.ToolID,
				"status":        "completed",
				"content": []any{map[string]any{
					"type":    "content",
					"content": map[string]any{"type": "text", "text": ev.Content},
				}},
			})
		case "error":
			stopReason = "refusal"
			s.sessionUpdate(p.SessionID, map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": ev.Content},
			})
		case "done":
			// handled by channel close
		}
	}
	if turnCtx.Err() != nil {
		stopReason = "cancelled"
	}

	s.reply(msg.ID, map[string]any{"stopReason": stopReason})
}

type cancelParams struct {
	SessionID string `json:"sessionId"`
}

func (s *Server) handleCancel(msg rpcMessage) {
	var p cancelParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		return // notification: no response
	}
	s.mu.Lock()
	if as, ok := s.sessions[p.SessionID]; ok && as.cancel != nil {
		as.cancel()
	}
	s.mu.Unlock()
}

// permissionFnFor returns a PermissionFn that asks the ACP client to approve a
// tool call via session/request_permission, falling back to denial on timeout.
func (s *Server) permissionFnFor(sessionID string) func(engine.PermissionRequest) {
	return func(req engine.PermissionRequest) {
		params := map[string]any{
			"sessionId": sessionID,
			"toolCall": map[string]any{
				"toolCallId": req.ToolID,
				"title":      req.Summary,
			},
			"options": []any{
				map[string]any{"optionId": "allow", "name": "Allow", "kind": "allow_once"},
				map[string]any{"optionId": "reject", "name": "Reject", "kind": "reject_once"},
			},
		}
		allowed := s.requestPermission(params)
		if req.Response != nil {
			req.Response <- allowed
		}
	}
}

type permissionResult struct {
	Outcome struct {
		Outcome  string `json:"outcome"`
		OptionID string `json:"optionId"`
	} `json:"outcome"`
}

// requestPermission issues a server->client session/request_permission request
// and blocks for the reply. Returns true only on an explicit "allow" selection.
func (s *Server) requestPermission(params map[string]any) bool {
	resp, ok := s.call("session/request_permission", params, 5*time.Minute)
	if !ok || resp.Error != nil || resp.Result == nil {
		return false
	}
	var pr permissionResult
	if err := json.Unmarshal(resp.Result, &pr); err != nil {
		return false
	}
	return pr.Outcome.Outcome == "selected" && pr.Outcome.OptionID == "allow"
}

// call sends a server-initiated JSON-RPC request and waits for the response.
func (s *Server) call(method string, params any, timeout time.Duration) (rpcMessage, bool) {
	s.pendMu.Lock()
	s.nextReqID++
	id := s.nextReqID
	ch := make(chan rpcMessage, 1)
	s.pending[id] = ch
	s.pendMu.Unlock()

	defer func() {
		s.pendMu.Lock()
		delete(s.pending, id)
		s.pendMu.Unlock()
	}()

	rawID, _ := json.Marshal(id)
	s.writeMessage(rpcMessage{JSONRPC: "2.0", ID: rawID, Method: method, Params: mustRaw(params)})

	select {
	case resp := <-ch:
		return resp, true
	case <-time.After(timeout):
		return rpcMessage{}, false
	}
}

func (s *Server) routeResponse(msg rpcMessage) {
	var id int
	if err := json.Unmarshal(msg.ID, &id); err != nil {
		return
	}
	s.pendMu.Lock()
	ch, ok := s.pending[id]
	s.pendMu.Unlock()
	if ok {
		ch <- msg
	}
}

// ---- JSON-RPC write helpers ----

func (s *Server) reply(id json.RawMessage, result any) {
	if len(id) == 0 {
		return // notification: no reply expected
	}
	s.writeMessage(rpcMessage{JSONRPC: "2.0", ID: id, Result: mustRaw(result)})
}

func (s *Server) writeError(id json.RawMessage, code int, message string) {
	s.writeMessage(rpcMessage{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

func (s *Server) sessionUpdate(sessionID string, update map[string]any) {
	s.writeMessage(rpcMessage{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params:  mustRaw(map[string]any{"sessionId": sessionID, "update": update}),
	})
}

func (s *Server) writeMessage(m rpcMessage) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, _ = s.w.Write(append(b, '\n'))
}

func mustRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}
