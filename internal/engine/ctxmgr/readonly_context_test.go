package ctxmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func createTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to create temp file %s: %v", path, err)
	}
	return path
}

func TestAddFileReadsAndStores(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "test.txt", "hello world")

	rc := NewReadOnlyContext(10000)
	if err := rc.AddFile(path); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	cf, exists := rc.Files[filepath.Clean(path)]
	if !exists {
		t.Fatal("file not found in context after AddFile")
	}
	if cf.Content != "hello world" {
		t.Errorf("expected content %q, got %q", "hello world", cf.Content)
	}
	if cf.TokenCount != TokenEstimate("hello world") {
		t.Errorf("unexpected token count: %d", cf.TokenCount)
	}
	if cf.Path != filepath.Clean(path) {
		t.Errorf("expected path %q, got %q", filepath.Clean(path), cf.Path)
	}
}

func TestIsReadOnlyReturnsTrueForAddedFiles(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "readonly.go", "package main")

	rc := NewReadOnlyContext(10000)
	if err := rc.AddFile(path); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	if !rc.IsReadOnly(path) {
		t.Error("IsReadOnly should return true for added file")
	}
	if rc.IsReadOnly(filepath.Join(dir, "other.go")) {
		t.Error("IsReadOnly should return false for unknown file")
	}
}

func TestRemoveFile(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "remove.txt", "content")

	rc := NewReadOnlyContext(10000)
	if err := rc.AddFile(path); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	rc.RemoveFile(path)

	if rc.IsReadOnly(path) {
		t.Error("file should not be read-only after removal")
	}
	if _, exists := rc.Files[filepath.Clean(path)]; exists {
		t.Error("file should not exist in Files map after removal")
	}
}

func TestReadOnlyContextBudgetEnforcement(t *testing.T) {
	dir := t.TempDir()
	// Create a file with content that uses ~25 tokens (100 bytes / 4)
	content := strings.Repeat("x", 100)
	path := createTempFile(t, dir, "big.txt", content)

	// Budget of 20 tokens - file needs 25
	rc := NewReadOnlyContext(20)
	err := rc.AddFile(path)
	if err == nil {
		t.Fatal("expected error when exceeding budget")
	}
	if !strings.Contains(err.Error(), "exceed budget") {
		t.Errorf("expected budget error, got: %v", err)
	}
}

func TestBudgetAllowsWithinLimit(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("x", 40) // 10 tokens
	path := createTempFile(t, dir, "small.txt", content)

	rc := NewReadOnlyContext(100)
	if err := rc.AddFile(path); err != nil {
		t.Fatalf("should allow file within budget: %v", err)
	}
}

func TestEvictRemovesLRUUnpinned(t *testing.T) {
	dir := t.TempDir()

	// Create 3 files, each ~25 tokens (100 bytes)
	path1 := createTempFile(t, dir, "file1.txt", strings.Repeat("a", 100))
	path2 := createTempFile(t, dir, "file2.txt", strings.Repeat("b", 100))
	path3 := createTempFile(t, dir, "file3.txt", strings.Repeat("c", 100))

	// Budget enough for all three
	rc := NewReadOnlyContext(100)
	if err := rc.AddFile(path1); err != nil {
		t.Fatalf("AddFile1 failed: %v", err)
	}
	// Ensure distinct AddedAt ordering
	time.Sleep(time.Millisecond)
	if err := rc.AddFile(path2); err != nil {
		t.Fatalf("AddFile2 failed: %v", err)
	}
	time.Sleep(time.Millisecond)
	if err := rc.AddFile(path3); err != nil {
		t.Fatalf("AddFile3 failed: %v", err)
	}

	// Now reduce budget to force eviction
	rc.MaxTokenBudget = 50

	evicted := rc.Evict()
	if len(evicted) == 0 {
		t.Fatal("expected at least one file to be evicted")
	}
	// Oldest file (file1) should be evicted first
	if evicted[0] != filepath.Clean(path1) {
		t.Errorf("expected oldest file %q to be evicted first, got %q", path1, evicted[0])
	}
}

func TestPinnedFilesSurviveEviction(t *testing.T) {
	dir := t.TempDir()

	path1 := createTempFile(t, dir, "pinned.txt", strings.Repeat("a", 100))
	path2 := createTempFile(t, dir, "unpinned.txt", strings.Repeat("b", 100))

	rc := NewReadOnlyContext(100)
	if err := rc.AddFile(path1, WithPinned()); err != nil {
		t.Fatalf("AddFile pinned failed: %v", err)
	}
	time.Sleep(time.Millisecond)
	if err := rc.AddFile(path2); err != nil {
		t.Fatalf("AddFile unpinned failed: %v", err)
	}

	// Reduce budget below total
	rc.MaxTokenBudget = 30

	evicted := rc.Evict()

	// Pinned file should still exist
	if !rc.IsReadOnly(path1) {
		t.Error("pinned file should survive eviction")
	}

	// Unpinned file should be evicted
	found := false
	for _, e := range evicted {
		if e == filepath.Clean(path2) {
			found = true
		}
	}
	if !found {
		t.Error("unpinned file should have been evicted")
	}
}

