package history

import (
	"strings"
	"testing"
)

func headTailMakeMessages(n int, tokensEach int) []WindowMessage {
	msgs := make([]WindowMessage, n)
	for i := range msgs {
		msgs[i] = WindowMessage{
			Role:    "user",
			Content: "message content",
			Tokens:  tokensEach,
			Index:   i,
		}
		if i%2 == 1 {
			msgs[i].Role = "assistant"
		}
	}
	return msgs
}

func TestNewHeadTailWindow_Defaults(t *testing.T) {
	w := NewHeadTailWindow(0, 0, 0)
	if w.HeadSize != 4 {
		t.Errorf("expected default HeadSize 4, got %d", w.HeadSize)
	}
	if w.TailSize != 12 {
		t.Errorf("expected default TailSize 12, got %d", w.TailSize)
	}
}

func TestNewHeadTailWindow_Custom(t *testing.T) {
	w := NewHeadTailWindow(6, 20, 8000)
	if w.HeadSize != 6 {
		t.Errorf("expected HeadSize 6, got %d", w.HeadSize)
	}
	if w.TailSize != 20 {
		t.Errorf("expected TailSize 20, got %d", w.TailSize)
	}
	if w.MaxTokens != 8000 {
		t.Errorf("expected MaxTokens 8000, got %d", w.MaxTokens)
	}
}

func TestApply_NoDropping(t *testing.T) {
	w := NewHeadTailWindow(4, 12, 10000)
	msgs := headTailMakeMessages(10, 100) // 10 messages fit within head+tail (16)

	result := w.Apply(msgs)

	if result.Dropped != 0 {
		t.Errorf("expected 0 dropped, got %d", result.Dropped)
	}
	if len(result.Head) != 10 {
		t.Errorf("expected 10 head messages, got %d", len(result.Head))
	}
	if result.Tail != nil {
		t.Errorf("expected nil tail when no dropping, got %d messages", len(result.Tail))
	}
	if result.TotalTokens != 1000 {
		t.Errorf("expected 1000 total tokens, got %d", result.TotalTokens)
	}
}

func TestApply_DropsMiddle(t *testing.T) {
	w := NewHeadTailWindow(4, 12, 100000)
	msgs := headTailMakeMessages(30, 100) // 30 messages, keep 4 head + 12 tail = 16

	result := w.Apply(msgs)

	if result.Dropped != 14 {
		t.Errorf("expected 14 dropped, got %d", result.Dropped)
	}
	if len(result.Head) != 4 {
		t.Errorf("expected 4 head messages, got %d", len(result.Head))
	}
	if len(result.Tail) != 12 {
		t.Errorf("expected 12 tail messages, got %d", len(result.Tail))
	}
	if result.TotalTokens != 1600 {
		t.Errorf("expected 1600 total tokens, got %d", result.TotalTokens)
	}
}

func TestApply_WithSummary(t *testing.T) {
	w := NewHeadTailWindow(2, 2, 100000)
	w.IncludeSummary = true

	msgs := []WindowMessage{
		{Role: "user", Content: "Set up auth", Tokens: 50, Index: 0},
		{Role: "assistant", Content: "Implementing authentication", Tokens: 50, Index: 1},
		{Role: "user", Content: "Fix the bug in auth module", Tokens: 50, Index: 2},
		{Role: "assistant", Content: "Fixed the authentication bug", Tokens: 50, Index: 3, IsToolResult: true},
		{Role: "user", Content: "Run the tests", Tokens: 50, Index: 4},
		{Role: "assistant", Content: "Test results look good", Tokens: 50, Index: 5},
	}

	result := w.Apply(msgs)

	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if !strings.Contains(result.Summary, "Earlier:") {
		t.Errorf("summary should start with 'Earlier:', got %q", result.Summary)
	}
}

func TestApply_MaxTokensTrimming(t *testing.T) {
	w := NewHeadTailWindow(4, 12, 500)    // very tight budget
	msgs := headTailMakeMessages(30, 100) // 30 * 100 = 3000 tokens total

	result := w.Apply(msgs)

	if result.TotalTokens > 500 {
		t.Errorf("expected total tokens <= 500, got %d", result.TotalTokens)
	}
}

func TestSummarizeDroppedWindow_Empty(t *testing.T) {
	s := summarizeDroppedWindow(nil)
	if s != "" {
		t.Errorf("expected empty string for nil, got %q", s)
	}
}

func TestSummarizeDroppedWindow_WithToolCalls(t *testing.T) {
	msgs := []WindowMessage{
		{Role: "assistant", Content: "Running auth check", Tokens: 50, IsToolResult: true},
		{Role: "assistant", Content: "Deploying the fix", Tokens: 50, IsToolResult: true},
		{Role: "user", Content: "Please implement the feature", Tokens: 50},
	}

	s := summarizeDroppedWindow(msgs)

	if !strings.Contains(s, "2 tool calls") {
		t.Errorf("expected mention of 2 tool calls, got %q", s)
	}
	if !strings.Contains(s, "Earlier:") {
		t.Errorf("expected 'Earlier:' prefix, got %q", s)
	}
}

