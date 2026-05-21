package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func isolateCredentialHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	_ = os.MkdirAll(filepath.Join(home, ".hawk"), 0o700)
	t.Setenv("HOME", home)
}

func TestEffectiveModelAndProvider_ClearsWithoutCredentials(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	if err := hawkconfig.SetActiveProvider(ctx, "openrouter"); err != nil {
		t.Fatal(err)
	}
	if err := hawkconfig.SetActiveModel(ctx, "moonshotai/kimi-k2.6"); err != nil {
		t.Fatal(err)
	}

	model, provider := effectiveModelAndProvider(hawkconfig.Settings{})
	if model != "" || provider != "" {
		t.Fatalf("expected empty selection without credentials, got model=%q provider=%q", model, provider)
	}
}

func TestEffectiveModelAndProvider_KeepsWithCredentials(t *testing.T) {
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
	if err := hawkconfig.SetActiveProvider(ctx, "openrouter"); err != nil {
		t.Fatal(err)
	}
	if err := hawkconfig.SetActiveModel(ctx, "openrouter/auto"); err != nil {
		t.Fatal(err)
	}

	model, provider := effectiveModelAndProvider(hawkconfig.Settings{})
	if provider == "" {
		t.Fatalf("expected provider with credentials, got model=%q provider=%q", model, provider)
	}
	if strings.TrimSpace(model) == "" {
		t.Fatalf("expected model preserved, got model=%q provider=%q", model, provider)
	}
}
