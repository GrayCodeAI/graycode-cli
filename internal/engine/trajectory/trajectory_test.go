package trajectory

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func bigToolMsg(payload string) types.GraycodeRouterMessage {
	return types.GraycodeRouterMessage{
		Role:        "assistant",
		ToolUse:     []types.ToolCall{{Name: "Bash", Arguments: map[string]interface{}{"command": payload}}},
		ToolResults: []types.ToolResult{{Content: payload}},
	}
}

func TestUnderBudgetUnchanged(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{{Role: "user", Content: "hi"}}
	out, changed := CompressTrajectory(msgs, 10000, 2, 2)
	if changed || len(out) != 1 {
		t.Fatal("small trajectory must be unchanged")
	}
}

func TestProtectHeadTailCompressMiddle(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{
		{Role: "user", Content: "start"},
		bigToolMsg(strings.Repeat("a", 4000)), // head turn with tool call
		{Role: "user", Content: strings.Repeat("b", 3000)},
		{Role: "assistant", Content: strings.Repeat("c", 3000)},
		{Role: "user", Content: strings.Repeat("d", 3000)},
		{Role: "user", Content: "end"}, // tail
	}
	// protectFirst=2, protectLast=1; target small enough to force compression
	out, changed := CompressTrajectory(msgs, 400, 2, 1)
	if !changed {
		t.Fatal("expected compression")
	}
	// head (2) + summary + tail (1) = 4 messages
	if len(out) != 4 {
		t.Fatalf("len = %d, want 4", len(out))
	}
	// head verbatim (incl. tool call)
	if out[0].Content != "start" {
		t.Fatalf("head[0] altered: %q", out[0].Content)
	}
	if len(out[1].ToolUse) != 1 || len(out[1].ToolResults) != 1 {
		t.Fatal("head tool call not preserved")
	}
	// summary
	if out[2].Role != "user" || !strings.Contains(out[2].Content, "[compressed middle") {
		t.Fatalf("summary missing: %+v", out[2])
	}
	// tail verbatim
	if out[3].Content != "end" {
		t.Fatalf("tail altered: %q", out[3].Content)
	}
}

func TestNoMiddleWhenProtectCoversAll(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{
		{Role: "user", Content: "a"},
		{Role: "user", Content: strings.Repeat("b", 5000)},
		{Role: "user", Content: "c"},
	}
	out, changed := CompressTrajectory(msgs, 100, 2, 2)
	if changed || len(out) != 3 {
		t.Fatal("overlapping protect must leave unchanged")
	}
}

func TestSummaryCapturesLatestRequest(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{
		{Role: "user", Content: "old"},
		{Role: "user", Content: "the real request here"},
		{Role: "assistant", Content: strings.Repeat("x", 3000)},
	}
	out, changed := CompressTrajectory(msgs, 200, 1, 0)
	if !changed {
		t.Fatal("expected change")
	}
	if !strings.Contains(out[1].Content, "the real request here") {
		t.Fatalf("summary lost latest request: %q", out[1].Content)
	}
}
