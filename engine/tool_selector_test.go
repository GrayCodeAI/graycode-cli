package engine

import (
	"strings"
	"sync"
	"testing"
)

func defaultTestTools() []ToolInfo {
	return []ToolInfo{
		{Name: "Read", Category: "file", Cost: "free", ReadOnly: true},
		{Name: "Edit", Category: "file", Cost: "free", ReadOnly: false},
		{Name: "Write", Category: "file", Cost: "free", ReadOnly: false},
		{Name: "Grep", Category: "search", Cost: "free", ReadOnly: true},
		{Name: "Glob", Category: "search", Cost: "free", ReadOnly: true},
		{Name: "LS", Category: "file", Cost: "free", ReadOnly: true},
		{Name: "Bash", Category: "exec", Cost: "cheap", ReadOnly: false},
		{Name: "WebSearch", Category: "web", Cost: "expensive", ReadOnly: true},
		{Name: "WebFetch", Category: "web", Cost: "expensive", ReadOnly: true},
		{Name: "Agent", Category: "agent", Cost: "expensive", ReadOnly: false},
		{Name: "NotebookEdit", Category: "file", Cost: "cheap", ReadOnly: false},
	}
}

func TestNewToolSelector(t *testing.T) {
	tools := defaultTestTools()
	ts := NewToolSelector(tools)

	if ts == nil {
		t.Fatal("expected non-nil ToolSelector")
	}
	if len(ts.AllTools) != len(tools) {
		t.Errorf("expected %d tools, got %d", len(tools), len(ts.AllTools))
	}
	if ts.UsageHistory == nil {
		t.Error("expected non-nil UsageHistory")
	}
	if ts.TaskPatterns == nil {
		t.Error("expected non-nil TaskPatterns")
	}
	// Should have seeded from TaskToolMap.
	if _, ok := ts.TaskPatterns["debug"]; !ok {
		t.Error("expected TaskPatterns to contain 'debug'")
	}
}

func TestSelectDebugTask(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	sel := ts.Select("fix auth bug", 6)

	if sel == nil {
		t.Fatal("expected non-nil selection")
	}
	if len(sel.Recommended) == 0 {
		t.Fatal("expected at least one recommended tool")
	}
	if len(sel.Recommended) > 6 {
		t.Errorf("expected at most 6 recommended, got %d", len(sel.Recommended))
	}

	// Should include debug-related tools.
	has := make(map[string]bool)
	for _, r := range sel.Recommended {
		has[r] = true
	}
	for _, expected := range []string{"Read", "Grep", "Bash", "Edit"} {
		if !has[expected] {
			t.Errorf("expected %q in recommended, got %v", expected, sel.Recommended)
		}
	}

	if sel.Confidence < 0.5 {
		t.Errorf("expected confidence >= 0.5, got %.2f", sel.Confidence)
	}

	if sel.Reason == "" {
		t.Error("expected non-empty reason")
	}
	if !strings.Contains(sel.Reason, "debug") {
		t.Errorf("expected reason to mention 'debug', got %q", sel.Reason)
	}
}

func TestSelectReadCodeTask(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	sel := ts.Select("read and understand the authentication module", 4)

	has := make(map[string]bool)
	for _, r := range sel.Recommended {
		has[r] = true
	}
	if !has["Read"] {
		t.Errorf("expected Read in recommended for reading task, got %v", sel.Recommended)
	}
	if !has["Grep"] {
		t.Errorf("expected Grep in recommended for reading task, got %v", sel.Recommended)
	}
}

func TestSelectRefactorTask(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	sel := ts.Select("refactor the database layer", 5)

	has := make(map[string]bool)
	for _, r := range sel.Recommended {
		has[r] = true
	}
	for _, expected := range []string{"Read", "Edit", "Write", "Grep", "Bash"} {
		if !has[expected] {
			t.Errorf("expected %q in recommended for refactor task, got %v", expected, sel.Recommended)
		}
	}
}

func TestSelectMaxToolsLimit(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	sel := ts.Select("refactor the database layer", 2)

	if len(sel.Recommended) > 2 {
		t.Errorf("expected at most 2 recommended tools, got %d", len(sel.Recommended))
	}
}

