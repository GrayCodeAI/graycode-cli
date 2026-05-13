package engine

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewSuggestionEngine(t *testing.T) {
	se := NewSuggestionEngine()
	if se == nil {
		t.Fatal("expected non-nil SuggestionEngine")
	}
	if len(se.Rules) == 0 {
		t.Fatal("expected built-in rules")
	}
	if len(se.History) != 0 {
		t.Fatalf("expected empty history, got %d", len(se.History))
	}
	if len(se.Context) != 0 {
		t.Fatalf("expected empty context, got %d", len(se.Context))
	}
}

func TestSuggestAfterEdit(t *testing.T) {
	se := NewSuggestionEngine()
	ctx := map[string]string{
		"files_edited": "3",
	}

	suggestions := se.Suggest(ctx)
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions after editing files")
	}

	found := false
	for _, s := range suggestions {
		if s.Command == "run tests" {
			found = true
			if s.Confidence != 0.92 {
				t.Fatalf("expected confidence 0.92, got %f", s.Confidence)
			}
			if s.Category != "validation" {
				t.Fatalf("expected category 'validation', got %s", s.Category)
			}
			if !strings.Contains(s.Description, "3 files") {
				t.Fatalf("expected description to mention file count, got: %s", s.Description)
			}
		}
	}
	if !found {
		t.Fatal("expected 'run tests' suggestion")
	}
}

func TestSuggestAfterTestFailure(t *testing.T) {
	se := NewSuggestionEngine()
	ctx := map[string]string{
		"test_status": "failed",
		"failed_test": "TestParseConfig",
	}

	suggestions := se.Suggest(ctx)
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions after test failure")
	}

	found := false
	for _, s := range suggestions {
		if s.Command == "fix the failing test" {
			found = true
			if s.Confidence != 0.95 {
				t.Fatalf("expected confidence 0.95, got %f", s.Confidence)
			}
			if !strings.Contains(s.Description, "TestParseConfig") {
				t.Fatalf("expected description to mention test name, got: %s", s.Description)
			}
		}
	}
	if !found {
		t.Fatal("expected 'fix the failing test' suggestion")
	}
}

func TestSuggestManyEditsCommit(t *testing.T) {
	se := NewSuggestionEngine()
	ctx := map[string]string{
		"files_edited": "7",
		"test_status":  "passed",
	}

	suggestions := se.Suggest(ctx)

	foundCommit := false
	for _, s := range suggestions {
		if s.Command == "commit changes" {
			foundCommit = true
			if s.Confidence != 0.78 {
				t.Fatalf("expected confidence 0.78, got %f", s.Confidence)
			}
			if !strings.Contains(s.Description, "7 files") {
				t.Fatalf("expected description to mention file count, got: %s", s.Description)
			}
			if !strings.Contains(s.Description, "tests passing") {
				t.Fatalf("expected description to mention tests passing, got: %s", s.Description)
			}
		}
	}
	if !foundCommit {
		t.Fatal("expected 'commit changes' suggestion for 7 files edited")
	}
}

func TestSuggestManyEditsThreshold(t *testing.T) {
	se := NewSuggestionEngine()

	// 4 files should NOT trigger commit suggestion
	ctx := map[string]string{
		"files_edited": "4",
	}
	suggestions := se.Suggest(ctx)
	for _, s := range suggestions {
		if s.Command == "commit changes" {
			t.Fatal("should not suggest commit for fewer than 5 files")
		}
	}

	// 5 files should trigger it
	ctx["files_edited"] = "5"
	suggestions = se.Suggest(ctx)
	foundCommit := false
	for _, s := range suggestions {
		if s.Command == "commit changes" {
			foundCommit = true
		}
	}
	if !foundCommit {
		t.Fatal("expected commit suggestion at 5 files")
	}
}

func TestSuggestNewSession(t *testing.T) {
	se := NewSuggestionEngine()
	ctx := map[string]string{
		"session_state": "new",
	}

	suggestions := se.Suggest(ctx)
	found := false
	for _, s := range suggestions {
		if s.Command == "review pending tasks" {
			found = true
			if s.Category != "planning" {
				t.Fatalf("expected category planning, got %s", s.Category)
			}
		}
	}
	if !found {
		t.Fatal("expected 'review pending tasks' suggestion for new session")
	}
}

func TestSuggestAfterError(t *testing.T) {
	se := NewSuggestionEngine()
	ctx := map[string]string{
		"last_error": "permission denied: /etc/hosts",
	}

	suggestions := se.Suggest(ctx)
	found := false
	for _, s := range suggestions {
		if s.Command == "try a different approach" {
			found = true
			if !strings.Contains(s.Description, "permission denied") {
				t.Fatalf("expected error message in description, got: %s", s.Description)
			}
		}
	}
	if !found {
		t.Fatal("expected 'try a different approach' suggestion after error")
	}
}

