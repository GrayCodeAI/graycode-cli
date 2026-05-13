package engine

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewStagingArea(t *testing.T) {
	sa := NewStagingArea()
	if sa == nil {
		t.Fatal("NewStagingArea returned nil")
	}
	if sa.Staged == nil {
		t.Fatal("Staged map is nil")
	}
	if len(sa.Staged) != 0 {
		t.Fatalf("expected empty staging area, got %d entries", len(sa.Staged))
	}
}

func TestStage(t *testing.T) {
	sa := NewStagingArea()

	original := "line1\nline2\nline3\n"
	modified := "line1\nline2 modified\nline3\nline4\n"
	sa.Stage("src/auth.go", original, modified, "update auth logic")

	if len(sa.Staged) != 1 {
		t.Fatalf("expected 1 staged change, got %d", len(sa.Staged))
	}

	change, ok := sa.Staged["src/auth.go"]
	if !ok {
		t.Fatal("staged change not found for src/auth.go")
	}
	if change.File != "src/auth.go" {
		t.Errorf("expected file src/auth.go, got %s", change.File)
	}
	if change.Status != "staged" {
		t.Errorf("expected status staged, got %s", change.Status)
	}
	if change.Description != "update auth logic" {
		t.Errorf("expected description 'update auth logic', got %s", change.Description)
	}
	if change.Original != original {
		t.Error("original content mismatch")
	}
	if change.Modified != modified {
		t.Error("modified content mismatch")
	}
	if len(change.Hunks) == 0 {
		t.Error("expected at least one hunk")
	}
}

func TestStageMultipleFiles(t *testing.T) {
	sa := NewStagingArea()

	sa.Stage("a.go", "old a\n", "new a\n", "update a")
	sa.Stage("b.go", "old b\n", "new b\n", "update b")
	sa.Stage("c.go", "", "new file\n", "add c")

	if len(sa.Staged) != 3 {
		t.Fatalf("expected 3 staged changes, got %d", len(sa.Staged))
	}
}

func TestStageOverwrite(t *testing.T) {
	sa := NewStagingArea()

	sa.Stage("file.go", "v1\n", "v2\n", "first change")
	sa.Stage("file.go", "v1\n", "v3\n", "second change")

	if len(sa.Staged) != 1 {
		t.Fatalf("expected 1 staged change after overwrite, got %d", len(sa.Staged))
	}
	if sa.Staged["file.go"].Description != "second change" {
		t.Error("expected overwritten description")
	}
}

func TestApplyFile(t *testing.T) {
	sa := NewStagingArea()
	dir := t.TempDir()
	file := filepath.Join(dir, "test.go")

	original := "package main\n\nfunc main() {}\n"
	modified := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"

	sa.Stage(file, original, modified, "add hello world")

	err := sa.ApplyFile(file)
	if err != nil {
		t.Fatalf("ApplyFile failed: %v", err)
	}

	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading applied file: %v", err)
	}
	if string(content) != modified {
		t.Errorf("applied content mismatch.\nGot:\n%s\nWant:\n%s", string(content), modified)
	}

	if sa.Staged[file].Status != "applied" {
		t.Errorf("expected status applied, got %s", sa.Staged[file].Status)
	}
}

