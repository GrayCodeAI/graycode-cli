package engine

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client"
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
	ft.RecordRead("") // empty path should be ignored

	if ft.ReadFiles["main.go"] != 2 {
		t.Errorf("main.go reads = %d, want 2", ft.ReadFiles["main.go"])
	}
	if ft.ReadFiles["config.go"] != 1 {
		t.Errorf("config.go reads = %d, want 1", ft.ReadFiles["config.go"])
	}
	if _, exists := ft.ReadFiles[""]; exists {
		t.Error("empty path should not be tracked")
	}
}

func TestFileTracker_RecordModified(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()

	ft.RecordModified("main.go")
	ft.RecordModified("main.go")
	ft.RecordModified("main.go")
	ft.RecordModified("")

	if ft.ModifiedFiles["main.go"] != 3 {
		t.Errorf("main.go modifications = %d, want 3", ft.ModifiedFiles["main.go"])
	}
	if len(ft.ModifiedFiles) != 1 {
		t.Errorf("expected 1 modified file, got %d", len(ft.ModifiedFiles))
	}
}

func TestFileTracker_ExtractFromMessages(t *testing.T) {
	t.Parallel()
	ft := NewFileTracker()

	messages := []client.EyrieMessage{
		{Role: "user", Content: "read main.go"},
		{Role: "assistant", ToolUse: []client.ToolCall{
			{Name: "Read", Arguments: map[string]interface{}{"file_path": "/src/main.go"}},
			{Name: "Edit", Arguments: map[string]interface{}{"file_path": "/src/config.go"}},
		}},
		{Role: "assistant", ToolUse: []client.ToolCall{
			{Name: "Write", Arguments: map[string]interface{}{"file_path": "/src/new.go"}},
			{Name: "Read", Arguments: map[string]interface{}{"file_path": "/src/main.go"}},
		}},
	}

	ft.ExtractFromMessages(messages)

	if ft.ReadFiles["/src/main.go"] != 2 {
		t.Errorf("main.go reads = %d, want 2", ft.ReadFiles["/src/main.go"])
	}
	if ft.ModifiedFiles["/src/config.go"] != 1 {
		t.Errorf("config.go mods = %d, want 1", ft.ModifiedFiles["/src/config.go"])
	}
	if ft.ModifiedFiles["/src/new.go"] != 1 {
		t.Errorf("new.go mods = %d, want 1", ft.ModifiedFiles["/src/new.go"])
	}
}

func TestFileTracker_FormatForSummary(t *testing.T) {
	t.Parallel()

	t.Run("empty tracker", func(t *testing.T) {
		t.Parallel()
		ft := NewFileTracker()
		if got := ft.FormatForSummary(); got != "" {
			t.Errorf("FormatForSummary() = %q, want empty", got)
		}
	})

	t.Run("with files", func(t *testing.T) {
		t.Parallel()
		ft := NewFileTracker()
		ft.RecordRead("main.go")
		ft.RecordRead("main.go")
		ft.RecordModified("config.go")

		result := ft.FormatForSummary()
		if !strings.Contains(result, "<tracked-files>") {
			t.Error("should contain <tracked-files> tag")
		}
		if !strings.Contains(result, "</tracked-files>") {
			t.Error("should contain </tracked-files> tag")
		}
		if !strings.Contains(result, "Read:") {
			t.Error("should contain Read: section")
		}
		if !strings.Contains(result, "Modified:") {
			t.Error("should contain Modified: section")
		}
		if !strings.Contains(result, "main.go") {
			t.Error("should contain main.go")
		}
	})
}

func TestFileTracker_ParseFromSummary(t *testing.T) {
	t.Parallel()

	t.Run("valid summary", func(t *testing.T) {
		t.Parallel()
		ft := NewFileTracker()
		summary := `Some context here.
<tracked-files>
Read: main.go (2x), config.go (1x)
Modified: handler.go (3x)
</tracked-files>
More context.`

		ft.ParseFromSummary(summary)

		if ft.ReadFiles["main.go"] != 2 {
			t.Errorf("main.go reads = %d, want 2", ft.ReadFiles["main.go"])
		}
		if ft.ReadFiles["config.go"] != 1 {
			t.Errorf("config.go reads = %d, want 1", ft.ReadFiles["config.go"])
		}
		if ft.ModifiedFiles["handler.go"] != 3 {
			t.Errorf("handler.go mods = %d, want 3", ft.ModifiedFiles["handler.go"])
		}
	})

	t.Run("no tracked-files block", func(t *testing.T) {
		t.Parallel()
		ft := NewFileTracker()
		ft.ParseFromSummary("just a regular summary with no tracking data")
		if len(ft.ReadFiles) != 0 || len(ft.ModifiedFiles) != 0 {
			t.Error("should not parse anything from summary without tracked-files")
		}
	})

	t.Run("empty block", func(t *testing.T) {
		t.Parallel()
		ft := NewFileTracker()
		ft.ParseFromSummary("<tracked-files>\n</tracked-files>")
		if len(ft.ReadFiles) != 0 || len(ft.ModifiedFiles) != 0 {
			t.Error("should not parse anything from empty block")
		}
	})
}

func TestFileTracker_Merge(t *testing.T) {
	t.Parallel()

	t.Run("merge into empty", func(t *testing.T) {
		t.Parallel()
		ft1 := NewFileTracker()
		ft2 := NewFileTracker()
		ft2.RecordRead("a.go")
		ft2.RecordModified("b.go")

		ft1.Merge(ft2)

		if ft1.ReadFiles["a.go"] != 1 {
			t.Errorf("a.go reads = %d, want 1", ft1.ReadFiles["a.go"])
		}
		if ft1.ModifiedFiles["b.go"] != 1 {
			t.Errorf("b.go mods = %d, want 1", ft1.ModifiedFiles["b.go"])
		}
	})

	t.Run("merge with overlap", func(t *testing.T) {
		t.Parallel()
		ft1 := NewFileTracker()
		ft1.RecordRead("shared.go")
		ft1.RecordRead("shared.go")

		ft2 := NewFileTracker()
		ft2.RecordRead("shared.go")

		ft1.Merge(ft2)

		if ft1.ReadFiles["shared.go"] != 3 {
			t.Errorf("shared.go reads = %d, want 3", ft1.ReadFiles["shared.go"])
		}
	})

	t.Run("merge nil", func(t *testing.T) {
		t.Parallel()
		ft1 := NewFileTracker()
		ft1.RecordRead("x.go")
		ft1.Merge(nil)
		if ft1.ReadFiles["x.go"] != 1 {
			t.Error("merge nil should not change tracker")
		}
	})
}

func TestFileTracker_RoundTrip(t *testing.T) {
	t.Parallel()
	ft1 := NewFileTracker()
	ft1.RecordRead("main.go")
	ft1.RecordRead("main.go")
	ft1.RecordRead("config.go")
	ft1.RecordModified("handler.go")
	ft1.RecordModified("handler.go")
	ft1.RecordModified("handler.go")

	summary := ft1.FormatForSummary()

	ft2 := NewFileTracker()
	ft2.ParseFromSummary(summary)

	if ft2.ReadFiles["main.go"] != 2 {
		t.Errorf("round-trip: main.go reads = %d, want 2", ft2.ReadFiles["main.go"])
	}
	if ft2.ReadFiles["config.go"] != 1 {
		t.Errorf("round-trip: config.go reads = %d, want 1", ft2.ReadFiles["config.go"])
	}
	if ft2.ModifiedFiles["handler.go"] != 3 {
		t.Errorf("round-trip: handler.go mods = %d, want 3", ft2.ModifiedFiles["handler.go"])
	}
}
