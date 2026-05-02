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
	seen := map[string]bool{}
	for _, m := range models {
		prefix := strings.Split(m, "-")[0]
		if seen[prefix] {
			t.Errorf("duplicate provider prefix %q in default council models", prefix)
		}
		seen[prefix] = true
	}
}

func TestBuildRankingPrompt(t *testing.T) {
	responses := []CouncilResponse{
		{Model: "model-a", Response: "Answer A"},
		{Model: "model-b", Response: "Answer B"},
	}
	prompt := buildRankingPrompt("What is 2+2?", responses)

	if !strings.Contains(prompt, "What is 2+2?") {
		t.Error("ranking prompt should contain the original question")
	}
	if !strings.Contains(prompt, "Response A") {
		t.Error("ranking prompt should contain anonymized Response A")
	}
	if !strings.Contains(prompt, "Response B") {
		t.Error("ranking prompt should contain anonymized Response B")
	}
	if !strings.Contains(prompt, "FINAL RANKING") {
		t.Error("ranking prompt should request FINAL RANKING format")
	}
}

func TestBuildChairmanPrompt(t *testing.T) {
	responses := []CouncilResponse{
		{Model: "model-a", Response: "Answer A"},
		{Model: "model-b", Response: "Answer B"},
	}
	rankings := []CouncilRanking{
		{Model: "model-a", Ranking: "FINAL RANKING: B, A"},
	}
	prompt := buildChairmanPrompt("What is 2+2?", responses, rankings)

	if !strings.Contains(prompt, "What is 2+2?") {
		t.Error("chairman prompt should contain the original question")
	}
	if !strings.Contains(prompt, "model-a") {
		t.Error("chairman prompt should identify models")
	}
	if !strings.Contains(prompt, "FINAL RANKING: B, A") {
		t.Error("chairman prompt should include rankings")
	}
	if !strings.Contains(prompt, "chairman") {
		t.Error("chairman prompt should mention chairman role")
	}
}

func TestCouncilConfig_EmptyModels(t *testing.T) {
	cfg := CouncilConfig{Models: []string{}}
	_, err := RunCouncil(nil, "test", cfg, nil)
	if err == nil {
		t.Fatal("expected error for empty models")
	}
	if !strings.Contains(err.Error(), "no models specified") {
		t.Errorf("unexpected error: %v", err)
	}
}
