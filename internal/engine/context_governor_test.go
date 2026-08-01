package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/types"
)

func TestResolveModelContextWindow_Fallback(t *testing.T) {
	if got := ResolveModelContextWindow("unknown-model-xyz", 0); got != DefaultContextWindow {
		t.Fatalf("expected default %d, got %d", DefaultContextWindow, got)
	}
	if got := ResolveModelContextWindow("any", 64_000); got != 64_000 {
		t.Fatalf("expected override 64000, got %d", got)
	}
}

func TestSession_compactConfig_ThresholdPct(t *testing.T) {
	s := NewSession("", "test-model", "sys", nil)
	s.Persistence().SetAutoCompactThresholdPct(85)
	s.Persistence().SetContextWindowCached(100_000)
	cfg := s.compactConfig()
	// Compaction triggers at 85% of a 100k window → 85k tokens.
	want := 85_000
	got := cfg.ContextWindowSize - cfg.AutoCompactBuffer - cfg.MaxOutputTokens
	if got != want {
		t.Fatalf("compact trigger at %d tokens, want %d", got, want)
	}
}

func TestMaybeSpillToolOutput_SmallUnchanged(t *testing.T) {
	in := "hello"
	if got := maybeSpillToolOutput(in, "Read", "id1"); got != in {
		t.Fatalf("expected unchanged small output, got %q", got)
	}
}

func TestMaybeSpillToolOutput_LargeSpills(t *testing.T) {
	t.Setenv("HAWK_CACHE_DIR", t.TempDir())
	in := strings.Repeat("x", toolOutputSpillMinChars+100)
	got := maybeSpillToolOutput(in, "Bash", "call-1")
	if !strings.Contains(got, "/scratch/") {
		t.Fatalf("expected spill path in output, got %q", got[:200])
	}
	if strings.Contains(got, strings.Repeat("x", toolOutputSpillMinChars)) {
		t.Fatal("expected full payload not inlined after spill")
	}
}

func TestManageContextBeforeTurn_CollapseOnly(t *testing.T) {
	s := NewSession("", "test-model", "sys", nil)
	s.messages = []types.EyrieMessage{
		{Role: "user", ToolResults: []types.ToolResult{{Content: "err", IsError: true}}},
		{Role: "user", ToolResults: []types.ToolResult{{Content: "err", IsError: true}}},
	}
	_, compacted := s.ManageContextBeforeTurn(context.Background())
	if compacted {
		t.Fatal("expected no compaction for tiny history")
	}
}
