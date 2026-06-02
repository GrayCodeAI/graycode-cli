package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/testutil"
)

func startTestDaemon(t *testing.T, srv *Server) string {
	t.Helper()
	addr, err := srv.Start()
	if err != nil {
		testutil.SkipIfLoopbackUnavailable(t, err)
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for the server to be ready to accept connections.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("daemon at %s not ready after 2s", addr)
	return addr
}

func TestDaemon_StartStop(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil) // port 0 = random free port
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	if addr == "" {
		t.Error("expected non-empty address")
	}
}

func TestDaemon_Health(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, err := http.Get("http://" + addr + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var health HealthResponse
	json.NewDecoder(resp.Body).Decode(&health)
	if health.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", health.Status)
	}
	if health.Version == "" {
		t.Error("expected non-empty version")
	}
}

func TestDaemon_Chat_NoEngine(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	body, _ := json.Marshal(ChatRequest{Prompt: "hello"})
	resp, err := http.Post("http://"+addr+"/v1/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/chat failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 503 {
		t.Errorf("expected 503 with nil factory, got %d", resp.StatusCode)
	}
}

func TestDaemon_ProtectedEndpointsRequireAPIKey(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost, APIKey: "secret"}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	body, _ := json.Marshal(ChatRequest{Prompt: "hello"})
	resp, err := http.Post("http://"+addr+"/v1/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/chat failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without key, got %d", resp.StatusCode)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://"+addr+"/v1/chat", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "secret")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/chat with key failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected authenticated request to reach handler, got %d", resp.StatusCode)
	}
}

func TestDaemon_RejectsOversizedBody(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	body := []byte(`{"prompt":"` + strings.Repeat("x", maxRequestBodyBytes+1) + `"}`)
	resp, err := http.Post("http://"+addr+"/v1/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/chat failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized body, got %d", resp.StatusCode)
	}
}

func TestDaemon_RejectsUnknownFields(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, err := http.Post("http://"+addr+"/v1/chat", "application/json", bytes.NewReader([]byte(`{"prompt":"hello","unknown":true}`)))
	if err != nil {
		t.Fatalf("POST /v1/chat failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d", resp.StatusCode)
	}
}

func TestDaemon_Chat_WithEngine(t *testing.T) {
	factory := func(req ChatRequest) (*engine.Session, error) {
		sess := engine.NewSession("", "test-model", "you are helpful", nil)
		sess.MaxTurns = 1
		return sess, nil
	}
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, factory)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	body, _ := json.Marshal(ChatRequest{Prompt: "hello", MaxTurns: 1})
	resp, err := http.Post("http://"+addr+"/v1/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/chat failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDaemon_Chat_EmptyPrompt(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	body, _ := json.Marshal(ChatRequest{})
	resp, err := http.Post("http://"+addr+"/v1/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/chat failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for empty prompt, got %d", resp.StatusCode)
	}
}

func TestDaemon_Sessions(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, err := http.Get("http://" + addr + "/v1/sessions")
	if err != nil {
		t.Fatalf("GET /v1/sessions failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDaemon_GracefulShutdown(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	_ = startTestDaemon(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Port != 4590 {
		t.Errorf("DefaultConfig().Port = %d, want 4590", cfg.Port)
	}
	if cfg.Host != testutil.LoopbackHost {
		t.Errorf("DefaultConfig().Host = %q, want %q", cfg.Host, testutil.LoopbackHost)
	}
}

func TestDaemon_Stats(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, err := http.Get("http://" + addr + "/v1/stats")
	if err != nil {
		t.Fatalf("GET /v1/stats failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDaemon_InvalidMethod(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	req, _ := http.NewRequest("DELETE", "http://"+addr+"/v1/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /v1/health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		t.Error("DELETE on health endpoint should not return 200")
	}
}

func TestDaemon_InvalidJSON(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, err := http.Post("http://"+addr+"/v1/chat", "application/json", bytes.NewReader([]byte("not json")))
	if err != nil {
		t.Fatalf("POST /v1/chat failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestDaemon_GetSession_MissingID(t *testing.T) {
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, err := http.Get("http://" + addr + "/v1/sessions/nonexistent-id")
	if err != nil {
		t.Fatalf("GET /v1/sessions/x failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Errorf("expected 404 for nonexistent session, got %d", resp.StatusCode)
	}
}

func TestChatRequest_JSON(t *testing.T) {
	req := ChatRequest{
		Prompt:    "test prompt",
		SessionID: "sess-123",
		Model:     "claude-sonnet",
		MaxTurns:  5,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded ChatRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Prompt != req.Prompt {
		t.Errorf("Prompt = %q, want %q", decoded.Prompt, req.Prompt)
	}
	if decoded.MaxTurns != req.MaxTurns {
		t.Errorf("MaxTurns = %d, want %d", decoded.MaxTurns, req.MaxTurns)
	}
}

func TestErrorResponse_JSON(t *testing.T) {
	resp := ErrorResponse{
		Error:   "something failed",
		Code:    "internal_error",
		Details: "stack trace here",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded ErrorResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Error != resp.Error {
		t.Errorf("Error = %q, want %q", decoded.Error, resp.Error)
	}
	if decoded.Code != resp.Code {
		t.Errorf("Code = %q, want %q", decoded.Code, resp.Code)
	}
}
