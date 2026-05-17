package engine

import (
	"strings"
	"testing"
)

func TestNewCritic(t *testing.T) {
	c := NewCritic("gpt-4o-mini")
	if c == nil {
		t.Fatal("expected non-nil critic")
	}
	if c.Model() != "gpt-4o-mini" {
		t.Errorf("expected model=gpt-4o-mini, got %q", c.Model())
	}
}

func TestCritic_PreScreenPatch(t *testing.T) {
	c := NewCritic("gpt-4o-mini")

	t.Run("no_change", func(t *testing.T) {
		original := "func hello() { return }"
		verdict := c.PreScreenPatch(original, original, "fix the bug")
		if verdict.Likely != "incorrect" {
			t.Errorf("identical content should be 'incorrect', got %q", verdict.Likely)
		}
		if len(verdict.Issues) == 0 {
			t.Error("should have at least one issue for no-change patch")
		}
	})

	t.Run("empty_patch", func(t *testing.T) {
		original := "func hello() { return }"
		verdict := c.PreScreenPatch(original, "", "refactor function")
		if verdict.Likely != "incorrect" {
			t.Errorf("empty patch should be 'incorrect', got %q", verdict.Likely)
		}
	})

	t.Run("massive_deletion", func(t *testing.T) {
		original := strings.Repeat("line\n", 100)
		patched := "line\n"
		verdict := c.PreScreenPatch(original, patched, "minor fix")
		if verdict.Likely != "incorrect" {
			t.Errorf("massive deletion should be 'incorrect', got %q", verdict.Likely)
		}
		if verdict.Confidence <= 0.7 {
			t.Errorf("high confidence expected for massive deletion, got %f", verdict.Confidence)
		}
	})

	t.Run("reasonable_change", func(t *testing.T) {
		original := "func hello() { return 1 }"
		patched := "func hello() { return 2 }"
		verdict := c.PreScreenPatch(original, patched, "change return value")
		if verdict.Likely != "uncertain" {
			t.Errorf("reasonable change should be 'uncertain' without model, got %q", verdict.Likely)
		}
	})
}

func TestCritic_BuildPromptAndParseVerdict(t *testing.T) {
	c := NewCritic("gpt-4o-mini")

	original := "func add(a, b int) int { return a + b }"
	patched := "func add(a, b int) int { return a - b }"
	intent := "fix addition function"

	prompt := c.BuildPrompt(original, patched, intent)
	if !strings.Contains(prompt, intent) {
		t.Error("prompt should contain the intent")
	}
	if !strings.Contains(prompt, "return a + b") {
		t.Error("prompt should contain original code")
	}
	if !strings.Contains(prompt, "return a - b") {
		t.Error("prompt should contain patched code")
	}
	if !strings.Contains(prompt, "VERDICT:") {
		t.Error("prompt should contain response format instructions")
	}

	// Test parsing a correct verdict
	correctResponse := "VERDICT: correct CONFIDENCE: 0.9\nISSUES: none"
	verdict := c.ParseVerdict(correctResponse)
	if verdict.Likely != "correct" {
		t.Errorf("expected 'correct', got %q", verdict.Likely)
	}
	if verdict.Confidence != 0.9 {
		t.Errorf("expected confidence=0.9, got %f", verdict.Confidence)
	}
	if len(verdict.Issues) != 0 {
		t.Errorf("expected no issues, got %v", verdict.Issues)
	}

	// Test parsing an incorrect verdict with issues
	incorrectResponse := "VERDICT: incorrect CONFIDENCE: 0.85\nISSUES: changes subtraction instead of addition, wrong operator"
	verdict2 := c.ParseVerdict(incorrectResponse)
	if verdict2.Likely != "incorrect" {
		t.Errorf("expected 'incorrect', got %q", verdict2.Likely)
	}
	if len(verdict2.Issues) != 2 {
		t.Errorf("expected 2 issues, got %d: %v", len(verdict2.Issues), verdict2.Issues)
	}
}

func TestCritic_ShouldBlock(t *testing.T) {
	c := NewCritic("gpt-4o-mini")

	tests := []struct {
		name    string
		verdict *PatchVerdict
		block   bool
	}{
		{
			name:    "nil verdict",
			verdict: nil,
			block:   false,
		},
		{
			name:    "correct high confidence",
			verdict: &PatchVerdict{Likely: "correct", Confidence: 0.95},
			block:   false,
		},
		{
			name:    "incorrect high confidence",
			verdict: &PatchVerdict{Likely: "incorrect", Confidence: 0.85},
			block:   true,
		},
		{
			name:    "incorrect low confidence",
			verdict: &PatchVerdict{Likely: "incorrect", Confidence: 0.6},
			block:   false,
		},
		{
			name:    "uncertain",
			verdict: &PatchVerdict{Likely: "uncertain", Confidence: 0.9},
			block:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.ShouldBlock(tt.verdict)
			if got != tt.block {
				t.Errorf("ShouldBlock() = %v, want %v", got, tt.block)
			}
		})
	}
}
