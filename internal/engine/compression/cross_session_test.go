package compression

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewCrossSessionLearner(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	if learner == nil {
		t.Fatal("expected non-nil learner")
	}
	if learner.Dir != dir {
		t.Errorf("expected dir %q, got %q", dir, learner.Dir)
	}
	if len(learner.Insights) != 0 {
		t.Errorf("expected 0 insights, got %d", len(learner.Insights))
	}
	if len(learner.Conventions) != 0 {
		t.Errorf("expected 0 conventions, got %d", len(learner.Conventions))
	}
	if len(learner.FailurePatterns) != 0 {
		t.Errorf("expected 0 failure patterns, got %d", len(learner.FailurePatterns))
	}
}

func TestLearnFromOutcome_Success(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	learner.LearnFromOutcome(
		"fix authentication bug",
		"check token expiry first",
		true,
		[]string{"grep", "edit"},
		[]string{"auth.go", "token.go"},
	)

	if len(learner.Insights) != 1 {
		t.Fatalf("expected 1 insight, got %d", len(learner.Insights))
	}

	ins := learner.Insights[0]
	if ins.Language != "go" {
		t.Errorf("expected language 'go', got %q", ins.Language)
	}
	if ins.Confidence != 0.6 {
		t.Errorf("expected confidence 0.6, got %f", ins.Confidence)
	}
	if ins.SuccessCount != 1 {
		t.Errorf("expected success count 1, got %d", ins.SuccessCount)
	}
	if !strings.Contains(ins.Content, "fix authentication bug") {
		t.Errorf("expected content to contain task, got %q", ins.Content)
	}
	if !strings.Contains(ins.Content, "grep, edit") {
		t.Errorf("expected content to contain tools, got %q", ins.Content)
	}
}

func TestLearnFromOutcome_Failure(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	learner.LearnFromOutcome(
		"deploy service",
		"direct push to prod",
		false,
		[]string{"bash"},
		[]string{"deploy.sh"},
	)

	if len(learner.Insights) != 0 {
		t.Errorf("expected 0 insights for failure, got %d", len(learner.Insights))
	}
	if len(learner.FailurePatterns) != 1 {
		t.Fatalf("expected 1 failure pattern, got %d", len(learner.FailurePatterns))
	}

	fp := learner.FailurePatterns[0]
	if !strings.Contains(fp.Pattern, "deploy service") {
		t.Errorf("expected pattern to contain task, got %q", fp.Pattern)
	}
	if fp.Occurrences != 1 {
		t.Errorf("expected 1 occurrence, got %d", fp.Occurrences)
	}
}

func TestLearnFromOutcome_DuplicateBoostsConfidence(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	learner.LearnFromOutcome(
		"fix authentication bug",
		"use token refresh",
		true,
		[]string{"edit"},
		[]string{"auth.go"},
	)

	initialConf := learner.Insights[0].Confidence

	// Same task/category should boost existing insight
	learner.LearnFromOutcome(
		"fix authentication bug",
		"use token refresh logic",
		true,
		[]string{"edit"},
		[]string{"auth.go"},
	)

	if len(learner.Insights) != 1 {
		t.Fatalf("expected 1 insight (deduplicated), got %d", len(learner.Insights))
	}
	if learner.Insights[0].Confidence <= initialConf {
		t.Error("expected confidence to increase on repeated success")
	}
	if learner.Insights[0].SuccessCount != 2 {
		t.Errorf("expected success count 2, got %d", learner.Insights[0].SuccessCount)
	}
}

func TestLearnConvention(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	learner.LearnConvention(
		"Always wrap errors with %w",
		[]string{"return fmt.Errorf(\"open: %w\", err)"},
		"code review",
	)

	if len(learner.Conventions) != 1 {
		t.Fatalf("expected 1 convention, got %d", len(learner.Conventions))
	}

	conv := learner.Conventions[0]
	if conv.Rule != "Always wrap errors with %w" {
		t.Errorf("unexpected rule: %q", conv.Rule)
	}
	if conv.Source != "code review" {
		t.Errorf("unexpected source: %q", conv.Source)
	}
	if conv.Confidence != 0.7 {
		t.Errorf("expected confidence 0.7, got %f", conv.Confidence)
	}
	if conv.AppliedCount != 1 {
		t.Errorf("expected applied count 1, got %d", conv.AppliedCount)
	}
	if len(conv.Examples) != 1 {
		t.Errorf("expected 1 example, got %d", len(conv.Examples))
	}
}

