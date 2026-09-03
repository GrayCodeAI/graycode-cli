package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/GrayCodeAI/graycode-cli/internal/provider/gateway"
)

func isolateCredentialHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	graycodeDir := filepath.Join(home, ".graycode")
	_ = os.MkdirAll(graycodeDir, 0o700)
	t.Setenv("HOME", home)
	t.Setenv("GRAYCODE_CONFIG_DIR", graycodeDir)
	t.Setenv("EYRIE_CONFIG_DIR", filepath.Join(home, "eyrie"))
}

func TestEffectiveModelAndProvider_ClearsWithoutCredentials(t *testing.T) {
	graycodeconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		graycodeconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	if err := graycodeconfig.SetActiveProvider(ctx, "openrouter"); err != nil {
		t.Fatal(err)
	}
	if err := graycodeconfig.SetActiveModel(ctx, "gpt-4o"); err != nil {
		t.Fatal(err)
	}

	model, provider := effectiveModelAndProvider(graycodeconfig.Settings{})
	if model != "" || provider != "" {
		t.Fatalf("expected empty selection without credentials, got model=%q provider=%q", model, provider)
	}
}

func TestEffectiveModelAndProvider_KeepsWithCredentials(t *testing.T) {
	graycodeconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		graycodeconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	_ = store.Set(ctx, gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	graycodeconfig.InvalidateConfigUICache()
	if err := graycodeconfig.SetActiveProvider(ctx, "openrouter"); err != nil {
		t.Fatal(err)
	}
	if err := graycodeconfig.SetActiveModel(ctx, "gpt-4o"); err != nil {
		t.Fatal(err)
	}

	model, provider := effectiveModelAndProvider(graycodeconfig.Settings{})
	if provider == "" {
		t.Fatalf("expected provider with credentials, got model=%q provider=%q", model, provider)
	}
	if strings.TrimSpace(model) == "" {
		t.Fatalf("expected model preserved, got model=%q provider=%q", model, provider)
	}
}
