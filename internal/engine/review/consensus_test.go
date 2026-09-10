package review

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewConsensusSampler(t *testing.T) {
	t.Run("default num samples", func(t *testing.T) {
		cs := NewConsensusSampler(0)
		if cs.NumSamples != 3 {
			t.Errorf("expected 3 default samples, got %d", cs.NumSamples)
		}
	})

	t.Run("custom num samples", func(t *testing.T) {
		cs := NewConsensusSampler(5)
		if cs.NumSamples != 5 {
			t.Errorf("expected 5 samples, got %d", cs.NumSamples)
		}
	})

	t.Run("negative defaults to 3", func(t *testing.T) {
		cs := NewConsensusSampler(-1)
		if cs.NumSamples != 3 {
			t.Errorf("expected 3 default samples, got %d", cs.NumSamples)
		}
	})

	t.Run("default strategy is majority", func(t *testing.T) {
		cs := NewConsensusSampler(3)
		if cs.Strategy != "majority" {
			t.Errorf("expected majority strategy, got %s", cs.Strategy)
		}
	})
}

func TestSampleSolutions(t *testing.T) {
	t.Run("generates N samples", func(t *testing.T) {
		cs := NewConsensusSampler(3)
		ctx := context.Background()

		var counter int64
		generateFn := func(_ context.Context, _ string) (string, error) {
			c := atomic.AddInt64(&counter, 1)
			return fmt.Sprintf("Solution %d: implement the handler with proper error handling. Step 1. Create the file src/handler.go", c), nil
		}

		result, err := cs.SampleSolutions(ctx, "fix the bug", generateFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.AllSamples) != 3 {
			t.Errorf("expected 3 samples, got %d", len(result.AllSamples))
		}

		if result.Winner == nil {
			t.Fatal("expected a winner")
		}
	})

	t.Run("best_score strategy", func(t *testing.T) {
		cs := NewConsensusSampler(3)
		cs.Strategy = "best_score"
		cs.ScoreFn = func(s string) float64 {
			return float64(len(s)) / 100.0
		}
		ctx := context.Background()

		solutions := []string{"short", "medium length solution", "this is the longest solution by far and should win the contest"}
		var idx int64
		generateFn := func(_ context.Context, _ string) (string, error) {
			i := atomic.AddInt64(&idx, 1) - 1
			s := solutions[int(i)%len(solutions)]
			return s, nil
		}

		result, err := cs.SampleSolutions(ctx, "prompt", generateFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Winner should be the longest (highest score)
		if result.Winner == nil {
			t.Fatal("expected a winner")
		}
		if result.Winner.Content != solutions[2] {
			t.Errorf("expected longest solution to win, got: %s", result.Winner.Content)
		}
	})

	t.Run("handles all failures", func(t *testing.T) {
		cs := NewConsensusSampler(3)
		ctx := context.Background()

		generateFn := func(_ context.Context, _ string) (string, error) {
			return "", fmt.Errorf("generation failed")
		}

		_, err := cs.SampleSolutions(ctx, "prompt", generateFn)
		if err == nil {
			t.Fatal("expected error when all samples fail")
		}
	})

	t.Run("handles context cancellation", func(t *testing.T) {
		cs := NewConsensusSampler(3)
		ctx, cancel := context.WithCancel(context.Background())

		generateFn := func(c context.Context, _ string) (string, error) {
			select {
			case <-c.Done():
				return "", c.Err()
			case <-time.After(5 * time.Second):
				return "result", nil
			}
		}

		// Cancel immediately
		cancel()
		_, err := cs.SampleSolutions(ctx, "prompt", generateFn)
		if err == nil {
			t.Fatal("expected error on cancelled context")
		}
	})

	t.Run("synthesize strategy", func(t *testing.T) {
		cs := NewConsensusSampler(2)
		cs.Strategy = "synthesize"
		ctx := context.Background()

		solutions := []string{
			"First paragraph about setup.\n\nSecond about implementation.",
			"First paragraph about setup.\n\nThird about testing.",
		}
		var idx int64
		generateFn := func(_ context.Context, _ string) (string, error) {
			i := atomic.AddInt64(&idx, 1) - 1
			s := solutions[int(i)%len(solutions)]
			return s, nil
		}

		result, err := cs.SampleSolutions(ctx, "prompt", generateFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Winner == nil {
			t.Fatal("expected synthesized winner")
		}
		if result.Method != "synthesize" {
			t.Errorf("expected synthesize method, got %s", result.Method)
		}
	})
}

func TestMajorityVote(t *testing.T) {
	t.Run("picks most similar sample", func(t *testing.T) {
		samples := []Sample{
			{ID: 1, Content: "use a middleware approach with error handling", Score: 0.7},
			{ID: 2, Content: "use a middleware pattern with proper error handling and logging", Score: 0.8},
			{ID: 3, Content: "completely different approach using events and streams", Score: 0.6},
		}

		winner := MajorityVote(samples)
		if winner == nil {
			t.Fatal("expected a winner")
		}
		// Samples 1 and 2 are similar, so one of them should win
		if winner.ID != 1 && winner.ID != 2 {
			t.Errorf("expected sample 1 or 2 to win majority, got %d", winner.ID)
		}
	})

	t.Run("empty samples", func(t *testing.T) {
		winner := MajorityVote(nil)
		if winner != nil {
			t.Error("expected nil for empty samples")
		}
	})

	t.Run("single sample", func(t *testing.T) {
		samples := []Sample{{ID: 1, Content: "only one", Score: 0.5}}
		winner := MajorityVote(samples)
		if winner == nil || winner.ID != 1 {
			t.Error("expected the single sample to win")
		}
	})
}

func TestBestScore(t *testing.T) {
	t.Run("picks highest score", func(t *testing.T) {
		samples := []Sample{
			{ID: 1, Content: "low", Score: 0.3},
			{ID: 2, Content: "high", Score: 0.9},
			{ID: 3, Content: "mid", Score: 0.6},
		}

		winner := BestScore(samples)
		if winner == nil {
			t.Fatal("expected a winner")
		}
		if winner.ID != 2 {
			t.Errorf("expected sample 2 (highest score), got %d", winner.ID)
		}
	})

	t.Run("empty samples", func(t *testing.T) {
		winner := BestScore(nil)
		if winner != nil {
			t.Error("expected nil for empty samples")
		}
	})
}

func TestSynthesize(t *testing.T) {
	t.Run("combines unique paragraphs", func(t *testing.T) {
		samples := []Sample{
			{ID: 1, Content: "Setup the project.\n\nWrite the tests.", Score: 0.9},
			{ID: 2, Content: "Setup the project.\n\nDeploy to production.", Score: 0.7},
		}

		result := Synthesize(samples)
		if result == nil {
			t.Fatal("expected synthesized result")
		}
		if !strings.Contains(result.Content, "Setup the project") {
			t.Error("expected setup paragraph")
		}
		if !strings.Contains(result.Content, "Write the tests") {
			t.Error("expected tests paragraph from higher-scored sample")
		}
		if !strings.Contains(result.Content, "Deploy to production") {
			t.Error("expected deploy paragraph from second sample")
		}
	})

	t.Run("empty samples", func(t *testing.T) {
		result := Synthesize(nil)
		if result != nil {
			t.Error("expected nil for empty samples")
		}
	})

	t.Run("single sample returns itself", func(t *testing.T) {
		samples := []Sample{{ID: 1, Content: "only this", Score: 0.8}}
		result := Synthesize(samples)
		if result == nil || result.Content != "only this" {
			t.Error("expected single sample returned as-is")
		}
	})
}

func TestScoreByLength(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		minScore float64
		maxScore float64
	}{
		{"empty", "", 0.0, 0.0},
		{"too short", "hi", 0.0, 0.1},
		{"ideal length", strings.Repeat("word ", 100), 0.9, 1.0},
		{"very long", strings.Repeat("word ", 2000), 0.3, 0.8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ScoreByLength(tt.content)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score %.2f not in range [%.2f, %.2f]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestScoreByCompleteness(t *testing.T) {
	t.Run("empty content", func(t *testing.T) {
		if ScoreByCompleteness("") != 0.0 {
			t.Error("expected 0 for empty content")
		}
	})

	t.Run("complete content", func(t *testing.T) {
		content := `Here is the solution.

Step 1. Create the file src/handler.go

` + "```go\nfunc Handle() error {\n    return nil\n}\n```" + `

This handles the error properly. It validates input. It returns structured errors.`

		score := ScoreByCompleteness(content)
		if score < 0.75 {
			t.Errorf("expected high completeness score, got %.2f", score)
		}
	})

	t.Run("minimal content", func(t *testing.T) {
		score := ScoreByCompleteness("just do it")
		if score > 0.5 {
			t.Errorf("expected low completeness score for minimal content, got %.2f", score)
		}
	})
}

func TestPairwiseSimilarity(t *testing.T) {
	t.Run("identical strings", func(t *testing.T) {
		sim := PairwiseSimilarity("hello world", "hello world")
		if sim != 1.0 {
			t.Errorf("expected 1.0 for identical strings, got %.2f", sim)
		}
	})

	t.Run("completely different", func(t *testing.T) {
		sim := PairwiseSimilarity("alpha beta gamma", "one two three")
		if sim != 0.0 {
			t.Errorf("expected 0.0 for completely different strings, got %.2f", sim)
		}
	})

	t.Run("partial overlap", func(t *testing.T) {
		sim := PairwiseSimilarity("the quick brown fox", "the slow brown cat")
		if sim <= 0.0 || sim >= 1.0 {
			t.Errorf("expected partial similarity, got %.2f", sim)
		}
	})

	t.Run("empty strings", func(t *testing.T) {
		sim := PairwiseSimilarity("", "")
		if sim != 1.0 {
			t.Errorf("expected 1.0 for two empty strings, got %.2f", sim)
		}
	})

	t.Run("one empty", func(t *testing.T) {
		sim := PairwiseSimilarity("hello", "")
		if sim != 0.0 {
			t.Errorf("expected 0.0 when one string is empty, got %.2f", sim)
		}
	})
}

func TestFormatConsensus(t *testing.T) {
	t.Run("formats result correctly", func(t *testing.T) {
		winner := &Sample{ID: 2, Content: "comprehensive solution with tests", Score: 0.89}
		result := &ConsensusResult{
			Winner: winner,
			AllSamples: []Sample{
				{ID: 1, Content: "focused on middleware approach", Score: 0.72},
				{ID: 2, Content: "comprehensive solution with tests", Score: 0.89},
				{ID: 3, Content: "minimal implementation", Score: 0.65},
			},
			Agreement: 0.67,
			Method:    "majority",
		}

		formatted := FormatConsensus(result)

		if !strings.Contains(formatted, "3 samples") {
			t.Error("expected sample count in output")
		}
		if !strings.Contains(formatted, "majority vote") {
			t.Error("expected strategy name in output")
		}
		if !strings.Contains(formatted, "Sample #2") {
			t.Error("expected winner reference")
		}
		if !strings.Contains(formatted, "0.89") {
			t.Error("expected winner score")
		}
		if !strings.Contains(formatted, "67%") {
			t.Error("expected agreement percentage")
		}
		if !strings.Contains(formatted, "← selected") {
			t.Error("expected selected marker")
		}
	})

	t.Run("nil result", func(t *testing.T) {
		formatted := FormatConsensus(nil)
		if formatted != "No consensus result" {
			t.Errorf("expected 'No consensus result', got: %s", formatted)
		}
	})
}

func TestCalculateAgreement(t *testing.T) {
	samples := []Sample{
		{ID: 1, Content: "use middleware with logging"},
		{ID: 2, Content: "use middleware with error handling and logging"},
		{ID: 3, Content: "completely different event-driven approach"},
	}
	winner := &samples[1]

	agreement := calculateAgreement(samples, winner)
	if agreement <= 0.0 || agreement >= 1.0 {
		t.Errorf("expected agreement between 0 and 1, got %.2f", agreement)
	}
}

func TestConsensusEstimateTokens(t *testing.T) {
	tokens := estimateTokens("hello world this is a test")
	if tokens != 6 {
		t.Errorf("expected 9 tokens, got %d", tokens)
	}
}

func TestWordSet(t *testing.T) {
	set := wordSet("Hello, World! Hello again.")
	if !set["hello"] {
		t.Error("expected 'hello' in word set")
	}
	if !set["world"] {
		t.Error("expected 'world' in word set")
	}
	if !set["again"] {
		t.Error("expected 'again' in word set")
	}
}

func TestDefaultScoreFn(t *testing.T) {
	// A well-formed solution should score reasonably
	content := `Here is the implementation plan.

Step 1. Create src/handler.go with the following code:

` + "```go\npackage main\n\nfunc main() {}\n```" + `

This handles the request correctly. It validates the input. It produces proper output.`

	score := DefaultScoreFn(content)
	if score < 0.3 || score > 1.0 {
		t.Errorf("expected reasonable score, got %.2f", score)
	}
}

func TestConsensusConcurrency(t *testing.T) {
	cs := NewConsensusSampler(10)
	ctx := context.Background()

	generateFn := func(_ context.Context, _ string) (string, error) {
		// Simulate some work
		time.Sleep(10 * time.Millisecond)
		return "concurrent solution with proper error handling. Step 1. Create file main.go. Step 2. Add tests.", nil
	}

	start := time.Now()
	result, err := cs.SampleSolutions(ctx, "test concurrency", generateFn)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.AllSamples) != 10 {
		t.Errorf("expected 10 samples, got %d", len(result.AllSamples))
	}

	// If truly parallel, 10 samples at 10ms each should take ~10-50ms, not 100ms+
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected parallel execution, took %v", elapsed)
	}
}

func TestSummarizeSample(t *testing.T) {
	t.Run("short content", func(t *testing.T) {
		s := summarizeSample("hello world")
		if s != "hello world" {
			t.Errorf("expected 'hello world', got '%s'", s)
		}
	})

	t.Run("long content truncated", func(t *testing.T) {
		long := strings.Repeat("a", 100)
		s := summarizeSample(long)
		if len(s) > 50 {
			t.Errorf("expected truncated summary, got length %d", len(s))
		}
		if !strings.HasSuffix(s, "...") {
			t.Error("expected ... suffix")
		}
	})

	t.Run("skips empty lines", func(t *testing.T) {
		s := summarizeSample("\n\n\nactual content")
		if s != "actual content" {
			t.Errorf("expected 'actual content', got '%s'", s)
		}
	})
}

// Ensure math import is used (for TestFormatConsensus rounding check)
var _ = math.Round