func TestRefreshStaleReReadsChangedFiles(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "refresh.txt", "original")

	rc := NewReadOnlyContext(10000)
	if err := rc.AddFile(path, WithAutoRefresh()); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	// Verify initial content
	if rc.Files[filepath.Clean(path)].Content != "original" {
		t.Fatal("initial content mismatch")
	}

	// Modify the file and update mtime to be after LastRefreshed
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("updated"), 0o644); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	if err := rc.RefreshStale(); err != nil {
		t.Fatalf("RefreshStale failed: %v", err)
	}

	cf := rc.Files[filepath.Clean(path)]
	if cf.Content != "updated" {
		t.Errorf("expected refreshed content %q, got %q", "updated", cf.Content)
	}
}

func TestRefreshStaleSkipsNonAutoRefresh(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "norefresh.txt", "original")

	rc := NewReadOnlyContext(10000)
	// AutoRefresh is false by default
	if err := rc.AddFile(path); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("changed"), 0o644); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	if err := rc.RefreshStale(); err != nil {
		t.Fatalf("RefreshStale failed: %v", err)
	}

	cf := rc.Files[filepath.Clean(path)]
	if cf.Content != "original" {
		t.Error("file without AutoRefresh should not be refreshed")
	}
}

func TestBuildContextBlockFormatsCorrectly(t *testing.T) {
	dir := t.TempDir()
	path1 := createTempFile(t, dir, "afile.md", "# Title\nContent here")
	path2 := createTempFile(t, dir, "bfile.go", "package main")

	rc := NewReadOnlyContext(10000)
	if err := rc.AddFile(path1); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}
	if err := rc.AddFile(path2); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	block := rc.BuildContextBlock()

	if !strings.Contains(block, "## Read-Only Context Files (do NOT modify these)") {
		t.Error("missing header")
	}
	if !strings.Contains(block, "### afile.md") {
		t.Error("missing afile.md section")
	}
	if !strings.Contains(block, "### bfile.go") {
		t.Error("missing bfile.go section")
	}
	if !strings.Contains(block, "# Title\nContent here") {
		t.Error("missing afile.md content")
	}
	if !strings.Contains(block, "package main") {
		t.Error("missing bfile.go content")
	}
	if !strings.Contains(block, "```") {
		t.Error("missing code fence markers")
	}
}

func TestBuildContextBlockEmpty(t *testing.T) {
	rc := NewReadOnlyContext(10000)
	block := rc.BuildContextBlock()
	if block != "" {
		t.Errorf("expected empty string for no files, got %q", block)
	}
}

func TestAddPatternResolvesGlobs(t *testing.T) {
	dir := t.TempDir()
	createTempFile(t, dir, "one.md", "first")
	createTempFile(t, dir, "two.md", "second")
	createTempFile(t, dir, "skip.go", "package skip")

	rc := NewReadOnlyContext(10000)
	pattern := filepath.Join(dir, "*.md")
	if err := rc.AddPattern(pattern); err != nil {
		t.Fatalf("AddPattern failed: %v", err)
	}

	if len(rc.Files) != 2 {
		t.Errorf("expected 2 files from pattern, got %d", len(rc.Files))
	}

	// Verify the pattern was stored
	if len(rc.Patterns) != 1 || rc.Patterns[0] != pattern {
		t.Error("pattern not stored correctly")
	}

	// .go file should NOT be included
	goPath := filepath.Join(dir, "skip.go")
	if rc.IsReadOnly(goPath) {
		t.Error(".go file should not be included by *.md pattern")
	}
}

func TestSuggestFilesFindsCommonFiles(t *testing.T) {
	dir := t.TempDir()

	// Create some common project files
	createTempFile(t, dir, "go.mod", "module test")
	createTempFile(t, dir, "README.md", "# readme")
	createTempFile(t, dir, "Makefile", "build:")

	suggestions := SuggestFiles(dir)

	if len(suggestions) != 3 {
		t.Errorf("expected 3 suggestions, got %d: %v", len(suggestions), suggestions)
	}

	expected := map[string]bool{
		filepath.Join(dir, "go.mod"):    true,
		filepath.Join(dir, "README.md"): true,
		filepath.Join(dir, "Makefile"):  true,
	}
	for _, s := range suggestions {
		if !expected[s] {
			t.Errorf("unexpected suggestion: %s", s)
		}
	}
}

