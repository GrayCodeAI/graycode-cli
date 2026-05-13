package engine

import (
	"strings"
	"sync"
	"testing"
)

func TestNewEfficientPrompter(t *testing.T) {
	ep := NewEfficientPrompter()
	if ep == nil {
		t.Fatal("NewEfficientPrompter returned nil")
	}
	if len(ep.Strategies) != 6 {
		t.Errorf("expected 6 strategies, got %d", len(ep.Strategies))
	}
	// All strategies should be enabled by default
	for _, s := range ep.Strategies {
		if !s.Enabled {
			t.Errorf("strategy %q should be enabled by default", s.Name)
		}
	}
}

func TestOptimize_CompressWhitespace(t *testing.T) {
	ep := NewEfficientPrompter()
	// Disable all except compress_whitespace
	for i := range ep.Strategies {
		if ep.Strategies[i].Name != "compress_whitespace" {
			ep.Strategies[i].Enabled = false
		}
	}

	result := ep.Optimize("hello    world   foo")
	if result.Optimized != "hello world foo" {
		t.Errorf("expected 'hello world foo', got %q", result.Optimized)
	}
	if !epContains(result.Applied, "compress_whitespace") {
		t.Error("compress_whitespace not in Applied list")
	}
}

func TestOptimize_RemoveFiller(t *testing.T) {
	ep := NewEfficientPrompter()
	for i := range ep.Strategies {
		if ep.Strategies[i].Name != "remove_filler" {
			ep.Strategies[i].Enabled = false
		}
	}

	result := ep.Optimize("please fix the bug")
	if result.Optimized != "fix the bug" {
		t.Errorf("expected 'fix the bug', got %q", result.Optimized)
	}

	result = ep.Optimize("Could you write a test")
	if result.Optimized != "write a test" {
		t.Errorf("expected 'write a test', got %q", result.Optimized)
	}

	result = ep.Optimize("I would like a refactor")
	if result.Optimized != "a refactor" {
		t.Errorf("expected 'a refactor', got %q", result.Optimized)
	}
}

func TestOptimize_AbbreviatePhrases(t *testing.T) {
	ep := NewEfficientPrompter()
	for i := range ep.Strategies {
		if ep.Strategies[i].Name != "abbreviate_phrases" {
			ep.Strategies[i].Enabled = false
		}
	}

	result := ep.Optimize("for example this works")
	if result.Optimized != "e.g. this works" {
		t.Errorf("expected 'e.g. this works', got %q", result.Optimized)
	}

	result = ep.Optimize("in order to fix this")
	if result.Optimized != "to fix this" {
		t.Errorf("expected 'to fix this', got %q", result.Optimized)
	}
}

func TestOptimize_ShortenPaths(t *testing.T) {
	ep := NewEfficientPrompter()
	for i := range ep.Strategies {
		if ep.Strategies[i].Name != "shorten_paths" {
			ep.Strategies[i].Enabled = false
		}
	}

	result := ep.Optimize("look at /Users/dev/projects/myapp/src/main.go")
	if !strings.Contains(result.Optimized, "./myapp/src/main.go") {
		t.Errorf("expected shortened path, got %q", result.Optimized)
	}
}

func TestOptimize_CollapseRepeated(t *testing.T) {
	ep := NewEfficientPrompter()
	for i := range ep.Strategies {
		if ep.Strategies[i].Name != "collapse_repeated" {
			ep.Strategies[i].Enabled = false
		}
	}

	input := "line1\nrepeated\nrepeated\nrepeated\nrepeated\nline2"
	result := ep.Optimize(input)
	if strings.Count(result.Optimized, "repeated") > 2 {
		t.Errorf("expected collapsed repeated lines, got %q", result.Optimized)
	}
	if !strings.Contains(result.Optimized, "more identical lines") {
		t.Errorf("expected '... more identical lines' indicator, got %q", result.Optimized)
	}
}

func TestOptimize_StripPleasantries(t *testing.T) {
	ep := NewEfficientPrompter()
	for i := range ep.Strategies {
		if ep.Strategies[i].Name != "strip_pleasantries" {
			ep.Strategies[i].Enabled = false
		}
	}

	result := ep.Optimize("Sure! I can do that.")
	if result.Optimized != "I can do that." {
		t.Errorf("expected 'I can do that.', got %q", result.Optimized)
	}

	result = ep.Optimize("Certainly! The answer is 42.")
	if result.Optimized != "The answer is 42." {
		t.Errorf("expected 'The answer is 42.', got %q", result.Optimized)
	}

	result = ep.Optimize("Here's the solution:\ncode here")
	if result.Optimized != "the solution:\ncode here" {
		t.Errorf("expected stripped pleasantry, got %q", result.Optimized)
	}
}

