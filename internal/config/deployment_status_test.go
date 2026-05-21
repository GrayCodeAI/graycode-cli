package config

import "testing"

func TestResolveCanonicalModelAlias(t *testing.T) {
	canonical := ResolveCanonicalModel("claude-sonnet-4-6")
	if canonical == "" {
		t.Fatal("expected canonical model")
	}
	if canonical != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("canonical = %q", canonical)
	}
}