func TestSuggestFileCreated(t *testing.T) {
	se := NewSuggestionEngine()
	ctx := map[string]string{
		"file_created": "handler.go",
	}

	suggestions := se.Suggest(ctx)
	found := false
	for _, s := range suggestions {
		if s.Command == "add tests for new code" {
			found = true
			if !strings.Contains(s.Description, "handler.go") {
				t.Fatalf("expected file name in description, got: %s", s.Description)
			}
			if s.Category != "testing" {
				t.Fatalf("expected category testing, got %s", s.Category)
			}
		}
	}
	if !found {
		t.Fatal("expected 'add tests for new code' suggestion")
	}
}

func TestSuggestLongSilence(t *testing.T) {
	se := NewSuggestionEngine()
	ctx := map[string]string{
		"idle": "true",
	}

	suggestions := se.Suggest(ctx)
	found := false
	for _, s := range suggestions {
		if s.Command == "shall I continue?" {
			found = true
			if s.Confidence != 0.60 {
				t.Fatalf("expected confidence 0.60, got %f", s.Confidence)
			}
		}
	}
	if !found {
		t.Fatal("expected 'shall I continue?' suggestion when idle")
	}
}

func TestSuggestNoMatch(t *testing.T) {
	se := NewSuggestionEngine()
	ctx := map[string]string{}

	suggestions := se.Suggest(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected no suggestions for empty context, got %d", len(suggestions))
	}
}

func TestSuggestSortedByConfidence(t *testing.T) {
	se := NewSuggestionEngine()
	// Trigger multiple rules at once
	ctx := map[string]string{
		"files_edited": "6",
		"test_status":  "failed",
		"failed_test":  "TestFoo",
		"last_error":   "compilation failed",
	}

	suggestions := se.Suggest(ctx)
	if len(suggestions) < 2 {
		t.Fatalf("expected at least 2 suggestions, got %d", len(suggestions))
	}

	// Verify sorted by confidence descending
	for i := 0; i < len(suggestions)-1; i++ {
		if suggestions[i].Confidence < suggestions[i+1].Confidence {
			t.Fatalf("suggestions not sorted by confidence: %.2f < %.2f at index %d",
				suggestions[i].Confidence, suggestions[i+1].Confidence, i)
		}
	}
}

func TestUpdateContext(t *testing.T) {
	se := NewSuggestionEngine()

	se.UpdateContext("files_edited", "3")
	se.UpdateContext("test_status", "passed")

	se.mu.RLock()
	if se.Context["files_edited"] != "3" {
		t.Fatalf("expected files_edited=3, got %s", se.Context["files_edited"])
	}
	if se.Context["test_status"] != "passed" {
		t.Fatalf("expected test_status=passed, got %s", se.Context["test_status"])
	}
	se.mu.RUnlock()

	// Update existing key
	se.UpdateContext("files_edited", "5")
	se.mu.RLock()
	if se.Context["files_edited"] != "5" {
		t.Fatalf("expected files_edited=5 after update, got %s", se.Context["files_edited"])
	}
	se.mu.RUnlock()
}

func TestRecordCommand(t *testing.T) {
	se := NewSuggestionEngine()

	se.RecordCommand("run tests")
	se.RecordCommand("commit changes")
	se.RecordCommand("run tests")

	se.mu.RLock()
	defer se.mu.RUnlock()

	if len(se.History) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(se.History))
	}
	if se.History[0] != "run tests" {
		t.Fatalf("expected first command 'run tests', got %s", se.History[0])
	}
	if se.History[1] != "commit changes" {
		t.Fatalf("expected second command 'commit changes', got %s", se.History[1])
	}
}

func TestGetTopSuggestion(t *testing.T) {
	se := NewSuggestionEngine()
	se.UpdateContext("files_edited", "3")

	top := se.GetTopSuggestion()
	if top == nil {
		t.Fatal("expected a top suggestion")
	}
	if top.Command != "run tests" {
		t.Fatalf("expected 'run tests' as top suggestion, got %s", top.Command)
	}
}

func TestGetTopSuggestionEmpty(t *testing.T) {
	se := NewSuggestionEngine()

	top := se.GetTopSuggestion()
	if top != nil {
		t.Fatalf("expected nil for empty context, got %v", top)
	}
}

