package engine

import (
	"strings"
	"testing"
	"time"
)

func TestNewSelfAssessor(t *testing.T) {
	sa := NewSelfAssessor()
	if sa == nil {
		t.Fatal("NewSelfAssessor returned nil")
	}
	if len(sa.History) != 0 {
		t.Errorf("expected empty history, got %d entries", len(sa.History))
	}
}

func TestAssessGoodTask(t *testing.T) {
	sa := NewSelfAssessor()
	ctx := TaskContext{
		Goal:          "fix a typo in README",
		ToolCalls:     3,
		Errors:        0,
		Retries:       0,
		FilesModified: 1,
		TestsPassed:   true,
		Duration:      10 * time.Second,
		TokensUsed:    1500,
		UserFeedback:  "positive",
	}

	a := sa.Assess(ctx)

	if a.Score < 0.8 {
		t.Errorf("expected high score for good task, got %.2f", a.Score)
	}
	if a.Dimensions["accuracy"] < 0.9 {
		t.Errorf("expected high accuracy for 0-error task, got %.2f", a.Dimensions["accuracy"])
	}
	if a.Dimensions["safety"] < 0.9 {
		t.Errorf("expected high safety score, got %.2f", a.Dimensions["safety"])
	}
	if len(sa.History) != 1 {
		t.Errorf("expected 1 assessment in history, got %d", len(sa.History))
	}
}

func TestAssessPoorTask(t *testing.T) {
	sa := NewSelfAssessor()
	ctx := TaskContext{
		Goal:          "refactor auth module",
		ToolCalls:     20,
		Errors:        4,
		Retries:       5,
		FilesModified: 2,
		TestsPassed:   false,
		Duration:      5 * time.Minute,
		TokensUsed:    15000,
		UserFeedback:  "negative",
	}

	a := sa.Assess(ctx)

	if a.Score > 0.6 {
		t.Errorf("expected low score for poor task, got %.2f", a.Score)
	}
	if a.Dimensions["accuracy"] > 0.5 {
		t.Errorf("expected low accuracy for high-error task, got %.2f", a.Dimensions["accuracy"])
	}
}

func TestIdentifyStrengths(t *testing.T) {
	sa := NewSelfAssessor()
	ctx := TaskContext{
		Goal:          "add helper function",
		ToolCalls:     4,
		Errors:        0,
		Retries:       0,
		FilesModified: 1,
		TestsPassed:   true,
		Duration:      15 * time.Second,
		TokensUsed:    1000,
	}

	strengths := sa.IdentifyStrengths(ctx)

	found := map[string]bool{
		"Completed in single attempt":  false,
		"Low token usage for complexity": false,
		"All tests passing":            false,
		"No errors encountered":        false,
		"Fast completion":              false,
	}

	for _, s := range strengths {
		found[s] = true
	}

	if !found["Completed in single attempt"] {
		t.Error("expected 'Completed in single attempt' strength")
	}
	if !found["Low token usage for complexity"] {
		t.Error("expected 'Low token usage for complexity' strength")
	}
	if !found["All tests passing"] {
		t.Error("expected 'All tests passing' strength")
	}
	if !found["No errors encountered"] {
		t.Error("expected 'No errors encountered' strength")
	}
	if !found["Fast completion"] {
		t.Error("expected 'Fast completion' strength")
	}
}

func TestIdentifyStrengthsWithRetries(t *testing.T) {
	sa := NewSelfAssessor()
	ctx := TaskContext{
		Goal:          "fix bug",
		ToolCalls:     6,
		Errors:        1,
		Retries:       2,
		FilesModified: 1,
		TestsPassed:   true,
		Duration:      45 * time.Second,
		TokensUsed:    3000,
	}

	strengths := sa.IdentifyStrengths(ctx)

	for _, s := range strengths {
		if s == "Completed in single attempt" {
			t.Error("should not report 'Completed in single attempt' with retries")
		}
	}
}

