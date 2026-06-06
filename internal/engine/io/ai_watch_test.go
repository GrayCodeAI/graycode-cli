package io

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestScanFile_GoComments(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	content := `package main

import "fmt"

func main() {
	// ai: add error handling here
	fmt.Println("hello")
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanFile(file)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	c := comments[0]
	if c.Line != 6 {
		t.Errorf("expected line 6, got %d", c.Line)
	}
	if c.Comment != "add error handling here" {
		t.Errorf("unexpected comment: %q", c.Comment)
	}
	if c.Language != "go" {
		t.Errorf("expected language 'go', got %q", c.Language)
	}
	if c.File != file {
		t.Errorf("unexpected file path: %q", c.File)
	}
}

func TestScanFile_PythonComments(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "script.py")
	content := `import os

def process():
    # ai: add input validation
    data = get_data()
    return data
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanFile(file)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	c := comments[0]
	if c.Line != 4 {
		t.Errorf("expected line 4, got %d", c.Line)
	}
	if c.Comment != "add input validation" {
		t.Errorf("unexpected comment: %q", c.Comment)
	}
	if c.Language != "python" {
		t.Errorf("expected language 'python', got %q", c.Language)
	}
}

func TestScanFile_JSComments(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "app.js")
	content := `const express = require('express');

// ai: add rate limiting middleware
app.get('/', (req, res) => {
    res.send('hello');
});
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanFile(file)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	c := comments[0]
	if c.Line != 3 {
		t.Errorf("expected line 3, got %d", c.Line)
	}
	if c.Comment != "add rate limiting middleware" {
		t.Errorf("unexpected comment: %q", c.Comment)
	}
	if c.Language != "javascript" {
		t.Errorf("expected language 'javascript', got %q", c.Language)
	}
}

func TestScanFile_BlockComment(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "style.css")
	content := `.header {
    /* ai: make this responsive */
    width: 800px;
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanFile(file)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	c := comments[0]
	if c.Line != 2 {
		t.Errorf("expected line 2, got %d", c.Line)
	}
	if c.Comment != "make this responsive" {
		t.Errorf("unexpected comment: %q", c.Comment)
	}
	if c.Language != "css" {
		t.Errorf("expected language 'css', got %q", c.Language)
	}
}

func TestScanFile_HTMLComment(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "index.html")
	content := `<html>
<body>
<!-- ai: add a navigation bar -->
<h1>Hello</h1>
</body>
</html>
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanFile(file)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	c := comments[0]
	if c.Line != 3 {
		t.Errorf("expected line 3, got %d", c.Line)
	}
	if c.Comment != "add a navigation bar" {
		t.Errorf("unexpected comment: %q", c.Comment)
	}
	if c.Language != "html" {
		t.Errorf("expected language 'html', got %q", c.Language)
	}
}

func TestScanFile_ContextExtraction(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	// Create a file with enough lines to test context extraction
	var lines []string
	for i := 1; i <= 20; i++ {
		if i == 10 {
			lines = append(lines, "// ai: fix this function")
		} else {
			lines = append(lines, "line"+string(rune('0'+i%10)))
		}
	}
	content := strings.Join(lines, "\n")
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanFile(file)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	c := comments[0]
	// Context should include lines 5-15 (5 before line 10, line 10 itself, 5 after)
	contextLines := strings.Split(c.Context, "\n")
	// 5 before + 1 current + 5 after = 11 lines
	if len(contextLines) != 11 {
		t.Errorf("expected 11 context lines, got %d", len(contextLines))
	}
	// The AI comment should be in the context
	if !strings.Contains(c.Context, "// ai: fix this function") {
		t.Errorf("context does not contain the AI comment")
	}
}

func TestScanFile_ContextAtFileStart(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "start.go")
	content := `// ai: add package docs
package main

func main() {}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanFile(file)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	c := comments[0]
	if c.Line != 1 {
		t.Errorf("expected line 1, got %d", c.Line)
	}
	// Context should start at line 1 (no panic from negative index)
	if !strings.Contains(c.Context, "// ai: add package docs") {
		t.Errorf("context does not contain the AI comment")
	}
}

