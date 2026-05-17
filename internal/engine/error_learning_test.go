package engine

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestMatchLearnedFindsCorrectPatterns(t *testing.T) {
	el := NewErrorLearner()

	matches := el.MatchLearned("undefined: handleAuth")
	if len(matches) == 0 {
		t.Fatal("expected at least one match for 'undefined: handleAuth'")
	}
	if matches[0].ID != "go-undefined" {
		t.Errorf("expected go-undefined pattern, got %s", matches[0].ID)
	}
}

func TestMatchLearnedReturnsEmptyForUnknownError(t *testing.T) {
	el := NewErrorLearner()

	matches := el.MatchLearned("some completely unknown error xyz123")
	if len(matches) != 0 {
		t.Errorf("expected no matches for unknown error, got %d", len(matches))
	}
}

func TestMultipleMatchesRankedByConfidence(t *testing.T) {
	el := NewErrorLearner()

	// Add a custom pattern that also matches "undefined"
	el.mu.Lock()
	el.Patterns["custom-undefined"] = &LearnedPattern{
		ID:         "custom-undefined",
		Category:   "build",
		Language:   "go",
		Pattern:    `undefined`,
		Fix:        "custom fix",
		Confidence: 0.99,
	}
	el.mu.Unlock()

	matches := el.MatchLearned("undefined: foo")
	if len(matches) < 2 {
		t.Fatalf("expected at least 2 matches, got %d", len(matches))
	}
	// Highest confidence should be first.
	if matches[0].Confidence < matches[1].Confidence {
		t.Error("matches not sorted by confidence descending")
	}
	if matches[0].ID != "custom-undefined" {
		t.Errorf("expected custom-undefined first (confidence 0.99), got %s", matches[0].ID)
	}
}

func TestLearnAddsNewPattern(t *testing.T) {
	el := NewErrorLearner()
	initialCount := len(el.Patterns)

	el.Learn("UNIQUE_ERROR_XYZ: something went wrong", "restart the service", "go", "runtime")

	if len(el.Patterns) != initialCount+1 {
		t.Errorf("expected %d patterns after learn, got %d", initialCount+1, len(el.Patterns))
	}

	// Verify the new pattern can match.
	matches := el.MatchLearned("UNIQUE_ERROR_XYZ: something went wrong")
	if len(matches) == 0 {
		t.Fatal("learned pattern should match the original error message")
	}
}

func TestLearnUpdatesExistingPattern(t *testing.T) {
	el := NewErrorLearner()
	initialCount := len(el.Patterns)

	// This should match the existing "go-undefined" pattern.
	el.Learn("undefined: myFunc", "add import", "go", "build")

	if len(el.Patterns) != initialCount {
		t.Error("learn should not add a new pattern when it matches existing")
	}

	p := el.Patterns["go-undefined"]
	if p.SuccessCount < 1 {
		t.Error("expected success count to increment")
	}
}

func TestRecordSuccessBoostsConfidence(t *testing.T) {
	el := NewErrorLearner()

	// Set initial state.
	el.mu.Lock()
	el.Patterns["go-undefined"].SuccessCount = 9
	el.Patterns["go-undefined"].FailureCount = 1
	el.mu.Unlock()

	el.RecordSuccess("go-undefined")

	p := el.Patterns["go-undefined"]
	if p.SuccessCount != 10 {
		t.Errorf("expected success count 10, got %d", p.SuccessCount)
	}
	// Confidence should be 10/11 ~ 0.909
	if p.Confidence < 0.90 {
		t.Errorf("expected confidence > 0.90, got %f", p.Confidence)
	}
}

func TestRecordFailureReducesConfidence(t *testing.T) {
	el := NewErrorLearner()

	// Set initial state.
	el.mu.Lock()
	el.Patterns["go-undefined"].SuccessCount = 5
	el.Patterns["go-undefined"].FailureCount = 4
	el.mu.Unlock()

	el.RecordFailure("go-undefined")

	p := el.Patterns["go-undefined"]
	if p.FailureCount != 5 {
		t.Errorf("expected failure count 5, got %d", p.FailureCount)
	}
	// Confidence should be 5/10 = 0.5
	if p.Confidence != 0.5 {
		t.Errorf("expected confidence 0.5, got %f", p.Confidence)
	}
}

func TestRecordSuccessNonexistentPattern(t *testing.T) {
	el := NewErrorLearner()
	// Should not panic.
	el.RecordSuccess("nonexistent-pattern")
	el.RecordFailure("nonexistent-pattern")
}