func TestIdentifyWeaknesses(t *testing.T) {
	sa := NewSelfAssessor()
	ctx := TaskContext{
		Goal:          "simple rename",
		ToolCalls:     15,
		Errors:        3,
		Retries:       4,
		FilesModified: 1,
		TestsPassed:   false,
		Duration:      3 * time.Minute,
		TokensUsed:    8000,
	}

	weaknesses := sa.IdentifyWeaknesses(ctx)

	hasRetry := false
	hasToolCalls := false
	hasDuration := false
	hasTokens := false
	hasErrors := false
	for _, w := range weaknesses {
		if strings.Contains(w, "retry") {
			hasRetry = true
		}
		if strings.Contains(w, "tool calls") {
			hasToolCalls = true
		}
		if strings.Contains(w, "longer than expected") {
			hasDuration = true
		}
		if strings.Contains(w, "token") {
			hasTokens = true
		}
		if strings.Contains(w, "errors") {
			hasErrors = true
		}
	}

	if !hasRetry {
		t.Error("expected retry weakness")
	}
	if !hasToolCalls {
		t.Error("expected tool calls weakness")
	}
	if !hasDuration {
		t.Error("expected duration weakness")
	}
	if !hasTokens {
		t.Error("expected token usage weakness")
	}
	if !hasErrors {
		t.Error("expected errors weakness")
	}
}

func TestIdentifyWeaknessesNoIssues(t *testing.T) {
	sa := NewSelfAssessor()
	ctx := TaskContext{
		Goal:          "add comment",
		ToolCalls:     2,
		Errors:        0,
		Retries:       0,
		FilesModified: 1,
		TestsPassed:   true,
		Duration:      5 * time.Second,
		TokensUsed:    500,
	}

	weaknesses := sa.IdentifyWeaknesses(ctx)
	if len(weaknesses) != 0 {
		t.Errorf("expected no weaknesses for good task, got %v", weaknesses)
	}
}

func TestSuggestImprovements(t *testing.T) {
	sa := NewSelfAssessor()
	ctx := TaskContext{
		Goal:          "refactor module",
		ToolCalls:     12,
		Errors:        2,
		Retries:       3,
		FilesModified: 2,
		TestsPassed:   false,
		Duration:      3 * time.Minute,
		TokensUsed:    12000,
	}

	improvements := sa.SuggestImprovements(ctx)

	hasReadFirst := false
	hasGrep := false
	hasTests := false
	hasValidate := false
	hasConcise := false
	for _, imp := range improvements {
		if strings.Contains(imp, "Read files before") {
			hasReadFirst = true
		}
		if strings.Contains(imp, "grep") {
			hasGrep = true
		}
		if strings.Contains(imp, "tests earlier") {
			hasTests = true
		}
		if strings.Contains(imp, "Validate approach") {
			hasValidate = true
		}
		if strings.Contains(imp, "concise") {
			hasConcise = true
		}
	}

	if !hasReadFirst {
		t.Error("expected 'read before edit' improvement")
	}
	if !hasGrep {
		t.Error("expected 'use grep' improvement")
	}
	if !hasTests {
		t.Error("expected 'run tests earlier' improvement")
	}
	if !hasValidate {
		t.Error("expected 'validate approach' improvement")
	}
	if !hasConcise {
		t.Error("expected 'be more concise' improvement")
	}
}

func TestSuggestImprovementsNoneNeeded(t *testing.T) {
	sa := NewSelfAssessor()
	ctx := TaskContext{
		Goal:          "add comment",
		ToolCalls:     2,
		Errors:        0,
		Retries:       0,
		FilesModified: 1,
		TestsPassed:   true,
		Duration:      5 * time.Second,
		TokensUsed:    500,
	}

	improvements := sa.SuggestImprovements(ctx)
	if len(improvements) != 0 {
		t.Errorf("expected no improvements for perfect task, got %v", improvements)
	}
}

func TestGetTrendImproving(t *testing.T) {
	sa := NewSelfAssessor()

	// Add assessments with improving efficiency.
	for i := 0; i < 10; i++ {
		a := Assessment{
			Score: float64(i) * 0.1,
			Dimensions: map[string]float64{
				"efficiency": 0.5 + float64(i)*0.05,
			},
			Timestamp: time.Now(),
		}
		sa.History = append(sa.History, a)
	}

	trend := sa.GetTrend("efficiency")
	if trend != "improving" {
		t.Errorf("expected 'improving', got %q", trend)
	}
}

func TestGetTrendDeclining(t *testing.T) {
	sa := NewSelfAssessor()

	// Add assessments with declining accuracy.
	for i := 0; i < 10; i++ {
		a := Assessment{
			Score: 0.8,
			Dimensions: map[string]float64{
				"accuracy": 0.95 - float64(i)*0.05,
			},
			Timestamp: time.Now(),
		}
		sa.History = append(sa.History, a)
	}

	trend := sa.GetTrend("accuracy")
	if trend != "declining" {
		t.Errorf("expected 'declining', got %q", trend)
	}
}

