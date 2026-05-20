package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

func TestChatConnectionStatus_WithModel(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	_ = store.Set(ctx, credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()
	_ = hawkconfig.SetActiveProvider(ctx, "openrouter")
	_ = hawkconfig.SetActiveModel(ctx, "moonshotai/kimi-k2.6")

	sess := &engine.Session{}
	sess.SetProvider("openrouter")
	sess.SetModel("moonshotai/kimi-k2.6")

	m := chatModel{session: sess}
	got := m.chatConnectionStatus()
	if !strings.Contains(got, "OpenRouter: ") {
		t.Fatalf("expected gateway prefix, got %q", got)
	}
	if !strings.Contains(got, "kimi-k2.6") {
		t.Fatalf("expected model name, got %q", got)
	}
	if strings.Contains(got, "moonshotai/kimi") {
		t.Fatalf("should not show owner slug as gateway label, got %q", got)
	}
}

func TestChatConnectionStatus_KeyNoModel(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	_ = store.Set(ctx, credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()
	_ = hawkconfig.ClearActiveSelection(ctx)
	_ = hawkconfig.SetActiveProvider(ctx, "openrouter")

	m := chatModel{session: &engine.Session{}}
	got := m.chatConnectionStatus()
	if got != "OpenRouter: pick model" {
		t.Fatalf("status = %q", got)
	}
}

func TestChatConnectionStatus_NoGatewayNoModel(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	_ = store.Set(ctx, credentials.AccountForEnv("ANTHROPIC_API_KEY"), "sk-ant-test-key-long-enough")
	hawkconfig.InvalidateConfigUICache()
	_ = hawkconfig.ClearActiveSelection(ctx)

	m := chatModel{session: &engine.Session{}}
	got := m.chatConnectionStatus()
	if got != "pick model" {
		t.Fatalf("status = %q", got)
	}
}

func TestWelcomeDockerRunning_States(t *testing.T) {
	m := chatModel{containerEnabled: false}
	if m.welcomeDockerRunning() != nil {
		t.Fatal("expected nil when container mode disabled")
	}

	m.containerEnabled = true
	m.containerReady = true
	running := m.welcomeDockerRunning()
	if running == nil || !*running {
		t.Fatalf("expected running=true when container ready, got %v", running)
	}

	m.containerReady = false
	m.containerErr = errors.New("docker not running")
	stopped := m.welcomeDockerRunning()
	if stopped == nil || *stopped {
		t.Fatalf("expected running=false when container errored, got %v", stopped)
	}
}

func TestBuildWelcomeMessage_IncludesDockerWhenEnabled(t *testing.T) {
	running := true
	msg := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, 80, &running)
	if !strings.Contains(msg, "Docker") {
		t.Fatalf("expected Docker indicator in welcome, got snippet without it")
	}
}

func TestBuildWelcomeMessage_OmitsDockerWhenDisabled(t *testing.T) {
	msg := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, 80, nil)
	if strings.Contains(msg, "Docker") {
		t.Fatal("expected no Docker indicator when container mode disabled")
	}
}

func TestBuildWelcomeMessage_UsesDisplayVersion(t *testing.T) {
	SetVersion("dev")
	msg := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, 80, nil)
	if strings.Contains(msg, "vdev") {
		t.Fatal("welcome should not show vdev; DisplayVersion should read VERSION file or dev")
	}
	if !strings.Contains(msg, "v") {
		t.Fatal("expected version line in welcome")
	}
}
