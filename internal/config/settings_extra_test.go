package config

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestAPIKeyForProvider(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	ctx := context.Background()
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "sk-test-key"); err != nil {
		t.Fatal(err)
	}
	key := APIKeyForProvider("openai")
	if key != "sk-test-key" {
		t.Errorf("APIKeyForProvider = %q, want sk-test-key", key)
	}
}

func TestAPIKeyForProvider_Missing(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	key := APIKeyForProvider("nonexistent_provider_xyz")
	if key != "" {
		t.Errorf("expected empty for missing key, got %q", key)
	}
}

func TestAllEnvKeyStatus(t *testing.T) {
	result := AllEnvKeyStatus()
	if result == "" {
		t.Error("AllEnvKeyStatus should return status string")
	}
}
