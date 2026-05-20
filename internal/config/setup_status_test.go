package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestHasConfiguredDeployment_FromStore(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	_ = store.Set(context.Background(), credentials.AccountForEnv("ANTHROPIC_API_KEY"), "sk-ant-test-key-long-enough")
	if !HasConfiguredDeployment(context.Background()) {
		t.Fatal("expected true when ANTHROPIC_API_KEY is in secure store")
	}
}

type emptyCredentialStore struct{}

func (emptyCredentialStore) Set(context.Context, string, string) error   { return nil }
func (emptyCredentialStore) Get(context.Context, string) (string, error) { return "", nil }
func (emptyCredentialStore) Delete(context.Context, string) error        { return nil }

func isolateCredentialEnv(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	_ = os.MkdirAll(filepath.Join(home, ".hawk"), 0o700)
	t.Setenv("HOME", home)
}

func TestHasConfiguredDeployment_RejectsPlaceholder(t *testing.T) {
	isolateCredentialEnv(t)
	credentials.SetDefaultStore(emptyCredentialStore{})
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	ctx := context.Background()
	compiled := CompiledCatalogV1()
	if compiled != nil {
		for _, k := range catalog.DiscoveryEnvKeysFromCatalog(compiled) {
			t.Setenv(k, "")
		}
	}
	t.Setenv("OPENROUTER_API_KEY", "changeme")
	// Placeholder in shell env must not count — only secure store is trusted.
	if HasConfiguredDeployment(ctx) {
		t.Fatal("placeholder should not count as configured")
	}
}

func TestEvaluateSetup_WithoutCredentials(t *testing.T) {
	isolateCredentialEnv(t)
	credentials.SetDefaultStore(emptyCredentialStore{})
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	ctx := context.Background()
	compiled := CompiledCatalogV1()
	if compiled != nil {
		for _, k := range catalog.DiscoveryEnvKeysFromCatalog(compiled) {
			t.Setenv(k, "")
		}
	}
	st := EvaluateSetup(ctx)
	if st.HasCredentials {
		t.Skip("environment already has credentials")
	}
	if !st.NeedsSetup {
		t.Fatal("expected setup needed without credentials")
	}
	if st.Hint == "" {
		t.Fatal("expected non-empty setup hint")
	}
}

func TestPersistAPIKey_RejectsPlaceholder(t *testing.T) {
	err := PersistAPIKey(context.Background(), "OPENAI_API_KEY", "your-api-key")
	if err == nil {
		t.Fatal("expected error for placeholder key")
	}
}
