package compact

import (
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// ---------------------------------------------------------------------------
// IsCompactableTool
// ---------------------------------------------------------------------------

func TestIsCompactableTool_AllCompactable(t *testing.T) {
	tools := []string{"Bash", "Read", "Grep", "Glob", "WebFetch", "WebSearch", "Edit", "Write", "LS", "ToolSearch"}
	for _, name := range tools {
		if !IsCompactableTool(name) {
			t.Errorf("IsCompactableTool(%q) = false, want true", name)
		}
	}
}

func TestIsCompactableTool_NonCompactable(t *testing.T) {
	tools := []string{"Agent", "AskUserQuestion", "TodoWrite", "LSP", "Config", "UnknownTool"}
	for _, name := range tools {
		if IsCompactableTool(name) {
			t.Errorf("IsCompactableTool(%q) = true, want false", name)
		}
	}
}

func TestIsCompactableTool_Empty(t *testing.T) {
	if IsCompactableTool("") {
		t.Error("IsCompactableTool('') = true, want false")
	}
}

// ---------------------------------------------------------------------------
// HasTextContent
// ---------------------------------------------------------------------------

func TestHasTextContent_WithContent(t *testing.T) {
	msg := types.EyrieMessage{Role: "assistant", Content: "Hello world"}
	if !HasTextContent(msg) {
		t.Error("expected true for message with content")
	}
}

func TestHasTextContent_EmptyContent(t *testing.T) {
	msg := types.EyrieMessage{Role: "assistant", Content: ""}
	if HasTextContent(msg) {
		t.Error("expected false for empty content")
	}
}

func TestHasTextContent_WhitespaceOnly(t *testing.T) {
	msg := types.EyrieMessage{Role: "assistant", Content: "   \n\t  "}
	if HasTextContent(msg) {
		t.Error("expected false for whitespace-only content")
	}
}

func TestHasTextContent_ToolResultIgnored(t *testing.T) {
	msg := types.EyrieMessage{
		Role:        "user",
		Content:     "has content",
		ToolResults: []types.ToolResult{{ToolUseID: "t1", Content: "output"}},
	}
	if HasTextContent(msg) {
		t.Error("expected false for tool result messages")
	}
}

func TestHasTextContent_WithToolUse(t *testing.T) {
	msg := types.EyrieMessage{
		Role:    "assistant",
		Content: "Using tool",
		ToolUse: []types.ToolCall{{ID: "t1", Name: "Bash"}},
	}
	if !HasTextContent(msg) {
		t.Error("expected true for assistant message with text and tool use")
	}
}

// ---------------------------------------------------------------------------
// DefaultCompactConfig
// ---------------------------------------------------------------------------

func TestDefaultCompactConfig(t *testing.T) {
	cfg := DefaultCompactConfig()
	if !cfg.AutoEnabled {
		t.Error("expected AutoEnabled to be true")
	}
	if cfg.ContextWindowSize != 200000 {
		t.Errorf("ContextWindowSize = %d, want 200000", cfg.ContextWindowSize)
	}
	if cfg.AutoCompactBuffer != 13000 {
		t.Errorf("AutoCompactBuffer = %d, want 13000", cfg.AutoCompactBuffer)
	}
	if cfg.MaxOutputTokens != 20000 {
		t.Errorf("MaxOutputTokens = %d, want 20000", cfg.MaxOutputTokens)
	}
	if cfg.MaxFailures != 3 {
		t.Errorf("MaxFailures = %d, want 3", cfg.MaxFailures)
	}
}

// ---------------------------------------------------------------------------
// CompactResult
// ---------------------------------------------------------------------------

func TestCompactResult_Fields(t *testing.T) {
	result := CompactResult{
		Summary:      "test summary",
		TokensBefore: 1000,
		TokensAfter:  200,
		Strategy:     "api",
	}
	if result.Summary != "test summary" {
		t.Errorf("Summary = %q", result.Summary)
	}
	if result.TokensBefore != 1000 {
		t.Errorf("TokensBefore = %d", result.TokensBefore)
	}
	if result.TokensAfter != 200 {
		t.Errorf("TokensAfter = %d", result.TokensAfter)
	}
	if result.Strategy != "api" {
		t.Errorf("Strategy = %q", result.Strategy)
	}
}

// ---------------------------------------------------------------------------
// AdjustIndexToPreserveAPIInvariants - additional edge cases
// ---------------------------------------------------------------------------

func TestAdjustIndex_StartIndexZero(t *testing.T) {
	msgs := []types.EyrieMessage{
		{Role: "assistant", ToolUse: []types.ToolCall{{ID: "t1", Name: "Bash"}}},
		{Role: "user", ToolResults: []types.ToolResult{{ToolUseID: "t1", Content: "ok"}}},
	}
	got := AdjustIndexToPreserveAPIInvariants(msgs, 0)
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestAdjustIndex_StartBeyondRange(t *testing.T) {
	msgs := []types.EyrieMessage{
		{Role: "user", Content: "hello"},
	}
	got := AdjustIndexToPreserveAPIInvariants(msgs, 100)
	if got != len(msgs) {
		t.Errorf("expected %d, got %d", len(msgs), got)
	}
}

func TestAdjustIndex_UnresolvedToolPair(t *testing.T) {
	msgs := []types.EyrieMessage{
		{Role: "user", Content: "do stuff"},
		{Role: "assistant", ToolUse: []types.ToolCall{{ID: "t1", Name: "Bash"}}},
		{Role: "user", ToolResults: []types.ToolResult{{ToolUseID: "t1", Content: "ok"}}},
		{Role: "assistant", ToolUse: []types.ToolCall{{ID: "t2", Name: "Read"}}},
		{Role: "user", ToolResults: []types.ToolResult{{ToolUseID: "t2", Content: "ok"}}},
		{Role: "assistant", Content: "done"},
	}
	got := AdjustIndexToPreserveAPIInvariants(msgs, 2)
	if got > 2 {
		t.Errorf("expected index <= 2, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// CompactionTrigger - additional edge cases
// ---------------------------------------------------------------------------

func TestCompactionTrigger_ShouldCompact_ZeroWindowSize_Split(t *testing.T) {
	ct := NewCompactionTrigger(0)
	if ct.ShouldCompact(100) {
		t.Error("expected ShouldCompact=false when WindowSize is 0")
	}
}

func TestCompactionTrigger_ShouldCompact_ImmediatelyAfterCompact(t *testing.T) {
	ct := NewCompactionTrigger(100)
	ct.MarkCompacted()
	if ct.ShouldCompact(95) {
		t.Error("expected ShouldCompact=false within MinInterval")
	}
}

// ---------------------------------------------------------------------------
// BuildCompactPrompt - additional variant tests
// ---------------------------------------------------------------------------

func TestBuildCompactPrompt_DiffersByVariant(t *testing.T) {
	base := BuildCompactPrompt(CompactBase)
	partial := BuildCompactPrompt(CompactPartial)
	if base == partial {
		t.Error("expected different prompts for base and partial variants")
	}
}

func TestBuildCompactPrompt_UpToVariant(t *testing.T) {
	prompt := BuildCompactPrompt(CompactUpTo)
	if prompt == "" {
		t.Fatal("expected non-empty prompt for UpTo variant")
	}
}

// ---------------------------------------------------------------------------
// FormatCompactSummary - additional edge cases
// ---------------------------------------------------------------------------

func TestFormatCompactSummary_StripsAnalysisOnly(t *testing.T) {
	raw := `<analysis>thinking here</analysis>

Some content without summary tags.`
	got := FormatCompactSummary(raw)
	if searchStr(got, "thinking here") {
		t.Error("analysis content should be stripped")
	}
	if !searchStr(got, "Some content") {
		t.Error("non-analysis content should remain")
	}
}

func TestFormatCompactSummary_EmptyInput(t *testing.T) {
	got := FormatCompactSummary("")
	if got != "" {
		t.Errorf("expected empty for empty input, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
