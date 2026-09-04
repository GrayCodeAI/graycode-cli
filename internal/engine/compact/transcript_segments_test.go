package compact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func segTestMessages() []types.GraycodeRouterMessage {
	return []types.GraycodeRouterMessage{
		{Role: "user", Content: "fix the flaky test in pkg/foo"},
		{
			Role: "assistant", Content: "Looking into it.",
			ToolUse: []types.ToolCall{{Name: "Read", Arguments: map[string]interface{}{"path": "pkg/foo_test.go"}}},
		},
		{
			Role: "user", Content: "",
			ToolResults: []types.ToolResult{{Content: "package foo\n\nfunc TestX(t *testing.T) {}"}},
		},
		{Role: "assistant", Content: "Found it: shared map without a mutex."},
	}
}

func TestRenderSegmentVerbose(t *testing.T) {
	out := RenderSegmentToMarkdown(segTestMessages(), 3, SegmentVerbose)
	for _, want := range []string{
		"# Compaction segment 3", "Turns: 4", "Detail: verbose",
		"## USER", "## ASSISTANT", "flaky test", "- tool: Read",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("segment missing %q", want)
		}
	}
}

func TestRenderSegmentMinimal(t *testing.T) {
	out := RenderSegmentToMarkdown(segTestMessages(), 0, SegmentMinimal)
	if strings.Contains(out, "package foo") {
		t.Fatal("minimal detail should not include full tool results")
	}
	if !strings.Contains(out, "- tool: Read") {
		t.Fatalf("minimal should keep one-line tool signatures")
	}
}

func TestRenderSegmentNone(t *testing.T) {
	out := RenderSegmentToMarkdown(segTestMessages(), 1, SegmentNone)
	if strings.Contains(out, "## USER") || strings.Contains(out, "flaky test") {
		t.Fatalf("none detail must omit turns, got %q", out)
	}
	if !strings.Contains(out, "Turns: 4") {
		t.Fatal("none detail still reports stats")
	}
}

func TestParseCompactionDetail(t *testing.T) {
	cases := map[string]CompactionDetail{
		"none": SegmentNone, "minimal": SegmentMinimal,
		"balanced": SegmentBalanced, "verbose": SegmentVerbose, "": SegmentVerbose,
	}
	for in, want := range cases {
		got, ok := ParseCompactionDetail(in)
		if !ok || got != want {
			t.Fatalf("ParseCompactionDetail(%q) = (%v,%v)", in, got, ok)
		}
	}
	if _, ok := ParseCompactionDetail("bogus"); ok {
		t.Fatal("bogus should not parse")
	}
}

func TestWriteCompactionSegmentAndIndex(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GRAYCODE_STATE_DIR", stateDir)
	sessionID := "seg-test-session"
	path, err := WriteCompactionSegment(sessionID, segTestMessages(), SegmentVerbose)
	if err != nil {
		t.Fatalf("WriteCompactionSegment: %v", err)
	}

	if filepath.Base(filepath.Dir(path)) != segmentsDirName {
		t.Fatalf("path = %q", path)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- test-owned path from API
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# Compaction segment 0") {
		t.Fatalf("content = %q", string(data)[:80])
	}

	idxData, err := os.ReadFile(IndexPath(sessionID)) // #nosec G304 -- test-owned path
	if err != nil {
		t.Fatalf("index missing: %v", err)
	}
	if !strings.Contains(string(idxData), "| [0](segment_0000.md)") {
		t.Fatalf("index = %q", string(idxData))
	}

	// Second write increments.
	path2, err := WriteCompactionSegment(sessionID, segTestMessages(), SegmentBalanced)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path2, "segment_0001.md") {
		t.Fatalf("second path = %q", path2)
	}
}

func TestWriteCompactionSegmentRequiresSessionID(t *testing.T) {
	if _, err := WriteCompactionSegment("", nil, SegmentVerbose); err == nil {
		t.Fatal("expected error for empty session id")
	}
}
