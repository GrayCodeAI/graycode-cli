package engine

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestSummarizeTrajectory_ToolUsage(t *testing.T) {
	messages := []types.EyrieMessage{
		{
			Role:    "assistant",
			Content: "I will edit the file.",
			ToolUse: []types.ToolCall{
				{
					ID:   "tc1",
					Name: "FileWrite",
					Arguments: map[string]interface{}{
						"path": "main.go",
					},
				},
			},
		},
		{
			Role:    "user",
			Content: "Error: permission denied",
			ToolResults: []types.ToolResult{{
				ToolUseID: "tc1",
				Content:   "Error: permission denied",
				IsError:   true,
			}},
		},
	}

	summary := SummarizeTrajectory(messages)
	if !strings.Contains(summary, "FileWrite") {
		t.Errorf("expected summary to mention FileWrite, got: %s", summary)
	}
	if !strings.Contains(summary, "main.go") {
		t.Errorf("expected summary to mention main.go, got: %s", summary)
	}
	if !strings.Contains(summary, "Failures") {
		t.Errorf("expected summary to mention failures, got: %s", summary)
	}
}

func TestSummarizeTrajectory_NoActions(t *testing.T) {
	messages := []types.EyrieMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}

	summary := SummarizeTrajectory(messages)
	if summary != "No significant actions recorded." {
		t.Errorf("expected 'No significant actions recorded.', got: %s", summary)
	}
}

func TestBestRun_PrefersSuccess(t *testing.T) {
	td := &TrajectoryDistiller{maxRuns: 3}
	runs := []TrajectoryRun{
		{ID: 1, Success: false, Messages: make([]types.EyrieMessage, 10)},
		{ID: 2, Success: true, Messages: make([]types.EyrieMessage, 5)},
		{ID: 3, Success: false, Messages: make([]types.EyrieMessage, 15)},
	}

	best := td.BestRun(runs)
	if best == nil {
		t.Fatal("expected a best run")
	}
	if best.ID != 2 {
		t.Errorf("expected best run ID 2 (successful), got %d", best.ID)
	}
}

func TestBestRun_FallsBackToMostProgress(t *testing.T) {
	td := &TrajectoryDistiller{maxRuns: 3}
	runs := []TrajectoryRun{
		{ID: 1, Success: false, Messages: make([]types.EyrieMessage, 5)},
		{ID: 2, Success: false, Messages: make([]types.EyrieMessage, 12)},
		{ID: 3, Success: false, Messages: make([]types.EyrieMessage, 8)},
	}

	best := td.BestRun(runs)
	if best == nil {
		t.Fatal("expected a best run")
	}
	if best.ID != 2 {
		t.Errorf("expected best run ID 2 (most messages), got %d", best.ID)
	}
}
