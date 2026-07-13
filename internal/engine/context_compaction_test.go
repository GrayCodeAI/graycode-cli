package engine

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
)

func TestContextUsedTokens_PrefersAPI(t *testing.T) {
	s := newTestSession()
	s.AddUser("hello")
	if got := s.ContextUsedTokens(); got <= 0 {
		t.Fatalf("expected estimate > 0, got %d", got)
	}
	s.RecordAPIUsage(42_000, 500)
	if got := s.ContextUsedTokens(); got != 42_000 {
		t.Fatalf("expected API prompt tokens 42000, got %d", got)
	}
}

func TestNativeCompactionSupportUsesEyrieCredentialStore(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	runtime, err := eyrieengine.New(eyrieengine.Options{SecretStore: store})
	if err != nil {
		t.Fatal(err)
	}
	s := NewSessionWithClient(newEyrieEngineClient(runtime), "anthropic", "claude-sonnet-4-6", "sys", nil, true)
	if s.supportsAnthropicNativeCompaction() {
		t.Fatal("expected no support before Eyrie has a credential")
	}
	if err := store.Set(ctx, credentials.AccountForEnv("ANTHROPIC_API_KEY"), "sk-test"); err != nil {
		t.Fatal(err)
	}
	if !s.supportsAnthropicNativeCompaction() {
		t.Fatal("expected support from Eyrie's injected credential store")
	}
}
