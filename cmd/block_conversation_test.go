package cmd

import (
	"strings"
	"testing"
)

func TestDetectBlockKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		title   string
		content string
		want    BlockKind
	}{
		{"Run tests", "PASS: all tests passed", BlockTest},
		{"Code diff", "--- a/main.go\n+++ b/main.go", BlockDiff},
		{"Thinking", "Let me reason about this...", BlockThinking},
		{"Review", "Found 3 findings", BlockReview},
		{"Plan", "The approach is...", BlockPlan},
		{"File read", "file content here", BlockToolUse},
	}

	for _, tt := range tests {
		got := DetectBlockKind(tt.title, tt.content)
		if got != tt.want {
			t.Errorf("DetectBlockKind(%q, %q) = %v, want %v", tt.title, tt.content, got, tt.want)
		}
	}
}

func TestRenderBlockSection_Collapsed(t *testing.T) {
	t.Parallel()
	block := BlockSection{
		Title:     "Tool call",
		Content:   "some content",
		Collapsed: true,
		Kind:      BlockToolUse,
	}
	output := RenderBlockSection(block, 80)
	if !strings.Contains(output, "Tool call") {
		t.Error("should contain title")
	}
	// Collapsed should not show content
	if strings.Contains(output, "some content") {
		t.Error("collapsed block should not show content")
	}
}

func TestRenderBlockSection_Expanded(t *testing.T) {
	t.Parallel()
	block := BlockSection{
		Title:     "Test results",
		Content:   "all tests passed",
		Collapsed: false,
		Kind:      BlockTest,
	}
	output := RenderBlockSection(block, 80)
	if !strings.Contains(output, "Test results") {
		t.Error("should contain title")
	}
	if !strings.Contains(output, "all tests passed") {
		t.Error("expanded block should show content")
	}
}

func TestCollapsibleBlocks(t *testing.T) {
	t.Parallel()
	cb := NewCollapsibleBlocks()
	cb.Add("Block 1", "content 1", BlockToolUse)
	cb.Add("Block 2", "content 2", BlockTest)

	if len(cb.Blocks) != 2 {
		t.Errorf("expected 2 blocks, got %d", len(cb.Blocks))
	}

	// Toggle
	cb.Toggle(0)
	if !cb.Blocks[0].Collapsed {
		t.Error("block 0 should be collapsed after toggle")
	}

	// CollapseAll
	cb.ExpandAll()
	cb.CollapseAll()
	for i, b := range cb.Blocks {
		if !b.Collapsed {
			t.Errorf("block %d should be collapsed", i)
		}
	}

	// ExpandAll
	cb.ExpandAll()
	for i, b := range cb.Blocks {
		if b.Collapsed {
			t.Errorf("block %d should be expanded", i)
		}
	}
}

func TestCollapsibleBlocks_ToggleOutOfBounds(t *testing.T) {
	t.Parallel()
	cb := NewCollapsibleBlocks()
	cb.Add("Block", "content", BlockToolUse)

	// Should not panic
	cb.Toggle(-1)
	cb.Toggle(100)
}

func TestCollapsibleBlocks_RenderAll(t *testing.T) {
	t.Parallel()
	cb := NewCollapsibleBlocks()
	cb.Add("Block 1", "content 1", BlockToolUse)
	cb.Add("Block 2", "content 2", BlockDiff)

	output := cb.RenderAll(80)
	if output == "" {
		t.Error("should produce output")
	}
	if !strings.Contains(output, "Block 1") {
		t.Error("should contain first block title")
	}
	if !strings.Contains(output, "Block 2") {
		t.Error("should contain second block title")
	}
}

func TestRenderBlockMessage(t *testing.T) {
	t.Parallel()
	// tool_use should be collapsed by default
	output := renderBlockMessage("tool_use", "reading file.go", 80)
	if output == "" {
		t.Error("tool_use should produce block output")
	}

	// tool_result should be expanded
	output = renderBlockMessage("tool_result", "file contents", 80)
	if output == "" {
		t.Error("tool_result should produce block output")
	}

	// user message should not be block-styled
	output = renderBlockMessage("user", "hello", 80)
	if output != "" {
		t.Error("user messages should not be block-styled")
	}
}

func TestBlockStyle(t *testing.T) {
	t.Parallel()
	kinds := []BlockKind{BlockToolUse, BlockThinking, BlockDiff, BlockTest, BlockReview, BlockPlan}
	for _, kind := range kinds {
		titleStyle, contentStyle := BlockStyle(kind)
		// Just verify they don't panic and return non-zero value styles
		_ = titleStyle.Render("test")
		_ = contentStyle.Render("test")
	}
}