func TestLearnConvention_Duplicate(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	learner.LearnConvention(
		"Use chi router middleware",
		[]string{"r.Use(authMiddleware)"},
		"codebase analysis",
	)
	learner.LearnConvention(
		"Use chi router middleware",
		[]string{"r.Use(logMiddleware)"},
		"codebase analysis",
	)

	if len(learner.Conventions) != 1 {
		t.Fatalf("expected 1 convention (deduplicated), got %d", len(learner.Conventions))
	}

	conv := learner.Conventions[0]
	if conv.AppliedCount != 2 {
		t.Errorf("expected applied count 2, got %d", conv.AppliedCount)
	}
	if len(conv.Examples) != 2 {
		t.Errorf("expected 2 examples after merge, got %d", len(conv.Examples))
	}
}

func TestRecordFailure(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	learner.RecordFailure(
		"SQLite concurrent write",
		"multiple goroutines writing simultaneously",
		"use WAL mode",
	)

	if len(learner.FailurePatterns) != 1 {
		t.Fatalf("expected 1 failure pattern, got %d", len(learner.FailurePatterns))
	}

	fp := learner.FailurePatterns[0]
	if fp.Pattern != "SQLite concurrent write" {
		t.Errorf("unexpected pattern: %q", fp.Pattern)
	}
	if fp.Context != "multiple goroutines writing simultaneously" {
		t.Errorf("unexpected context: %q", fp.Context)
	}
	if fp.Resolution != "use WAL mode" {
		t.Errorf("unexpected resolution: %q", fp.Resolution)
	}
	if fp.Occurrences != 1 {
		t.Errorf("expected 1 occurrence, got %d", fp.Occurrences)
	}
}

func TestRecordFailure_Duplicate(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	learner.RecordFailure("timeout error", "API call", "")
	learner.RecordFailure("timeout error", "API call", "increase timeout to 30s")

	if len(learner.FailurePatterns) != 1 {
		t.Fatalf("expected 1 failure pattern (deduplicated), got %d", len(learner.FailurePatterns))
	}

	fp := learner.FailurePatterns[0]
	if fp.Occurrences != 2 {
		t.Errorf("expected 2 occurrences, got %d", fp.Occurrences)
	}
	if fp.Resolution != "increase timeout to 30s" {
		t.Errorf("expected resolution to be updated, got %q", fp.Resolution)
	}
}

func TestGetRelevantInsights(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	// Add several insights
	now := time.Now()
	learner.Insights = []Insight{
		{
			ID:         "i1",
			Content:    "For authentication tasks, always check token expiry first",
			Category:   "approach",
			Language:   "go",
			Confidence: 0.9,
			CreatedAt:  now,
			LastUsed:   now,
		},
		{
			ID:         "i2",
			Content:    "For database migrations, backup before altering tables",
			Category:   "approach",
			Language:   "go",
			Confidence: 0.8,
			CreatedAt:  now,
			LastUsed:   now,
		},
		{
			ID:         "i3",
			Content:    "Use table-driven tests in this project",
			Category:   "preference",
			Language:   "go",
			Confidence: 0.95,
			CreatedAt:  now,
			LastUsed:   now,
		},
	}

	results := learner.GetRelevantInsights("fix authentication token handling", 2)
	if len(results) == 0 {
		t.Fatal("expected at least 1 relevant insight")
	}
	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}

	// The auth-related insight should rank highest
	if !strings.Contains(results[0].Content, "authentication") && !strings.Contains(results[0].Content, "token") {
		t.Errorf("expected top result to be auth-related, got %q", results[0].Content)
	}
}

func TestGetRelevantInsights_Empty(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	results := learner.GetRelevantInsights("anything", 5)
	if results != nil {
		t.Errorf("expected nil for empty learner, got %v", results)
	}
}

func TestGetConventions(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	learner.LearnConvention("wrap errors with %w", []string{"example1"}, "code review")
	learner.LearnConvention("use chi router", []string{"example2"}, "project docs")

	convs := learner.GetConventions()
	if len(convs) != 2 {
		t.Fatalf("expected 2 conventions, got %d", len(convs))
	}

	// Verify it returns a copy
	convs[0].Rule = "modified"
	original := learner.GetConventions()
	if original[0].Rule == "modified" {
		t.Error("GetConventions should return a copy, not a reference")
	}
}

