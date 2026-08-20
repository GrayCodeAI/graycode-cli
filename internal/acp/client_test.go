package acp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// mockSession creates a minimal engine session for testing.
func mockSession() (*engine.Session, error) {
	sess := engine.NewSession("mock", "mock-model", "system prompt", nil)
	return sess, nil
}

func setupClientServer(t *testing.T, opts ...ClientOption) (*Client, *Server, func()) {
	t.Helper()
	serverR, clientW := io.Pipe()
	clientR, serverW := io.Pipe()

	server := NewServer(mockSession)
	ctx, cancel := context.WithCancel(context.Background())

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		_ = server.Serve(ctx, serverR, serverW)
	}()

	client, err := Connect(ctx, clientR, clientW, opts...)
	if err != nil {
		cancel()
		t.Fatalf("failed to connect ACP client: %v", err)
	}

	cleanup := func() {
		_ = client.Close()
		cancel()
		_ = clientW.Close()
		_ = serverW.Close()
		_ = clientR.Close()
		_ = serverR.Close()
		<-serverDone
	}

	return client, server, cleanup
}

func TestClientServer_InitializeAndNewSession(t *testing.T) {
	client, _, cleanup := setupClientServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create session
	sessID, err := client.NewSession(ctx, "/test/workspace")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	if sessID == "" {
		t.Fatal("expected non-empty session ID")
	}
}

func TestClientServer_PromptAndCancel(t *testing.T) {
	client, _, cleanup := setupClientServer(t)
	defer cleanup()

	ctx := context.Background()

	sessID, err := client.NewSession(ctx, "/test/workspace")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	// Cancel session
	if err := client.Cancel(ctx, sessID); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
}

func TestClientServer_PermissionRequest(t *testing.T) {
	permApproved := false
	var receivedReq PermissionRequest

	client, server, cleanup := setupClientServer(t, WithOnPermissionRequest(func(req PermissionRequest) (bool, error) {
		permApproved = true
		receivedReq = req
		return true, nil
	}))
	defer cleanup()

	ctx := context.Background()
	sessID, err := client.NewSession(ctx, "/test/workspace")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	// Server-initiated request_permission to client
	as := server.lookupSession(sessID)
	if as == nil {
		t.Fatalf("session %s not found on server", sessID)
	}

	// Trigger permission request through server
	reqPermPayload := map[string]any{
		"sessionId": sessID,
		"toolName":  "FileWrite",
		"summary":   "Write main.go",
	}
	resp, ok := server.call("session/request_permission", reqPermPayload, 5*time.Second)
	if !ok {
		t.Fatal("server.call timed out or failed")
	}

	var permResult struct {
		Outcome struct {
			Outcome  string `json:"outcome"`
			OptionID string `json:"optionId"`
		} `json:"outcome"`
	}
	_ = json.Unmarshal(resp.Result, &permResult)

	if !permApproved || permResult.Outcome.OptionID != "allow" {
		t.Fatalf("expected permission to be approved by client callback, got outcome=%#v", permResult.Outcome)
	}
	if receivedReq.ToolName != "FileWrite" {
		t.Errorf("expected toolName FileWrite, got %s", receivedReq.ToolName)
	}
}

func TestClient_CloseIdempotence(t *testing.T) {
	client, _, cleanup := setupClientServer(t)
	defer cleanup()

	if err := client.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}

	// Operations after Close return ErrClientClosed
	_, err := client.NewSession(context.Background(), "/test")
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("expected client closed error, got: %v", err)
	}
}

func TestClient_HandshakeFailureTimeout(t *testing.T) {
	// A broken pipe that provides no response
	r, w := io.Pipe()
	defer func() { _ = r.Close(); _ = w.Close() }()

	ctx := context.Background()
	_, err := Connect(ctx, r, w, WithTimeout(50*time.Millisecond))
	if err == nil {
		t.Fatal("expected handshake timeout error, got nil")
	}
}