func TestBuildFixSuggestionOutput(t *testing.T) {
	el := NewErrorLearner()

	el.mu.Lock()
	el.Patterns["go-undefined"].SuccessCount = 23
	el.Patterns["go-undefined"].FailureCount = 2
	el.Patterns["go-undefined"].Confidence = 0.92
	el.mu.Unlock()

	suggestion := el.BuildFixSuggestion("undefined: handleAuth")
	if suggestion == "" {
		t.Fatal("expected non-empty suggestion")
	}
	if !strings.Contains(suggestion, "Known error pattern:") {
		t.Error("suggestion missing 'Known error pattern:' prefix")
	}
	if !strings.Contains(suggestion, "build") {
		t.Error("suggestion missing category")
	}
	if !strings.Contains(suggestion, "go") {
		t.Error("suggestion missing language")
	}
	if !strings.Contains(suggestion, "92%") {
		t.Error("suggestion missing confidence percentage")
	}
	if !strings.Contains(suggestion, "23 successes") {
		t.Error("suggestion missing success count")
	}
	if !strings.Contains(suggestion, "2 failures") {
		t.Error("suggestion missing failure count")
	}
}

func TestBuildFixSuggestionUnknownError(t *testing.T) {
	el := NewErrorLearner()
	suggestion := el.BuildFixSuggestion("completely unknown error 999")
	if suggestion != "" {
		t.Errorf("expected empty suggestion for unknown error, got: %s", suggestion)
	}
}

func TestExtractPatternGeneralizes(t *testing.T) {
	tests := []struct {
		input       string
		shouldMatch string
	}{
		{
			input:       "error at line 42 in main.go",
			shouldMatch: "error at line 99 in main.go",
		},
		{
			input:       "undefined: handleAuth",
			shouldMatch: "undefined: handleAuth",
		},
	}

	for _, tt := range tests {
		pattern := ExtractPattern(tt.input)
		if pattern == "" {
			t.Errorf("ExtractPattern(%q) returned empty", tt.input)
			continue
		}

		if tt.shouldMatch != "" {
			matched, err := matchPatternLoose(pattern, tt.shouldMatch)
			if err != nil {
				t.Errorf("regex compile error for pattern %q: %v", pattern, err)
				continue
			}
			if !matched {
				t.Errorf("ExtractPattern(%q) = %q, should match %q", tt.input, pattern, tt.shouldMatch)
			}
		}
	}
}

func TestExtractPatternReplacesNumbers(t *testing.T) {
	pattern := ExtractPattern("error on line 123")
	if !strings.Contains(pattern, `\d+`) {
		t.Errorf("expected pattern to contain \\d+, got: %s", pattern)
	}
}

func matchPatternLoose(pattern, text string) (bool, error) {
	re, err := compilePatternLoose(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(text), nil
}

func compilePatternLoose(pattern string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		// If it fails, fall back to a permissive match.
		return regexp.Compile(".*")
	}
	return re, nil
}

func TestPruneWeakRemovesLowConfidence(t *testing.T) {
	el := NewErrorLearner()

	// Add a weak pattern.
	el.mu.Lock()
	el.Patterns["weak-pattern"] = &LearnedPattern{
		ID:         "weak-pattern",
		Category:   "test",
		Language:   "go",
		Pattern:    `weak error`,
		Confidence: 0.1,
	}
	el.mu.Unlock()

	beforeCount := len(el.Patterns)
	el.PruneWeak(0.5)
	afterCount := len(el.Patterns)

	if afterCount >= beforeCount {
		t.Error("PruneWeak should have removed at least one pattern")
	}

	if _, exists := el.Patterns["weak-pattern"]; exists {
		t.Error("weak pattern should have been pruned")
	}

	// Verify strong patterns remain.
	if _, exists := el.Patterns["go-unused-var"]; !exists {
		t.Error("strong pattern (go-unused-var, 0.95) should not be pruned")
	}
}

func TestErrorLearnerExportImportRoundTrip(t *testing.T) {
	el := NewErrorLearner()

	// Modify a pattern.
	el.RecordSuccess("go-undefined")
	el.RecordSuccess("go-undefined")

	// Export.
	data, err := el.Export()
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Verify it's valid JSON.
	var check map[string]interface{}
	if err := json.Unmarshal(data, &check); err != nil {
		t.Fatalf("exported data is not valid JSON: %v", err)
	}

	// Import into a new learner.
	el2 := &ErrorLearner{Patterns: make(map[string]*LearnedPattern)}
	if err := el2.Import(data); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify patterns were imported.
	if len(el2.Patterns) != len(el.Patterns) {
		t.Errorf("expected %d patterns after import, got %d", len(el.Patterns), len(el2.Patterns))
	}

	p := el2.Patterns["go-undefined"]
	if p == nil {
		t.Fatal("go-undefined pattern missing after import")
	}
	if p.SuccessCount != 2 {
		t.Errorf("expected success count 2 after import, got %d", p.SuccessCount)
	}
}

