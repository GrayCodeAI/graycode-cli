package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func TestNewSolutionReviewer(t *testing.T) {
	t.Run("default max attempts", func(t *testing.T) {
		sr := NewSolutionReviewer(0)
		if sr.MaxAttempts != 3 {
			t.Errorf("expected 3 default attempts, got %d", sr.MaxAttempts)
		}
	})

	t.Run("custom max attempts", func(t *testing.T) {
		sr := NewSolutionReviewer(5)
		if sr.MaxAttempts != 5 {
			t.Errorf("expected 5 attempts, got %d", sr.MaxAttempts)
		}
	})

	t.Run("negative defaults to 3", func(t *testing.T) {
		sr := NewSolutionReviewer(-1)
		if sr.MaxAttempts != 3 {
			t.Errorf("expected 3 default attempts, got %d", sr.MaxAttempts)
		}
	})

	t.Run("nil ScoreFn by default", func(t *testing.T) {
		sr := NewSolutionReviewer(3)
		if sr.ScoreFn != nil {
			t.Error("expected nil ScoreFn by default")
		}
	})
}

func TestReviewAndSelect(t *testing.T) {
	t.Run("runs multiple attempts and selects best", func(t *testing.T) {
		sr := NewSolutionReviewer(3)
		ctx := context.Background()

		attempts := 0
		solveFn := func(_ context.Context, _ string) (*Solution, error) {
			attempts++
			return &Solution{
				Content:       fmt.Sprintf("```go\nfunc fix%d() {}\n```\nThis fixes the issue in handler.go", attempts),
				TokensUsed:    100 * attempts,
				FilesModified: []string{"handler.go"},
			}, nil
		}

		result, err := sr.ReviewAndSelect(ctx, "fix the bug", solveFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Best == nil {
			t.Fatal("expected a best solution")
		}

		if result.Attempts == 0 {
			t.Error("expected at least one attempt")
		}

		if result.Best.Score <= 0 {
			t.Errorf("expected positive score, got %f", result.Best.Score)
		}
	})

	t.Run("handles all failures gracefully", func(t *testing.T) {
		sr := NewSolutionReviewer(3)
		ctx := context.Background()

		solveFn := func(_ context.Context, _ string) (*Solution, error) {
			return nil, errors.New("compilation failed")
		}

		result, err := sr.ReviewAndSelect(ctx, "fix the bug", solveFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Even failed attempts are recorded
		if result.Attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", result.Attempts)
		}

		// Best should be the least-bad solution (all scored 0)
		if result.Best.Score != 0 {
			t.Errorf("expected score 0 for all-failed, got %f", result.Best.Score)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		sr := NewSolutionReviewer(10)
		ctx, cancel := context.WithCancel(context.Background())

		attempts := 0
		solveFn := func(_ context.Context, _ string) (*Solution, error) {
			attempts++
			if attempts >= 2 {
				cancel()
			}
			return &Solution{
				Content:       "```go\nfunc handler() { return nil }\n```",
				FilesModified: []string{"main.go"},
			}, nil
		}

		result, err := sr.ReviewAndSelect(ctx, "implement feature", solveFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have stopped after context cancellation
		if result.Attempts > 3 {
			t.Errorf("expected early stop, got %d attempts", result.Attempts)
		}
	})

	t.Run("stops early on high score", func(t *testing.T) {
		sr := NewSolutionReviewer(5)
		sr.ScoreFn = func(_ string) float64 {
			return 0.99
		}
		ctx := context.Background()

		attempts := 0
		solveFn := func(_ context.Context, _ string) (*Solution, error) {
			attempts++
			return &Solution{
				Content:       "excellent solution",
				FilesModified: []string{"a.go"},
			}, nil
		}

		_, err := sr.ReviewAndSelect(ctx, "task", solveFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if attempts > 1 {
			t.Errorf("expected early stop on high score, got %d attempts", attempts)
		}
	})

	t.Run("tracks total duration and tokens", func(t *testing.T) {
		sr := NewSolutionReviewer(2)
		sr.ScoreFn = func(_ string) float64 { return 0.5 }
		ctx := context.Background()

		solveFn := func(_ context.Context, _ string) (*Solution, error) {
			return &Solution{
				Content:    "fix applied",
				Duration:   50 * time.Millisecond,
				TokensUsed: 200,
			}, nil
		}

		result, err := sr.ReviewAndSelect(ctx, "task", solveFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.TotalTokens != 400 {
			t.Errorf("expected 400 total tokens, got %d", result.TotalTokens)
		}
	})
}

func TestScoreSolution(t *testing.T) {
	t.Run("default scoring with full marks", func(t *testing.T) {
		sr := NewSolutionReviewer(3)
		sol := &Solution{
			Content:       "```go\nfunc fixBug() error {\n\treturn nil\n}\n```\nThis addresses the issue by handling the nil case properly.",
			FilesModified: []string{"handler.go", "handler_test.go"},
			Errors:        nil,
		}

		score := sr.ScoreSolution(sol)
		if score < 0.9 {
			t.Errorf("expected high score for good solution, got %f", score)
		}
	})

	t.Run("empty solution scores low", func(t *testing.T) {
		sr := NewSolutionReviewer(3)
		sol := &Solution{
			Content: "",
			Errors:  []string{"failed to generate"},
		}

		score := sr.ScoreSolution(sol)
		if score > 0.1 {
			t.Errorf("expected very low score for empty solution with errors, got %f", score)
		}
	})

	t.Run("solution with errors penalized", func(t *testing.T) {
		sr := NewSolutionReviewer(3)
		solNoErr := &Solution{
			Content:       "```go\nfunc fix() {}\n```",
			FilesModified: []string{"main.go"},
		}
		solWithErr := &Solution{
			Content:       "```go\nfunc fix() {}\n```",
			FilesModified: []string{"main.go"},
			Errors:        []string{"lint warning"},
		}

		scoreNoErr := sr.ScoreSolution(solNoErr)
		scoreWithErr := sr.ScoreSolution(solWithErr)

		if scoreWithErr >= scoreNoErr {
			t.Errorf("solution with errors should score lower: no_err=%.2f, with_err=%.2f",
				scoreNoErr, scoreWithErr)
		}
	})

	t.Run("custom score function", func(t *testing.T) {
		sr := NewSolutionReviewer(3)
		sr.ScoreFn = func(s string) float64 {
			return float64(len(s)) / 1000.0
		}

		sol := &Solution{Content: strings.Repeat("x", 500)}
		score := sr.ScoreSolution(sol)

		if math.Abs(score-0.5) > 0.01 {
			t.Errorf("expected 0.5 from custom fn, got %f", score)
		}
	})

	t.Run("nil solution scores zero", func(t *testing.T) {
		sr := NewSolutionReviewer(3)
		score := defaultScoreSolution(nil)
		if score != 0.0 {
			t.Errorf("expected 0 for nil solution, got %f", score)
		}
	})
}

func TestCompareApproaches(t *testing.T) {
	t.Run("no solutions", func(t *testing.T) {
		result := CompareApproaches(nil)
		if result != "No solutions to compare" {
			t.Errorf("unexpected result: %s", result)
		}
	})

	t.Run("single solution", func(t *testing.T) {
		solutions := []Solution{{ID: 1, Content: "fix"}}
		result := CompareApproaches(solutions)
		if !strings.Contains(result, "Only one attempt") {
			t.Errorf("expected single attempt message, got: %s", result)
		}
	})

	t.Run("multiple varied solutions", func(t *testing.T) {
		solutions := []Solution{
			{ID: 1, Content: "refactor the entire module to use interfaces"},
			{ID: 2, Content: "add a simple nil check to fix the crash"},
			{ID: 3, Content: "write comprehensive tests and fix the root cause"},
		}

		result := CompareApproaches(solutions)
		if !strings.Contains(result, "3 attempts") {
			t.Errorf("expected 3 attempts header, got: %s", result)
		}
		if !strings.Contains(result, "#1:") || !strings.Contains(result, "#2:") || !strings.Contains(result, "#3:") {
			t.Errorf("expected all solutions listed, got: %s", result)
		}
	})

	t.Run("similar solutions detected", func(t *testing.T) {
		solutions := []Solution{
			{ID: 1, Content: "add nil check in handler function to prevent crash"},
			{ID: 2, Content: "add nil check in the handler function to prevent the crash"},
		}

		result := CompareApproaches(solutions)
		if !strings.Contains(result, "similar") {
			t.Errorf("expected similar detection, got: %s", result)
		}
	})
}

func TestFormatReview(t *testing.T) {
	t.Run("nil result", func(t *testing.T) {
		result := FormatReview(nil)
		if result != "No review result" {
			t.Errorf("unexpected: %s", result)
		}
	})

	t.Run("formatted output contains key info", func(t *testing.T) {
		best := Solution{ID: 2, Score: 0.91, Content: "```go\nfunc test() {}\n```", FilesModified: []string{"a.go", "b.go", "c.go"}}
		result := &ReviewResult{
			Best: &best,
			All: []Solution{
				{ID: 1, Score: 0.72, Content: "minimal fix", FilesModified: []string{"a.go"}},
				best,
				{ID: 3, Score: 0.65, Content: "incomplete", Errors: []string{"timeout"}},
			},
			Attempts:      3,
			TotalDuration: 5 * time.Second,
			TotalTokens:   1500,
			Agreement:     0.45,
		}

		formatted := FormatReview(result)

		if !strings.Contains(formatted, "3 attempts") {
			t.Errorf("missing attempts count in: %s", formatted)
		}
		if !strings.Contains(formatted, "SELECTED") {
			t.Errorf("missing SELECTED marker in: %s", formatted)
		}
		if !strings.Contains(formatted, "#2") {
			t.Errorf("missing best solution ID in: %s", formatted)
		}
		if !strings.Contains(formatted, "0.91") {
			t.Errorf("missing best score in: %s", formatted)
		}
		if !strings.Contains(formatted, "45%") {
			t.Errorf("missing agreement percentage in: %s", formatted)
		}
		if !strings.Contains(formatted, "approaches varied") {
			t.Errorf("missing agreement description in: %s", formatted)
		}
	})

	t.Run("high agreement label", func(t *testing.T) {
		best := Solution{ID: 1, Score: 0.8, Content: "fix"}
		result := &ReviewResult{
			Best:      &best,
			All:       []Solution{best},
			Attempts:  1,
			Agreement: 0.85,
		}

		formatted := FormatReview(result)
		if !strings.Contains(formatted, "high convergence") {
			t.Errorf("expected high convergence label, got: %s", formatted)
		}
	})
}

func TestShouldRetry(t *testing.T) {
	t.Run("empty solutions means retry", func(t *testing.T) {
		if !ShouldRetry(nil) {
			t.Error("expected true for empty solutions")
		}
	})

	t.Run("low score means retry", func(t *testing.T) {
		solutions := []Solution{
			{ID: 1, Score: 0.4},
			{ID: 2, Score: 0.5},
		}
		if !ShouldRetry(solutions) {
			t.Error("expected true for low scores")
		}
	})

	t.Run("high score means no retry", func(t *testing.T) {
		solutions := []Solution{
			{ID: 1, Score: 0.4},
			{ID: 2, Score: 0.85},
		}
		if ShouldRetry(solutions) {
			t.Error("expected false when best score >= 0.7")
		}
	})

	t.Run("borderline score at 0.7 means no retry", func(t *testing.T) {
		solutions := []Solution{
			{ID: 1, Score: 0.7},
		}
		if ShouldRetry(solutions) {
			t.Error("expected false at exactly 0.7")
		}
	})
}

func TestHasCodeChanges(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"empty", "", false},
		{"code block", "here is the fix:\n```go\nfunc foo() {}\n```", true},
		{"func keyword", "func handler() error { return nil }", true},
		{"diff format", "diff --git a/main.go b/main.go", true},
		{"plain text", "I think we should fix the bug", false},
		{"import statement", "import fmt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasCodeChanges(tt.content)
			if got != tt.want {
				t.Errorf("hasCodeChanges(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestReasonableLengthScore(t *testing.T) {
	t.Run("empty is zero", func(t *testing.T) {
		if reasonableLengthScore("") != 0.0 {
			t.Error("expected 0 for empty")
		}
	})

	t.Run("ideal range is 1.0", func(t *testing.T) {
		content := strings.Repeat("x", 500)
		score := reasonableLengthScore(content)
		if score != 1.0 {
			t.Errorf("expected 1.0 for ideal length, got %f", score)
		}
	})

	t.Run("too short is penalized", func(t *testing.T) {
		content := strings.Repeat("x", 50)
		score := reasonableLengthScore(content)
		if score >= 1.0 {
			t.Errorf("expected penalty for short content, got %f", score)
		}
		if score != 0.5 {
			t.Errorf("expected 0.5 for 50/100, got %f", score)
		}
	})

	t.Run("very long is penalized", func(t *testing.T) {
		content := strings.Repeat("x", 15000)
		score := reasonableLengthScore(content)
		if score >= 1.0 {
			t.Errorf("expected penalty for very long content, got %f", score)
		}
	})
}

func TestSolutionSimilarity(t *testing.T) {
	t.Run("identical strings", func(t *testing.T) {
		sim := solutionSimilarity("hello world", "hello world")
		if sim != 1.0 {
			t.Errorf("expected 1.0 for identical, got %f", sim)
		}
	})

	t.Run("completely different", func(t *testing.T) {
		sim := solutionSimilarity("alpha beta gamma", "delta epsilon zeta")
		if sim != 0.0 {
			t.Errorf("expected 0.0 for no overlap, got %f", sim)
		}
	})

	t.Run("both empty", func(t *testing.T) {
		sim := solutionSimilarity("", "")
		if sim != 1.0 {
			t.Errorf("expected 1.0 for both empty, got %f", sim)
		}
	})

	t.Run("one empty", func(t *testing.T) {
		sim := solutionSimilarity("hello", "")
		if sim != 0.0 {
			t.Errorf("expected 0.0 when one is empty, got %f", sim)
		}
	})

	t.Run("partial overlap", func(t *testing.T) {
		sim := solutionSimilarity("fix the handler bug", "fix the router bug")
		if sim <= 0.0 || sim >= 1.0 {
			t.Errorf("expected partial similarity, got %f", sim)
		}
	})
}

func TestCalculateSolutionAgreement(t *testing.T) {
	t.Run("single solution is 1.0", func(t *testing.T) {
		solutions := []Solution{{ID: 1, Content: "fix"}}
		agreement := calculateSolutionAgreement(solutions)
		if agreement != 1.0 {
			t.Errorf("expected 1.0, got %f", agreement)
		}
	})

	t.Run("identical solutions have high agreement", func(t *testing.T) {
		solutions := []Solution{
			{ID: 1, Content: "add nil check to prevent crash"},
			{ID: 2, Content: "add nil check to prevent crash"},
		}
		agreement := calculateSolutionAgreement(solutions)
		if agreement < 0.9 {
			t.Errorf("expected high agreement for identical solutions, got %f", agreement)
		}
	})
}
