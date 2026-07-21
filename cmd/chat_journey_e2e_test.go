package cmd

import (
	"context"
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/provider/gateway"
)

func requireChatModel(t *testing.T, model any) *chatModel {
	t.Helper()
	switch v := model.(type) {
	case *chatModel:
		return v
	case chatModel:
		return &v
	default:
		t.Fatalf("unexpected model type %T", model)
		return nil
	}
}

func TestChatJourney_ConfigPermissionsAndCoreCommands(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	if err := store.Set(ctx, gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890"); err != nil {
		t.Fatal(err)
	}
	hawkconfig.InvalidateConfigUICache()

	m := newTestChatModel()

	result, cmd := m.handleCommand("/config")
	if cmd != nil {
		t.Fatal("expected /config to open inline panel without tea command")
	}
	m = requireChatModel(t, result)
	if !m.configOpen || m.configTab != configTabModels {
		t.Fatalf("/config should open models tab once credentials exist, got open=%v tab=%d", m.configOpen, m.configTab)
	}

	result, _ = m.handleCommand("/config set provider openrouter")
	m = requireChatModel(t, result)
	if got := strings.ToLower(lastSystemMessage(m.messages)); !strings.Contains(got, "openrouter") {
		t.Fatalf("unexpected provider update message: %q", got)
	}

	if got := m.session.Provider(); got != "openrouter" {
		t.Fatalf("session provider = %q, want openrouter", got)
	}

	result, _ = m.handleCommand("/autonomy allow Bash(git:*)")
	m = requireChatModel(t, result)
	if got := lastSystemMessage(m.messages); !strings.Contains(got, "Allow rules updated.") {
		t.Fatalf("unexpected allow update message: %q", got)
	}

	result, _ = m.handleCommand("/autonomy rules")
	m = requireChatModel(t, result)
	if got := lastSystemMessage(m.messages); !strings.Contains(got, "Bash(git:*)") {
		t.Fatalf("permission rules summary missing allow rule: %q", got)
	}

	for _, slash := range []string{"/help", "/status", "/tools"} {
		result, _ = m.handleCommand(slash)
		m = requireChatModel(t, result)
		if msg := strings.TrimSpace(lastSystemMessage(m.messages)); msg == "" {
			t.Fatalf("%s should append a system message", slash)
		}
	}
}