func TestGetFailureResolutions(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	learner.RecordFailure(
		"database locked SQLite",
		"concurrent writes",
		"enable WAL mode",
	)
	learner.RecordFailure(
		"connection refused port 5432",
		"postgres not running",
		"start postgres service",
	)

	results := learner.GetFailureResolutions("SQLite database is locked")
	if len(results) == 0 {
		t.Fatal("expected at least 1 matching failure")
	}
	if results[0].Resolution != "enable WAL mode" {
		t.Errorf("expected WAL mode resolution, got %q", results[0].Resolution)
	}
}

func TestGetFailureResolutions_NoMatch(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	learner.RecordFailure("specific rare error", "unusual context", "unique fix")

	results := learner.GetFailureResolutions("completely unrelated problem")
	if len(results) != 0 {
		t.Errorf("expected 0 results for unrelated query, got %d", len(results))
	}
}

func TestBuildSessionPrimer(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	now := time.Now()
	learner.Insights = []Insight{
		{
			ID:         "i1",
			Content:    "For auth issues check token expiry first",
			Category:   "approach",
			Language:   "go",
			Confidence: 0.9,
			CreatedAt:  now,
			LastUsed:   now,
		},
	}
	learner.Conventions = []SessionConvention{
		{
			ID:   "c1",
			Rule: "Always wrap errors with %w",
		},
	}
	learner.FailurePatterns = []FailurePattern{
		{
			ID:         "f1",
			Pattern:    "SQLite concurrent write issue",
			Resolution: "use WAL mode",
		},
	}

	primer := learner.BuildSessionPrimer("fix auth token bug")

	if !strings.Contains(primer, "## Cross-Session Learning") {
		t.Error("expected primer to contain header")
	}
	if !strings.Contains(primer, "### Relevant Insights") {
		t.Error("expected primer to contain insights section")
	}
	if !strings.Contains(primer, "### Conventions") {
		t.Error("expected primer to contain conventions section")
	}
	if !strings.Contains(primer, "### Known Pitfalls") {
		t.Error("expected primer to contain pitfalls section")
	}
	if !strings.Contains(primer, "wrap errors with %w") {
		t.Error("expected primer to contain convention rule")
	}
	if !strings.Contains(primer, "WAL mode") {
		t.Error("expected primer to contain failure resolution")
	}
}

func TestBuildSessionPrimer_Empty(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	primer := learner.BuildSessionPrimer("any task")
	if !strings.Contains(primer, "## Cross-Session Learning") {
		t.Error("expected header even when empty")
	}
	// Should not contain sections when empty
	if strings.Contains(primer, "### Relevant Insights") {
		t.Error("should not have insights section when empty")
	}
}

func TestDecay(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	now := time.Now()
	learner.Insights = []Insight{
		{ID: "i1", Content: "test insight", Confidence: 0.9, CreatedAt: now, LastUsed: now},
		{ID: "i2", Content: "another insight", Confidence: 0.5, CreatedAt: now, LastUsed: now},
	}
	learner.Conventions = []SessionConvention{
		{ID: "c1", Rule: "test rule", Confidence: 0.8},
	}

	learner.Decay(0.9)

	if math.Abs(learner.Insights[0].Confidence-0.9*0.9) > 1e-9 {
		t.Errorf("expected confidence %.2f, got %.2f", 0.9*0.9, learner.Insights[0].Confidence)
	}
	if math.Abs(learner.Insights[1].Confidence-0.5*0.9) > 1e-9 {
		t.Errorf("expected confidence %.2f, got %.2f", 0.5*0.9, learner.Insights[1].Confidence)
	}
	if math.Abs(learner.Conventions[0].Confidence-0.8*0.9) > 1e-9 {
		t.Errorf("expected convention confidence %.2f, got %.2f", 0.8*0.9, learner.Conventions[0].Confidence)
	}
}

func TestDecay_Floor(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	learner.Insights = []Insight{
		{ID: "i1", Content: "barely alive", Confidence: 0.02},
	}

	learner.Decay(0.1)

	// Should not go below 0.01
	if learner.Insights[0].Confidence < 0.01 {
		t.Errorf("confidence should not go below 0.01, got %f", learner.Insights[0].Confidence)
	}
}

func TestCrossSessionSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	now := time.Now().Truncate(time.Millisecond) // Truncate for JSON round-trip

	learner.Insights = []Insight{
		{
			ID:           "i1",
			Content:      "test insight content",
			Category:     "approach",
			Language:     "go",
			Confidence:   0.85,
			SuccessCount: 3,
			CreatedAt:    now,
			LastUsed:     now,
		},
	}
	learner.Conventions = []SessionConvention{
		{
			ID:           "c1",
			Rule:         "wrap errors",
			Examples:     []string{"fmt.Errorf(\"%w\", err)"},
			Source:       "review",
			Confidence:   0.9,
			AppliedCount: 5,
		},
	}
	learner.FailurePatterns = []FailurePattern{
		{
			ID:          "f1",
			Pattern:     "deadlock detected",
			Context:     "concurrent map access",
			Resolution:  "use sync.RWMutex",
			Language:    "go",
			Occurrences: 2,
			LastSeen:    now,
		},
	}

	if err := learner.Save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "cross_session.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}

	// Load into a new learner
	loaded := NewCrossSessionLearner(dir)
	if err := loaded.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(loaded.Insights) != 1 {
		t.Fatalf("expected 1 insight after load, got %d", len(loaded.Insights))
	}
	if loaded.Insights[0].Content != "test insight content" {
		t.Errorf("insight content mismatch: %q", loaded.Insights[0].Content)
	}
	if loaded.Insights[0].Confidence != 0.85 {
		t.Errorf("insight confidence mismatch: %f", loaded.Insights[0].Confidence)
	}

	if len(loaded.Conventions) != 1 {
		t.Fatalf("expected 1 convention after load, got %d", len(loaded.Conventions))
	}
	if loaded.Conventions[0].Rule != "wrap errors" {
		t.Errorf("convention rule mismatch: %q", loaded.Conventions[0].Rule)
	}

	if len(loaded.FailurePatterns) != 1 {
		t.Fatalf("expected 1 failure pattern after load, got %d", len(loaded.FailurePatterns))
	}
	if loaded.FailurePatterns[0].Resolution != "use sync.RWMutex" {
		t.Errorf("failure resolution mismatch: %q", loaded.FailurePatterns[0].Resolution)
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	// Should not error on missing file
	if err := learner.Load(); err != nil {
		t.Fatalf("load should not error on missing file: %v", err)
	}
}

func TestCrossSessionStats(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	learner.Insights = []Insight{
		{ID: "i1", Confidence: 0.8},
		{ID: "i2", Confidence: 0.6},
	}
	learner.Conventions = []SessionConvention{
		{ID: "c1"},
		{ID: "c2"},
		{ID: "c3"},
	}
	learner.FailurePatterns = []FailurePattern{
		{ID: "f1"},
	}

	stats := learner.Stats()

	if stats.InsightCount != 2 {
		t.Errorf("expected 2 insights, got %d", stats.InsightCount)
	}
	if stats.ConventionCount != 3 {
		t.Errorf("expected 3 conventions, got %d", stats.ConventionCount)
	}
	if stats.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", stats.FailureCount)
	}

	expectedAvg := (0.8 + 0.6) / 2.0
	if stats.AvgConfidence != expectedAvg {
		t.Errorf("expected avg confidence %f, got %f", expectedAvg, stats.AvgConfidence)
	}
}

func TestCrossSessionStats_Empty(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	stats := learner.Stats()
	if stats.InsightCount != 0 || stats.ConventionCount != 0 || stats.FailureCount != 0 {
		t.Error("expected all counts to be 0")
	}
	if stats.AvgConfidence != 0 {
		t.Errorf("expected avg confidence 0, got %f", stats.AvgConfidence)
	}
}

func TestDetectLanguageFromFiles(t *testing.T) {
	tests := []struct {
		files    []string
		expected string
	}{
		{[]string{"main.go", "handler.go"}, "go"},
		{[]string{"app.py", "utils.py"}, "python"},
		{[]string{"index.ts", "app.tsx"}, "typescript"},
		{[]string{"bundle.js", "app.jsx"}, "javascript"},
		{[]string{"lib.rs", "main.rs"}, "rust"},
		{[]string{}, "generic"},
		{[]string{"Makefile", "Dockerfile"}, "generic"},
	}

	for _, tt := range tests {
		got := detectLanguageFromFiles(tt.files)
		if got != tt.expected {
			t.Errorf("detectLanguageFromFiles(%v) = %q, want %q", tt.files, got, tt.expected)
		}
	}
}

