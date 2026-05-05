package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/engine"
)

func TestDaemon_StartStop(t *testing.T) {
	srv := New(Config{Port: 0, Host: "127.0.0.1"}, nil) // port 0 = random free port
	addr, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer srv.Stop(context.Background())

	if addr == "" {
		t.Error("expected non-empty address")
	}
}

func TestDaemon_Health(t *testing.T) {
	srv := New(Config{Port: 0, Host: "127.0.0.1"}, nil)
	addr, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
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
	srv := New(Config{Port: 0, Host: "127.0.0.1"}, nil)
	addr, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
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

func TestDaemon_Chat_WithEngine(t *testing.T) {
	factory := func(req ChatRequest) (*engine.Session, error) {
		sess := engine.NewSession("", "test-model", "you are helpful", nil)
		sess.MaxTurns = 1
		return sess, nil
	}
	srv := New(Config{Port: 0, Host: "127.0.0.1"}, factory)
	addr, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
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
	srv := New(Config{Port: 0, Host: "127.0.0.1"}, nil)
	addr, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
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
	srv := New(Config{Port: 0, Host: "127.0.0.1"}, nil)
	addr, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
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
	srv := New(Config{Port: 0, Host: "127.0.0.1"}, nil)
	_, err := srv.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}