func TestApplyFileNotFound(t *testing.T) {
	sa := NewStagingArea()
	err := sa.ApplyFile("/nonexistent/file.go")
	if err == nil {
		t.Fatal("expected error for non-staged file")
	}
	if !strings.Contains(err.Error(), "no staged change") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestApplyFileAlreadyApplied(t *testing.T) {
	sa := NewStagingArea()
	dir := t.TempDir()
	file := filepath.Join(dir, "test.go")

	sa.Stage(file, "old\n", "new\n", "change")
	_ = sa.ApplyFile(file)

	err := sa.ApplyFile(file)
	if err == nil {
		t.Fatal("expected error for already-applied file")
	}
	if !strings.Contains(err.Error(), "not in staged status") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestApplyAll(t *testing.T) {
	sa := NewStagingArea()
	dir := t.TempDir()

	file1 := filepath.Join(dir, "a.go")
	file2 := filepath.Join(dir, "b.go")

	sa.Stage(file1, "", "package a\n", "create a")
	sa.Stage(file2, "", "package b\n", "create b")

	applied, err := sa.ApplyAll()
	if err != nil {
		t.Fatalf("ApplyAll failed: %v", err)
	}
	if len(applied) != 2 {
		t.Fatalf("expected 2 applied files, got %d", len(applied))
	}

	// Verify files written
	for _, f := range []string{file1, file2} {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Errorf("file %s was not created", f)
		}
	}

	// Verify statuses
	for _, change := range sa.Staged {
		if change.Status != "applied" {
			t.Errorf("expected applied status for %s, got %s", change.File, change.Status)
		}
	}
}

func TestApplyAllCreatesDirectories(t *testing.T) {
	sa := NewStagingArea()
	dir := t.TempDir()
	nested := filepath.Join(dir, "deep", "nested", "dir", "file.go")

	sa.Stage(nested, "", "package deep\n", "create nested file")

	applied, err := sa.ApplyAll()
	if err != nil {
		t.Fatalf("ApplyAll failed: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("expected 1 applied file, got %d", len(applied))
	}

	content, err := os.ReadFile(nested)
	if err != nil {
		t.Fatalf("reading nested file: %v", err)
	}
	if string(content) != "package deep\n" {
		t.Errorf("unexpected content: %s", string(content))
	}
}

func TestReject(t *testing.T) {
	sa := NewStagingArea()
	sa.Stage("file.go", "old\n", "new\n", "test change")

	sa.Reject("file.go")

	change := sa.Staged["file.go"]
	if change.Status != "rejected" {
		t.Errorf("expected rejected status, got %s", change.Status)
	}
}

func TestRejectNonExistent(t *testing.T) {
	sa := NewStagingArea()
	// Should not panic
	sa.Reject("nonexistent.go")
}

func TestRejectHunk(t *testing.T) {
	sa := NewStagingArea()

	original := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	modified := "line1\nMODIFIED2\nline3\nline4\nline5\nline6\nline7\nMODIFIED8\nline9\nline10\n"

	sa.Stage("multi.go", original, modified, "multiple hunks")

	change := sa.Staged["multi.go"]
	if len(change.Hunks) < 2 {
		t.Fatalf("expected at least 2 hunks, got %d", len(change.Hunks))
	}

	// Reject the second hunk
	sa.RejectHunk("multi.go", 1)

	if change.Hunks[1].Approved {
		t.Error("expected hunk 1 to be rejected (not approved)")
	}
	if !change.Hunks[0].Approved {
		t.Error("expected hunk 0 to remain approved")
	}
}

func TestRejectHunkOutOfBounds(t *testing.T) {
	sa := NewStagingArea()
	sa.Stage("file.go", "old\n", "new\n", "change")

	// Should not panic
	sa.RejectHunk("file.go", 99)
	sa.RejectHunk("file.go", -1)
	sa.RejectHunk("nonexistent.go", 0)
}

func TestApproveHunk(t *testing.T) {
	sa := NewStagingArea()
	sa.Stage("file.go", "old\n", "new\n", "change")

	// First reject, then approve
	sa.RejectHunk("file.go", 0)
	if sa.Staged["file.go"].Hunks[0].Approved {
		t.Error("hunk should be rejected")
	}

	sa.ApproveHunk("file.go", 0)
	if !sa.Staged["file.go"].Hunks[0].Approved {
		t.Error("hunk should be approved after ApproveHunk")
	}
}

func TestApproveHunkOutOfBounds(t *testing.T) {
	sa := NewStagingArea()
	sa.Stage("file.go", "old\n", "new\n", "change")

	// Should not panic
	sa.ApproveHunk("file.go", 99)
	sa.ApproveHunk("file.go", -1)
	sa.ApproveHunk("nonexistent.go", 0)
}

func TestGetStaged(t *testing.T) {
	sa := NewStagingArea()
	sa.Stage("a.go", "old\n", "new\n", "change a")
	sa.Stage("b.go", "old\n", "new\n", "change b")

	staged := sa.GetStaged()
	if len(staged) != 2 {
		t.Fatalf("expected 2 staged changes, got %d", len(staged))
	}
	if _, ok := staged["a.go"]; !ok {
		t.Error("missing a.go from GetStaged")
	}
	if _, ok := staged["b.go"]; !ok {
		t.Error("missing b.go from GetStaged")
	}
}

func TestGetDiff(t *testing.T) {
	sa := NewStagingArea()

	original := "line1\nline2\nline3\n"
	modified := "line1\nline2 changed\nline3\n"

	sa.Stage("file.go", original, modified, "test")

	diff := sa.GetDiff("file.go")
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(diff, "--- a/file.go") {
		t.Error("diff missing --- header")
	}
	if !strings.Contains(diff, "+++ b/file.go") {
		t.Error("diff missing +++ header")
	}
	if !strings.Contains(diff, "-line2") {
		t.Error("diff missing removed line")
	}
	if !strings.Contains(diff, "+line2 changed") {
		t.Error("diff missing added line")
	}
}

func TestGetDiffNonExistent(t *testing.T) {
	sa := NewStagingArea()
	diff := sa.GetDiff("nonexistent.go")
	if diff != "" {
		t.Error("expected empty diff for non-staged file")
	}
}

func TestGetDiffNoChanges(t *testing.T) {
	sa := NewStagingArea()
	sa.Stage("same.go", "content\n", "content\n", "no change")

	diff := sa.GetDiff("same.go")
	// When content is identical, diff should be empty or minimal
	if strings.Contains(diff, "-content") || strings.Contains(diff, "+content") {
		t.Error("expected no diff for identical content")
	}
}

func TestFormatStaging(t *testing.T) {
	sa := NewStagingArea()

	sa.Stage("src/auth.go", "line1\nline2\nline3\n", "line1\nline2 modified\nline3\nnewline1\nnewline2\n", "add validation")
	sa.Stage("src/handler.go", "old handler\n", "new handler\nmore\n", "add error handling")
	sa.Stage("src/middleware.go", "", "package middleware\n\nfunc RateLimit() {}\n", "new rate limiting")

	output := sa.FormatStaging()

	if !strings.Contains(output, "Staging Area (3 files):") {
		t.Errorf("missing header in output:\n%s", output)
	}
	if !strings.Contains(output, "─────────────────────────") {
		t.Errorf("missing separator in output:\n%s", output)
	}
	if !strings.Contains(output, "src/auth.go") {
		t.Errorf("missing auth.go in output:\n%s", output)
	}
	if !strings.Contains(output, "src/handler.go") {
		t.Errorf("missing handler.go in output:\n%s", output)
	}
	if !strings.Contains(output, "src/middleware.go") {
		t.Errorf("missing middleware.go in output:\n%s", output)
	}
	if !strings.Contains(output, "A") {
		t.Errorf("missing A (added) marker in output:\n%s", output)
	}
	if !strings.Contains(output, "M") {
		t.Errorf("missing M (modified) marker in output:\n%s", output)
	}
	if !strings.Contains(output, "Hunk") {
		t.Errorf("missing Hunk reference in output:\n%s", output)
	}
	if !strings.Contains(output, "Ready to apply") {
		t.Errorf("missing 'Ready to apply' summary in output:\n%s", output)
	}
}

func TestFormatStagingEmpty(t *testing.T) {
	sa := NewStagingArea()
	output := sa.FormatStaging()
	if !strings.Contains(output, "0 files") {
		t.Errorf("expected empty staging message, got:\n%s", output)
	}
}

func TestFormatStagingWithRejectedHunks(t *testing.T) {
	sa := NewStagingArea()

	original := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	modified := "line1\nMOD2\nline3\nline4\nline5\nline6\nline7\nMOD8\nline9\nline10\n"

	sa.Stage("file.go", original, modified, "changes")
	sa.RejectHunk("file.go", 0)

	output := sa.FormatStaging()
	if !strings.Contains(output, "○") {
		t.Errorf("expected pending marker for rejected hunk:\n%s", output)
	}
	if !strings.Contains(output, "pending review") {
		t.Errorf("expected 'pending review' in summary:\n%s", output)
	}
}

func TestClear(t *testing.T) {
	sa := NewStagingArea()
	sa.Stage("a.go", "old\n", "new\n", "change a")
	sa.Stage("b.go", "old\n", "new\n", "change b")

	sa.Clear()

	if len(sa.Staged) != 0 {
		t.Fatalf("expected empty staging after Clear, got %d entries", len(sa.Staged))
	}
}

func TestHasPending(t *testing.T) {
	sa := NewStagingArea()

	if sa.HasPending() {
		t.Error("expected no pending changes in empty staging area")
	}

	sa.Stage("file.go", "old\n", "new\n", "change")
	if !sa.HasPending() {
		t.Error("expected pending changes after staging")
	}

	sa.Reject("file.go")
	if sa.HasPending() {
		t.Error("expected no pending changes after rejection")
	}
}

func TestHasPendingAfterApply(t *testing.T) {
	sa := NewStagingArea()
	dir := t.TempDir()
	file := filepath.Join(dir, "test.go")

	sa.Stage(file, "", "new\n", "create")
	_ = sa.ApplyFile(file)

	if sa.HasPending() {
		t.Error("expected no pending changes after apply")
	}
}

func TestConcurrentAccess(t *testing.T) {
	sa := NewStagingArea()
	var wg sync.WaitGroup

	// Concurrent stages
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			file := filepath.Join("src", "file"+strings.Repeat("x", n%5)+".go")
			sa.Stage(file, "old\n", "new\n", "concurrent change")
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sa.GetStaged()
			sa.HasPending()
			sa.FormatStaging()
		}()
	}

	wg.Wait()
}