func TestGetTrendStable(t *testing.T) {
	sa := NewSelfAssessor()

	// Add assessments with stable scores.
	for i := 0; i < 10; i++ {
		a := Assessment{
			Score: 0.8,
			Dimensions: map[string]float64{
				"speed": 0.75,
			},
			Timestamp: time.Now(),
		}
		sa.History = append(sa.History, a)
	}

	trend := sa.GetTrend("speed")
	if trend != "stable" {
		t.Errorf("expected 'stable', got %q", trend)
	}
}

func TestGetTrendInsufficientHistory(t *testing.T) {
	sa := NewSelfAssessor()

	// Only 2 assessments - not enough for trend analysis.
	sa.History = append(sa.History, Assessment{
		Dimensions: map[string]float64{"efficiency": 0.5},
	})
	sa.History = append(sa.History, Assessment{
		Dimensions: map[string]float64{"efficiency": 0.9},
	})

	trend := sa.GetTrend("efficiency")
	if trend != "stable" {
		t.Errorf("expected 'stable' with insufficient history, got %q", trend)
	}
}

func TestFormatSelfAssessment(t *testing.T) {
	a := &Assessment{
		Score: 0.82,
		Dimensions: map[string]float64{
			"efficiency":   0.75,
			"accuracy":     0.90,
			"completeness": 0.95,
			"speed":        0.70,
			"safety":       1.00,
		},
		Strengths:    []string{"completed goal", "tests pass"},
		Weaknesses:   []string{"3 retries", "high token usage"},
		Improvements: []string{"read before edit", "grep first"},
	}

	output := FormatSelfAssessment(a)

	if !strings.Contains(output, "0.82/1.00") {
		t.Error("expected overall score in output")
	}
	if !strings.Contains(output, "Efficiency:") {
		t.Error("expected efficiency dimension in output")
	}
	if !strings.Contains(output, "Accuracy:") {
		t.Error("expected accuracy dimension in output")
	}
	if !strings.Contains(output, "Completeness:") {
		t.Error("expected completeness dimension in output")
	}
	if !strings.Contains(output, "Speed:") {
		t.Error("expected speed dimension in output")
	}
	if !strings.Contains(output, "Safety:") {
		t.Error("expected safety dimension in output")
	}
	if !strings.Contains(output, "█") {
		t.Error("expected bar characters in output")
	}
	if !strings.Contains(output, "Strengths:") {
		t.Error("expected strengths section")
	}
	if !strings.Contains(output, "Weaknesses:") {
		t.Error("expected weaknesses section")
	}
	if !strings.Contains(output, "Improvements:") {
		t.Error("expected improvements section")
	}
	if !strings.Contains(output, "completed goal") {
		t.Error("expected strength content in output")
	}
}

func TestFormatSelfAssessmentEmptySections(t *testing.T) {
	a := &Assessment{
		Score: 1.0,
		Dimensions: map[string]float64{
			"efficiency":   1.0,
			"accuracy":     1.0,
			"completeness": 1.0,
			"speed":        1.0,
			"safety":       1.0,
		},
		Strengths:    nil,
		Weaknesses:   nil,
		Improvements: nil,
	}

	output := FormatSelfAssessment(a)

	if strings.Contains(output, "Strengths:") {
		t.Error("should not show Strengths section when empty")
	}
	if strings.Contains(output, "Weaknesses:") {
		t.Error("should not show Weaknesses section when empty")
	}
	if strings.Contains(output, "Improvements:") {
		t.Error("should not show Improvements section when empty")
	}
}

func TestAverageScore(t *testing.T) {
	sa := NewSelfAssessor()

	// No history.
	if avg := sa.AverageScore(5); avg != 0.0 {
		t.Errorf("expected 0.0 for empty history, got %.2f", avg)
	}

	// Add some assessments.
	scores := []float64{0.70, 0.80, 0.90, 0.60, 0.85}
	for _, s := range scores {
		sa.mu.Lock()
		sa.History = append(sa.History, Assessment{Score: s})
		sa.mu.Unlock()
	}

	// Average of last 3: (0.90 + 0.60 + 0.85) / 3 = 0.783...
	avg3 := sa.AverageScore(3)
	if avg3 < 0.78 || avg3 > 0.79 {
		t.Errorf("expected ~0.78 for last 3, got %.2f", avg3)
	}

	// Average of all 5: (0.70 + 0.80 + 0.90 + 0.60 + 0.85) / 5 = 0.77
	avg5 := sa.AverageScore(5)
	if avg5 < 0.76 || avg5 > 0.78 {
		t.Errorf("expected ~0.77 for all 5, got %.2f", avg5)
	}

	// n=0 means all.
	avgAll := sa.AverageScore(0)
	if avgAll != avg5 {
		t.Errorf("expected n=0 to equal all, got %.2f vs %.2f", avgAll, avg5)
	}

	// n exceeds history.
	avgOver := sa.AverageScore(100)
	if avgOver != avg5 {
		t.Errorf("expected n>len to equal all, got %.2f vs %.2f", avgOver, avg5)
	}
}

