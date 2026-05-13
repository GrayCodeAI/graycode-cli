package engine

import (
	"strings"
	"testing"
	"time"
)

func TestNewPromptOptimizer(t *testing.T) {
	po := NewPromptOptimizer()
	if po == nil {
		t.Fatal("NewPromptOptimizer returned nil")
	}
	if po.MaxExamples != 5 {
		t.Errorf("expected MaxExamples=5, got %d", po.MaxExamples)
	}
	if len(po.Examples) != 0 {
		t.Errorf("expected empty examples, got %d", len(po.Examples))
	}
	if po.Metrics == nil {
		t.Error("expected non-nil Metrics map")
	}
}

func TestRecordOutcome_AddsToPool(t *testing.T) {
	po := NewPromptOptimizer()

	po.RecordOutcome("fix the login bug", "used grep to find auth code then patched", "success", []string{"grep", "edit"}, 500)

	if len(po.Examples) != 1 {
		t.Fatalf("expected 1 example, got %d", len(po.Examples))
	}

	ex := po.Examples[0]
	if ex.Task != "fix the login bug" {
		t.Errorf("unexpected task: %s", ex.Task)
	}
	if ex.Approach != "used grep to find auth code then patched" {
		t.Errorf("unexpected approach: %s", ex.Approach)
	}
	if ex.Outcome != "success" {
		t.Errorf("unexpected outcome: %s", ex.Outcome)
	}
	if len(ex.ToolsUsed) != 2 {
		t.Errorf("expected 2 tools, got %d", len(ex.ToolsUsed))
	}
	if ex.TokensUsed != 500 {
		t.Errorf("expected 500 tokens, got %d", ex.TokensUsed)
	}
	if ex.Score <= 0 || ex.Score > 1 {
		t.Errorf("score out of range: %f", ex.Score)
	}
}

func TestRecordOutcome_FailureNotAdded(t *testing.T) {
	po := NewPromptOptimizer()

	po.RecordOutcome("fix the login bug", "tried but failed", "failure", []string{"grep"}, 2000)

	if len(po.Examples) != 0 {
		t.Errorf("failure should not be added, got %d examples", len(po.Examples))
	}

	// But metrics should be updated
	if len(po.Metrics) == 0 {
		t.Error("metrics should be updated even for failures")
	}
}

func TestRecordOutcome_DiversityEnforcement(t *testing.T) {
	po := NewPromptOptimizer()

	// Record very similar tasks - only first should be kept
	po.RecordOutcome("fix the login bug in auth module", "patched auth", "success", []string{"edit"}, 500)
	po.RecordOutcome("fix the login bug in auth system", "patched auth code", "success", []string{"edit"}, 600)

	if len(po.Examples) != 1 {
		t.Errorf("expected 1 example (diversity filter), got %d", len(po.Examples))
	}

	// Record a different task - should be added
	po.RecordOutcome("implement new REST endpoint for users", "created handler", "success", []string{"write"}, 800)

	if len(po.Examples) != 2 {
		t.Errorf("expected 2 examples (different task), got %d", len(po.Examples))
	}
}

func TestSelectExamples_ReturnsRelevant(t *testing.T) {
	po := NewPromptOptimizer()

	po.RecordOutcome("fix authentication bug", "found issue in auth middleware", "success", []string{"grep", "edit"}, 500)
	po.RecordOutcome("add new database migration", "created migration file", "success", []string{"write"}, 800)
	po.RecordOutcome("optimize SQL query performance", "added index", "success", []string{"read", "edit"}, 600)

	// Select examples relevant to "fix the auth error"
	results := po.SelectExamples("fix the auth error", 2)

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	// The auth-related example should be first/most relevant
	found := false
	for _, r := range results {
		if strings.Contains(r.Task, "auth") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected auth-related example to be selected")
	}
}