func TestCategorizeApproach(t *testing.T) {
	tests := []struct {
		approach string
		tools    []string
		expected string
	}{
		{"avoid using global state", nil, "avoidance"},
		{"don't modify the API contract", nil, "avoidance"},
		{"prefer composition over inheritance", nil, "preference"},
		{"always use context for cancellation", nil, "preference"},
		{"used grep to find references", []string{"grep"}, "tool_usage"},
		{"refactored the handler", nil, "approach"},
	}

	for _, tt := range tests {
		got := categorizeApproach(tt.approach, tt.tools)
		if got != tt.expected {
			t.Errorf("categorizeApproach(%q, %v) = %q, want %q", tt.approach, tt.tools, got, tt.expected)
		}
	}
}

func TestCrossSessionTokenize(t *testing.T) {
	words := tokenize("Fix the authentication-token bug in auth.go")
	// Should include words > 2 chars
	if len(words) == 0 {
		t.Fatal("expected non-empty tokenization")
	}
	for _, w := range words {
		if len(w) <= 2 {
			t.Errorf("found short word %q, expected > 2 chars", w)
		}
	}
	// "fix" should be included (3 chars)
	found := false
	for _, w := range words {
		if w == "fix" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'fix' in tokenized output")
	}
}

func TestWordOverlap(t *testing.T) {
	a := []string{"fix", "auth", "token", "bug"}
	b := []string{"auth", "token", "expiry", "check"}

	overlap := wordOverlap(a, b)
	// 2 matches (auth, token) out of 4 words in a = 0.5
	if overlap != 0.5 {
		t.Errorf("expected overlap 0.5, got %f", overlap)
	}

	// Empty sets
	if wordOverlap(nil, b) != 0 {
		t.Error("expected 0 overlap with nil a")
	}
	if wordOverlap(a, nil) != 0 {
		t.Error("expected 0 overlap with nil b")
	}
}

func TestCrossSessionConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	done := make(chan struct{})

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			learner.LearnFromOutcome("task", "approach", true, nil, []string{"file.go"})
			learner.LearnConvention("rule", []string{"ex"}, "src")
			learner.RecordFailure("pattern", "ctx", "fix")
		}
		close(done)
	}()

	// Reader goroutine
	for i := 0; i < 100; i++ {
		_ = learner.GetRelevantInsights("task", 5)
		_ = learner.GetConventions()
		_ = learner.GetFailureResolutions("error")
		_ = learner.BuildSessionPrimer("task")
		_ = learner.Stats()
	}

	<-done
}

func TestSaveCreatesDirectory(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "deep", "dir")
	learner := NewCrossSessionLearner(dir)

	learner.LearnConvention("test rule", nil, "test")

	if err := learner.Save(); err != nil {
		t.Fatalf("save should create nested directories: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "cross_session.json")); err != nil {
		t.Fatalf("file should exist after save: %v", err)
	}
}

func TestClampConfidence(t *testing.T) {
	if clampConfidence(1.5) != 1.0 {
		t.Error("expected clamped to 1.0")
	}
	if clampConfidence(-0.5) != 0.0 {
		t.Error("expected clamped to 0.0")
	}
	if clampConfidence(0.5) != 0.5 {
		t.Error("expected 0.5 to remain unchanged")
	}
}

func TestGetRelevantInsights_Recency(t *testing.T) {
	dir := t.TempDir()
	learner := NewCrossSessionLearner(dir)

	now := time.Now()
	old := now.Add(-365 * 24 * time.Hour) // 1 year ago

	learner.Insights = []Insight{
		{
			ID:         "old",
			Content:    "For auth tasks check the token handler",
			Category:   "approach",
			Confidence: 0.9,
			CreatedAt:  old,
			LastUsed:   old,
		},
		{
			ID:         "recent",
			Content:    "For auth tasks validate the token refresh",
			Category:   "approach",
			Confidence: 0.7,
			CreatedAt:  now,
			LastUsed:   now,
		},
	}

	results := learner.GetRelevantInsights("auth token handling", 2)
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Recent insight should rank higher due to recency even with lower confidence
	if results[0].ID != "recent" {
		t.Errorf("expected recent insight to rank first, got %q", results[0].ID)
	}
}