func TestScanFile_MultipleComments(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "multi.go")
	content := `package main

// ai: add logging
func processA() {}

// ai: handle timeout
func processB() {}

// ai: add retry logic
func processC() {}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanFile(file)
	if len(comments) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(comments))
	}

	expected := []struct {
		line    int
		comment string
	}{
		{3, "add logging"},
		{6, "handle timeout"},
		{9, "add retry logic"},
	}

	for i, exp := range expected {
		if comments[i].Line != exp.line {
			t.Errorf("comment %d: expected line %d, got %d", i, exp.line, comments[i].Line)
		}
		if comments[i].Comment != exp.comment {
			t.Errorf("comment %d: expected %q, got %q", i, exp.comment, comments[i].Comment)
		}
	}
}

func TestScanFile_MultiWordInstructions(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "complex.go")
	content := `package main

// ai: refactor this function to use a map instead of a slice for O(1) lookups
func findItem(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanFile(file)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	c := comments[0]
	expected := "refactor this function to use a map instead of a slice for O(1) lookups"
	if c.Comment != expected {
		t.Errorf("expected %q, got %q", expected, c.Comment)
	}
}

func TestScanFile_IgnoresRegularComments(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "normal.go")
	content := `package main

// This is a regular comment
// TODO: fix this later
// NOTE: important
/* A block comment explaining something */
// ai: this is an AI instruction
func main() {}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanFile(file)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment (only AI), got %d", len(comments))
	}
	if comments[0].Comment != "this is an AI instruction" {
		t.Errorf("unexpected comment: %q", comments[0].Comment)
	}
}

func TestScanFile_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "caps.go")
	content := `package main

// AI: uppercase variant
// ai: lowercase variant
// Todo-AI: todo variant
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanFile(file)
	if len(comments) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(comments))
	}
	if comments[0].Comment != "uppercase variant" {
		t.Errorf("comment 0: unexpected %q", comments[0].Comment)
	}
	if comments[1].Comment != "lowercase variant" {
		t.Errorf("comment 1: unexpected %q", comments[1].Comment)
	}
	if comments[2].Comment != "todo variant" {
		t.Errorf("comment 2: unexpected %q", comments[2].Comment)
	}
}