func TestPartialApplyWithRejectedHunks(t *testing.T) {
	sa := NewStagingArea()
	dir := t.TempDir()
	file := filepath.Join(dir, "partial.go")

	original := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	modified := "line1\nMODIFIED2\nline3\nline4\nline5\nline6\nline7\nMODIFIED8\nline9\nline10\n"

	sa.Stage(file, original, modified, "partial apply test")

	change := sa.Staged[file]
	if len(change.Hunks) < 2 {
		t.Skipf("need at least 2 hunks for this test, got %d", len(change.Hunks))
	}

	// Reject second hunk, keep first approved
	sa.RejectHunk(file, 1)

	err := sa.ApplyFile(file)
	if err != nil {
		t.Fatalf("ApplyFile failed: %v", err)
	}

	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	result := string(content)
	// First hunk should be applied (MODIFIED2)
	if !strings.Contains(result, "MODIFIED2") {
		t.Error("expected first hunk (MODIFIED2) to be applied")
	}
	// Second hunk should NOT be applied (line8 should remain)
	if strings.Contains(result, "MODIFIED8") {
		t.Error("expected second hunk (MODIFIED8) to NOT be applied")
	}
}

func TestDetectChangeType(t *testing.T) {
	tests := []struct {
		original string
		modified string
		expected string
	}{
		{"", "new content\n", "A"},
		{"old content\n", "", "D"},
		{"old\n", "new\n", "M"},
	}

	for _, tt := range tests {
		got := detectChangeType(tt.original, tt.modified)
		if got != tt.expected {
			t.Errorf("detectChangeType(%q, %q) = %q, want %q", tt.original, tt.modified, got, tt.expected)
		}
	}
}

