package cmd

import (
	"strings"
	"testing"
)

func TestDefaultResearchConfig(t *testing.T) {
	cfg := DefaultResearchConfig()
	if cfg.Direction != "lower" {
		t.Errorf("expected direction 'lower', got %q", cfg.Direction)
	}
	if cfg.Budget != 5 {
		t.Errorf("expected budget 5, got %d", cfg.Budget)
	}
	if cfg.BranchPrefix != "autoresearch" {
		t.Errorf("expected branch prefix 'autoresearch', got %q", cfg.BranchPrefix)
	}
	if cfg.ResultsFile != "results.tsv" {
		t.Errorf("expected results file 'results.tsv', got %q", cfg.ResultsFile)
	}
}

func TestBuildResearchPromptContainsKey(t *testing.T) {
	cfg := ResearchConfig{
		MetricCmd:  "go test -bench .",
		MetricGrep: "^BenchmarkSort",
		Direction:  "higher",
		Budget:     3,
	}
	prompt := BuildResearchPrompt(cfg)

	checks := []string{
		"go test -bench .",
		"^BenchmarkSort",
		"higher is better",
		"NEVER STOP",
		"results.tsv",
		"git reset --hard",
		"tail -n 50 run.log",
		"Max 3 fix attempts",
		"Simplicity criterion",
		"Baseline first",
		"LOOP FOREVER",
		"increased",
	}
	for _, want := range checks {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildResearchPromptLowerDirection(t *testing.T) {
	cfg := ResearchConfig{
		MetricCmd: "uv run train.py",
		Direction: "lower",
	}
	prompt := BuildResearchPrompt(cfg)
	if !strings.Contains(prompt, "lower is better") {
		t.Error("prompt should say 'lower is better'")
	}
	if !strings.Contains(prompt, "decreased") {
		t.Error("prompt should use 'decreased' for lower direction")
	}
}

func TestBuildResearchPromptDefaults(t *testing.T) {
	cfg := ResearchConfig{MetricCmd: "make test"}
	prompt := BuildResearchPrompt(cfg)
	if !strings.Contains(prompt, "autoresearch") {
		t.Error("prompt should use default branch prefix")
	}
	if !strings.Contains(prompt, "results.tsv") {
		t.Error("prompt should use default results file")
	}
}

func TestParseResearchArgs(t *testing.T) {
	cfg := parseResearchArgs("--grep '^val_bpb:' --direction lower --budget 10 --branch myexp --results out.tsv uv run train.py")
	if cfg.MetricCmd != "uv run train.py" {
		t.Errorf("expected metric cmd 'uv run train.py', got %q", cfg.MetricCmd)
	}
	if cfg.MetricGrep != "'^val_bpb:'" {
		t.Errorf("expected grep '^val_bpb:', got %q", cfg.MetricGrep)
	}
	if cfg.Direction != "lower" {
		t.Errorf("expected direction 'lower', got %q", cfg.Direction)
	}
	if cfg.Budget != 10 {
		t.Errorf("expected budget 10, got %d", cfg.Budget)
	}
	if cfg.BranchPrefix != "myexp" {
		t.Errorf("expected branch 'myexp', got %q", cfg.BranchPrefix)
	}
	if cfg.ResultsFile != "out.tsv" {
		t.Errorf("expected results 'out.tsv', got %q", cfg.ResultsFile)
	}
}

func TestParseResearchArgsMinimal(t *testing.T) {
	cfg := parseResearchArgs("go test -bench .")
	if cfg.MetricCmd != "go test -bench ." {
		t.Errorf("expected 'go test -bench .', got %q", cfg.MetricCmd)
	}
	if cfg.Direction != "lower" {
		t.Errorf("expected default direction 'lower', got %q", cfg.Direction)
	}
}
