package docs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocUpdater_DetectStaleDocumentation(t *testing.T) {
	du := NewDocUpdater()
	oldContent := `// MyFunc does something.
func MyFunc(a int) string { return "" }`
	newContent := `// MyFunc does something.
func MyFunc(a int, b string) string { return "" }`

	updates := du.DetectStaleDocumentation("test.go", oldContent, newContent)
	if len(updates) == 0 {
		t.Fatal("expected stale doc detection")
	}
	if updates[0].Reason != "signature_changed (added b parameter)" {
		t.Errorf("reason = %q", updates[0].Reason)
	}
}

func TestDocUpdater_NoChanges(t *testing.T) {
	du := NewDocUpdater()
	content := `// MyFunc does something.
func MyFunc(a int) string { return "" }`
	updates := du.DetectStaleDocumentation("test.go", content, content)
	if len(updates) != 0 {
		t.Errorf("expected no updates, got %d", len(updates))
	}
}

func TestDocUpdater_FormatUpdates(t *testing.T) {
	du := NewDocUpdater()
	updates := []DocUpdate{
		{File: "test.go", Line: 5, Symbol: "MyFunc", Reason: "signature_changed"},
	}
	result := du.FormatUpdates(updates)
	if !fileExists(t, result) {
		t.Error("expected formatted output")
	}

	empty := du.FormatUpdates(nil)
	if empty != "No stale documentation found." {
		t.Errorf("expected no-stale message, got %q", empty)
	}
}

func TestDocUpdater_ApplyUpdates(t *testing.T) {
	du := NewDocUpdater()
	content := "// Old doc.\nfunc Foo() {}"
	updates := []DocUpdate{
		{Line: 2, OldDoc: "// Old doc.", NewDoc: "// New doc."},
	}
	result := du.ApplyUpdates(updates, content)
	if result != "// New doc.\nfunc Foo() {}" {
		t.Errorf("got %q", result)
	}
}

func TestDocUpdater_ScanProjectForStaleDocs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\n// Uses NonexistentType for processing.\nfunc Process() {}"), 0o644)

	du := NewDocUpdater()
	updates := du.ScanProjectForStaleDocs(dir)
	_ = updates
}

func fileExists(t *testing.T, s string) bool {
	t.Helper()
	return len(s) > 0
}