func TestFormatCommandSuggestionsEmpty(t *testing.T) {
	result := FormatCommandSuggestions(nil)
	if result != "No suggestions." {
		t.Fatalf("expected 'No suggestions.', got: %s", result)
	}
}

func TestFormatCommandSuggestions(t *testing.T) {
	suggestions := []*CommandSuggestion{
		{
			Command:     "run tests",
			Description: "You've edited 3 files — verify nothing broke",
			Confidence:  0.92,
			Category:    "validation",
		},
		{
			Command:     "commit changes",
			Description: "5 files modified, tests passing",
			Confidence:  0.78,
			Category:    "workflow",
		},
	}

	result := FormatCommandSuggestions(suggestions)

	if !strings.Contains(result, "Suggestions:") {
		t.Fatal("expected header 'Suggestions:'")
	}
	if !strings.Contains(result, "1.") {
		t.Fatal("expected numbered item 1")
	}
	if !strings.Contains(result, "2.") {
		t.Fatal("expected numbered item 2")
	}
	if !strings.Contains(result, "\U0001f4a1") {
		t.Fatal("expected lightbulb emoji")
	}
	if !strings.Contains(result, "Run tests") {
		t.Fatal("expected capitalized command 'Run tests'")
	}
	if !strings.Contains(result, "confidence: 0.92") {
		t.Fatal("expected confidence value 0.92")
	}
	if !strings.Contains(result, "confidence: 0.78") {
		t.Fatal("expected confidence value 0.78")
	}
	if !strings.Contains(result, "You've edited 3 files") {
		t.Fatal("expected description text")
	}
	if !strings.Contains(result, "5 files modified") {
		t.Fatal("expected second description text")
	}
}

func TestAddRule(t *testing.T) {
	se := NewSuggestionEngine()
	initialCount := len(se.Rules)

	se.AddRule(SuggestionRule{
		Name: "custom_rule",
		Condition: func(ctx map[string]string) bool {
			return ctx["deploy_ready"] == "true"
		},
		Suggest: func(ctx map[string]string) *CommandSuggestion {
			return &CommandSuggestion{
				Command:     "deploy to staging",
				Description: "All checks passing — ready to deploy",
				Confidence:  0.88,
				Category:    "deployment",
				Context:     "deploy_ready",
			}
		},
		Priority: 1,
	})

	if len(se.Rules) != initialCount+1 {
		t.Fatalf("expected %d rules after add, got %d", initialCount+1, len(se.Rules))
	}

	// Verify the custom rule triggers
	ctx := map[string]string{"deploy_ready": "true"}
	suggestions := se.Suggest(ctx)

	found := false
	for _, s := range suggestions {
		if s.Command == "deploy to staging" {
			found = true
			if s.Confidence != 0.88 {
				t.Fatalf("expected confidence 0.88, got %f", s.Confidence)
			}
		}
	}
	if !found {
		t.Fatal("expected custom rule suggestion")
	}
}

func TestDismissSuggestion(t *testing.T) {
	se := NewSuggestionEngine()
	ctx := map[string]string{
		"files_edited": "3",
	}

	// Before dismiss, should get suggestion
	suggestions := se.Suggest(ctx)
	found := false
	for _, s := range suggestions {
		if s.Command == "run tests" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'run tests' before dismiss")
	}

	// Dismiss it
	se.Dismiss("run tests")

	// After dismiss, should not get it
	suggestions = se.Suggest(ctx)
	for _, s := range suggestions {
		if s.Command == "run tests" {
			t.Fatal("'run tests' should not appear after dismiss")
		}
	}
}

func TestDismissDoesNotAffectOtherSuggestions(t *testing.T) {
	se := NewSuggestionEngine()
	ctx := map[string]string{
		"files_edited": "6",
		"test_status":  "failed",
		"failed_test":  "TestFoo",
	}

	se.Dismiss("run tests")

	suggestions := se.Suggest(ctx)
	// Should still get "fix the failing test" and "commit changes"
	foundFix := false
	foundCommit := false
	for _, s := range suggestions {
		if s.Command == "run tests" {
			t.Fatal("dismissed suggestion should not appear")
		}
		if s.Command == "fix the failing test" {
			foundFix = true
		}
		if s.Command == "commit changes" {
			foundCommit = true
		}
	}
	if !foundFix {
		t.Fatal("expected 'fix the failing test' to remain after dismissing 'run tests'")
	}
	if !foundCommit {
		t.Fatal("expected 'commit changes' to remain after dismissing 'run tests'")
	}
}

