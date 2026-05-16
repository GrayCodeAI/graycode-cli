package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBeforeModifyCapturesState(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "test.go")
	content := []byte("package main\n\nfunc main() {}\n")
	if err := os.WriteFile(file, content, 0o644); err != nil {
		t.Fatal(err)
	}

	um := NewUndoManager()
	if err := um.BeforeModify(file); err != nil {
		t.Fatalf("BeforeModify failed: %v", err)
	}

	um.mu.Lock()
	cap, ok := um.pending[file]
	um.mu.Unlock()

	if !ok {
		t.Fatal("expected file to be in pending captures")
	}
	if string(cap.content) != string(content) {
		t.Errorf("captured content mismatch: got %q, want %q", cap.content, content)
	}
	if cap.wasNew {
		t.Error("expected wasNew to be false for existing file")
	}
	if cap.mode != 0o644 {
		t.Errorf("expected mode 0644, got %v", cap.mode)
	}
}

func TestBeforeModifyNonexistentFile(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "newfile.go")

	um := NewUndoManager()
	if err := um.BeforeModify(file); err != nil {
		t.Fatalf("BeforeModify failed for nonexistent file: %v", err)
	}

	um.mu.Lock()
	cap, ok := um.pending[file]
	um.mu.Unlock()

	if !ok {
		t.Fatal("expected file to be in pending captures")
	}
	if !cap.wasNew {
		t.Error("expected wasNew to be true for nonexistent file")
	}
	if cap.content != nil {
		t.Error("expected nil content for nonexistent file")
	}
}

func TestRecordChangeCreatesEntry(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "main.go")
	original := []byte("package main\n")
	if err := os.WriteFile(file, original, 0o644); err != nil {
		t.Fatal(err)
	}

	um := NewUndoManager()
	if err := um.BeforeModify(file); err != nil {
		t.Fatal(err)
	}

	// Simulate modification.
	modified := []byte("package main\n\nimport \"fmt\"\n")
	if err := os.WriteFile(file, modified, 0o644); err != nil {
		t.Fatal(err)
	}

	args := map[string]interface{}{"path": file, "content": string(modified)}
	id := um.RecordChange("Edit main.go: added import", "edit", args, []string{file})

	if id == "" {
		t.Fatal("expected non-empty ID")
	}
	if um.Size() != 1 {
		t.Fatalf("expected stack size 1, got %d", um.Size())
	}

	entry := um.Peek()
	if entry == nil {
		t.Fatal("expected non-nil entry from Peek")
	}
	if entry.ID != id {
		t.Errorf("entry ID mismatch: got %q, want %q", entry.ID, id)
	}
	if entry.Description != "Edit main.go: added import" {
		t.Errorf("description mismatch: got %q", entry.Description)
	}
	if entry.ToolName != "edit" {
		t.Errorf("tool name mismatch: got %q", entry.ToolName)
	}
	if len(entry.Files) != 1 {
		t.Fatalf("expected 1 file snapshot, got %d", len(entry.Files))
	}
	if string(entry.Files[0].OriginalContent) != string(original) {
		t.Error("original content mismatch in snapshot")
	}
	if string(entry.Files[0].ModifiedContent) != string(modified) {
		t.Error("modified content mismatch in snapshot")
	}
}

func TestUndoRestoresOriginalFile(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "config.go")
	original := []byte("timeout := 30\n")
	if err := os.WriteFile(file, original, 0o644); err != nil {
		t.Fatal(err)
	}

	um := NewUndoManager()
	if err := um.BeforeModify(file); err != nil {
		t.Fatal(err)
	}

	modified := []byte("timeout := 60\n")
	if err := os.WriteFile(file, modified, 0o644); err != nil {
		t.Fatal(err)
	}

	um.RecordChange("Edit config.go: updated timeout", "edit", nil, []string{file})

	entry, err := um.Undo()
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry from Undo")
	}

	restored, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(original) {
		t.Errorf("file not restored: got %q, want %q", restored, original)
	}

	if um.Size() != 0 {
		t.Errorf("expected empty stack after undo, got %d", um.Size())
	}
}

func TestUndoDeletesNewFiles(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "new_handler.go")

	um := NewUndoManager()
	// File doesn't exist yet.
	if err := um.BeforeModify(file); err != nil {
		t.Fatal(err)
	}

	// Create the file (simulates a write tool).
	content := []byte("package handlers\n\nfunc Handle() {}\n")
	if err := os.WriteFile(file, content, 0o644); err != nil {
		t.Fatal(err)
	}

	um.RecordChange("Write new_handler.go: created handler", "write", nil, []string{file})

	_, err := um.Undo()
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	if _, statErr := os.Stat(file); !os.IsNotExist(statErr) {
		t.Error("expected file to be deleted after undo of WasNew file")
	}
}