func TestSelectExcluded(t *testing.T) {
	tools := defaultTestTools()
	ts := NewToolSelector(tools)
	sel := ts.Select("fix a bug in auth", 4)

	// Excluded should contain tools not in Recommended.
	recSet := make(map[string]bool)
	for _, r := range sel.Recommended {
		recSet[r] = true
	}
	for _, ex := range sel.Excluded {
		if recSet[ex] {
			t.Errorf("tool %q is both recommended and excluded", ex)
		}
	}

	// Total recommended + excluded should equal all tools (for tools present in AllTools).
	allNames := make(map[string]bool)
	for _, ti := range tools {
		allNames[ti.Name] = true
	}
	for _, ex := range sel.Excluded {
		if !allNames[ex] {
			t.Errorf("excluded tool %q not in AllTools", ex)
		}
	}
}

func TestSelectUnknownTask(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	sel := ts.Select("do something mysterious", 4)

	// Should fall back to a default set.
	if len(sel.Recommended) == 0 {
		t.Error("expected fallback recommendations for unknown task")
	}
	if sel.Confidence > 0.6 {
		t.Errorf("expected low confidence for unknown task, got %.2f", sel.Confidence)
	}
}

func TestRecordUsage(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	ts.RecordUsage("WebSearch", "search for authentication docs")
	ts.RecordUsage("WebSearch", "search for authentication docs")

	key := "WebSearch:search for authentication docs"
	if ts.UsageHistory[key] != 2 {
		t.Errorf("expected usage count 2, got %d", ts.UsageHistory[key])
	}
}

func TestRecordUsageInfluencesSelection(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	// Record heavy usage of WebSearch for authentication tasks.
	for i := 0; i < 10; i++ {
		ts.RecordUsage("WebSearch", "authentication")
	}

	sel := ts.Select("fix authentication problem", 8)
	has := make(map[string]bool)
	for _, r := range sel.Recommended {
		has[r] = true
	}
	if !has["WebSearch"] {
		t.Errorf("expected WebSearch to be boosted by usage history, got %v", sel.Recommended)
	}
}

func TestGetRecommendedForIntent(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())

	tools := ts.GetRecommendedForIntent("debug")
	if tools == nil {
		t.Fatal("expected non-nil result for 'debug' intent")
	}
	expected := []string{"Read", "Grep", "Bash", "Edit"}
	if len(tools) != len(expected) {
		t.Fatalf("expected %d tools, got %d", len(expected), len(tools))
	}
	for i, e := range expected {
		if tools[i] != e {
			t.Errorf("expected tools[%d] = %q, got %q", i, e, tools[i])
		}
	}
}

func TestGetRecommendedForIntentUnknown(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	tools := ts.GetRecommendedForIntent("unknown_intent")
	if tools != nil {
		t.Errorf("expected nil for unknown intent, got %v", tools)
	}
}

func TestFilterExpensive(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	input := []string{"Read", "WebSearch", "Edit", "Agent", "Bash", "WebFetch"}
	filtered := ts.FilterExpensive(input)

	for _, f := range filtered {
		if f == "WebSearch" || f == "WebFetch" || f == "Agent" {
			t.Errorf("expected expensive tool %q to be filtered out", f)
		}
	}
	// Should still contain the non-expensive tools.
	has := make(map[string]bool)
	for _, f := range filtered {
		has[f] = true
	}
	for _, expected := range []string{"Read", "Edit", "Bash"} {
		if !has[expected] {
			t.Errorf("expected %q to remain after filtering, got %v", expected, filtered)
		}
	}
}

func TestFilterExpensiveEmpty(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	filtered := ts.FilterExpensive([]string{"WebSearch", "Agent", "WebFetch"})
	if len(filtered) != 0 {
		t.Errorf("expected empty result when all tools are expensive, got %v", filtered)
	}
}

func TestFormatSelection(t *testing.T) {
	sel := &ToolSelection{
		Recommended: []string{"Read", "Grep", "Edit", "Bash", "Glob", "LS"},
		Excluded:    []string{"WebSearch", "WebFetch", "Agent", "NotebookEdit"},
		Reason:      "debugging task — prioritize file access + execution",
		Confidence:  0.88,
	}
	result := FormatSelection("fix auth bug", sel)

	if !strings.Contains(result, `Tool Selection for "fix auth bug"`) {
		t.Errorf("expected task name in output, got:\n%s", result)
	}
	if !strings.Contains(result, "Recommended (6)") {
		t.Errorf("expected recommended count, got:\n%s", result)
	}
	if !strings.Contains(result, "Excluded (4)") {
		t.Errorf("expected excluded count, got:\n%s", result)
	}
	if !strings.Contains(result, "debugging task") {
		t.Errorf("expected reason in output, got:\n%s", result)
	}
	if !strings.Contains(result, "0.88") {
		t.Errorf("expected confidence in output, got:\n%s", result)
	}
}

