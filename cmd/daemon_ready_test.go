package cmd

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/daemon"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

// TestDaemonReadyProbe_NilFactory verifies the probe reports not-ready when no
// session factory is wired.
func TestDaemonReadyProbe_NilFactory(t *testing.T) {
	ok, reason := daemonReadyProbe(nil)()
	if ok {
		t.Fatalf("nil factory should not be ready")
	}
	if reason == "" {
		t.Errorf("expected a non-empty reason when not ready")
	}
}

// TestDaemonReadyProbe_FactoryWired verifies the conservative fallback: with a
// factory wired the probe reports ready even when preflight is uncertain (no
// credentials configured in the test environment).
func TestDaemonReadyProbe_FactoryWired(t *testing.T) {
	factory := func(daemon.ChatRequest) (*engine.Session, error) { return nil, nil }
	ok, _ := daemonReadyProbe(factory)()
	if !ok {
		t.Fatalf("factory-wired daemon should report ready (conservative fallback)")
	}
}

// TestDaemonReadyProbe_AffectsReadyEndpoint verifies that installing the probe
// via SetReadyFn changes the GET /v1/ready HTTP response.
func TestDaemonReadyProbe_AffectsReadyEndpoint(t *testing.T) {
	factory := func(daemon.ChatRequest) (*engine.Session, error) { return nil, nil }
	srv := daemon.New(daemon.Config{Port: 0, Host: "127.0.0.1"}, factory)
	srv.SetReadyFn(daemonReadyProbe(factory))

	addr, err := srv.Start()
	if err != nil {
		t.Skipf("daemon start failed (loopback unavailable?): %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	// Wait for the listener.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.DialTimeout("tcp", addr, 50*time.Millisecond); err == nil {
			c.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	resp, err := http.Get("http://" + addr + "/v1/ready")
	if err != nil {
		t.Fatalf("GET /v1/ready: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from /v1/ready with factory wired, got %d", resp.StatusCode)
	}
	var body daemon.ReadyResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode ready response: %v", err)
	}
	if !body.Ready {
		t.Errorf("expected Ready=true, got %+v", body)
	}
}