func TestAssessmentBar(t *testing.T) {
	tests := []struct {
		score    float64
		expected string
	}{
		{0.0, "░░░░░░░░░░"},
		{0.5, "█████░░░░░"},
		{1.0, "██████████"},
		{0.3, "███░░░░░░░"},
		{0.75, "████████░░"},
	}

	for _, tt := range tests {
		got := assessmentBar(tt.score)
		if got != tt.expected {
			t.Errorf("assessmentBar(%.2f) = %q, want %q", tt.score, got, tt.expected)
		}
	}
}

func TestAssessmentClampScore(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{-0.5, 0.0},
		{0.0, 0.0},
		{0.5, 0.5},
		{1.0, 1.0},
		{1.5, 1.0},
	}

	for _, tt := range tests {
		got := clampScore(tt.input)
		if got != tt.expected {
			t.Errorf("clampScore(%.2f) = %.2f, want %.2f", tt.input, got, tt.expected)
		}
	}
}

func TestAssessmentClassifyTask(t *testing.T) {
	tests := []struct {
		name     string
		ctx      TaskContext
		expected string
	}{
		{
			name:     "research task",
			ctx:      TaskContext{FilesModified: 0, ToolCalls: 5},
			expected: "research",
		},
		{
			name:     "feature task",
			ctx:      TaskContext{FilesModified: 5, TestsPassed: true, ToolCalls: 10},
			expected: "feature",
		},
		{
			name:     "quick fix",
			ctx:      TaskContext{FilesModified: 1, ToolCalls: 3},
			expected: "quick-fix",
		},
		{
			name:     "general edit",
			ctx:      TaskContext{FilesModified: 2, ToolCalls: 8},
			expected: "edit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyTask(tt.ctx)
			if got != tt.expected {
				t.Errorf("classifyTask() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAssessConcurrency(t *testing.T) {
	sa := NewSelfAssessor()
	done := make(chan struct{})

	// Run multiple assessments concurrently.
	for i := 0; i < 20; i++ {
		go func(n int) {
			ctx := TaskContext{
				Goal:          "concurrent task",
				ToolCalls:     n + 1,
				Errors:        n % 3,
				Retries:       n % 2,
				FilesModified: 1,
				TestsPassed:   n%2 == 0,
				Duration:      time.Duration(n) * time.Second,
				TokensUsed:    1000 + n*100,
			}
			sa.Assess(ctx)
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	sa.mu.RLock()
	histLen := len(sa.History)
	sa.mu.RUnlock()

	if histLen != 20 {
		t.Errorf("expected 20 assessments, got %d", histLen)
	}
}

func TestAssessStoresTimestamp(t *testing.T) {
	sa := NewSelfAssessor()
	before := time.Now()
	ctx := TaskContext{
		Goal:          "timestamp test",
		ToolCalls:     1,
		FilesModified: 1,
		TestsPassed:   true,
		Duration:      time.Second,
		TokensUsed:    100,
	}
	a := sa.Assess(ctx)
	after := time.Now()

	if a.Timestamp.Before(before) || a.Timestamp.After(after) {
		t.Errorf("timestamp %v not between %v and %v", a.Timestamp, before, after)
	}
}

func TestScoreDimensions(t *testing.T) {
	sa := NewSelfAssessor()
	ctx := TaskContext{
		Goal:          "test dimensions",
		ToolCalls:     5,
		Errors:        0,
		Retries:       0,
		FilesModified: 2,
		TestsPassed:   true,
		Duration:      20 * time.Second,
		TokensUsed:    2000,
		UserFeedback:  "positive",
	}

	a := sa.Assess(ctx)

	for dim, score := range a.Dimensions {
		if score < 0.0 || score > 1.0 {
			t.Errorf("dimension %q score %.2f out of [0,1] range", dim, score)
		}
	}

	expectedDims := []string{"efficiency", "accuracy", "completeness", "speed", "safety"}
	for _, dim := range expectedDims {
		if _, ok := a.Dimensions[dim]; !ok {
			t.Errorf("missing dimension %q", dim)
		}
	}
}
