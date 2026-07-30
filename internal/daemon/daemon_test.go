package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/storage"
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

func TestDaemon_Ready(t *testing.T) {
	factory := func(req ChatRequest) (*engine.Session, error) { return nil, nil }

	tests := []struct {
		name       string
		factory    SessionFactory
		readyFn    func() (bool, string)
		wantStatus int
		wantReady  bool
	}{
		{
			name:       "no engine wired is not ready",
			factory:    nil,
			wantStatus: http.StatusServiceUnavailable,
			wantReady:  false,
		},
		{
			name:       "engine wired without Eyrie probe is not ready",
			factory:    factory,
			wantStatus: http.StatusServiceUnavailable,
			wantReady:  false,
		},
		{
			name:       "custom probe forces not ready",
			factory:    factory,
			readyFn:    func() (bool, string) { return false, "provider unreachable" },
			wantStatus: http.StatusServiceUnavailable,
			wantReady:  false,
		},
		{
			name:       "custom probe forces ready",
			factory:    nil,
			readyFn:    func() (bool, string) { return true, "" },
			wantStatus: http.StatusOK,
			wantReady:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, tt.factory)
			if tt.readyFn != nil {
				srv.SetReadyFn(tt.readyFn)
			}
			addr := startTestDaemon(t, srv)
			defer srv.Stop(context.Background())

			// Liveness is always 200 regardless of readiness.
			hResp, err := http.Get("http://" + addr + "/v1/health")
			if err != nil {
				t.Fatalf("GET /v1/health failed: %v", err)
			}
			hResp.Body.Close()
			if hResp.StatusCode != http.StatusOK {
				t.Errorf("health: expected 200, got %d", hResp.StatusCode)
			}

			resp, err := http.Get("http://" + addr + "/v1/ready")
			if err != nil {
				t.Fatalf("GET /v1/ready failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("ready: expected %d, got %d", tt.wantStatus, resp.StatusCode)
			}
			var ready ReadyResponse
			if err := json.NewDecoder(resp.Body).Decode(&ready); err != nil {
				t.Fatalf("decode ready: %v", err)
			}
			if ready.Ready != tt.wantReady {
				t.Errorf("ready.Ready = %v, want %v (reason=%q)", ready.Ready, tt.wantReady, ready.Reason)
			}
			if !tt.wantReady && ready.Reason == "" {
				t.Error("expected a non-empty reason when not ready")
			}
		})
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
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	factory := func(req ChatRequest) (*engine.Session, error) {
		sess := engine.NewSession("", "test-model", "you are helpful", nil)
		if err := sess.SetMaxTurns(1); err != nil {
			t.Fatalf("set max turns: %v", err)
		}
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

func daemonTestSessionFactory(seen chan<- ChatRequest) SessionFactory {
	return func(req ChatRequest) (*engine.Session, error) {
		if seen != nil {
			seen <- req
		}
		model := req.Model
		if model == "" {
			model = "test-model"
		}
		sess := engine.NewSessionWithClient(
			engine.NewMockClientForTest(),
			"test-provider",
			model,
			"test system prompt",
			nil,
			false,
		)
		if err := sess.SetMaxTurns(1); err != nil {
			return nil, err
		}
		return sess, nil
	}
}

func postDaemonChat(t *testing.T, addr string, request ChatRequest, accept string) (*http.Response, ChatResponse) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal chat request: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://"+addr+"/v1/chat", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new chat request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/chat: %v", err)
	}
	var decoded ChatResponse
	if accept == "" && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
			resp.Body.Close()
			t.Fatalf("decode chat response: %v", err)
		}
	}
	return resp, decoded
}

func TestDaemon_ChatPersistsRetrievableSessionAndRequestMetadata(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	requestedCWD := t.TempDir()
	canonicalCWD, err := canonicalSessionCWD(requestedCWD)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(chan ChatRequest, 1)
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, daemonTestSessionFactory(seen))
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, chat := postDaemonChat(t, addr, ChatRequest{
		Prompt: "persist me",
		Model:  "test-model-override",
		CWD:    requestedCWD,
		Agent:  "reviewer",
	}, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/chat status = %d, want 200", resp.StatusCode)
	}
	if chat.SessionID == "" || resp.Header.Get("X-Hawk-Session-ID") != chat.SessionID {
		t.Fatalf("session ID response/header mismatch: body=%q header=%q", chat.SessionID, resp.Header.Get("X-Hawk-Session-ID"))
	}

	factoryReq := <-seen
	if factoryReq.SessionID != chat.SessionID || factoryReq.CWD != canonicalCWD || factoryReq.Agent != "reviewer" {
		t.Fatalf("factory request = %+v, want durable ID, canonical cwd, and agent", factoryReq)
	}

	saved, err := session.Load(chat.SessionID)
	if err != nil {
		t.Fatalf("returned session ID is not retrievable: %v", err)
	}
	if saved.CWD != canonicalCWD || saved.Agent != "reviewer" || saved.Model != "test-model-override" {
		t.Fatalf("persisted metadata = %+v", saved)
	}
	if len(saved.Messages) != 2 || saved.Messages[0].Content != "persist me" || saved.Messages[1].Role != "assistant" {
		t.Fatalf("persisted transcript = %+v, want user and assistant turns", saved.Messages)
	}

	detailResp, err := http.Get("http://" + addr + "/v1/sessions/" + chat.SessionID)
	if err != nil {
		t.Fatalf("GET persisted session: %v", err)
	}
	defer detailResp.Body.Close()
	var detail SessionDetailResponse
	if err := json.NewDecoder(detailResp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode session detail: %v", err)
	}
	if detailResp.StatusCode != http.StatusOK || detail.ID != chat.SessionID || detail.MessageCount != 2 || detail.Agent != "reviewer" {
		t.Fatalf("session detail status=%d body=%+v", detailResp.StatusCode, detail)
	}
}

