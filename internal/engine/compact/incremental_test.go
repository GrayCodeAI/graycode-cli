package compact

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/types"
)

func TestBuildIncrementalCompactPromptIncludesPriorSummary(t *testing.T) {
	prior := "## Goal\n- build auth\n\n## Progress\n### Done\n- x"
	p := BuildIncrementalCompactPrompt(prior)
	if !strings.Contains(p, prior) {
		t.Fatal("incremental prompt must embed the prior summary")
	}
	if !strings.Contains(p, "<previous-summary>") {
		t.Fatal("incremental prompt must use the previous-summary tags")
	}
	if !strings.Contains(p, "## Progress") {
		t.Fatal("incremental prompt must preserve the section structure")
	}
}

func TestBuildIncrementalCompactPromptEmptyPriorFallsBack(t *testing.T) {
	p := BuildIncrementalCompactPrompt("")
	if strings.Contains(p, "<previous-summary>") {
		t.Fatal("empty prior summary should fall back to the base prompt")
	}
	if !strings.Contains(p, "## Next Step") {
		t.Fatal("fallback prompt must be the full structured summary template")
	}
}

func TestExtractPriorSummaryFound(t *testing.T) {
	msgs := []types.EyrieMessage{
		{Role: "user", Content: "[Conversation summary]\n## Goal\n- build\n\n[Continue from the recent messages below.]"},
		{Role: "assistant", Content: "ok"},
	}
	got := ExtractPriorSummary(msgs)
	if !strings.Contains(got, "## Goal") || strings.Contains(got, "[Continue") {
		t.Fatalf("ExtractPriorSummary = %q", got)
	}
}

func TestExtractPriorSummaryNone(t *testing.T) {
	msgs := []types.EyrieMessage{
		{Role: "user", Content: "hello"},
	}
	if got := ExtractPriorSummary(msgs); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestExtractPriorSummaryIgnoresNonSummaryFirst(t *testing.T) {
	msgs := []types.EyrieMessage{
		{Role: "user", Content: "[Session memory summary]\nstuff"},
		{Role: "user", Content: "[Conversation summary]\n## Goal\n- x"},
	}
	got := ExtractPriorSummary(msgs)
	if !strings.Contains(got, "## Goal") {
		t.Fatalf("ExtractPriorSummary should find the conversation summary, got %q", got)
	}
}