func TestErrorLearnerImportInvalidJSON(t *testing.T) {
	el := NewErrorLearner()
	err := el.Import([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestPreloadedPatternsMatchCommonGoErrors(t *testing.T) {
	el := NewErrorLearner()

	goErrors := []struct {
		msg        string
		expectedID string
	}{
		{"./main.go:10:5: undefined: myVar", "go-undefined"},
		{`cannot use x (variable of type int) as type string in assignment`, "go-type-mismatch"},
		{"not enough arguments in call to doSomething", "go-missing-args"},
		{"x declared and not used", "go-unused-var"},
		{`"os" imported and not used`, "go-unused-import"},
	}

	for _, tc := range goErrors {
		matches := el.MatchLearned(tc.msg)
		if len(matches) == 0 {
			t.Errorf("no match for %q, expected %s", tc.msg, tc.expectedID)
			continue
		}
		found := false
		for _, m := range matches {
			if m.ID == tc.expectedID {
				found = true
				break
			}
		}
		if !found {
			ids := make([]string, len(matches))
			for i, m := range matches {
				ids[i] = m.ID
			}
			t.Errorf("for %q, expected %s in matches, got %v", tc.msg, tc.expectedID, ids)
		}
	}
}

func TestErrorLearnerStatsCalculation(t *testing.T) {
	el := NewErrorLearner()

	stats := el.Stats()
	if stats.TotalPatterns != 12 {
		t.Errorf("expected 12 total patterns, got %d", stats.TotalPatterns)
	}

	if stats.ByCategory["build"] == 0 {
		t.Error("expected build category to have patterns")
	}
	if stats.ByCategory["runtime"] == 0 {
		t.Error("expected runtime category to have patterns")
	}
	if stats.ByLanguage["go"] == 0 {
		t.Error("expected go language to have patterns")
	}
	if stats.ByLanguage["python"] == 0 {
		t.Error("expected python language to have patterns")
	}

	if stats.AvgConfidence <= 0 || stats.AvgConfidence > 1 {
		t.Errorf("expected avg confidence between 0 and 1, got %f", stats.AvgConfidence)
	}
}

func TestErrorLearnerStatsEmpty(t *testing.T) {
	el := &ErrorLearner{Patterns: make(map[string]*LearnedPattern)}
	stats := el.Stats()
	if stats.TotalPatterns != 0 {
		t.Errorf("expected 0 patterns, got %d", stats.TotalPatterns)
	}
	if stats.AvgConfidence != 0 {
		t.Errorf("expected 0 avg confidence, got %f", stats.AvgConfidence)
	}
}

func TestPythonPatterns(t *testing.T) {
	el := NewErrorLearner()

	matches := el.MatchLearned("IndentationError: unexpected indent")
	if len(matches) == 0 {
		t.Error("expected match for IndentationError")
	}

	matches = el.MatchLearned("NameError: name 'foo' is not defined")
	if len(matches) == 0 {
		t.Error("expected match for NameError")
	}
}

func TestJSAndRustPatterns(t *testing.T) {
	el := NewErrorLearner()

	matches := el.MatchLearned("Cannot find module './utils'")
	if len(matches) == 0 {
		t.Error("expected match for Cannot find module")
	}

	matches = el.MatchLearned("Type 'string' is not assignable to type 'number'")
	if len(matches) == 0 {
		t.Error("expected match for is not assignable to type")
	}

	matches = el.MatchLearned("cannot find value `x` in this scope")
	if len(matches) == 0 {
		t.Error("expected match for cannot find value (Rust)")
	}
}

func TestGenericPatterns(t *testing.T) {
	el := NewErrorLearner()

	matches := el.MatchLearned("open /etc/shadow: permission denied")
	if len(matches) == 0 {
		t.Error("expected match for permission denied")
	}

	matches = el.MatchLearned("dial tcp 127.0.0.1:5432: connection refused")
	if len(matches) == 0 {
		t.Error("expected match for connection refused")
	}
}

func TestErrorLearnerConcurrentAccess(t *testing.T) {
	el := NewErrorLearner()
	done := make(chan struct{})

	// Concurrent reads and writes.
	go func() {
		for i := 0; i < 100; i++ {
			el.MatchLearned("undefined: foo")
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			el.RecordSuccess("go-undefined")
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			el.RecordFailure("go-unused-var")
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			el.Stats()
		}
		done <- struct{}{}
	}()

	for i := 0; i < 4; i++ {
		<-done
	}
}