func TestSummarizeDroppedWindow_TopicExtraction(t *testing.T) {
	msgs := []WindowMessage{
		{Role: "user", Content: "Fix the authentication bug", Tokens: 50},
		{Role: "assistant", Content: "I'll refactor the auth module and test it", Tokens: 50},
	}

	s := summarizeDroppedWindow(msgs)

	if !strings.Contains(s, "auth") {
		t.Errorf("expected topic 'auth' in summary, got %q", s)
	}
}

func TestAdaptiveSizes_MinimumGuarantees(t *testing.T) {
	msgs := headTailMakeMessages(3, 100)
	head, tail := AdaptiveSizes(msgs, 1000)

	if head < 2 {
		t.Errorf("expected head >= 2, got %d", head)
	}
	if tail < 4 {
		t.Errorf("expected tail >= 4, got %d", tail)
	}
}

func TestAdaptiveSizes_LargeBudget(t *testing.T) {
	msgs := headTailMakeMessages(100, 100) // 10000 tokens total
	head, tail := AdaptiveSizes(msgs, 8000)

	// With large budget, should allocate more to tail.
	if tail <= head {
		t.Errorf("expected tail > head with large budget, got head=%d tail=%d", head, tail)
	}
	if head < 2 {
		t.Errorf("expected head >= 2, got %d", head)
	}
	if tail < 4 {
		t.Errorf("expected tail >= 4, got %d", tail)
	}
}

func TestAdaptiveSizes_TightBudget(t *testing.T) {
	msgs := headTailMakeMessages(100, 100)
	head, tail := AdaptiveSizes(msgs, 600) // only enough for ~6 messages

	if head < 2 {
		t.Errorf("expected head >= 2, got %d", head)
	}
	if tail < 4 {
		t.Errorf("expected tail >= 4, got %d", tail)
	}
}

func TestPreserveToolPairs_NoSplitting(t *testing.T) {
	msgs := []WindowMessage{
		{Role: "user", Content: "msg0", Tokens: 50, Index: 0},
		{Role: "assistant", Content: "msg1", Tokens: 50, Index: 1},
		{Role: "assistant", Content: "msg2", Tokens: 50, Index: 2},
		{Role: "tool", Content: "result", Tokens: 50, Index: 3, IsToolResult: true},
		{Role: "user", Content: "msg4", Tokens: 50, Index: 4},
		{Role: "assistant", Content: "msg5", Tokens: 50, Index: 5},
		{Role: "user", Content: "msg6", Tokens: 50, Index: 6},
		{Role: "assistant", Content: "msg7", Tokens: 50, Index: 7},
	}

	// head=2, tail=2 would split: head=[0,1], dropped=[2,3,4,5], tail=[6,7]
	// msg3 is a tool result — but it's in the dropped section so no split at boundary.
	result, extra := PreserveToolPairs(msgs, 2, 2)

	// The tail starts at index 6 which is not a tool result, so no adjustment needed.
	if extra != 0 {
		t.Errorf("expected 0 extra, got %d", extra)
	}
	if len(result) != 4 {
		t.Errorf("expected 4 messages, got %d", len(result))
	}
}

func TestPreserveToolPairs_TailStartsWithToolResult(t *testing.T) {
	msgs := []WindowMessage{
		{Role: "user", Content: "msg0", Tokens: 50, Index: 0},
		{Role: "assistant", Content: "msg1", Tokens: 50, Index: 1},
		{Role: "assistant", Content: "msg2", Tokens: 50, Index: 2},
		{Role: "user", Content: "msg3", Tokens: 50, Index: 3},
		{Role: "assistant", Content: "tool_use", Tokens: 50, Index: 4},
		{Role: "tool", Content: "tool_result", Tokens: 50, Index: 5, IsToolResult: true},
		{Role: "user", Content: "msg6", Tokens: 50, Index: 6},
		{Role: "assistant", Content: "msg7", Tokens: 50, Index: 7},
	}

	// head=2, tail=3 → tailStart=5, which is a tool_result
	// Should pull in msg at index 4 to preserve the pair.
	result, extra := PreserveToolPairs(msgs, 2, 3)

	if extra != 1 {
		t.Errorf("expected 1 extra to preserve tool pair, got %d", extra)
	}
	// Should have head(2) + adjusted tail(4) = 6
	if len(result) != 6 {
		t.Errorf("expected 6 messages, got %d", len(result))
	}
}