func TestUndoNReversesMultipleChanges(t *testing.T) {
	tmp := t.TempDir()

	// Create 3 files and record 3 changes.
	for i := 0; i < 3; i++ {
		file := filepath.Join(tmp, fmt.Sprintf("file%d.go", i))
		original := []byte(fmt.Sprintf("version := %d\n", i))
		if err := os.WriteFile(file, original, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	um := NewUndoManager()

	for i := 0; i < 3; i++ {
		file := filepath.Join(tmp, fmt.Sprintf("file%d.go", i))
		if err := um.BeforeModify(file); err != nil {
			t.Fatal(err)
		}

		modified := []byte(fmt.Sprintf("version := %d\n", i+10))
		if err := os.WriteFile(file, modified, 0o644); err != nil {
			t.Fatal(err)
		}
		um.RecordChange(fmt.Sprintf("Edit file%d.go", i), "edit", nil, []string{file})
	}

	if um.Size() != 3 {
		t.Fatalf("expected 3 entries, got %d", um.Size())
	}

	undone, err := um.UndoN(2)
	if err != nil {
		t.Fatalf("UndoN failed: %v", err)
	}
	if len(undone) != 2 {
		t.Fatalf("expected 2 undone entries, got %d", len(undone))
	}

	if um.Size() != 1 {
		t.Errorf("expected 1 entry remaining, got %d", um.Size())
	}

	// Verify file2 and file1 are restored.
	for i := 1; i < 3; i++ {
		file := filepath.Join(tmp, fmt.Sprintf("file%d.go", i))
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		expected := fmt.Sprintf("version := %d\n", i)
		if string(data) != expected {
			t.Errorf("file%d.go not restored: got %q, want %q", i, data, expected)
		}
	}

	// file0 should still be modified.
	data, _ := os.ReadFile(filepath.Join(tmp, "file0.go"))
	if string(data) != "version := 10\n" {
		t.Errorf("file0.go should remain modified: got %q", data)
	}
}

func TestUndoToGoesBackToSpecificPoint(t *testing.T) {
	tmp := t.TempDir()
	um := NewUndoManager()

	var targetID string
	for i := 0; i < 5; i++ {
		file := filepath.Join(tmp, fmt.Sprintf("f%d.go", i))
		original := []byte(fmt.Sprintf("val := %d\n", i))
		if err := os.WriteFile(file, original, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := um.BeforeModify(file); err != nil {
			t.Fatal(err)
		}

		modified := []byte(fmt.Sprintf("val := %d\n", i*100))
		if err := os.WriteFile(file, modified, 0o644); err != nil {
			t.Fatal(err)
		}
		id := um.RecordChange(fmt.Sprintf("Edit f%d.go", i), "edit", nil, []string{file})

		if i == 2 {
			targetID = id
		}
	}

	if um.Size() != 5 {
		t.Fatalf("expected 5 entries, got %d", um.Size())
	}

	undone, err := um.UndoTo(targetID)
	if err != nil {
		t.Fatalf("UndoTo failed: %v", err)
	}
	// Should undo entries 4, 3, 2 (3 total).
	if len(undone) != 3 {
		t.Fatalf("expected 3 undone entries, got %d", len(undone))
	}
	if um.Size() != 2 {
		t.Errorf("expected 2 remaining, got %d", um.Size())
	}

	// Files 2, 3, 4 should be restored.
	for i := 2; i < 5; i++ {
		file := filepath.Join(tmp, fmt.Sprintf("f%d.go", i))
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		expected := fmt.Sprintf("val := %d\n", i)
		if string(data) != expected {
			t.Errorf("f%d.go not restored: got %q, want %q", i, data, expected)
		}
	}
}

func TestHistoryReturnsCorrectEntries(t *testing.T) {
	um := NewUndoManager()

	// Manually push entries for testing.
	um.mu.Lock()
	for i := 0; i < 5; i++ {
		um.Stack = append(um.Stack, UndoEntry{
			ID:          fmt.Sprintf("id-%d", i),
			Timestamp:   time.Now().Add(-time.Duration(5-i) * time.Minute),
			Description: fmt.Sprintf("change %d", i),
		})
	}
	um.mu.Unlock()

	// Get last 3.
	entries := um.History(3)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Should be newest first.
	if entries[0].ID != "id-4" {
		t.Errorf("expected newest entry first, got %q", entries[0].ID)
	}
	if entries[2].ID != "id-2" {
		t.Errorf("expected third entry to be id-2, got %q", entries[2].ID)
	}
}

func TestMaxEntriesTrimsOldEntries(t *testing.T) {
	tmp := t.TempDir()
	um := NewUndoManager()
	um.MaxEntries = 5

	for i := 0; i < 10; i++ {
		file := filepath.Join(tmp, fmt.Sprintf("x%d.go", i))
		if err := os.WriteFile(file, []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := um.BeforeModify(file); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("b"), 0o644); err != nil {
			t.Fatal(err)
		}
		um.RecordChange(fmt.Sprintf("edit x%d.go", i), "edit", nil, []string{file})
	}

	if um.Size() != 5 {
		t.Errorf("expected stack to be trimmed to 5, got %d", um.Size())
	}

	// The earliest entry should be from edit 5 (0-4 trimmed).
	entries := um.History(5)
	if entries[4].Description != "edit x5.go" {
		t.Errorf("expected oldest entry to be 'edit x5.go', got %q", entries[4].Description)
	}
}

func TestPeekDoesNotRemove(t *testing.T) {
	um := NewUndoManager()
	um.mu.Lock()
	um.Stack = append(um.Stack, UndoEntry{
		ID:          "peek-test",
		Description: "test entry",
	})
	um.mu.Unlock()

	entry := um.Peek()
	if entry == nil {
		t.Fatal("expected non-nil from Peek")
	}
	if entry.ID != "peek-test" {
		t.Errorf("expected ID 'peek-test', got %q", entry.ID)
	}

	// Stack should still have the entry.
	if um.Size() != 1 {
		t.Error("Peek should not remove the entry from stack")
	}
}

func TestPeekEmptyStack(t *testing.T) {
	um := NewUndoManager()
	if entry := um.Peek(); entry != nil {
		t.Error("expected nil from Peek on empty stack")
	}
}

func TestFormatHistoryOutput(t *testing.T) {
	entries := []UndoEntry{
		{
			ID:          "a1",
			Timestamp:   time.Now().Add(-2 * time.Minute),
			Description: "Edit auth.go: added JWT validation",
			Files: []FileSnapshot{
				{
					OriginalContent: []byte("line1\nline2\nline3\n"),
					ModifiedContent: []byte("line1\nline2\nline3\nline4\nline5\n"),
				},
			},
		},
		{
			ID:          "a2",
			Timestamp:   time.Now().Add(-5 * time.Minute),
			Description: "Write middleware.go: created rate limiter",
			Files: []FileSnapshot{
				{
					WasNew:          true,
					ModifiedContent: []byte("line1\nline2\nline3\n"),
				},
			},
		},
	}

	um := NewUndoManager()
	output := um.FormatHistory(entries)

	if !strings.Contains(output, "Undo History (last 2):") {
		t.Error("expected header in output")
	}
	if !strings.Contains(output, "Edit auth.go: added JWT validation") {
		t.Error("expected first entry description")
	}
	if !strings.Contains(output, "Write middleware.go: created rate limiter") {
		t.Error("expected second entry description")
	}
	if !strings.Contains(output, "2m ago") {
		t.Errorf("expected '2m ago' in output, got:\n%s", output)
	}
}

func TestFormatHistoryEmpty(t *testing.T) {
	um := NewUndoManager()
	output := um.FormatHistory(nil)
	if !strings.Contains(output, "empty") {
		t.Errorf("expected 'empty' in output for nil entries, got: %q", output)
	}
}

func TestDiffEntryShowsChanges(t *testing.T) {
	um := NewUndoManager()

	entry := &UndoEntry{
		ID: "diff-test",
		Files: []FileSnapshot{
			{
				Path:            "/tmp/test.go",
				OriginalContent: []byte("func old() {}\n"),
				ModifiedContent: []byte("func new() {}\n"),
			},
		},
	}

	diff := um.DiffEntry(entry)

	if !strings.Contains(diff, "--- /tmp/test.go") {
		t.Error("expected --- header in diff")
	}
	if !strings.Contains(diff, "+++ /tmp/test.go") {
		t.Error("expected +++ header in diff")
	}
	if !strings.Contains(diff, "-func old() {}") {
		t.Errorf("expected removed line in diff, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+func new() {}") {
		t.Errorf("expected added line in diff, got:\n%s", diff)
	}
}

func TestDiffEntryNewFile(t *testing.T) {
	um := NewUndoManager()

	entry := &UndoEntry{
		ID: "new-file-diff",
		Files: []FileSnapshot{
			{
				Path:            "/tmp/new.go",
				WasNew:          true,
				ModifiedContent: []byte("package new\n\nfunc Init() {}\n"),
			},
		},
	}

	diff := um.DiffEntry(entry)
	if !strings.Contains(diff, "(new file)") {
		t.Error("expected '(new file)' marker in diff")
	}
	if !strings.Contains(diff, "+package new") {
		t.Errorf("expected added lines in diff for new file, got:\n%s", diff)
	}
}

func TestDiffEntryNil(t *testing.T) {
	um := NewUndoManager()
	if diff := um.DiffEntry(nil); diff != "" {
		t.Errorf("expected empty string for nil entry, got %q", diff)
	}
}

func TestEmptyStackReturnsError(t *testing.T) {
	um := NewUndoManager()

	_, err := um.Undo()
	if err == nil {
		t.Error("expected error from Undo on empty stack")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error message, got %q", err.Error())
	}

	_, err = um.UndoN(1)
	if err == nil {
		t.Error("expected error from UndoN on empty stack")
	}

	_, err = um.UndoTo("nonexistent")
	if err == nil {
		t.Error("expected error from UndoTo with bad ID")
	}
}

func TestUndoNInvalidCount(t *testing.T) {
	um := NewUndoManager()
	um.mu.Lock()
	um.Stack = append(um.Stack, UndoEntry{ID: "x"})
	um.mu.Unlock()

	_, err := um.UndoN(0)
	if err == nil {
		t.Error("expected error for n=0")
	}

	_, err = um.UndoN(5)
	if err == nil {
		t.Error("expected error when n exceeds stack size")
	}
}

func TestConcurrentAccessSafety(t *testing.T) {
	tmp := t.TempDir()
	um := NewUndoManager()

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	// Concurrent writers.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			file := filepath.Join(tmp, fmt.Sprintf("concurrent%d.go", idx))
			if err := os.WriteFile(file, []byte(fmt.Sprintf("v%d", idx)), 0o644); err != nil {
				errCh <- err
				return
			}
			if err := um.BeforeModify(file); err != nil {
				errCh <- err
				return
			}
			if err := os.WriteFile(file, []byte(fmt.Sprintf("v%d-modified", idx)), 0o644); err != nil {
				errCh <- err
				return
			}
			um.RecordChange(fmt.Sprintf("edit %d", idx), "edit", nil, []string{file})
		}(i)
	}

	// Concurrent readers.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = um.Size()
			_ = um.Peek()
			_ = um.History(5)
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent error: %v", err)
	}

	// Verify no panic occurred and stack is non-empty.
	if um.Size() == 0 {
		t.Error("expected non-empty stack after concurrent writes")
	}
}

func TestUndoClear(t *testing.T) {
	um := NewUndoManager()
	um.mu.Lock()
	um.Stack = append(um.Stack, UndoEntry{ID: "a"}, UndoEntry{ID: "b"})
	um.pending["foo"] = fileCapture{}
	um.mu.Unlock()

	um.Clear()

	if um.Size() != 0 {
		t.Error("expected empty stack after Clear")
	}
	um.mu.Lock()
	pendingLen := len(um.pending)
	um.mu.Unlock()
	if pendingLen != 0 {
		t.Error("expected empty pending map after Clear")
	}
}

func TestMultipleFilesInSingleEntry(t *testing.T) {
	tmp := t.TempDir()
	um := NewUndoManager()

	files := make([]string, 3)
	for i := 0; i < 3; i++ {
		files[i] = filepath.Join(tmp, fmt.Sprintf("multi%d.go", i))
		if err := os.WriteFile(files[i], []byte(fmt.Sprintf("original%d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := um.BeforeModify(files[i]); err != nil {
			t.Fatal(err)
		}
	}

	// Modify all files.
	for i, f := range files {
		if err := os.WriteFile(f, []byte(fmt.Sprintf("modified%d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	um.RecordChange("Edit multiple files", "edit", nil, files)

	if um.Size() != 1 {
		t.Fatalf("expected 1 entry, got %d", um.Size())
	}

	entry := um.Peek()
	if len(entry.Files) != 3 {
		t.Fatalf("expected 3 file snapshots, got %d", len(entry.Files))
	}

	// Undo should restore all 3 files.
	if _, err := um.Undo(); err != nil {
		t.Fatal(err)
	}

	for i, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		expected := fmt.Sprintf("original%d", i)
		if string(data) != expected {
			t.Errorf("file %d not restored: got %q, want %q", i, data, expected)
		}
	}
}
