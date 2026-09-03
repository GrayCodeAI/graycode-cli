package compact

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestFileTracker_NewFileTracker(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()
	if ft == nil {
		t.Fatal("NewFileTracker returned nil")
	}
	if ft.ReadFiles == nil || ft.ModifiedFiles == nil {
		t.Error("maps should be initialized")
	}
	if len(ft.ReadFiles) != 0 || len(ft.ModifiedFiles) != 0 {
		t.Error("new tracker should have empty maps")
	}
}

func TestFileTracker_RecordRead(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()

	ft.RecordRead("main.go")
	ft.RecordRead("main.go")
	ft.RecordRead("config.go")

	if ft.ReadFiles["main.go"] != 2 {
		t.Errorf("expected 2 reads for main.go, got %d", ft.ReadFiles["main.go"])
	}
	if ft.ReadFiles["config.go"] != 1 {
		t.Errorf("expected 1 read for config.go, got %d", ft.ReadFiles["config.go"])
	}
	if len(ft.ReadFiles) != 2 {
		t.Errorf("expected 2 entries, got %d", len(ft.ReadFiles))
	}
}

func TestFileTracker_RecordRead_Empty(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()
	ft.RecordRead("") // should not panic
	if len(ft.ReadFiles) != 0 {
		t.Error("empty path should not be recorded")
	}
}

func TestFileTracker_RecordModified(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()
	ft.RecordModified("edit.go")
	if ft.ModifiedFiles["edit.go"] != 1 {
		t.Errorf("expected 1 mod for edit.go, got %d", ft.ModifiedFiles["edit.go"])
	}
}

func TestFileTracker_RecordModified_Empty(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()
	ft.RecordModified("") // should not panic
}

func TestFileTracker_ExtractFromMessages(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()
	messages := []types.EyrieMessage{
		{
			Role: "assistant",
			ToolUse: []types.ToolCall{
				{Name: "Read", Arguments: map[string]interface{}{"path": "main.go"}},
				{Name: "Write", Arguments: map[string]interface{}{"file_path": "output.go"}},
			},
		},
	}
	ft.ExtractFromMessages(messages)
	if ft.ReadFiles["main.go"] != 1 {
		t.Errorf("expected 1 read for main.go, got %d", ft.ReadFiles["main.go"])
	}
	if ft.ModifiedFiles["output.go"] != 1 {
		t.Errorf("expected 1 mod for output.go, got %d", ft.ModifiedFiles["output.go"])
	}
}

func TestFileTracker_ExtractFromMessages_SkipNonAssistant(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()
	messages := []types.EyrieMessage{
		{Role: "user", ToolUse: []types.ToolCall{{Name: "Read", Arguments: map[string]interface{}{"path": "x.go"}}}},
	}
	ft.ExtractFromMessages(messages)
	if len(ft.ReadFiles) != 0 {
		t.Error("should skip non-assistant messages")
	}
}

func TestFileTracker_Merge(t *testing.T) {
	t.Parallel()
	a := NewFileTracker()
	a.RecordRead("main.go")
	a.RecordRead("main.go")

	b := NewFileTracker()
	b.RecordRead("main.go")
	b.RecordRead("config.go")

	a.Merge(b)
	if a.ReadFiles["main.go"] != 3 {
		t.Errorf("expected 3 reads for main.go, got %d", a.ReadFiles["main.go"])
	}
	if a.ReadFiles["config.go"] != 1 {
		t.Errorf("expected 1 read for config.go, got %d", a.ReadFiles["config.go"])
	}
}

func TestFileTracker_Merge_Nil(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()
	ft.Merge(nil) // should not panic
}

func TestFileTracker_FormatForSummary(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()
	ft.RecordRead("a.go")
	ft.RecordModified("b.go")
	result := ft.FormatForSummary()
	if !strings.Contains(result, "<tracked-files>") {
		t.Error("expected <tracked-files> tag")
	}
	if !strings.Contains(result, "Read:") {
		t.Error("expected Read section")
	}
	if !strings.Contains(result, "Modified:") {
		t.Error("expected Modified section")
	}
	if !strings.Contains(result, "</tracked-files>") {
		t.Error("expected closing </tracked-files> tag")
	}
}

func TestFileTracker_FormatForSummary_Empty(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()
	if ft.FormatForSummary() != "" {
		t.Error("empty tracker should return empty string")
	}
}

func TestFileTracker_ParseFromSummary(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()
	summary := `<tracked-files>
Read: main.go (2x), config.go (1x)
Modified: edit.go (1x)
</tracked-files>`
	ft.ParseFromSummary(summary)
	if ft.ReadFiles["main.go"] != 2 {
		t.Errorf("expected 2 reads for main.go, got %d", ft.ReadFiles["main.go"])
	}
	if ft.ReadFiles["config.go"] != 1 {
		t.Errorf("expected 1 read for config.go, got %d", ft.ReadFiles["config.go"])
	}
	if ft.ModifiedFiles["edit.go"] != 1 {
		t.Errorf("expected 1 mod for edit.go, got %d", ft.ModifiedFiles["edit.go"])
	}
}

func TestFileTracker_ParseFromSummary_NoMatch(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()
	ft.ParseFromSummary("no tags here") // should not panic
}

func TestFileTracker_ParseFromSummary_AbsentBlock(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()
	ft.ParseFromSummary("<tracked-files>\n</tracked-files>") // empty block
}

func TestFileTracker_CanonicalToolName(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"bash", "Bash"},
		{"Read", "Read"},
		{"file_read", "Read"},
		{"file_write", "Write"},
		{"EDIT", "Edit"},
		{"ls", "LS"},
		{"Glob", "Glob"},
		{"web_fetch", "WebFetch"},
		{"web_search", "WebSearch"},
		{"Tool_Search", "ToolSearch"},
		{"Unknown", "Unknown"},
	}
	for _, tt := range tests {
		got := canonicalToolName(tt.input)
		if got != tt.expected {
			t.Errorf("canonicalToolName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFileTracker_CumulativeAcrossOperations(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()
	ft.RecordRead("a.go")
	ft.RecordRead("b.go")
	ft.RecordModified("a.go")

	ft.RecordRead("a.go")
	ft.RecordModified("b.go")

	if ft.ReadFiles["a.go"] != 2 {
		t.Errorf("a.go reads: expected 2, got %d", ft.ReadFiles["a.go"])
	}
	if ft.ReadFiles["b.go"] != 1 {
		t.Errorf("b.go reads: expected 1, got %d", ft.ReadFiles["b.go"])
	}
	if ft.ModifiedFiles["a.go"] != 1 {
		t.Errorf("a.go mods: expected 1, got %d", ft.ModifiedFiles["a.go"])
	}
	if ft.ModifiedFiles["b.go"] != 1 {
		t.Errorf("b.go mods: expected 1, got %d", ft.ModifiedFiles["b.go"])
	}
}
