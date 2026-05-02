package engine

import (
	"strings"
	"testing"
)

func TestDefaultCouncilModels(t *testing.T) {
	models := DefaultCouncilModels()
	if len(models) != 3 {
		t.Fatalf("expected 3 default council models, got %d", len(models))
	}
	// Verify diversity: no two models should share a provider prefix
	seen := map[string]bool{}
	for _, m := range models {
		prefix := strings.Split(m, "-")[0]
		if seen[prefix] {
			t.Errorf("duplicate provider prefix %q in default council models", prefix)
		}
		seen[prefix] = true
	}
}

func TestBuildSynthesisPrompt(t *testing.T) {
	responses := []CouncilResponse{
		{Model: "model-a", Response: "Answer A"},
		{Model: "model-b", Response: "Answer B"},
	}
	prompt := buildSynthesisPrompt("What is 2+2?", responses)

	if !strings.Contains(prompt, "What is 2+2?") {
		t.Error("synthesis prompt should contain the original prompt")
	}
	if !strings.Contains(prompt, "Answer A") {
		t.Error("synthesis prompt should contain response A")
	}
	if !strings.Contains(prompt, "Answer B") {
		t.Error("synthesis prompt should contain response B")
	}
	if !strings.Contains(prompt, "model-a") {
		t.Error("synthesis prompt should identify model-a")
	}
	if !strings.Contains(prompt, "2 responses") {
		t.Error("synthesis prompt should state the count of responses")
	}
}

func TestCouncilConfig_EmptyModels(t *testing.T) {
	cfg := CouncilConfig{Models: []string{}}
	_, _, err := RunCouncil(nil, nil, "test", cfg)
	if err == nil {
		t.Fatal("expected error for empty models")
	}
	if !strings.Contains(err.Error(), "no models specified") {
		t.Errorf("unexpected error: %v", err)
	}
}
