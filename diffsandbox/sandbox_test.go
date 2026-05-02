package diffsandbox

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestProposeCreateAndApply(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	c := sb.ProposeCreate("hello.txt", "hello world\n")
	if c == nil {
		t.Fatal("expected non-nil change")
	}
	if c.Type != ChangeCreate {
		t.Errorf("expected ChangeCreate, got %v", c.Type)
	}
	if len(c.ID) != 8 {
		t.Errorf("expected 8-char ID, got %q", c.ID)
	}

	if err := sb.Apply(); err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if string(data) != "hello world\n" {
		t.Errorf("expected 'hello world\\n', got %q", string(data))
	}

	if sb.HasChanges() {
		t.Error("sandbox should be empty after Apply")
	}
}

func TestProposeModifyAndApply(t *testing.T) {
	dir := t.TempDir()
	// Create original file
	origPath := filepath.Join(dir, "mod.txt")
	if err := os.WriteFile(origPath, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sb := New(dir)
	c, err := sb.ProposeModify("mod.txt", "modified\n")
	if err != nil {
		t.Fatalf("ProposeModify error: %v", err)
	}
	if c.Type != ChangeModify {
		t.Errorf("expected ChangeModify, got %v", c.Type)
	}
	if c.Original != "original\n" {
		t.Errorf("expected original content, got %q", c.Original)
	}

	if err := sb.Apply(); err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	data, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "modified\n" {
		t.Errorf("expected 'modified\\n', got %q", string(data))
	}
}

func TestProposeDeleteAndApply(t *testing.T) {
	dir := t.TempDir()
	delPath := filepath.Join(dir, "del.txt")
	if err := os.WriteFile(delPath, []byte("bye\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sb := New(dir)
	c := sb.ProposeDelete(delPath) // absolute path
	if c.Type != ChangeDelete {
		t.Errorf("expected ChangeDelete, got %v", c.Type)
	}

	if err := sb.Apply(); err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	if _, err := os.Stat(delPath); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}

func TestDiffOutputFormat(t *testing.T) {
	dir := t.TempDir()
	origPath := filepath.Join(dir, "diff.txt")
	if err := os.WriteFile(origPath, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sb := New(dir)
	_, err := sb.ProposeModify("diff.txt", "line1\nline2\nline3\n")
	if err != nil {
		t.Fatal(err)
	}

	diff := sb.Diff()
	if !strings.Contains(diff, "---") {
		t.Error("diff should contain --- header")
	}
	if !strings.Contains(diff, "+++") {
		t.Error("diff should contain +++ header")
	}
	if !strings.Contains(diff, "@@") {
		t.Error("diff should contain @@ hunk header")
	}
	if !strings.Contains(diff, "+line3") {
		t.Errorf("diff should show added line, got:\n%s", diff)
	}
}

func TestDiffFileOutput(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)
	sb.ProposeCreate("new.txt", "content\n")

	diff := sb.DiffFile("new.txt")
	if !strings.Contains(diff, "+content") {
		t.Errorf("DiffFile should show added content, got:\n%s", diff)
	}

	if sb.DiffFile("nonexistent.txt") != "" {
		t.Error("DiffFile for nonexistent should return empty")
	}
}

func TestDiscard(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	sb.ProposeCreate("a.txt", "aaa\n")
	sb.ProposeCreate("b.txt", "bbb\n")

	if !sb.HasChanges() {
		t.Error("should have changes before discard")
	}

	sb.Discard()

	if sb.HasChanges() {
		t.Error("should not have changes after discard")
	}

	// Verify filesystem unchanged
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); !os.IsNotExist(err) {
		t.Error("a.txt should not exist after discard")
	}
}

func TestDiscardFile(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	sb.ProposeCreate("a.txt", "aaa\n")
	sb.ProposeCreate("b.txt", "bbb\n")

	sb.DiscardFile("a.txt")

	changes := sb.Changes()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Path != "b.txt" {
		t.Errorf("expected b.txt remaining, got %s", changes[0].Path)
	}
}

func TestMultipleChangesAccumulated(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	sb.ProposeCreate("a.txt", "aaa\n")
	sb.ProposeCreate("b.txt", "bbb\n")
	sb.ProposeCreate("c.txt", "ccc\n")

	changes := sb.Changes()
	if len(changes) != 3 {
		t.Fatalf("expected 3 changes, got %d", len(changes))
	}

	// Should be in insertion order
	if changes[0].Path != "a.txt" || changes[1].Path != "b.txt" || changes[2].Path != "c.txt" {
		t.Errorf("unexpected order: %v, %v, %v", changes[0].Path, changes[1].Path, changes[2].Path)
	}
}

func TestStatsCalculation(t *testing.T) {
	dir := t.TempDir()
	origPath := filepath.Join(dir, "mod.txt")
	if err := os.WriteFile(origPath, []byte("old1\nold2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sb := New(dir)
	sb.ProposeCreate("new.txt", "line1\nline2\nline3\n")
	if _, err := sb.ProposeModify("mod.txt", "old1\nnew2\n"); err != nil {
		t.Fatal(err)
	}
	sb.ProposeDelete("del.txt")

	stats := sb.Stats()
	if stats.FilesCreated != 1 {
		t.Errorf("expected 1 created, got %d", stats.FilesCreated)
	}
	if stats.FilesModified != 1 {
		t.Errorf("expected 1 modified, got %d", stats.FilesModified)
	}
	if stats.FilesDeleted != 1 {
		t.Errorf("expected 1 deleted, got %d", stats.FilesDeleted)
	}
	if stats.LinesAdded < 1 {
		t.Errorf("expected at least 1 line added, got %d", stats.LinesAdded)
	}
	if stats.LinesRemoved < 1 {
		t.Errorf("expected at least 1 line removed, got %d", stats.LinesRemoved)
	}
}

func TestSummaryOutput(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	summary := sb.Summary()
	if !strings.Contains(summary, "No pending changes") {
		t.Errorf("empty sandbox should say no pending changes, got: %s", summary)
	}

	sb.ProposeCreate("test.go", "package main\n")
	summary = sb.Summary()
	if !strings.Contains(summary, "1 file(s)") {
		t.Errorf("expected 1 file(s) in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "[create]") {
		t.Errorf("expected [create] in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "Stats:") {
		t.Errorf("expected Stats: in summary, got: %s", summary)
	}
}

func TestHasChanges(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	if sb.HasChanges() {
		t.Error("new sandbox should not have changes")
	}

	sb.ProposeCreate("test.txt", "content\n")
	if !sb.HasChanges() {
		t.Error("should have changes after ProposeCreate")
	}

	sb.Discard()
	if sb.HasChanges() {
		t.Error("should not have changes after Discard")
	}
}

func TestOverwritePreviousProposal(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	sb.ProposeCreate("test.txt", "version1\n")
	sb.ProposeCreate("test.txt", "version2\n")

	changes := sb.Changes()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change after overwrite, got %d", len(changes))
	}
	if changes[0].Content != "version2\n" {
		t.Errorf("expected version2 content, got %q", changes[0].Content)
	}

	// Apply and verify final content
	if err := sb.Apply(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "version2\n" {
		t.Errorf("expected version2, got %q", string(data))
	}
}

func TestApplyFile(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	sb.ProposeCreate("a.txt", "aaa\n")
	sb.ProposeCreate("b.txt", "bbb\n")

	// Apply only a.txt
	if err := sb.ApplyFile("a.txt"); err != nil {
		t.Fatalf("ApplyFile error: %v", err)
	}

	// a.txt should exist on disk
	data, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "aaa\n" {
		t.Errorf("expected aaa, got %q", string(data))
	}

	// b.txt should still be pending
	changes := sb.Changes()
	if len(changes) != 1 {
		t.Fatalf("expected 1 remaining change, got %d", len(changes))
	}
	if changes[0].Path != "b.txt" {
		t.Errorf("expected b.txt remaining, got %s", changes[0].Path)
	}

	// b.txt should not exist on disk yet
	if _, err := os.Stat(filepath.Join(dir, "b.txt")); !os.IsNotExist(err) {
		t.Error("b.txt should not exist on disk yet")
	}
}

func TestApplyFileNotFound(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	err := sb.ApplyFile("nonexistent.txt")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	var wg sync.WaitGroup
	n := 50

	// Concurrent creates
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			path := filepath.Join("concurrent", strings.Replace(
				strings.Replace(t.Name(), "/", "_", -1), " ", "_", -1),
				strings.Replace(filepath.Base(t.Name()), " ", "_", -1)+
					"_"+strings.Replace(string(rune('a'+idx%26)), " ", "", -1)+".txt")
			sb.ProposeCreate(path, "content\n")
		}(i)
	}

	// Concurrent reads
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sb.HasChanges()
			_ = sb.Changes()
			_ = sb.Diff()
			_ = sb.Stats()
			_ = sb.Summary()
		}()
	}

	wg.Wait()

	if !sb.HasChanges() {
		t.Error("should have changes after concurrent writes")
	}

	// Concurrent discard
	var wg2 sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			sb.Discard()
		}()
	}
	wg2.Wait()

	if sb.HasChanges() {
		t.Error("should not have changes after concurrent discard")
	}
}

