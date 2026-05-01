package cmd

import (
	"strings"
	"testing"
)

func TestContextVisualization_PercentUsed(t *testing.T) {
	cv := NewContextVisualization(200000)
	cv.Update(100000)
	if cv.PercentUsed() != 50 {
		t.Errorf("expected 50%%, got %.1f%%", cv.PercentUsed())
	}
}

func TestContextVisualization_State(t *testing.T) {
	cv := NewContextVisualization(200000)

	cv.Update(50000)
	if cv.State() != ContextNormal {
		t.Error("50k/200k should be normal")
	}

	cv.Update(165000)
	if cv.State() != ContextWarning {
		t.Error("165k/200k should be warning")
	}

	cv.Update(185000)
	if cv.State() != ContextError {
		t.Error("185k/200k should be error")
	}

	cv.Update(195000)
	if cv.State() != ContextBlocking {
		t.Error("195k/200k should be blocking")
	}
}

func TestContextVisualization_Render(t *testing.T) {
	cv := NewContextVisualization(200000)
	cv.Update(100000)

	rendered := cv.Render(40)
	if rendered == "" {
		t.Error("render should not be empty")
	}
	if !strings.Contains(rendered, "50%") {
		t.Errorf("render should contain 50%%, got %q", rendered)
	}
}

func TestContextVisualization_RenderCompact(t *testing.T) {
	cv := NewContextVisualization(200000)
	cv.Update(100000)

	rendered := cv.Render(10)
	if !strings.Contains(rendered, "CTX:") {
		t.Errorf("compact render should have CTX prefix, got %q", rendered)
	}
}

func TestRenderBreakdown(t *testing.T) {
	tb := TokenBreakdown{
		System:     5000,
		UserMsgs:   20000,
		Assistant:  30000,
		ToolUse:    10000,
		ToolResult: 35000,
		Total:      100000,
	}

	output := RenderBreakdown(tb, 200000)
	if !strings.Contains(output, "Context Window") {
		t.Error("should contain header")
	}
	if !strings.Contains(output, "System") {
		t.Error("should contain system row")
	}
	if !strings.Contains(output, "Tool results") {
		t.Error("should contain tool results row")
	}
}
