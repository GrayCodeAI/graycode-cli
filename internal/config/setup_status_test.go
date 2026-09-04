package config

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/testutil"
	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/credentials"
)

func TestHasConfiguredDeployment_FromStore(t *testing.T) {
	InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		InvalidateConfigUICache()
	})
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
	testutil.IsolateStorage(t)
}

func TestHasConfiguredDeployment_RejectsPlaceholder(t *testing.T) {
	InvalidateConfigUICache()
	isolateCredentialEnv(t)
	credentials.SetDefaultStore(emptyCredentialStore{})
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		InvalidateConfigUICache()
	})

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
	InvalidateConfigUICache()
	isolateCredentialEnv(t)
	credentials.SetDefaultStore(emptyCredentialStore{})
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		InvalidateConfigUICache()
	})

	ctx := context.Background()
	compiled := CompiledCatalogV1()
	if compiled != nil {
		for _, k := range catalog.DiscoveryEnvKeysFromCatalog(compiled) {
			t.Setenv(k, "")
		}
	}
	st := EvaluateSetup(ctx)
	if st.HasCredentials {
		// FIXME: environment already has credentials
		t.Skip("environment already has credentials")
	}
	if !st.NeedsSetup {
		t.Fatal("expected setup needed without credentials")
	}
	if st.Hint != "" {
		t.Fatalf("expected no welcome banner hint without credentials, got %q", st.Hint)
	}
}

func TestSyncSelectionWithCredentials_ClearsStaleModel(t *testing.T) {
	InvalidateConfigUICache()
	isolateCredentialEnv(t)
	credentials.SetDefaultStore(emptyCredentialStore{})
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		InvalidateConfigUICache()
	})

	ctx := context.Background()
	if err := SetActiveProvider(ctx, "openrouter"); err != nil {
		t.Fatal(err)
	}
	if err := SetActiveModel(ctx, "gpt-4o"); err != nil {
		t.Fatal(err)
	}
	SyncSelectionWithCredentials(ctx)
	if HasSelectedModel() {
		t.Fatalf("expected stale model cleared, active = %q", ActiveModel(ctx))
	}
	if p := strings.TrimSpace(ActiveProvider(ctx)); p != "" {
		t.Fatalf("expected stale provider cleared, got %q", p)
	}
}

func TestSyncSelectionWithCredentials_KeepsWhenGatewayHasKey(t *testing.T) {
	InvalidateConfigUICache()
	isolateCredentialEnv(t)
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		InvalidateConfigUICache()
	})

	ctx := context.Background()
	_ = store.Set(ctx, credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	InvalidateConfigUICache()
	if err := SetActiveProvider(ctx, "openrouter"); err != nil {
		t.Fatal(err)
	}
	if err := SetActiveModel(ctx, "gpt-4o"); err != nil {
		t.Fatal(err)
	}

	SyncSelectionWithCredentials(ctx)
	if ActiveModel(ctx) != "openai/gpt-4o" {
		t.Fatalf("model = %q", ActiveModel(ctx))
	}
}

func TestFirstRunSetupHint_NoAutoOpen(t *testing.T) {
	InvalidateConfigUICache()
	isolateCredentialEnv(t)
	credentials.SetDefaultStore(emptyCredentialStore{})
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		InvalidateConfigUICache()
	})

	hint := FirstRunSetupHint(context.Background())
	if hint != "" {
		t.Fatalf("expected no splash hint without credentials, got %q", hint)
	}
}

func TestPersistAPIKey_RejectsPlaceholder(t *testing.T) {
	err := PersistAPIKey(context.Background(), "OPENAI_API_KEY", "your-api-key")
	if err == nil {
		t.Fatal("expected error for placeholder key")
	}
}

func TestEvaluateSetupCached_MatchesWarmSnapshot(t *testing.T) {
	InvalidateConfigUICache()
	isolateCredentialEnv(t)
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		InvalidateConfigUICache()
	})

	ctx := context.Background()
	_ = store.Set(ctx, credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	RefreshConfigCredSnapshot(ctx)

	cached := EvaluateSetupCached(ctx)
	if !cached.HasCredentials {
		t.Fatal("expected cached credentials")
	}
	if cached.HasModel {
		t.Fatal("expected no model selected in isolated home")
	}
	if cached.Hint != "Almost ready: /config → finish setup" {
		t.Fatalf("hint = %q", cached.Hint)
	}
}
