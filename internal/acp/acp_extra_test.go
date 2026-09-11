package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// --- Error factory for testing ---

func errorFactory() (*engine.Session, error) {
	return nil, errors.New("factory error")
}

// --- handleSessionNew tests ---

func TestHandleSessionNew_FactoryError(t *testing.T) {
	srv := NewServer(errorFactory)
	var buf bytes.Buffer
	srv.w = &buf

	msg := rpcMessage{
		ID:     []byte(`1`),
		Method: "session/new",
	}
	srv.handleSessionNew(msg)

	// Should write an error response
	if buf.Len() == 0 {
		t.Fatal("expected error response to be written")
	}
	var resp rpcMessage
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error in response")
	}
	if resp.Error.Code != errCodeInternal {
		t.Errorf("error code = %d, want %d", resp.Error.Code, errCodeInternal)
	}
}

// --- handleSetMode / setIsolation / status tests ---

func newSrvWithSession(t *testing.T, id string) (*Server, *bytes.Buffer) {
	t.Helper()
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf
	sess, err := testFactory()
	if err != nil {
		t.Fatalf("testFactory: %v", err)
	}
	srv.mu.Lock()
	srv.sessions[id] = &acpSession{sess: sess}
	srv.mu.Unlock()
	return srv, &buf
}

func TestHandleSetMode_Plan(t *testing.T) {
	srv, buf := newSrvWithSession(t, "s1")
	msg := rpcMessage{
		ID: []byte(`1`), Method: "session/setMode",
		Params: []byte(`{"sessionId":"s1","mode":"plan"}`),
	}
	srv.handleSetMode(msg)
	var resp struct {
		Result struct {
			SessionID string `json:"sessionId"`
			WorkMode  string `json:"workMode"`
		} `json:"result"`
	}
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v (%s)", err, buf.String())
	}
	if resp.Result.WorkMode != "plan" {
		t.Fatalf("workMode = %q, want plan", resp.Result.WorkMode)
	}
}

func TestHandleSetMode_InvalidMode(t *testing.T) {
	srv, buf := newSrvWithSession(t, "s1")
	msg := rpcMessage{
		ID: []byte(`1`), Method: "session/setMode",
		Params: []byte(`{"sessionId":"s1","mode":"banana"}`),
	}
	srv.handleSetMode(msg)
	var resp rpcMessage
	_ = json.Unmarshal(buf.Bytes(), &resp)
	if resp.Error == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestHandleSetMode_UnknownSession(t *testing.T) {
	srv, buf := newSrvWithSession(t, "s1")
	msg := rpcMessage{
		ID: []byte(`1`), Method: "session/setMode",
		Params: []byte(`{"sessionId":"nope","mode":"plan"}`),
	}
	srv.handleSetMode(msg)
	var resp rpcMessage
	_ = json.Unmarshal(buf.Bytes(), &resp)
	if resp.Error == nil {
		t.Fatal("expected error for unknown session")
	}
}

func TestHandleSetIsolation_Workspace(t *testing.T) {
	srv, buf := newSrvWithSession(t, "s1")
	msg := rpcMessage{
		ID: []byte(`1`), Method: "session/setIsolation",
		Params: []byte(`{"sessionId":"s1","profile":"workspace"}`),
	}
	srv.handleSetIsolation(msg)
	var resp struct {
		Result struct {
			Isolation string `json:"isolation"`
		} `json:"result"`
	}
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v (%s)", err, buf.String())
	}
	if resp.Result.Isolation != "workspace" {
		t.Fatalf("isolation = %q, want workspace", resp.Result.Isolation)
	}
}

func TestHandleStatus_Snapshot(t *testing.T) {
	srv, buf := newSrvWithSession(t, "s1")
	msg := rpcMessage{
		ID: []byte(`1`), Method: "session/status",
		Params: []byte(`{"sessionId":"s1"}`),
	}
	srv.handleStatus(msg)
	var resp struct {
		Result struct {
			WorkMode   string `json:"workMode"`
			Isolation  string `json:"isolation"`
			AutoCommit bool   `json:"autoCommit"`
		} `json:"result"`
	}
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v (%s)", err, buf.String())
	}
	if resp.Result.WorkMode == "" {
		t.Fatal("expected non-empty workMode in status")
	}
}

// --- handleCancel tests ---

