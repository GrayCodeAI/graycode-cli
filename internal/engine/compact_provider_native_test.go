package engine

import "testing"

func TestAnthropicCompactionModel(t *testing.T) {
	cases := map[string]bool{
		"claude-sonnet-4-6":     true,
		"claude-opus-4-8":       true,
		"claude-mythos-preview": true,
		"gpt-4o":                false,
		"":                      false,
	}
	for model, want := range cases {
		if got := anthropicCompactionModel(model); got != want {
			t.Errorf("anthropicCompactionModel(%q) = %v, want %v", model, got, want)
		}
	}
}
