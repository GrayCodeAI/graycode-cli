package cmd

import (
	"testing"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
)

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

func TestModelOptionIsActive(t *testing.T) {
	opt := configModelOption{ID: "anthropic/claude-opus-4.6"}
	if !modelOptionIsActive(opt, "anthropic/claude-opus-4.6") {
		t.Fatal("expected active match")
	}
	if modelOptionIsActive(opt, "other/model") {
		t.Fatal("expected no match")
	}
}

func TestConfigModelOptionsCarryResolvedEngineIdentity(t *testing.T) {
	opts := configModelOptionsFromEyrie([]graycodeconfig.EngineModel{{
		ID: "models/gemini-pro", CanonicalID: "google/gemini-pro",
		ProviderID: "google", GatewayID: "gemini", Capabilities: []string{"tools", "vision"},
	}})
	if len(opts) != 1 || opts[0].CanonicalID != "google/gemini-pro" ||
		opts[0].ProviderID != "google" || opts[0].GatewayID != "gemini" {
		t.Fatalf("resolved engine identity was lost: %+v", opts)
	}
	if !modelOptionIsActiveResolved(opts[0], "google/gemini-pro", "google/gemini-pro") {
		t.Fatal("canonical identity did not match without catalog lookup")
	}
	if len(opts[0].Capabilities) != 2 || opts[0].Capabilities[1] != "vision" {
		t.Fatalf("capabilities were lost: %+v", opts[0].Capabilities)
	}
}