func TestConcurrency(t *testing.T) {
	se := NewSuggestionEngine()

	var wg sync.WaitGroup

	// Concurrent context updates
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			se.UpdateContext("counter", strings.Repeat("x", n))
		}(i)
	}

	// Concurrent command recording
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			se.RecordCommand("cmd")
		}(i)
	}

	// Concurrent suggestions
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = se.GetTopSuggestion()
		}()
	}

	// Concurrent dismissals
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			se.Dismiss("run tests")
		}()
	}

	// Concurrent AddRule
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			se.AddRule(SuggestionRule{
				Name:      "dynamic",
				Condition: func(ctx map[string]string) bool { return false },
				Suggest:   func(ctx map[string]string) *CommandSuggestion { return nil },
				Priority:  5,
			})
		}()
	}

	wg.Wait()
}

func TestCommandSuggestionStruct(t *testing.T) {
	cs := &CommandSuggestion{
		Command:     "deploy",
		Description: "Ready to deploy",
		Confidence:  0.99,
		Category:    "deployment",
		Context:     "all_green",
	}

	if cs.Command != "deploy" {
		t.Fatal("unexpected Command")
	}
	if cs.Description != "Ready to deploy" {
		t.Fatal("unexpected Description")
	}
	if cs.Confidence != 0.99 {
		t.Fatal("unexpected Confidence")
	}
	if cs.Category != "deployment" {
		t.Fatal("unexpected Category")
	}
	if cs.Context != "all_green" {
		t.Fatal("unexpected Context")
	}
}

func TestSuggestionRuleStruct(t *testing.T) {
	rule := SuggestionRule{
		Name:     "test_rule",
		Priority: 3,
		Condition: func(ctx map[string]string) bool {
			return ctx["ready"] == "yes"
		},
		Suggest: func(ctx map[string]string) *CommandSuggestion {
			return &CommandSuggestion{Command: "go", Confidence: 1.0}
		},
	}

	if rule.Name != "test_rule" {
		t.Fatal("unexpected Name")
	}
	if rule.Priority != 3 {
		t.Fatal("unexpected Priority")
	}
	if !rule.Condition(map[string]string{"ready": "yes"}) {
		t.Fatal("expected condition to match")
	}
	if rule.Condition(map[string]string{"ready": "no"}) {
		t.Fatal("expected condition to not match")
	}
}

func TestSuggestNilConditionOrSuggest(t *testing.T) {
	se := NewSuggestionEngine()

	// Add rules with nil functions
	se.AddRule(SuggestionRule{
		Name:      "nil_condition",
		Condition: nil,
		Suggest:   func(ctx map[string]string) *CommandSuggestion { return nil },
		Priority:  1,
	})
	se.AddRule(SuggestionRule{
		Name:      "nil_suggest",
		Condition: func(ctx map[string]string) bool { return true },
		Suggest:   nil,
		Priority:  1,
	})

	// Should not panic
	ctx := map[string]string{"files_edited": "1"}
	suggestions := se.Suggest(ctx)
	_ = suggestions
}

func TestSuggestReturnsNilSuggestion(t *testing.T) {
	se := &SuggestionEngine{
		Rules:     make([]SuggestionRule, 0),
		History:   make([]string, 0),
		Context:   make(map[string]string),
		dismissed: make(map[string]time.Time),
	}

	se.AddRule(SuggestionRule{
		Name:      "returns_nil",
		Condition: func(ctx map[string]string) bool { return true },
		Suggest:   func(ctx map[string]string) *CommandSuggestion { return nil },
		Priority:  1,
	})

	suggestions := se.Suggest(map[string]string{"anything": "true"})
	if len(suggestions) != 0 {
		t.Fatalf("expected no suggestions when Suggest returns nil, got %d", len(suggestions))
	}
}

func TestCapitalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"run tests", "Run tests"},
		{"", ""},
		{"A", "A"},
		{"hello", "Hello"},
	}

	for _, tt := range tests {
		result := capitalizeSuggestion(tt.input)
		if result != tt.expected {
			t.Fatalf("capitalizeSuggestion(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestErrorMessageTruncation(t *testing.T) {
	se := NewSuggestionEngine()
	longError := strings.Repeat("x", 100)
	ctx := map[string]string{
		"last_error": longError,
	}

	suggestions := se.Suggest(ctx)
	found := false
	for _, s := range suggestions {
		if s.Command == "try a different approach" {
			found = true
			// Description should contain truncated error
			if !strings.Contains(s.Description, "...") {
				t.Fatal("expected truncated error message with ellipsis")
			}
			// Should not contain the full 100-char error
			if strings.Contains(s.Description, longError) {
				t.Fatal("error should be truncated")
			}
		}
	}
	if !found {
		t.Fatal("expected error recovery suggestion")
	}
}