func TestScanDirectory_PatternMatching(t *testing.T) {
	dir := t.TempDir()

	// Create Go file with AI comment
	goFile := filepath.Join(dir, "main.go")
	goContent := `package main
// ai: fix this
func main() {}
`
	if err := os.WriteFile(goFile, []byte(goContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create Python file with AI comment
	pyFile := filepath.Join(dir, "script.py")
	pyContent := `# ai: add docstring
def hello():
    pass
`
	if err := os.WriteFile(pyFile, []byte(pyContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a text file that should be ignored
	txtFile := filepath.Join(dir, "notes.txt")
	txtContent := `// ai: this should be ignored
`
	if err := os.WriteFile(txtFile, []byte(txtContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Scan only Go files
	comments := ScanDirectory(dir, []string{"*.go"})
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment (Go only), got %d", len(comments))
	}
	if comments[0].Comment != "fix this" {
		t.Errorf("unexpected comment: %q", comments[0].Comment)
	}

	// Scan Go and Python files
	comments = ScanDirectory(dir, []string{"*.go", "*.py"})
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments (Go + Python), got %d", len(comments))
	}
}

func TestScanDirectory_SkipsGitDir(t *testing.T) {
	dir := t.TempDir()

	// Create a .git directory with a file containing AI comments
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitFile := filepath.Join(gitDir, "config.go")
	if err := os.WriteFile(gitFile, []byte("// ai: should be ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a normal file
	srcFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(srcFile, []byte("// ai: should be found\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanDirectory(dir, []string{"*.go"})
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment (skipping .git), got %d", len(comments))
	}
	if comments[0].Comment != "should be found" {
		t.Errorf("unexpected comment: %q", comments[0].Comment)
	}
}

func TestScanDirectory_SubDirectories(t *testing.T) {
	dir := t.TempDir()

	// Create nested directory structure
	subDir := filepath.Join(dir, "pkg", "util")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(subDir, "helpers.go")
	content := `package util
// ai: add unit tests
func Helper() {}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanDirectory(dir, []string{"*.go"})
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	// Check that relative path is used
	expectedRel := filepath.Join("pkg", "util", "helpers.go")
	if comments[0].File != expectedRel {
		t.Errorf("expected relative path %q, got %q", expectedRel, comments[0].File)
	}
}

func TestBuildPrompt_Format(t *testing.T) {
	comment := AIComment{
		File:    "pkg/server/handler.go",
		Line:    42,
		Comment: "add request validation",
		Context: "func handleRequest(w http.ResponseWriter, r *http.Request) {\n\t// ai: add request validation\n\tbody := readBody(r)\n}",
		Marker:  "\t// ai: add request validation",
	}

	prompt := BuildPrompt(comment)

	// Check that the prompt contains key elements
	if !strings.Contains(prompt, "pkg/server/handler.go") {
		t.Error("prompt should contain file path")
	}
	if !strings.Contains(prompt, "line 42") {
		t.Error("prompt should contain line number")
	}
	if !strings.Contains(prompt, "add request validation") {
		t.Error("prompt should contain the instruction")
	}
	if !strings.Contains(prompt, "Please implement this change") {
		t.Error("prompt should contain implementation request")
	}
	if !strings.Contains(prompt, "remove the AI comment") {
		t.Error("prompt should mention removing the AI comment")
	}
	// The AI comment line should be highlighted
	if !strings.Contains(prompt, ">>>") {
		t.Error("prompt should highlight the AI comment line")
	}
}

func TestRemoveComment_RemovesEntireLine(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.go")
	content := `package main

func main() {
	// ai: fix this
	doSomething()
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RemoveComment(file, 4, "\t// ai: fix this")
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	result := string(data)
	if strings.Contains(result, "ai: fix this") {
		t.Error("AI comment should have been removed")
	}
	if !strings.Contains(result, "doSomething()") {
		t.Error("other code should be preserved")
	}

	// The file should have one fewer line
	lines := strings.Split(result, "\n")
	originalLines := strings.Split(content, "\n")
	if len(lines) != len(originalLines)-1 {
		t.Errorf("expected %d lines after removal, got %d", len(originalLines)-1, len(lines))
	}
}

func TestRemoveComment_MarkerMismatch(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.go")
	content := `package main
func main() {}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RemoveComment(file, 1, "// ai: something else")
	if err == nil {
		t.Fatal("expected error for marker mismatch")
	}
	if !strings.Contains(err.Error(), "marker mismatch") {
		t.Errorf("expected marker mismatch error, got: %v", err)
	}
}

func TestRemoveComment_LineOutOfRange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.go")
	content := `package main
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RemoveComment(file, 100, "// ai: fix")
	if err == nil {
		t.Fatal("expected error for line out of range")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out of range error, got: %v", err)
	}
}

func TestStartFsnotify_InitialAndNewComments(t *testing.T) {
	dir := t.TempDir()

	// A file with a directive already present before watching starts.
	existing := filepath.Join(dir, "existing.go")
	if err := os.WriteFile(existing, []byte("package main\n// ai: existing directive\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := NewAIWatcher(dir, []string{"*.go"})
	w.Debounce = 50 * time.Millisecond

	var mu sync.Mutex
	var seen []string
	w.OnComment = func(c AIComment) {
		mu.Lock()
		seen = append(seen, c.Comment)
		mu.Unlock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- w.StartFsnotify(ctx) }()

	// Wait for the initial scan to deliver the existing directive.
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, s := range seen {
			if s == "existing directive" {
				return true
			}
		}
		return false
	}, 2*time.Second, "initial directive not detected")

	// Now create a new file with a directive; the fsnotify backend should pick it up.
	added := filepath.Join(dir, "added.go")
	if err := os.WriteFile(added, []byte("package main\n// ai: added directive\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, s := range seen {
			if s == "added directive" {
				return true
			}
		}
		return false
	}, 3*time.Second, "newly-added directive not detected")

	w.Stop()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("StartFsnotify did not return after Stop")
	}
}

func TestStartFsnotify_OnChangeFiresPerBurst(t *testing.T) {
	dir := t.TempDir()
	w := NewAIWatcher(dir, []string{"*.go"})
	w.Debounce = 50 * time.Millisecond

	var mu sync.Mutex
	changes := 0
	w.OnChange = func() {
		mu.Lock()
		changes++
		mu.Unlock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- w.StartFsnotify(ctx) }()

	// Give the initial scan/watch setup a moment.
	time.Sleep(100 * time.Millisecond)

	// Write a file with no ai: comment at all — OnComment would not fire, but
	// OnChange must, since the file tree changed.
	if err := os.WriteFile(filepath.Join(dir, "plain.go"), []byte("package main\n// AI! act now\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return changes >= 1
	}, 3*time.Second, "OnChange did not fire on filesystem change")

	w.Stop()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("StartFsnotify did not return after Stop")
	}
}

// waitFor polls cond until it returns true or the timeout elapses.
func waitFor(t *testing.T, cond func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestNewAIWatcher_Defaults(t *testing.T) {
	w := NewAIWatcher("/tmp/test", nil)
	if w.RootDir != "/tmp/test" {
		t.Errorf("unexpected RootDir: %q", w.RootDir)
	}
	if w.Debounce != 2*time.Second {
		t.Errorf("unexpected Debounce: %v", w.Debounce)
	}
	if len(w.Patterns) == 0 {
		t.Error("expected default patterns to be set")
	}
	// Check that common extensions are in the defaults
	hasGo := false
	hasPy := false
	for _, p := range w.Patterns {
		if p == "*.go" {
			hasGo = true
		}
		if p == "*.py" {
			hasPy = true
		}
	}
	if !hasGo || !hasPy {
		t.Error("default patterns should include *.go and *.py")
	}
}

func TestNewAIWatcher_CustomPatterns(t *testing.T) {
	patterns := []string{"*.rs", "*.toml"}
	w := NewAIWatcher("/tmp/test", patterns)
	if len(w.Patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(w.Patterns))
	}
	if w.Patterns[0] != "*.rs" || w.Patterns[1] != "*.toml" {
		t.Errorf("unexpected patterns: %v", w.Patterns)
	}
}

func TestScanFile_TodoAIPrefix(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "todo.go")
	content := `package main

// todo-ai: implement caching
func getData() {}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanFile(file)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Comment != "implement caching" {
		t.Errorf("unexpected comment: %q", comments[0].Comment)
	}
}

func TestScanFile_ShellComments(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "deploy.sh")
	content := `#!/bin/bash

# ai: add error checking for each step
deploy() {
    rsync -av . server:/app
    ssh server 'systemctl restart app'
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := ScanFile(file)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Comment != "add error checking for each step" {
		t.Errorf("unexpected comment: %q", comments[0].Comment)
	}
	if comments[0].Language != "shell" {
		t.Errorf("expected language 'shell', got %q", comments[0].Language)
	}
}

func TestMatchesAnyPattern(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		expected bool
	}{
		{"main.go", []string{"*.go"}, true},
		{"main.go", []string{"*.py"}, false},
		{"test.py", []string{"*.go", "*.py"}, true},
		{"README.md", []string{"*.go", "*.py", "*.js"}, false},
		{"app.tsx", []string{"*.tsx"}, true},
	}

	for _, tt := range tests {
		got := matchesAnyPattern(tt.name, tt.patterns)
		if got != tt.expected {
			t.Errorf("matchesAnyPattern(%q, %v) = %v, want %v",
				tt.name, tt.patterns, got, tt.expected)
		}
	}
}

func TestCommentHash_Uniqueness(t *testing.T) {
	c1 := AIComment{File: "a.go", Line: 1, Comment: "fix this"}
	c2 := AIComment{File: "a.go", Line: 2, Comment: "fix this"}
	c3 := AIComment{File: "b.go", Line: 1, Comment: "fix this"}
	c4 := AIComment{File: "a.go", Line: 1, Comment: "fix that"}

	h1 := commentHash(c1)
	h2 := commentHash(c2)
	h3 := commentHash(c3)
	h4 := commentHash(c4)

	if h1 == h2 {
		t.Error("different lines should produce different hashes")
	}
	if h1 == h3 {
		t.Error("different files should produce different hashes")
	}
	if h1 == h4 {
		t.Error("different comments should produce different hashes")
	}
}
