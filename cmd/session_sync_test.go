package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/provider/gateway"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

func TestSyncSessionFromPersistedSelection_FillsEmptySessionModel(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	_ = store.Set(ctx, gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()
	_ = hawkconfig.SetActiveProvider(ctx, "openrouter")
	_ = hawkconfig.SetActiveModel(ctx, "gpt-4o")

	sess := engine.NewSession("", "", "test", nil)
	syncSessionFromPersistedSelection(sess)

	if got := sess.Model(); got != "openai/gpt-4o" {
		t.Fatalf("model = %q, want openai/gpt-4o", got)
	}
	if got := sess.Provider(); got != "openrouter" {
		t.Fatalf("provider = %q, want openrouter", got)
	}
}

func TestEnsureSessionReadyForChat_UsesPersistedModel(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	_ = store.Set(ctx, gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()
	_ = hawkconfig.SetActiveProvider(ctx, "openrouter")
	_ = hawkconfig.SetActiveModel(ctx, "gpt-4o")

	m := &chatModel{session: engine.NewSession("", "", "test", nil)}
	if err := m.ensureSessionReadyForChat(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(m.session.Model()) == "" {
		t.Fatal("expected session model after ensure")
	}
}

func TestEnsureSessionReadyForChat_NoModel(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	hawkconfig.InvalidateConfigUICache()
	_ = hawkconfig.ClearActiveSelection(ctx)

	m := &chatModel{session: engine.NewSession("", "", "test", nil)}
	if err := m.ensureSessionReadyForChat(); err == nil {
		t.Fatal("expected error when no model selected")
	}
}

func TestEnsureSessionReadyForChat_AppliesDeferredSystemContextOnce(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	_ = store.Set(ctx, gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()
	_ = hawkconfig.SetActiveProvider(ctx, "openrouter")
	_ = hawkconfig.SetActiveModel(ctx, "gpt-4o")

	m := &chatModel{
		session:                    engine.NewSession("", "", "base", nil),
		deferredSystemContext:      "deferred context",
		deferredSystemContextReady: true,
	}
	if err := m.ensureSessionReadyForChat(); err != nil {
		t.Fatal(err)
	}
	if !m.deferredSystemContextApplied {
		t.Fatal("expected deferred system context to be applied")
	}
	if got := m.session.Persistence().System(); strings.Count(got, "deferred context") != 1 {
		t.Fatalf("expected deferred context once in system prompt, got %q", got)
	}

	if err := m.ensureSessionReadyForChat(); err != nil {
		t.Fatal(err)
	}
	if got := m.session.Persistence().System(); strings.Count(got, "deferred context") != 1 {
		t.Fatalf("expected deferred context to stay deduplicated, got %q", got)
	}
}