func TestOptimize_AllStrategies(t *testing.T) {
	ep := NewEfficientPrompter()

	input := "Could you please   fix the bug for example in this file"
	result := ep.Optimize(input)

	// Should be shorter than original
	if len(result.Optimized) >= len(result.Original) {
		t.Errorf("optimized (%d) should be shorter than original (%d): %q",
			len(result.Optimized), len(result.Original), result.Optimized)
	}

	if result.TokensSaved <= 0 {
		t.Errorf("expected positive token savings, got %d", result.TokensSaved)
	}

	if len(result.Applied) == 0 {
		t.Error("expected at least one strategy applied")
	}
}

func TestOptimize_NoChange(t *testing.T) {
	ep := NewEfficientPrompter()
	// Disable everything
	for i := range ep.Strategies {
		ep.Strategies[i].Enabled = false
	}

	result := ep.Optimize("simple text")
	if result.Optimized != "simple text" {
		t.Errorf("expected no change, got %q", result.Optimized)
	}
	if len(result.Applied) != 0 {
		t.Errorf("expected no strategies applied, got %v", result.Applied)
	}
}

func TestOptimizeOutput(t *testing.T) {
	ep := NewEfficientPrompter()

	input := "Sure! Here's the output:\nline1\nline1\nline1\nline1\nresult"
	output := ep.OptimizeOutput(input)

	if strings.Contains(output, "Sure!") {
		t.Error("expected pleasantries to be stripped")
	}
	if strings.Count(output, "line1") > 2 {
		t.Error("expected repeated lines to be collapsed")
	}
}

func TestOptimizeMessages(t *testing.T) {
	ep := NewEfficientPrompter()

	messages := []PromptMsg{
		{Role: "user", Content: "Could you please help me with this very long message that contains a lot of content and filler words for example in order to test the compression", Tokens: 30},
		{Role: "assistant", Content: "Sure! Here's a long response with many details that should be compressed more aggressively since it is an older message in the conversation history", Tokens: 28},
		{Role: "user", Content: "Could you please do more", Tokens: 5},
		{Role: "assistant", Content: "Sure! Here is the result", Tokens: 5},
		{Role: "user", Content: "thanks   that   works", Tokens: 4},
	}

	result := ep.OptimizeMessages(messages)

	if len(result) != len(messages) {
		t.Errorf("expected same number of messages, got %d vs %d", len(result), len(messages))
	}

	// First two messages (older) should be compressed more aggressively
	if len(result[0].Content) >= len(messages[0].Content) {
		t.Errorf("expected older message to be compressed: %d >= %d", len(result[0].Content), len(messages[0].Content))
	}

	// Last message (recent) should have light compression
	if result[4].Content != "thanks that works" {
		t.Errorf("expected light compression on recent message, got %q", result[4].Content)
	}
}

func TestOptimizeMessages_Empty(t *testing.T) {
	ep := NewEfficientPrompter()
	result := ep.OptimizeMessages(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}

	result = ep.OptimizeMessages([]PromptMsg{})
	if len(result) != 0 {
		t.Errorf("expected empty for empty input, got %v", result)
	}
}

func TestEfficientPromptEstimateSavings(t *testing.T) {
	ep := NewEfficientPrompter()

	savings := ep.EstimateSavings("Could you please   help me for example with this")
	if savings <= 0 {
		t.Errorf("expected positive savings estimate, got %d", savings)
	}

	// Should not modify stats
	if ep.Stats.CallCount != 0 {
		t.Errorf("EstimateSavings should not modify stats, got CallCount=%d", ep.Stats.CallCount)
	}
}

func TestEstimateSavings_NoChange(t *testing.T) {
	ep := NewEfficientPrompter()
	savings := ep.EstimateSavings("hi")
	if savings != 0 {
		t.Errorf("expected 0 savings for short text, got %d", savings)
	}
}

func TestFormatEfficientStats(t *testing.T) {
	ep := NewEfficientPrompter()

	// With no calls
	s := ep.FormatEfficientStats()
	if !strings.Contains(s, "Calls: 0") {
		t.Errorf("expected 'Calls: 0' in output, got %q", s)
	}

	// After some optimization
	ep.Optimize("Could you please help me for example with this longer sentence that has filler")
	s = ep.FormatEfficientStats()
	if !strings.Contains(s, "Token Efficiency:") {
		t.Errorf("expected 'Token Efficiency:' header, got %q", s)
	}
	if !strings.Contains(s, "Calls: 1") {
		t.Errorf("expected 'Calls: 1', got %q", s)
	}
	if !strings.Contains(s, "tokens") {
		t.Errorf("expected 'tokens' in output, got %q", s)
	}
	if !strings.Contains(s, "%") {
		t.Errorf("expected percentage in output, got %q", s)
	}
}