func TestProposeModifyNonexistentFile(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	_, err := sb.ProposeModify("nonexistent.txt", "content\n")
	if err == nil {
		t.Error("expected error when modifying nonexistent file")
	}
}

func TestCreateSubdirectory(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	sb.ProposeCreate("sub/dir/file.txt", "nested\n")
	if err := sb.Apply(); err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "sub", "dir", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "nested\n" {
		t.Errorf("expected nested content, got %q", string(data))
	}
}

func TestDiffCreateFromEmpty(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	sb.ProposeCreate("new.go", "package main\n\nfunc main() {}\n")
	diff := sb.Diff()

	if !strings.Contains(diff, "+package main") {
		t.Errorf("diff should show added lines, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+func main()") {
		t.Errorf("diff should show added func, got:\n%s", diff)
	}
}

func TestDiffDelete(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	// ProposeDelete with original content loaded manually
	sb.mu.Lock()
	c := &Change{
		ID:       generateID(),
		Path:     "removed.txt",
		Type:     ChangeDelete,
		Original: "this will be gone\n",
	}
	sb.changes["removed.txt"] = c
	sb.order = append(sb.order, "removed.txt")
	sb.mu.Unlock()

	diff := sb.Diff()
	if !strings.Contains(diff, "-this will be gone") {
		t.Errorf("diff should show removed line, got:\n%s", diff)
	}
}

func TestChangeTypeString(t *testing.T) {
	tests := []struct {
		ct   ChangeType
		want string
	}{
		{ChangeCreate, "create"},
		{ChangeModify, "modify"},
		{ChangeDelete, "delete"},
		{ChangeType(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.ct.String(); got != tt.want {
			t.Errorf("ChangeType(%d).String() = %q, want %q", tt.ct, got, tt.want)
		}
	}
}

func TestSortedPaths(t *testing.T) {
	dir := t.TempDir()
	sb := New(dir)

	sb.ProposeCreate("c.txt", "c\n")
	sb.ProposeCreate("a.txt", "a\n")
	sb.ProposeCreate("b.txt", "b\n")

	paths := sb.SortedPaths()
	if len(paths) != 3 {
		t.Fatalf("expected 3 paths, got %d", len(paths))
	}
	if paths[0] != "a.txt" || paths[1] != "b.txt" || paths[2] != "c.txt" {
		t.Errorf("expected sorted paths, got %v", paths)
	}
}
