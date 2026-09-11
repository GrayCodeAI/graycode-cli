package config

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/provider/gateway"
)

func TestCatalogEmptyHint_NoCredentials(t *testing.T) {
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() { gateway.SetDefaultStore(nil) })

	hint := CatalogEmptyHint(context.Background())
	if !strings.Contains(hint, "/config") {
		t.Fatalf("expected /config guidance, got %q", hint)
	}
}

func TestCatalogEmptyHint_WithCredentials(t *testing.T) {
	InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		InvalidateConfigUICache()
	})

	ctx := context.Background()
	_ = store.Set(ctx, gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	InvalidateConfigUICache()

	hint := CatalogEmptyHint(ctx)
	if strings.Contains(hint, "paste an API key") {
		t.Fatalf("should not ask for key when credentials exist: %q", hint)
	}
	if !strings.Contains(hint, "preflight") {
		t.Fatalf("expected preflight guidance, got %q", hint)
	}
}

func TestEnsureCatalogAvailable_MissingCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EYRIE_MODEL_CATALOG_PATH", dir+"/missing.json")

	err := EnsureCatalogAvailable(context.Background())
	if err == nil || !strings.Contains(err.Error(), "/config") {
		t.Fatalf("expected /config in error, got %v", err)
	}
}

func TestCatalogStatusLine_Empty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EYRIE_MODEL_CATALOG_PATH", dir+"/missing.json")

	line := CatalogStatusLine(context.Background())
	if !strings.Contains(line, "missing") && !strings.Contains(line, "empty") {
		t.Fatalf("expected missing/empty status, got %q", line)
	}
	if !strings.Contains(line, "/config") {
		t.Fatalf("expected /config in status line, got %q", line)
	}
}
