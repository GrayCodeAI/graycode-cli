package engine

import (
	"context"
	"testing"
)

func TestAgentIntelligence_SelectMode(t *testing.T) {
	ai := NewAgentIntelligence()

	tests := []struct {
		prompt string
		want   SubAgentMode
	}{
		{"find all files using deprecated API", SubAgentExplore},
		{"search for authentication code", SubAgentExplore},
		{"analyze the dependency graph", SubAgentExplore},
		{"implement the new endpoint", SubAgentGeneral},
		{"refactor the auth module", SubAgentGeneral},
		{"fix the bug in parser.go", SubAgentGeneral},
	}
	for _, tt := range tests {
		got := ai.SelectMode(tt.prompt)
		if got != tt.want {
			t.Errorf("SelectMode(%q) = %v, want %v", tt.prompt, got, tt.want)
		}
	}
}

func TestAgentIntelligence_AnalyzeForParallelism_Simple(t *testing.T) {
	ai := NewAgentIntelligence()

	// Simple task — no parallelism
	d := ai.AnalyzeForParallelism("fix the typo")
	if d.ShouldParallelize {
		t.Error("simple task should not parallelize")
	}
}

func TestAgentIntelligence_AnalyzeForParallelism_Conjunction(t *testing.T) {
	ai := NewAgentIntelligence()

	// "X and Y" with major scale
	d := ai.AnalyzeForParallelism("refactor the auth module and redesign the database schema")
	if !d.ShouldParallelize {
		t.Error("conjunction task should parallelize")
	}
	if len(d.SubTasks) < 2 {
		t.Errorf("expected 2+ subtasks, got %d", len(d.SubTasks))
	}
}

func TestAgentIntelligence_AnalyzeForParallelism_Pipeline(t *testing.T) {
	ai := NewAgentIntelligence()

	// "research" + "build" + major scale keyword
	d := ai.AnalyzeForParallelism("research the architecture patterns then build a new authentication system from scratch")
	if !d.ShouldParallelize {
		t.Error("research+build should parallelize as pipeline")
	}
	if d.Strategy != StrategyPipeline {
		t.Errorf("expected pipeline strategy, got %v", d.Strategy)
	}
}

func TestAgentIntelligence_ExecuteWithIntelligence_Single(t *testing.T) {
	ai := NewAgentIntelligence()

	called := false
	result, err := ai.ExecuteWithIntelligence(context.Background(), "fix typo", func(_ context.Context, prompt string, mode SubAgentMode) (string, error) {
		called = true
		return "fixed", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("exec function should be called")
	}
	if result != "fixed" {
		t.Errorf("result = %q, want 'fixed'", result)
	}
}

func TestSelfAwareness_ShouldDelegate(t *testing.T) {
	sa := &SelfAwareness{MaxComplexity: ScaleMinor, Specialties: []string{"code-review"}}

	if sa.ShouldDelegate("fix typo", ScalePatch) {
		t.Error("patch should not delegate")
	}
	if !sa.ShouldDelegate("build new service", ScaleEpic) {
		t.Error("epic should delegate when max is minor")
	}
}

func TestSynthesisPrompt(t *testing.T) {
	subtasks := []SubTask{
		{ID: "a", Prompt: "find files", Mode: SubAgentExplore},
		{ID: "b", Prompt: "fix them", Mode: SubAgentGeneral},
	}
	results := map[string]string{"a": "found 3 files", "b": "fixed all"}
	prompt := MergeSynthesisPrompt(subtasks, results)
	if !hasSubstr(prompt, "found 3 files") {
		t.Error("expected result a in synthesis")
	}
	if !hasSubstr(prompt, "fixed all") {
		t.Error("expected result b in synthesis")
	}
	if !hasSubstr(prompt, "Synthesis") {
		t.Error("expected synthesis section")
	}
}
