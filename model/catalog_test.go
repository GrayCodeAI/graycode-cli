package model

import "testing"

func TestFind(t *testing.T) {
	m, ok := Find("gpt-4o")
	if !ok {
		t.Fatal("expected to find gpt-4o")
	}
	if m.Provider != "openai" {
		t.Fatalf("expected provider openai, got %s", m.Provider)
	}
}

func TestFindNotFound(t *testing.T) {
	_, ok := Find("nonexistent-model")
	if ok {
		t.Fatal("expected not to find nonexistent model")
	}
}

func TestByProvider(t *testing.T) {
	models := ByProvider("anthropic")
	if len(models) == 0 {
		t.Fatal("expected anthropic models")
	}
	for _, m := range models {
		if m.Provider != "anthropic" {
			t.Fatalf("expected anthropic, got %s", m.Provider)
		}
	}
}

func TestRecommended(t *testing.T) {
	m, ok := Recommended("anthropic")
	if !ok {
		t.Fatal("expected recommended anthropic model")
	}
	if !m.Recommended {
		t.Fatal("expected model to be recommended")
	}
}

func TestDefaultModel(t *testing.T) {
	for _, provider := range AllProviders() {
		model := DefaultModel(provider)
		if model == "" {
			t.Fatalf("expected default model for %s", provider)
		}
	}
}

func TestAllProviders(t *testing.T) {
	providers := AllProviders()
	if len(providers) == 0 {
		t.Fatal("expected providers")
	}
	seen := make(map[string]bool)
	for _, p := range providers {
		if seen[p] {
			t.Fatalf("duplicate provider: %s", p)
		}
		seen[p] = true
	}
}