func TestHandleCancel_ValidSession(t *testing.T) {
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	// Create a session first
	sess, _ := testFactory()
	srv.mu.Lock()
	srv.sessions["test-sess"] = &acpSession{sess: sess}
	srv.mu.Unlock()

	msg := rpcMessage{
		ID:     []byte(`1`),
		Method: "session/cancel",
		Params: []byte(`{"sessionId":"test-sess"}`),
	}
	srv.handleCancel(msg)
	// Cancel is a notification, no response expected
}

func TestHandleCancel_UnknownSession(t *testing.T) {
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	msg := rpcMessage{
		ID:     []byte(`1`),
		Method: "session/cancel",
		Params: []byte(`{"sessionId":"unknown"}`),
	}
	srv.handleCancel(msg)
	// Should not crash, no response for unknown session
}

func TestHandleCancel_InvalidParams(t *testing.T) {
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	msg := rpcMessage{
		ID:     []byte(`1`),
		Method: "session/cancel",
		Params: []byte(`{invalid json`),
	}
	srv.handleCancel(msg)
	// Should not crash on invalid params
}

// --- handlePrompt tests ---

func TestHandlePrompt_InvalidParams(t *testing.T) {
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	msg := rpcMessage{
		ID:     []byte(`1`),
		Method: "session/prompt",
		Params: []byte(`{invalid json`),
	}
	srv.handlePrompt(context.Background(), msg)

	if buf.Len() == 0 {
		t.Fatal("expected error response to be written")
	}
	var resp rpcMessage
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error in response")
	}
	if resp.Error.Code != errCodeInvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, errCodeInvalidParams)
	}
}

func TestHandlePrompt_UnknownSession(t *testing.T) {
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	msg := rpcMessage{
		ID:     []byte(`1`),
		Method: "session/prompt",
		Params: []byte(`{"sessionId":"unknown","prompt":[{"type":"text","text":"hi"}]}`),
	}
	srv.handlePrompt(context.Background(), msg)

	if buf.Len() == 0 {
		t.Fatal("expected error response to be written")
	}
	var resp rpcMessage
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error in response")
	}
}

// --- permissionFnFor tests ---

// Note: We can't easily test permissionFnFor timeout because it blocks for 5 minutes
// waiting for a client response. This is by design - it's a blocking RPC call.
// The existing integration tests cover the happy path.

// --- routeResponse tests ---

func TestRouteResponse_UnknownID(t *testing.T) {
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	// Route a response for an unknown request ID
	msg := rpcMessage{
		ID:     []byte(`999`),
		Result: []byte(`{"result":"ok"}`),
	}
	srv.routeResponse(msg)
	// Should not crash
}

func TestRouteResponse_InvalidID(t *testing.T) {
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	// Route a response with invalid ID
	msg := rpcMessage{
		ID:     []byte(`not-a-number`),
		Result: []byte(`{"result":"ok"}`),
	}
	srv.routeResponse(msg)
	// Should not crash
}

// --- writeMessage tests ---

func TestWriteMessage_ValidMessage(t *testing.T) {
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	msg := rpcMessage{
		JSONRPC: "2.0",
		ID:      []byte(`1`),
		Result:  []byte(`{"test":"data"}`),
	}
	srv.writeMessage(msg)

	if buf.Len() == 0 {
		t.Fatal("expected message to be written")
	}
	// Should end with newline
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Error("expected message to end with newline")
	}
}

// --- mustRaw tests ---

func TestMustRaw_ValidValue(t *testing.T) {
	result := mustRaw(map[string]any{"key": "value"})
	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if parsed["key"] != "value" {
		t.Errorf("expected key=value, got %v", parsed["key"])
	}
}

func TestMustRaw_NilValue(t *testing.T) {
	result := mustRaw(nil)
	// Should return "null" for nil
	if string(result) != "null" {
		t.Errorf("expected 'null', got %q", string(result))
	}
}

// --- reply tests ---

func TestReply_WithValidID(t *testing.T) {
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	srv.reply([]byte(`1`), map[string]any{"result": "ok"})
	if buf.Len() == 0 {
		t.Fatal("expected reply to be written")
	}
}

func TestReply_WithEmptyID(t *testing.T) {
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	// Empty ID means notification, no reply expected
	srv.reply([]byte(``), map[string]any{"result": "ok"})
	if buf.Len() != 0 {
		t.Error("expected no reply for empty ID")
	}
}

// --- writeError tests ---

func TestWriteError_ValidError(t *testing.T) {
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	srv.writeError([]byte(`1`), errCodeMethodNotFound, "method not found")
	if buf.Len() == 0 {
		t.Fatal("expected error to be written")
	}
	var resp rpcMessage
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error in response")
	}
	if resp.Error.Code != errCodeMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, errCodeMethodNotFound)
	}
}