func TestSuggestFilesReturnsEmptyForEmptyDir(t *testing.T) {
	dir := t.TempDir()
	suggestions := SuggestFiles(dir)
	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions for empty dir, got %v", suggestions)
	}
}

func TestStatsCalculation(t *testing.T) {
	dir := t.TempDir()
	path1 := createTempFile(t, dir, "a.txt", strings.Repeat("x", 40)) // 10 tokens
	path2 := createTempFile(t, dir, "b.txt", strings.Repeat("y", 80)) // 20 tokens

	rc := NewReadOnlyContext(100)
	if err := rc.AddFile(path1, WithPinned()); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}
	if err := rc.AddFile(path2); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	stats := rc.Stats()

	if stats.TotalFiles != 2 {
		t.Errorf("expected 2 total files, got %d", stats.TotalFiles)
	}
	if stats.TotalTokens != 30 {
		t.Errorf("expected 30 total tokens, got %d", stats.TotalTokens)
	}
	if stats.PinnedCount != 1 {
		t.Errorf("expected 1 pinned, got %d", stats.PinnedCount)
	}
	expectedBudget := 30.0
	if stats.BudgetUsed != expectedBudget {
		t.Errorf("expected budget used %.1f%%, got %.1f%%", expectedBudget, stats.BudgetUsed)
	}
}

func TestTokenEstimateApproximation(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"abc", 1},          // len=3, 3/4=0, but >0 so returns 1
		{"abcd", 1},         // len=4, 4/4=1
		{"hello world!", 3}, // len=12, 12/4=3
		{strings.Repeat("x", 100), 25},
		{strings.Repeat("y", 1000), 250},
	}

	for _, tt := range tests {
		got := TokenEstimate(tt.input)
		if got != tt.expected {
			t.Errorf("TokenEstimate(%q): expected %d, got %d", tt.input[:min(len(tt.input), 20)], tt.expected, got)
		}
	}
}

func TestAutoRefreshFlagBehavior(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "auto.txt", "content")

	rc := NewReadOnlyContext(10000)
	if err := rc.AddFile(path, WithAutoRefresh()); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	cf := rc.Files[filepath.Clean(path)]
	if !cf.AutoRefresh {
		t.Error("expected AutoRefresh to be true")
	}
}

func TestWithPinnedOption(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "pinned.txt", "content")

	rc := NewReadOnlyContext(10000)
	if err := rc.AddFile(path, WithPinned()); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	cf := rc.Files[filepath.Clean(path)]
	if !cf.Pinned {
		t.Error("expected Pinned to be true")
	}
}

func TestReadOnlyContextConcurrentAccess(t *testing.T) {
	dir := t.TempDir()

	// Create 20 files
	var paths []string
	for i := 0; i < 20; i++ {
		p := createTempFile(t, dir, fmt.Sprintf("concurrent_%d.txt", i), strings.Repeat("x", 40))
		paths = append(paths, p)
	}

	rc := NewReadOnlyContext(100000)

	var wg sync.WaitGroup

	// Concurrent adds
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = rc.AddFile(paths[idx])
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rc.IsReadOnly(paths[idx])
		}(i)
	}

	// Concurrent stats
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rc.Stats()
		}()
	}

	// Concurrent build context
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rc.BuildContextBlock()
		}()
	}

	wg.Wait()

	// Verify no panic and state is consistent
	stats := rc.Stats()
	if stats.TotalFiles != 20 {
		t.Errorf("expected 20 files after concurrent adds, got %d", stats.TotalFiles)
	}
}

func TestAddFileNonexistent(t *testing.T) {
	rc := NewReadOnlyContext(10000)
	err := rc.AddFile("/nonexistent/path/to/file.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "cannot read") {
		t.Errorf("expected 'cannot read' in error, got: %v", err)
	}
}

func TestEvictReturnsNilWhenUnderBudget(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "small.txt", "hi")

	rc := NewReadOnlyContext(10000)
	if err := rc.AddFile(path); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	evicted := rc.Evict()
	if evicted != nil {
		t.Errorf("expected nil evicted list when under budget, got %v", evicted)
	}
}

func TestAddPatternInvalidGlob(t *testing.T) {
	rc := NewReadOnlyContext(10000)
	err := rc.AddPattern("[invalid")
	if err == nil {
		t.Fatal("expected error for invalid glob pattern")
	}
}
