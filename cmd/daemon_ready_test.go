package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/GrayCodeAI/graycode-cli/internal/daemon"
	"github.com/GrayCodeAI/graycode-cli/internal/engine"
	"github.com/GrayCodeAI/graycode-cli/internal/storage"
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

// TestDaemonReadyProbe_FailedEyriePreflight verifies a factory alone does not
// make the daemon ready when Eyrie's authoritative preflight is incomplete.
func TestDaemonReadyProbe_FailedEyriePreflight(t *testing.T) {
	factory := func(daemon.ChatRequest) (*engine.Session, error) { return nil, nil }
	probe := daemonReadyProbeWithPreflight(factory, func(context.Context) graycodeconfig.EnginePreflight {
		return graycodeconfig.EnginePreflight{Ready: false}
	})
	ok, reason := probe()
	if ok {
		t.Fatal("failed Eyrie preflight must report not ready")
	}
	if reason == "" {
		t.Fatal("failed Eyrie preflight must include a reason")
	}
}

func TestDaemonReadyProbe_ReadyEyriePreflight(t *testing.T) {
	factory := func(daemon.ChatRequest) (*engine.Session, error) { return nil, nil }
	probe := daemonReadyProbeWithPreflight(factory, func(context.Context) graycodeconfig.EnginePreflight {
		return graycodeconfig.EnginePreflight{Ready: true}
	})
	ok, reason := probe()
	if !ok || reason != "" {
		t.Fatalf("ready Eyrie preflight = (%v, %q), want (true, empty)", ok, reason)
	}
}

// TestDaemonReadyProbe_AffectsReadyEndpoint verifies that installing the probe
// via SetReadyFn changes the GET /v1/ready HTTP response.
func TestDaemonReadyProbe_AffectsReadyEndpoint(t *testing.T) {
	factory := func(daemon.ChatRequest) (*engine.Session, error) { return nil, nil }
	srv := daemon.New(daemon.Config{Port: 0, Host: "127.0.0.1"}, factory)
	srv.SetReadyFn(daemonReadyProbeWithPreflight(factory, func(context.Context) graycodeconfig.EnginePreflight {
		return graycodeconfig.EnginePreflight{Ready: false}
	}))

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
		if c, dialErr := net.DialTimeout("tcp", addr, 50*time.Millisecond); dialErr == nil {
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

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 from /v1/ready when Eyrie is not ready, got %d", resp.StatusCode)
	}
	var body daemon.ReadyResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode ready response: %v", err)
	}
	if body.Ready || body.Reason == "" {
		t.Errorf("expected Ready=false with reason, got %+v", body)
	}
}

func TestDaemonAgentConfig_AppliesNamedPersonaAndModel(t *testing.T) {
	storage.SetTestDirs(t, t.TempDir())
	if err := os.MkdirAll(storage.PersonasDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	definition := `---
name: reviewer
description: Reviews code
model: reviewer-model
---
You are the reviewer persona.`
	if err := os.WriteFile(filepath.Join(storage.PersonasDir(), "reviewer.md"), []byte(definition), 0o600); err != nil {
		t.Fatal(err)
	}

	prompt, model, err := daemonAgentConfig("reviewer", "base prompt")
	if err != nil {
		t.Fatalf("daemonAgentConfig: %v", err)
	}
	if model != "reviewer-model" {
		t.Fatalf("model = %q, want reviewer-model", model)
	}
	if !strings.HasPrefix(prompt, "You are the reviewer persona.\n\n") || !strings.HasSuffix(prompt, "base prompt") {
		t.Fatalf("persona prompt was not prepended: %q", prompt)
	}
}

func TestDaemonAgentConfig_UnknownPersonaIsInvalidRequest(t *testing.T) {
	storage.SetTestDirs(t, t.TempDir())
	_, _, err := daemonAgentConfig("missing", "base prompt")
	var requestErr *daemon.InvalidChatRequestError
	if !errors.As(err, &requestErr) || requestErr.Message == "" {
		t.Fatalf("error = %v, want InvalidChatRequestError with public message", err)
	}
}
