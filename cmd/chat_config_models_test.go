package cmd

import "testing"

func TestFilterConfigModelOptions(t *testing.T) {
	opts := []configModelOption{
		{ID: "anthropic/claude-opus-4.6", DisplayName: "anthropic/claude-opus-4.6", Owner: "anthropic"},
		{ID: "qwen/qwen3.7-max", DisplayName: "qwen/qwen3.7-max", Owner: "qwen"},
		{ID: "openai/gpt-4.1-mini", DisplayName: "openai/gpt-4.1-mini", Owner: "openai"},
	}

	got := filterConfigModelOptions(opts, "claude")
	if len(got) != 1 || got[0].ID != "anthropic/claude-opus-4.6" {
		t.Fatalf("claude filter = %+v", got)
	}

	got = filterConfigModelOptions(opts, "qwen3")
	if len(got) != 1 || got[0].Owner != "qwen" {
		t.Fatalf("qwen3 filter = %+v", got)
	}

	got = filterConfigModelOptions(opts, "")
	if len(got) != len(opts) {
		t.Fatalf("empty filter should keep all models, got %d", len(got))
	}

	got = filterConfigModelOptions(opts, "missing-model")
	if len(got) != 0 {
		t.Fatalf("expected no matches, got %+v", got)
	}
}
