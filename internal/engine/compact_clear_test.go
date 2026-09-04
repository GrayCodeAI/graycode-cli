package engine

import (
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func mkMsg(role string, trs []types.ToolResult) types.GraycodeRouterMessage {
	return types.GraycodeRouterMessage{Role: role, ToolResults: trs}
}

func TestClearOldToolResultsNoOpWhenBelowThreshold(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{
		mkMsg("user", []types.ToolResult{{Content: "aaaa"}}),
	}
	out, freed := clearOldToolResults(msgs, 100, 1000)
	if freed != 0 || len(out) != 1 {
		t.Fatalf("freed=%d len=%d, want no-op", freed, len(out))
	}
}

func TestClearOldToolResultsClearsBiggestFirst(t *testing.T) {
	// 6 tool-result messages; threshold low so clearing triggers; keep last 4.
	msgs := make([]types.GraycodeRouterMessage, 6)
	for i := range msgs {
		msgs[i] = mkMsg("user", []types.ToolResult{{Content: "12345678"}}) // 8 bytes
	}
	out, freed := clearOldToolResults(msgs, 100000, 50000)
	if freed == 0 {
		t.Fatal("expected tokens freed")
	}
	// Keep last 4 intact; first 2 clearable messages get cleared.
	if out[0].ToolResults[0].Content != outputClearedPlaceholder {
		t.Fatalf("oldest result not cleared: %q", out[0].ToolResults[0].Content)
	}
	// Most recent 4 intact.
	if out[5].ToolResults[0].Content == outputClearedPlaceholder {
		t.Fatal("most recent result should be kept intact")
	}
	// Original slice untouched (we cloned).
	if msgs[0].ToolResults[0].Content == outputClearedPlaceholder {
		t.Fatal("input slice must not be mutated")
	}
}

func TestClearOldToolResultsStopsWhenUnderThreshold(t *testing.T) {
	// Huge contents, small threshold: it clears until under threshold, then stops.
	msgs := make([]types.GraycodeRouterMessage, 6)
	for i := range msgs {
		msgs[i] = mkMsg("user", []types.ToolResult{{Content: "12345678"}})
	}
	out, _ := clearOldToolResults(msgs, 48, 20)
	cleared := 0
	for i := 0; i < 2; i++ {
		if out[i].ToolResults[0].Content == outputClearedPlaceholder {
			cleared++
		}
	}
	if cleared == 0 {
		t.Fatalf("expected at least the oldest result cleared: %+v", out[0])
	}
}

func TestClearOldToolResultsSkipsAlreadyCleared(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{
		mkMsg("user", []types.ToolResult{{Content: outputClearedPlaceholder}}),
		mkMsg("user", []types.ToolResult{{Content: outputClearedPlaceholder}}),
		mkMsg("user", []types.ToolResult{{Content: outputClearedPlaceholder}}),
		mkMsg("user", []types.ToolResult{{Content: outputClearedPlaceholder}}),
		mkMsg("user", []types.ToolResult{{Content: outputClearedPlaceholder}}),
		mkMsg("user", []types.ToolResult{{Content: outputClearedPlaceholder}}),
	}
	out, freed := clearOldToolResults(msgs, 100, 50)
	if freed != 0 {
		t.Fatalf("already-cleared results should free nothing, freed=%d", freed)
	}
	_ = out
}