func TestAdaptAddsTool(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())

	// Before adapt, "debug" pattern should not contain "WebSearch".
	before := ts.GetRecommendedForIntent("debug")
	for _, tool := range before {
		if tool == "WebSearch" {
			t.Fatal("WebSearch should not be in debug pattern initially")
		}
	}

	// Adapt with feedback mentioning debug.
	ts.Adapt("needed WebSearch but it wasn't available for debug task")

	after := ts.GetRecommendedForIntent("debug")
	found := false
	for _, tool := range after {
		if tool == "WebSearch" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected WebSearch to be added to debug pattern after adapt, got %v", after)
	}
}

func TestAdaptWithKeywordInference(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())

	// Feedback that mentions "fix" which maps to "debug" intent.
	ts.Adapt("needed Agent to fix a complex issue")

	after := ts.GetRecommendedForIntent("debug")
	found := false
	for _, tool := range after {
		if tool == "Agent" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Agent to be added to debug pattern via keyword inference, got %v", after)
	}
}

func TestAdaptNoToolDetected(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	patternsBefore := len(ts.TaskPatterns["debug"])

	// Feedback without "needed" keyword should do nothing.
	ts.Adapt("the tools were fine, no issues")

	patternsAfter := len(ts.TaskPatterns["debug"])
	if patternsBefore != patternsAfter {
		t.Errorf("expected no change when no 'needed' keyword found")
	}
}

func TestAdaptGenericBoost(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())

	// Feedback that doesn't match any intent.
	ts.Adapt("needed SpecialTool for something unusual")

	key := "SpecialTool:general"
	if ts.UsageHistory[key] != 3 {
		t.Errorf("expected generic boost of 3, got %d", ts.UsageHistory[key])
	}
}

func TestConcurrentAccess(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	var wg sync.WaitGroup

	// Run multiple goroutines doing selects and record usage concurrently.
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			ts.Select("fix a bug", 4)
		}()
		go func() {
			defer wg.Done()
			ts.RecordUsage("Read", "testing")
		}()
		go func() {
			defer wg.Done()
			ts.GetRecommendedForIntent("debug")
		}()
	}
	wg.Wait()
}

func TestSelectSearchTask(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	sel := ts.Select("search for all usages of deprecated API", 4)

	has := make(map[string]bool)
	for _, r := range sel.Recommended {
		has[r] = true
	}
	if !has["Grep"] {
		t.Errorf("expected Grep for search task, got %v", sel.Recommended)
	}
	if !has["Glob"] {
		t.Errorf("expected Glob for search task, got %v", sel.Recommended)
	}
}

func TestSelectTestTask(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	sel := ts.Select("test the payment module", 4)

	has := make(map[string]bool)
	for _, r := range sel.Recommended {
		has[r] = true
	}
	if !has["Bash"] {
		t.Errorf("expected Bash for test task, got %v", sel.Recommended)
	}
}

func TestSelectDeployTask(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	sel := ts.Select("deploy the release to production", 4)

	has := make(map[string]bool)
	for _, r := range sel.Recommended {
		has[r] = true
	}
	if !has["Bash"] {
		t.Errorf("expected Bash for deploy task, got %v", sel.Recommended)
	}
	if !has["Read"] {
		t.Errorf("expected Read for deploy task, got %v", sel.Recommended)
	}
}

func TestHighConfidenceMultipleKeywords(t *testing.T) {
	ts := NewToolSelector(defaultTestTools())
	// "fix" and "bug" and "error" all map to debug — should give high confidence.
	sel := ts.Select("fix the bug causing the error", 6)
	if sel.Confidence < 0.9 {
		t.Errorf("expected high confidence (>=0.9) with multiple keyword matches, got %.2f", sel.Confidence)
	}
}
