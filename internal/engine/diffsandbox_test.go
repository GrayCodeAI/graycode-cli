package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffSandbox_StageAndList(t *testing.T) {
	ds := NewDiffSandbox()

	ds.Stage("/tmp/a.go", "create", "", "package main\n")
	ds.Stage("/tmp/b.go", "edit", "old line\n", "new line\n")

	list := ds.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 pending changes, got %d", len(list))
	}
	// List should be sorted by path
	if list[0].Path != "/tmp/a.go" {
		t.Errorf("expected first path /tmp/a.go, got %s", list[0].Path)
	}
	if list[1].Path != "/tmp/b.go" {
		t.Errorf("expected second path /tmp/b.go, got %s", list[1].Path)
	}
}

func TestDiffSandbox_Get(t *testing.T) {
	ds := NewDiffSandbox()
	ds.Stage("/tmp/test.go", "create", "", "package main\n")

	c := ds.Get("/tmp/test.go")
	if c == nil {
		t.Fatal("expected non-nil change")
	}
	if c.Action != "create" {
		t.Errorf("expected action create, got %s", c.Action)
	}

	if ds.Get("/tmp/nonexistent.go") != nil {
		t.Error("expected nil for nonexistent path")
	}
}

func TestDiffSandbox_Apply(t *testing.T) {
	ds := NewDiffSandbox()
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")

	ds.Stage(path, "create", "", "hello world\n")

	if err := ds.Apply(path); err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if string(data) != "hello world\n" {
		t.Errorf("expected 'hello world\\n', got %q", string(data))
	}

	// Should be removed from sandbox
	if ds.Get(path) != nil {
		t.Error("change should be removed after apply")
	}
}

func TestDiffSandbox_ApplyAll(t *testing.T) {
	ds := NewDiffSandbox()
	dir := t.TempDir()

	ds.Stage(filepath.Join(dir, "a.txt"), "create", "", "aaa\n")
	ds.Stage(filepath.Join(dir, "b.txt"), "create", "", "bbb\n")

	n, err := ds.ApplyAll()
	if err != nil {
		t.Fatalf("ApplyAll error: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 applied, got %d", n)
	}

	list := ds.List()
	if len(list) != 0 {
		t.Error("sandbox should be empty after ApplyAll")
	}
}

func TestDiffSandbox_Apply_NotFound(t *testing.T) {
	ds := NewDiffSandbox()
	err := ds.Apply("/tmp/does-not-exist-in-sandbox")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestDiffSandbox_Reject(t *testing.T) {
	ds := NewDiffSandbox()
	ds.Stage("/tmp/a.go", "create", "", "package main\n")
	ds.Stage("/tmp/b.go", "create", "", "package main\n")

	ds.Reject("/tmp/a.go")
	if ds.Get("/tmp/a.go") != nil {
		t.Error("a.go should be rejected")
	}
	if ds.Get("/tmp/b.go") == nil {
		t.Error("b.go should still exist")
	}
}

func TestDiffSandbox_RejectAll(t *testing.T) {
	ds := NewDiffSandbox()
	ds.Stage("/tmp/a.go", "create", "", "package main\n")
	ds.Stage("/tmp/b.go", "create", "", "package main\n")

	ds.RejectAll()
	if len(ds.List()) != 0 {
		t.Error("all changes should be rejected")
	}
}

func TestDiffSandbox_EnableDisable(t *testing.T) {
	ds := NewDiffSandbox()
	if !ds.IsEnabled() {
		t.Error("should be enabled by default")
	}
	ds.Disable()
	if ds.IsEnabled() {
		t.Error("should be disabled")
	}
	ds.Enable()
	if !ds.IsEnabled() {
		t.Error("should be enabled again")
	}
}

func TestDiffSandbox_DiffFor(t *testing.T) {
	ds := NewDiffSandbox()
	ds.Stage("test.go", "edit", "line1\nline2\n", "line1\nline2\nline3\n")

	diff := ds.DiffFor("test.go")
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(diff, "+line3") {
		t.Errorf("diff should show added line3, got:\n%s", diff)
	}
}

func TestDiffSandbox_DiffAll(t *testing.T) {
	ds := NewDiffSandbox()
	ds.Stage("a.go", "edit", "old\n", "new\n")
	ds.Stage("b.go", "create", "", "content\n")

	all := ds.DiffAll()
	if !strings.Contains(all, "a.go") || !strings.Contains(all, "b.go") {
		t.Errorf("DiffAll should contain both files, got:\n%s", all)
	}
}

func TestDiffSandbox_DiffAll_Empty(t *testing.T) {
	ds := NewDiffSandbox()
	if ds.DiffAll() != "" {
		t.Error("empty sandbox should return empty diff")
	}
}

func TestDiffSandbox_Format(t *testing.T) {
	ds := NewDiffSandbox()
	if !strings.Contains(ds.Format(), "No pending changes") {
		t.Error("empty sandbox should say no pending changes")
	}

	ds.Stage("test.go", "edit", "a\n", "b\n")
	formatted := ds.Format()
	if !strings.Contains(formatted, "1 files") {
		t.Errorf("expected 1 files in format, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "[edit]") {
		t.Errorf("expected [edit] in format, got:\n%s", formatted)
	}
}

func TestUnifiedDiff_Addition(t *testing.T) {
	old := "line1\nline2\n"
	new := "line1\nline2\nline3\n"
	diff := unifiedDiff(old, new, "test.go")

	if !strings.Contains(diff, "--- a/test.go") {
		t.Error("missing old file header")
	}
	if !strings.Contains(diff, "+++ b/test.go") {
		t.Error("missing new file header")
	}
	if !strings.Contains(diff, "+line3") {
		t.Errorf("should show added line, got:\n%s", diff)
	}
}

func TestUnifiedDiff_Deletion(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nline3\n"
	diff := unifiedDiff(old, new, "test.go")

	if !strings.Contains(diff, "-line2") {
		t.Errorf("should show removed line, got:\n%s", diff)
	}
}

func TestUnifiedDiff_Replacement(t *testing.T) {
	old := "hello\n"
	new := "world\n"
	diff := unifiedDiff(old, new, "test.go")

	if !strings.Contains(diff, "-hello") {
		t.Errorf("should show removed line, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+world") {
		t.Errorf("should show added line, got:\n%s", diff)
	}
}

func TestUnifiedDiff_Empty(t *testing.T) {
	diff := unifiedDiff("", "", "test.go")
	// No hunks expected for identical empty files
	if strings.Contains(diff, "@@") {
		t.Error("identical empty content should produce no hunks")
	}
}

func TestUnifiedDiff_CreateFromEmpty(t *testing.T) {
	diff := unifiedDiff("", "new content\n", "test.go")
	if !strings.Contains(diff, "+new content") {
		t.Errorf("should show added content, got:\n%s", diff)
	}
}