func TestPreserveToolPairs_HeadEndsBeforeToolResult(t *testing.T) {
	msgs := []WindowMessage{
		{Role: "user", Content: "msg0", Tokens: 50, Index: 0},
		{Role: "assistant", Content: "tool_use", Tokens: 50, Index: 1},
		{Role: "tool", Content: "tool_result", Tokens: 50, Index: 2, IsToolResult: true},
		{Role: "user", Content: "msg3", Tokens: 50, Index: 3},
		{Role: "assistant", Content: "msg4", Tokens: 50, Index: 4},
		{Role: "user", Content: "msg5", Tokens: 50, Index: 5},
		{Role: "assistant", Content: "msg6", Tokens: 50, Index: 6},
		{Role: "user", Content: "msg7", Tokens: 50, Index: 7},
	}

	// head=2, tail=2 → headEnd=2, tailStart=6
	// msg at index 2 is a tool result right after head boundary.
	result, extra := PreserveToolPairs(msgs, 2, 2)

	if extra != 1 {
		t.Errorf("expected 1 extra to include tool result with head, got %d", extra)
	}
	// head(2) + extra(1) + tail(2) = 5
	if len(result) != 5 {
		t.Errorf("expected 5 messages, got %d", len(result))
	}
}

func TestFormatWindow(t *testing.T) {
	result := &WindowResult{
		Head:        make([]WindowMessage, 4),
		Tail:        make([]WindowMessage, 12),
		Dropped:     18,
		Summary:     "Discussed auth, fixed 2 bugs, ran tests",
		TotalTokens: 6400,
	}

	formatted := FormatWindow(result)

	if !strings.Contains(formatted, "Context Window (head-tail):") {
		t.Error("missing header")
	}
	if !strings.Contains(formatted, "Head: 4 messages (initial context)") {
		t.Errorf("missing head info in: %s", formatted)
	}
	if !strings.Contains(formatted, "[... 18 messages dropped ...]") {
		t.Errorf("missing dropped info in: %s", formatted)
	}
	if !strings.Contains(formatted, "Summary: Discussed auth, fixed 2 bugs, ran tests") {
		t.Errorf("missing summary in: %s", formatted)
	}
	if !strings.Contains(formatted, "Tail: 12 messages (recent)") {
		t.Errorf("missing tail info in: %s", formatted)
	}
	if !strings.Contains(formatted, "Total: 16 messages, 6,400 tokens") {
		t.Errorf("missing total info in: %s", formatted)
	}
}

func TestFormatWindow_Nil(t *testing.T) {
	formatted := FormatWindow(nil)
	if !strings.Contains(formatted, "empty") {
		t.Errorf("expected 'empty' for nil result, got %q", formatted)
	}
}

func TestFormatWindow_NoDropped(t *testing.T) {
	result := &WindowResult{
		Head:        make([]WindowMessage, 5),
		TotalTokens: 500,
	}

	formatted := FormatWindow(result)

	if strings.Contains(formatted, "dropped") {
		t.Errorf("should not mention dropped when 0, got %s", formatted)
	}
}

func TestShouldApply_UnderBudget(t *testing.T) {
	msgs := headTailMakeMessages(10, 100)
	if ShouldApply(msgs, 2000) {
		t.Error("expected false when under budget")
	}
}

func TestShouldApply_OverBudget(t *testing.T) {
	msgs := headTailMakeMessages(10, 100)
	if !ShouldApply(msgs, 500) {
		t.Error("expected true when over budget")
	}
}

func TestShouldApply_ZeroBudget(t *testing.T) {
	msgs := headTailMakeMessages(10, 100)
	if ShouldApply(msgs, 0) {
		t.Error("expected false when budget is 0 (no limit)")
	}
}

func TestShouldApply_ExactBudget(t *testing.T) {
	msgs := headTailMakeMessages(10, 100)
	if ShouldApply(msgs, 1000) {
		t.Error("expected false when exactly at budget")
	}
}

func TestFormatWindowTokens(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{6400, "6,400"},
		{100000, "100,000"},
		{1234567, "1,234,567"},
	}

	for _, tt := range tests {
		got := formatWindowTokens(tt.input)
		if got != tt.expected {
			t.Errorf("formatWindowTokens(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestApply_ConcurrentSafety(t *testing.T) {
	w := NewHeadTailWindow(4, 12, 100000)
	msgs := headTailMakeMessages(30, 100)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			result := w.Apply(msgs)
			if result == nil {
				t.Error("got nil result")
			}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestApply_ExactHeadPlusTailBoundary(t *testing.T) {
	w := NewHeadTailWindow(4, 12, 100000)
	msgs := headTailMakeMessages(16, 100) // exactly head + tail

	result := w.Apply(msgs)

	if result.Dropped != 0 {
		t.Errorf("expected 0 dropped at exact boundary, got %d", result.Dropped)
	}
	if len(result.Head) != 16 {
		t.Errorf("expected all 16 in head, got %d", len(result.Head))
	}
}

func TestAdaptiveSizes_ZeroTokenMessages(t *testing.T) {
	msgs := headTailMakeMessages(20, 0)
	head, tail := AdaptiveSizes(msgs, 1000)

	if head < 2 {
		t.Errorf("expected head >= 2, got %d", head)
	}
	if tail < 4 {
		t.Errorf("expected tail >= 4, got %d", tail)
	}
}
