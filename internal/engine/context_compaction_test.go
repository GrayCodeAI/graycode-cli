package engine

import (
	"testing"
)

func TestContextUsedTokens_PrefersAPI(t *testing.T) {
	s := NewSession("", "test", "sys", nil)
	s.AddUser("hello")
	if got := s.ContextUsedTokens(); got <= 0 {
		t.Fatalf("expected estimate > 0, got %d", got)
	}
	s.RecordAPIUsage(42_000, 500)
	if got := s.ContextUsedTokens(); got != 42_000 {
		t.Fatalf("expected API prompt tokens 42000, got %d", got)
	}
}

func TestSupportsAnthropicNativeCompaction_Model(t *testing.T) {
	s := NewSession("", "claude-sonnet-4-6", "sys", nil)
	s.mu.Lock()
	s.apiKeys["anthropic"] = "sk-test"
	s.provider = "anthropic"
	s.mu.Unlock()
	if !s.supportsAnthropicNativeCompaction() {
		t.Fatal("expected anthropic native compaction support")
	}
	s.SetModel("gpt-4o")
	if s.supportsAnthropicNativeCompaction() {
		t.Fatal("expected no support for non-claude model")
	}
}