func TestSelectExamples_RespectsLimit(t *testing.T) {
	po := NewPromptOptimizer()

	po.RecordOutcome("task alpha for testing", "approach alpha", "success", []string{"read"}, 100)
	po.RecordOutcome("task beta for development", "approach beta", "success", []string{"write"}, 200)
	po.RecordOutcome("task gamma for debugging", "approach gamma", "success", []string{"grep"}, 300)
	po.RecordOutcome("task delta for optimization", "approach delta", "success", []string{"edit"}, 400)

	results := po.SelectExamples("testing development debugging optimization", 2)
	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

func TestSelectExamples_DiversityInSelection(t *testing.T) {
	po := NewPromptOptimizer()
	po.MaxExamples = 10 // increase so we can test diversity in selection

	// Add distinct examples manually
	now := time.Now()
	po.Examples = []DSPyExample{
		{Task: "fix login authentication issue", Approach: "patched middleware", Outcome: "success", ToolsUsed: []string{"edit"}, Score: 0.9, Timestamp: now},
		{Task: "fix login session timeout", Approach: "extended timeout", Outcome: "success", ToolsUsed: []string{"edit"}, Score: 0.9, Timestamp: now},
		{Task: "create new REST endpoint", Approach: "added handler", Outcome: "success", ToolsUsed: []string{"write"}, Score: 0.8, Timestamp: now},
	}

	results := po.SelectExamples("fix login problems and issues", 3)

	// Should not return both login-related examples due to diversity
	loginCount := 0
	for _, r := range results {
		if strings.Contains(r.Task, "login") {
			loginCount++
		}
	}
	if loginCount > 1 {
		t.Errorf("diversity filter should prevent selecting %d similar login examples", loginCount)
	}
}

func TestBuildOptimizedPrompt_InjectsExamples(t *testing.T) {
	po := NewPromptOptimizer()

	po.RecordOutcome("fix authentication bug", "found issue in middleware", "success", []string{"grep", "edit"}, 500)

	result := po.BuildOptimizedPrompt("You are a helpful assistant.", "fix the auth error")

	if !strings.Contains(result, "You are a helpful assistant.") {
		t.Error("base prompt should be preserved")
	}
	if !strings.Contains(result, "## Successful Approaches (learn from these)") {
		t.Error("should contain section header")
	}
	if !strings.Contains(result, "Task: fix authentication bug") {
		t.Error("should contain example task")
	}
	if !strings.Contains(result, "Approach: found issue in middleware") {
		t.Error("should contain example approach")
	}
	if !strings.Contains(result, "Tools used: grep, edit") {
		t.Error("should contain tools used")
	}
}

func TestBuildOptimizedPrompt_NoExamples(t *testing.T) {
	po := NewPromptOptimizer()

	result := po.BuildOptimizedPrompt("You are a helpful assistant.", "do something")

	if result != "You are a helpful assistant." {
		t.Errorf("with no examples, should return base prompt unchanged, got: %s", result)
	}
}

func TestScoreExample_ComputesCorrectRelevance(t *testing.T) {
	po := NewPromptOptimizer()

	// Highly relevant, recent, successful example
	relevant := DSPyExample{
		Task:      "fix authentication bug in login module",
		Approach:  "patched middleware",
		Outcome:   "success",
		Score:     0.9,
		Timestamp: time.Now(),
	}

	// Old, less relevant example
	old := DSPyExample{
		Task:      "add database migration for users table",
		Approach:  "created migration",
		Outcome:   "success",
		Score:     0.7,
		Timestamp: time.Now().Add(-30 * 24 * time.Hour),
	}

	task := "fix the authentication error"

	scoreRelevant := po.ScoreExample(relevant, task)
	scoreOld := po.ScoreExample(old, task)

	if scoreRelevant <= scoreOld {
		t.Errorf("relevant example (%.3f) should score higher than old irrelevant one (%.3f)",
			scoreRelevant, scoreOld)
	}

	// Score should be between 0 and 1
	if scoreRelevant < 0 || scoreRelevant > 1 {
		t.Errorf("score out of bounds: %f", scoreRelevant)
	}
}

func TestScoreExample_RecencyBonus(t *testing.T) {
	po := NewPromptOptimizer()

	recent := DSPyExample{
		Task:      "fix bug in module",
		Approach:  "patched code",
		Outcome:   "success",
		Score:     0.8,
		Timestamp: time.Now().Add(-1 * time.Hour),
	}

	older := DSPyExample{
		Task:      "fix bug in module",
		Approach:  "patched code",
		Outcome:   "success",
		Score:     0.8,
		Timestamp: time.Now().Add(-10 * 24 * time.Hour),
	}

	task := "fix bug in module"

	scoreRecent := po.ScoreExample(recent, task)
	scoreOlder := po.ScoreExample(older, task)

	if scoreRecent <= scoreOlder {
		t.Errorf("recent example (%.3f) should score higher than older one (%.3f)",
			scoreRecent, scoreOlder)
	}
}

func TestPruneExamples_RemovesOld(t *testing.T) {
	po := NewPromptOptimizer()

	now := time.Now()
	po.Examples = []DSPyExample{
		{Task: "recent task", Score: 0.9, Timestamp: now.Add(-1 * time.Hour)},
		{Task: "old task", Score: 0.9, Timestamp: now.Add(-48 * time.Hour)},
		{Task: "very old task", Score: 0.9, Timestamp: now.Add(-72 * time.Hour)},
	}

	po.PruneExamples(24 * time.Hour)

	if len(po.Examples) != 1 {
		t.Errorf("expected 1 example after pruning, got %d", len(po.Examples))
	}
	if po.Examples[0].Task != "recent task" {
		t.Errorf("expected recent task to survive, got %s", po.Examples[0].Task)
	}
}

func TestPruneExamples_RemovesLowScoring(t *testing.T) {
	po := NewPromptOptimizer()
	po.MaxExamples = 2 // soft limit = 6

	now := time.Now()
	// Add 8 examples (over the soft limit of 6)
	for i := 0; i < 8; i++ {
		po.Examples = append(po.Examples, DSPyExample{
			Task:      "task " + string(rune('A'+i)),
			Score:     float64(i) / 10.0,
			Timestamp: now,
		})
	}

	po.PruneExamples(24 * time.Hour)

	if len(po.Examples) > 6 {
		t.Errorf("expected at most 6 examples after pruning, got %d", len(po.Examples))
	}

	// Verify highest scoring examples survive
	for _, ex := range po.Examples {
		if ex.Score < 0.2 {
			t.Errorf("low-scoring example (%.1f) should have been pruned", ex.Score)
		}
	}
}

func TestExportImport_RoundTrip(t *testing.T) {
	po := NewPromptOptimizer()

	po.RecordOutcome("fix bug", "patched", "success", []string{"edit"}, 500)
	po.RecordOutcome("add feature", "implemented", "success", []string{"write", "read"}, 1000)

	data, err := po.ExportExamples()
	if err != nil {
		t.Fatalf("ExportExamples failed: %v", err)
	}

	// Import into a fresh optimizer
	po2 := NewPromptOptimizer()
	err = po2.ImportExamples(data)
	if err != nil {
		t.Fatalf("ImportExamples failed: %v", err)
	}

	if len(po2.Examples) != len(po.Examples) {
		t.Fatalf("expected %d examples after import, got %d", len(po.Examples), len(po2.Examples))
	}

	for i := range po.Examples {
		if po.Examples[i].Task != po2.Examples[i].Task {
			t.Errorf("task mismatch at %d: %s vs %s", i, po.Examples[i].Task, po2.Examples[i].Task)
		}
		if po.Examples[i].Approach != po2.Examples[i].Approach {
			t.Errorf("approach mismatch at %d", i)
		}
		if len(po.Examples[i].ToolsUsed) != len(po2.Examples[i].ToolsUsed) {
			t.Errorf("tools mismatch at %d", i)
		}
	}
}

func TestImportExamples_InvalidJSON(t *testing.T) {
	po := NewPromptOptimizer()
	err := po.ImportExamples([]byte("not valid json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestABTest_RecordResult(t *testing.T) {
	ab := NewABTest(
		DSPyVariant{ID: "v1", Template: "template A"},
		DSPyVariant{ID: "v2", Template: "template B"},
	)

	ab.RecordResult("A", true)
	ab.RecordResult("A", true)
	ab.RecordResult("A", false)
	ab.RecordResult("B", true)
	ab.RecordResult("B", false)
	ab.RecordResult("B", false)

	if ab.SuccessesA != 2 {
		t.Errorf("expected 2 successes for A, got %d", ab.SuccessesA)
	}
	if ab.FailuresA != 1 {
		t.Errorf("expected 1 failure for A, got %d", ab.FailuresA)
	}
	if ab.SuccessesB != 1 {
		t.Errorf("expected 1 success for B, got %d", ab.SuccessesB)
	}
	if ab.FailuresB != 2 {
		t.Errorf("expected 2 failures for B, got %d", ab.FailuresB)
	}
	if ab.VariantA.UsageCount != 3 {
		t.Errorf("expected 3 usages for A, got %d", ab.VariantA.UsageCount)
	}
	if ab.VariantB.UsageCount != 3 {
		t.Errorf("expected 3 usages for B, got %d", ab.VariantB.UsageCount)
	}
}

func TestABTest_WinnerInsufficientData(t *testing.T) {
	ab := NewABTest(
		DSPyVariant{ID: "v1", Template: "template A"},
		DSPyVariant{ID: "v2", Template: "template B"},
	)

	// Only 10 trials total - not enough
	for i := 0; i < 7; i++ {
		ab.RecordResult("A", true)
	}
	for i := 0; i < 3; i++ {
		ab.RecordResult("B", false)
	}

	winner := ab.Winner()
	if winner != "" {
		t.Errorf("expected empty winner with insufficient data, got %s", winner)
	}
}

func TestABTest_WinnerDeterminesCorrectly(t *testing.T) {
	ab := NewABTest(
		DSPyVariant{ID: "v1", Template: "template A"},
		DSPyVariant{ID: "v2", Template: "template B"},
	)

	// Give A a clear advantage: 18 successes, 2 failures
	for i := 0; i < 18; i++ {
		ab.RecordResult("A", true)
	}
	for i := 0; i < 2; i++ {
		ab.RecordResult("A", false)
	}

	// Give B a clear disadvantage: 2 successes, 18 failures
	for i := 0; i < 2; i++ {
		ab.RecordResult("B", true)
	}
	for i := 0; i < 18; i++ {
		ab.RecordResult("B", false)
	}

	winner := ab.Winner()
	if winner != "A" {
		t.Errorf("expected winner A with clear advantage, got %q", winner)
	}
}

func TestABTest_WinnerB(t *testing.T) {
	ab := NewABTest(
		DSPyVariant{ID: "v1", Template: "template A"},
		DSPyVariant{ID: "v2", Template: "template B"},
	)

	// A is bad
	for i := 0; i < 2; i++ {
		ab.RecordResult("A", true)
	}
	for i := 0; i < 18; i++ {
		ab.RecordResult("A", false)
	}

	// B is good
	for i := 0; i < 18; i++ {
		ab.RecordResult("B", true)
	}
	for i := 0; i < 2; i++ {
		ab.RecordResult("B", false)
	}

	winner := ab.Winner()
	if winner != "B" {
		t.Errorf("expected winner B, got %q", winner)
	}
}

func TestABTest_PickVariant(t *testing.T) {
	ab := NewABTest(
		DSPyVariant{ID: "v1", Template: "template A"},
		DSPyVariant{ID: "v2", Template: "template B"},
	)

	// With no data, should return either A or B
	pick := ab.PickVariant()
	if pick != "A" && pick != "B" {
		t.Errorf("expected A or B, got %s", pick)
	}

	// After giving A many successes, should mostly pick A
	for i := 0; i < 50; i++ {
		ab.RecordResult("A", true)
	}
	for i := 0; i < 50; i++ {
		ab.RecordResult("B", false)
	}

	aCount := 0
	for i := 0; i < 100; i++ {
		if ab.PickVariant() == "A" {
			aCount++
		}
	}
	// Should pick A most of the time (>70%)
	if aCount < 70 {
		t.Errorf("expected to pick A most of the time, but only picked %d/100 times", aCount)
	}
}

func TestPromptOptimizerJaccardSimilarity(t *testing.T) {
	tests := []struct {
		a, b     string
		minScore float64
		maxScore float64
	}{
		{"fix the login bug", "fix the login bug", 0.99, 1.01},
		{"fix the login bug", "implement new feature", 0.0, 0.2},
		{"fix authentication error", "fix auth bug in login", 0.1, 0.6},
		{"", "", 0.99, 1.01},
		{"hello", "", 0.0, 0.01},
	}

	for _, tt := range tests {
		score := optimizerJaccardSimilarity(tt.a, tt.b)
		if score < tt.minScore || score > tt.maxScore {
			t.Errorf("jaccardSimilarity(%q, %q) = %.3f, expected [%.2f, %.2f]",
				tt.a, tt.b, score, tt.minScore, tt.maxScore)
		}
	}
}

func TestPromptOptimizerTokenize(t *testing.T) {
	tokens := tokenize("Fix the login-bug! (quickly)")
	// Should filter words <= 2 chars and strip punctuation
	for _, tok := range tokens {
		if len(tok) <= 2 {
			t.Errorf("token %q should have been filtered (too short)", tok)
		}
		for _, r := range tok {
			if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
				t.Errorf("token %q contains non-alphanumeric character %c", tok, r)
			}
		}
	}
}

func TestRecordOutcome_MetricsUpdated(t *testing.T) {
	po := NewPromptOptimizer()

	po.RecordOutcome("fix the login bug", "patched", "success", []string{"edit"}, 500)
	po.RecordOutcome("fix the login bug", "tried again", "failure", nil, 1000)

	if len(po.Metrics) == 0 {
		t.Error("expected metrics to be populated")
	}
}

func TestPromptOptimizerConcurrentAccess(t *testing.T) {
	po := NewPromptOptimizer()
	done := make(chan bool, 4)

	// Concurrent writes
	go func() {
		for i := 0; i < 50; i++ {
			po.RecordOutcome("concurrent task alpha", "approach", "success", []string{"edit"}, 100+i)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			po.RecordOutcome("concurrent task beta", "approach", "success", []string{"read"}, 200+i)
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < 50; i++ {
			po.SelectExamples("concurrent task", 3)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			po.BuildOptimizedPrompt("base prompt", "concurrent task")
		}
		done <- true
	}()

	for i := 0; i < 4; i++ {
		<-done
	}
}