func TestCountAddsDels(t *testing.T) {
	tests := []struct {
		old      string
		new      string
		wantAdds int
		wantDels int
	}{
		{"line1\nline2\n", "line1\nline2\nline3\n", 1, 0},
		{"line1\nline2\nline3\n", "line1\nline3\n", 0, 1},
		{"line1\nline2\n", "line1\nmodified\n", 1, 1},
		{"", "new\n", 1, 0},
		{"old\n", "", 0, 1},
	}

	for _, tt := range tests {
		adds, dels := countAddsDels(tt.old, tt.new)
		if adds != tt.wantAdds || dels != tt.wantDels {
			t.Errorf("countAddsDels(%q, %q) = (%d, %d), want (%d, %d)",
				tt.old, tt.new, adds, dels, tt.wantAdds, tt.wantDels)
		}
	}
}

func TestFormatStats(t *testing.T) {
	tests := []struct {
		adds     int
		dels     int
		expected string
	}{
		{5, 3, "+5, -3"},
		{10, 0, "+10"},
		{0, 2, "-2"},
		{0, 0, "no changes"},
	}

	for _, tt := range tests {
		got := formatStats(tt.adds, tt.dels)
		if got != tt.expected {
			t.Errorf("formatStats(%d, %d) = %q, want %q", tt.adds, tt.dels, got, tt.expected)
		}
	}
}

func TestComputeStagedHunks(t *testing.T) {
	original := "a\nb\nc\n"
	modified := "a\nB\nc\n"

	hunks := computeStagedHunks(original, modified)
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	if len(hunks[0].OldLines) != 1 || hunks[0].OldLines[0] != "b" {
		t.Errorf("unexpected old lines: %v", hunks[0].OldLines)
	}
	if len(hunks[0].NewLines) != 1 || hunks[0].NewLines[0] != "B" {
		t.Errorf("unexpected new lines: %v", hunks[0].NewLines)
	}
	if !hunks[0].Approved {
		t.Error("hunks should default to approved")
	}
}

func TestComputeStagedHunksMultiple(t *testing.T) {
	original := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	modified := "1\nX\n3\n4\n5\n6\n7\nY\n9\n10\n"

	hunks := computeStagedHunks(original, modified)
	if len(hunks) < 2 {
		t.Fatalf("expected at least 2 hunks, got %d", len(hunks))
	}
}

func TestApplyAllSkipsRejected(t *testing.T) {
	sa := NewStagingArea()
	dir := t.TempDir()

	file1 := filepath.Join(dir, "apply.go")
	file2 := filepath.Join(dir, "reject.go")

	sa.Stage(file1, "", "applied content\n", "will apply")
	sa.Stage(file2, "", "rejected content\n", "will reject")
	sa.Reject(file2)

	applied, err := sa.ApplyAll()
	if err != nil {
		t.Fatalf("ApplyAll failed: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("expected 1 applied file, got %d", len(applied))
	}
	if applied[0] != file1 {
		t.Errorf("expected %s to be applied, got %s", file1, applied[0])
	}

	// Rejected file should not exist
	if _, err := os.Stat(file2); !os.IsNotExist(err) {
		t.Error("rejected file should not have been created")
	}
}

func TestSplitLinesStaging(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"a\n", []string{"a"}},
		{"a\nb\n", []string{"a", "b"}},
		{"a\nb\nc", []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		got := splitLinesStaging(tt.input)
		if len(got) != len(tt.expected) {
			t.Errorf("splitLinesStaging(%q) len = %d, want %d", tt.input, len(got), len(tt.expected))
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("splitLinesStaging(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
			}
		}
	}
}