func TestFormatEfficientStats_LargeNumbers(t *testing.T) {
	ep := NewEfficientPrompter()
	ep.Stats = EfficientStats{
		OriginalTokens:  125000,
		OptimizedTokens: 98000,
		TotalSavings:    27000,
		CallCount:       45,
	}

	s := ep.FormatEfficientStats()
	if !strings.Contains(s, "27,000") {
		t.Errorf("expected formatted number '27,000', got %q", s)
	}
	if !strings.Contains(s, "Calls: 45") {
		t.Errorf("expected 'Calls: 45', got %q", s)
	}
	if !strings.Contains(s, "21.6%") {
		t.Errorf("expected '21.6%%', got %q", s)
	}
}

func TestEnableDisableStrategy(t *testing.T) {
	ep := NewEfficientPrompter()

	// Disable a strategy
	ep.DisableStrategy("remove_filler")
	for _, s := range ep.Strategies {
		if s.Name == "remove_filler" && s.Enabled {
			t.Error("remove_filler should be disabled")
		}
	}

	// Test that the disabled strategy is not applied
	result := ep.Optimize("please do this")
	if !strings.Contains(result.Optimized, "do this") {
		t.Errorf("expected filler not removed when disabled, got %q", result.Optimized)
	}

	// Re-enable
	ep.EnableStrategy("remove_filler")
	for _, s := range ep.Strategies {
		if s.Name == "remove_filler" && !s.Enabled {
			t.Error("remove_filler should be enabled")
		}
	}

	result = ep.Optimize("please do this")
	if result.Optimized != "do this" {
		t.Errorf("expected filler removed when enabled, got %q", result.Optimized)
	}
}

func TestEnableDisableStrategy_NonExistent(t *testing.T) {
	ep := NewEfficientPrompter()
	// Should not panic
	ep.EnableStrategy("nonexistent")
	ep.DisableStrategy("nonexistent")
}

func TestEfficientPromptConcurrentAccess(t *testing.T) {
	ep := NewEfficientPrompter()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 4 {
			case 0:
				ep.Optimize("please could you help me for example with this")
			case 1:
				ep.OptimizeOutput("Sure! Here's the output\nline\nline\nline")
			case 2:
				ep.EstimateSavings("in order to test concurrency")
			case 3:
				ep.FormatEfficientStats()
			}
		}(i)
	}
	wg.Wait()

	if ep.Stats.CallCount == 0 {
		t.Error("expected some calls recorded")
	}
}

func TestEpEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hi", 1},
		{"hello world", 2},
		{"this is a longer sentence with more tokens", 10},
	}

	for _, tt := range tests {
		got := epEstimateTokens(tt.input)
		if got != tt.expected {
			t.Errorf("epEstimateTokens(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestEpFormatNumber(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{42, "42"},
		{999, "999"},
		{1000, "1,000"},
		{27000, "27,000"},
		{1000000, "1,000,000"},
		{-1500, "-1,500"},
	}

	for _, tt := range tests {
		got := epFormatNumber(tt.input)
		if got != tt.expected {
			t.Errorf("epFormatNumber(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCompressWhitespace_BlankLines(t *testing.T) {
	input := "line1\n\n\n\n\nline2"
	result := compressWhitespace(input)
	if strings.Count(result, "\n") > 2 {
		t.Errorf("expected collapsed blank lines, got %q", result)
	}
}

func TestStripPleasantries_Multiline(t *testing.T) {
	input := "first line\nHere's the fix:\ncode"
	result := stripPleasantries(input)
	if strings.Contains(result, "Here's ") {
		t.Errorf("expected pleasantry stripped from middle of text, got %q", result)
	}
}

func TestOptimize_StatsAccumulate(t *testing.T) {
	ep := NewEfficientPrompter()

	ep.Optimize("Could you please help me for example with this long sentence")
	ep.Optimize("I would like in order to get this done as well as testing")

	if ep.Stats.CallCount != 2 {
		t.Errorf("expected CallCount=2, got %d", ep.Stats.CallCount)
	}
	if ep.Stats.TotalSavings <= 0 {
		t.Errorf("expected positive TotalSavings, got %d", ep.Stats.TotalSavings)
	}
	if ep.Stats.OriginalTokens <= ep.Stats.OptimizedTokens {
		t.Errorf("expected OriginalTokens > OptimizedTokens, got %d vs %d",
			ep.Stats.OriginalTokens, ep.Stats.OptimizedTokens)
	}
}

// --- Helpers ---

func epContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