func TestDaemon_ChatContinuationReusesDurableSession(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	requestedCWD := t.TempDir()
	seen := make(chan ChatRequest, 2)
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, daemonTestSessionFactory(seen))
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	firstResp, first := postDaemonChat(t, addr, ChatRequest{
		Prompt: "first turn",
		Model:  "continuation-model",
		CWD:    requestedCWD,
		Agent:  "reviewer",
	}, "")
	firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("first chat status = %d", firstResp.StatusCode)
	}

	secondResp, second := postDaemonChat(t, addr, ChatRequest{
		Prompt:    "second turn",
		SessionID: first.SessionID,
	}, "")
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusOK {
		t.Fatalf("continuation status = %d", secondResp.StatusCode)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("continuation returned ID %q, want %q", second.SessionID, first.SessionID)
	}

	<-seen // first request
	continuedReq := <-seen
	if continuedReq.Model != "continuation-model" || continuedReq.Agent != "reviewer" || continuedReq.CWD == "" {
		t.Fatalf("continuation did not inherit persisted metadata: %+v", continuedReq)
	}
	saved, err := session.Load(first.SessionID)
	if err != nil {
		t.Fatalf("load continued session: %v", err)
	}
	if len(saved.Messages) != 4 {
		t.Fatalf("continued transcript has %d messages, want 4: %+v", len(saved.Messages), saved.Messages)
	}
	if saved.Messages[2].Role != "user" || saved.Messages[2].Content != "second turn" {
		t.Fatalf("continued user turn = %+v", saved.Messages[2])
	}
}

func TestDaemon_ChatRejectsMissingContinuation(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, daemonTestSessionFactory(nil))
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, _ := postDaemonChat(t, addr, ChatRequest{Prompt: "continue", SessionID: "does-not-exist"}, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing continuation status = %d, want 404", resp.StatusCode)
	}
}

func TestDaemon_ChatDoesNotMisreportCorruptContinuationAsMissing(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	if err := os.MkdirAll(storage.SessionsDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	const id = "corrupt-session"
	if err := os.WriteFile(filepath.Join(storage.SessionsDir(), id+".jsonl"), []byte("{not-json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, daemonTestSessionFactory(nil))
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, _ := postDaemonChat(t, addr, ChatRequest{Prompt: "continue", SessionID: id}, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("corrupt continuation status = %d, want 500", resp.StatusCode)
	}
	var body ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "session_load_failed" {
		t.Fatalf("corrupt continuation code = %q, want session_load_failed", body.Code)
	}
}

func TestDaemon_ChatRejectsUnsafeSessionIDAndInvalidCWD(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, daemonTestSessionFactory(nil))
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	tests := []ChatRequest{
		{Prompt: "continue", SessionID: "../escape"},
		{Prompt: "start", CWD: t.TempDir() + "/missing"},
	}
	for _, request := range tests {
		resp, _ := postDaemonChat(t, addr, request, "")
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("request %+v status = %d, want 400", request, resp.StatusCode)
		}
	}
}

func TestDaemon_ChatSSEExposesRetrievableSessionID(t *testing.T) {
	t.Skip("TODO: https://github.com/GrayCodeAI/hawk/issues/153")
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, daemonTestSessionFactory(nil))
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, _ := postDaemonChat(t, addr, ChatRequest{Prompt: "stream me"}, "text/event-stream")
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read SSE response: %v", err)
	}
	id := resp.Header.Get("X-Hawk-Session-ID")
	if resp.StatusCode != http.StatusOK || id == "" || !strings.Contains(string(body), `"session_id":"`+id+`"`) {
		t.Fatalf("SSE status=%d id=%q body=%q", resp.StatusCode, id, body)
	}
	if _, err := session.Load(id); err != nil {
		t.Fatalf("SSE session ID is not retrievable: %v", err)
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