// --- sessionUpdate tests ---

func TestSessionUpdate_ValidUpdate(t *testing.T) {
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	srv.sessionUpdate("test-session", map[string]any{
		"sessionUpdate": "agent_message_chunk",
		"content":       map[string]any{"type": "text", "text": "hello"},
	})
	if buf.Len() == 0 {
		t.Fatal("expected session update to be written")
	}
	var msg rpcMessage
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}
	if msg.Method != "session/update" {
		t.Errorf("method = %q, want %q", msg.Method, "session/update")
	}
}

// --- Serve tests ---

func TestServe_CancelledContext(t *testing.T) {
	srv := NewServer(testFactory)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Serve with cancelled context should return context error
	err := srv.Serve(ctx, strings.NewReader(""), io.Discard)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestServe_EmptyInput(t *testing.T) {
	srv := NewServer(testFactory)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Empty input should return nil (clean EOF)
	err := srv.Serve(ctx, strings.NewReader(""), io.Discard)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestServe_InvalidJSON(t *testing.T) {
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Invalid JSON should write parse error
	err := srv.Serve(ctx, strings.NewReader("{invalid\n"), &buf)
	// May or may not error depending on timing, but should write parse error
	if buf.Len() == 0 && err == nil {
		t.Error("expected parse error to be written")
	}
}

func TestACP_FullSessionLifecycle(t *testing.T) {
	lines := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"jsonrpc":"2.0","id":2,"method":"session/new"}`,
		`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"sess_1","prompt":[{"type":"text","text":"hello"}]}}`,
		`{"jsonrpc":"2.0","id":4,"method":"session/cancel","params":{"sessionId":"sess_1"}}`,
	}
	msgs := runServer(t, testFactory, lines)

	// Should have responses for all requests
	ids := make(map[int]bool)
	for _, m := range msgs {
		if len(m.ID) > 0 && m.Method == "" {
			var id int
			if err := json.Unmarshal(m.ID, &id); err == nil {
				ids[id] = true
			}
		}
	}
	for _, expected := range []int{1, 2, 3} {
		if !ids[expected] {
			t.Errorf("missing response for id %d", expected)
		}
	}
}

func TestACP_MultipleSessions(t *testing.T) {
	// Create a single server for both sessions
	srv := NewServer(testFactory)
	var buf bytes.Buffer
	srv.w = &buf

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}
{"jsonrpc":"2.0","id":2,"method":"session/new"}
{"jsonrpc":"2.0","id":3,"method":"session/new"}
`)
	pr, pw := io.Pipe()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = srv.Serve(ctx, in, pw)
		_ = pw.Close()
	}()

	var msgs []rpcMessage
	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var m rpcMessage
		if err := json.Unmarshal(line, &m); err == nil {
			msgs = append(msgs, m)
		}
	}
	wg.Wait()

	sessionIDs := make(map[string]bool)
	for _, m := range msgs {
		if hasID(m, 2) || hasID(m, 3) {
			var r struct {
				SessionID string `json:"sessionId"`
			}
			if err := json.Unmarshal(m.Result, &r); err == nil && r.SessionID != "" {
				sessionIDs[r.SessionID] = true
			}
		}
	}

	if len(sessionIDs) != 2 {
		t.Errorf("expected 2 unique session IDs, got %d", len(sessionIDs))
	}
}

func TestACP_EmptyPrompt(t *testing.T) {
	lines := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"jsonrpc":"2.0","id":2,"method":"session/new"}`,
		`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"sess_1","prompt":[]}}`,
	}
	msgs := runServer(t, testFactory, lines)

	var gotPromptResult bool
	for _, m := range msgs {
		if hasID(m, 3) {
			gotPromptResult = true
		}
	}
	if !gotPromptResult {
		t.Error("expected prompt result even with empty prompt")
	}
}

func TestACP_MixedPromptTypes(t *testing.T) {
	lines := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"jsonrpc":"2.0","id":2,"method":"session/new"}`,
		`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"sess_1","prompt":[{"type":"text","text":"hello"},{"type":"image","data":"base64data"}]}}`,
	}
	msgs := runServer(t, testFactory, lines)

	var gotPromptResult bool
	for _, m := range msgs {
		if hasID(m, 3) {
			gotPromptResult = true
		}
	}
	if !gotPromptResult {
		t.Error("expected prompt result with mixed prompt types")
	}
}
