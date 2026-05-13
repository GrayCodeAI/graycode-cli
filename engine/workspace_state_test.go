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

func TestNewWorkspaceState(t *testing.T) {
	ws := NewWorkspaceState("/tmp/testproject")

	if ws.ProjectDir != "/tmp/testproject" {
		t.Errorf("expected ProjectDir /tmp/testproject, got %s", ws.ProjectDir)
	}
	if ws.OpenFiles == nil {
		t.Error("OpenFiles should be initialized")
	}
	if ws.ModifiedFiles == nil {
		t.Error("ModifiedFiles should be initialized")
	}
	if ws.StagedFiles != nil {
		t.Error("StagedFiles should be nil initially")
	}
	if ws.fileHashes == nil {
		t.Error("fileHashes should be initialized")
	}
}

func TestMarkOpened(t *testing.T) {
	dir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ws := NewWorkspaceState(dir)
	ws.MarkOpened(testFile)

	opened := ws.GetOpened()
	if len(opened) != 1 {
		t.Fatalf("expected 1 opened file, got %d", len(opened))
	}
	if opened[0] != "main.go" {
		t.Errorf("expected main.go, got %s", opened[0])
	}

	// Verify file state
	fs := ws.OpenFiles["main.go"]
	if fs == nil {
		t.Fatal("FileState should be set")
	}
	if fs.Language != "Go" {
		t.Errorf("expected Go language, got %s", fs.Language)
	}
	if fs.IsTest {
		t.Error("main.go should not be a test file")
	}
	if fs.Hash == "" {
		t.Error("Hash should be set")
	}
}

func TestMarkOpenedTestFile(t *testing.T) {
	dir := t.TempDir()

	testFile := filepath.Join(dir, "main_test.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ws := NewWorkspaceState(dir)
	ws.MarkOpened(testFile)

	fs := ws.OpenFiles["main_test.go"]
	if fs == nil {
		t.Fatal("FileState should be set")
	}
	if !fs.IsTest {
		t.Error("main_test.go should be a test file")
	}
}

func TestMarkModified(t *testing.T) {
	dir := t.TempDir()

	testFile := filepath.Join(dir, "handler.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ws := NewWorkspaceState(dir)
	ws.MarkModified(testFile)

	modified := ws.GetModified()
	if len(modified) != 1 {
		t.Fatalf("expected 1 modified file, got %d", len(modified))
	}
	if modified[0] != "handler.go" {
		t.Errorf("expected handler.go, got %s", modified[0])
	}
}

func TestMarkStaged(t *testing.T) {
	dir := t.TempDir()

	ws := NewWorkspaceState(dir)
	ws.MarkStaged("auth.go")
	ws.MarkStaged("handler.go")

	if len(ws.StagedFiles) != 2 {
		t.Fatalf("expected 2 staged files, got %d", len(ws.StagedFiles))
	}

	// Deduplication
	ws.MarkStaged("auth.go")
	if len(ws.StagedFiles) != 2 {
		t.Fatalf("expected 2 staged files after duplicate, got %d", len(ws.StagedFiles))
	}
}

func TestHasChanged(t *testing.T) {
	dir := t.TempDir()

	testFile := filepath.Join(dir, "config.go")
	if err := os.WriteFile(testFile, []byte("package config\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ws := NewWorkspaceState(dir)
	ws.MarkOpened(testFile)

	// No change yet
	if ws.HasChanged(testFile) {
		t.Error("file should not have changed yet")
	}

	// Modify the file externally
	if err := os.WriteFile(testFile, []byte("package config\n\nvar X = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if !ws.HasChanged(testFile) {
		t.Error("file should have changed after external modification")
	}
}

func TestHasChangedUntracked(t *testing.T) {
	dir := t.TempDir()
	ws := NewWorkspaceState(dir)

	// Untracked file should not report as changed
	if ws.HasChanged("nonexistent.go") {
		t.Error("untracked file should not be reported as changed")
	}
}

func TestDetectExternalChanges(t *testing.T) {
	dir := t.TempDir()

	file1 := filepath.Join(dir, "a.go")
	file2 := filepath.Join(dir, "b.go")
	file3 := filepath.Join(dir, "c.go")

	if err := os.WriteFile(file1, []byte("package a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("package b\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file3, []byte("package c\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ws := NewWorkspaceState(dir)
	ws.MarkOpened(file1)
	ws.MarkOpened(file2)
	ws.MarkOpened(file3)

	// Modify file1 as hawk (should not be external)
	if err := os.WriteFile(file1, []byte("package a\n\nvar X = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ws.MarkModified(file1)

	// Modify file2 externally (should be detected)
	if err := os.WriteFile(file2, []byte("package b\n\nvar Y = 2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// file3 unchanged

	changes := ws.DetectExternalChanges()
	if len(changes) != 1 {
		t.Fatalf("expected 1 external change, got %d: %v", len(changes), changes)
	}
	if changes[0] != "b.go" {
		t.Errorf("expected b.go as external change, got %s", changes[0])
	}
}

func TestScan(t *testing.T) {
	dir := t.TempDir()

	// Create project structure
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "handler.go"), []byte("package src\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "handler_test.go"), []byte("package src\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ws := NewWorkspaceState(dir)
	if err := ws.Scan(); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if ws.LastScan.IsZero() {
		t.Error("LastScan should be set after scan")
	}

	// Should have found files
	if len(ws.scanState) < 3 {
		t.Errorf("expected at least 3 files in scan state, got %d", len(ws.scanState))
	}

	// Verify test detection
	testFs := ws.scanState[filepath.Join("src", "handler_test.go")]
	if testFs == nil {
		t.Fatal("handler_test.go should be in scan state")
	}
	if !testFs.IsTest {
		t.Error("handler_test.go should be marked as test")
	}
}

func TestScanSkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()

	// Create hidden directory
	if err := os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[core]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ws := NewWorkspaceState(dir)
	if err := ws.Scan(); err != nil {
		t.Fatal(err)
	}

	// Should not include .git files
	for path := range ws.scanState {
		if strings.HasPrefix(path, ".git") {
			t.Errorf("scan should skip .git directory, found: %s", path)
		}
	}
}

func TestScanSkipsNodeModules(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "express"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "express", "index.js"), []byte("module.exports = {};\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.js"), []byte("const app = require('express');\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ws := NewWorkspaceState(dir)
	if err := ws.Scan(); err != nil {
		t.Fatal(err)
	}

	for path := range ws.scanState {
		if strings.HasPrefix(path, "node_modules") {
			t.Errorf("scan should skip node_modules, found: %s", path)
		}
	}
}

func TestSummary(t *testing.T) {
	dir := t.TempDir()

	testFile := filepath.Join(dir, "auth.go")
	if err := os.WriteFile(testFile, []byte("package auth\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ws := NewWorkspaceState(dir)
	ws.MarkOpened(testFile)
	ws.MarkModified(testFile)
	ws.MarkStaged("auth.go")
	ws.LastScan = time.Now().Add(-30 * time.Second)

	summary := ws.Summary()

	if !strings.Contains(summary, "Workspace State:") {
		t.Error("summary should contain header")
	}
	if !strings.Contains(summary, dir) {
		t.Errorf("summary should contain project dir %s", dir)
	}
	if !strings.Contains(summary, "Modified: 1 files") {
		t.Error("summary should show modified files")
	}
	if !strings.Contains(summary, "Opened: 1 files") {
		t.Error("summary should show opened files")
	}
	if !strings.Contains(summary, "Staged: 1 files") {
		t.Error("summary should show staged files")
	}
	if !strings.Contains(summary, "Last scan:") {
		t.Error("summary should show last scan time")
	}
}

func TestBuildContextForAgent(t *testing.T) {
	dir := t.TempDir()

	testFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ws := NewWorkspaceState(dir)
	ws.MarkOpened(testFile)
	ws.MarkModified(testFile)
	ws.MarkStaged("main.go")

	ctx := ws.BuildContextForAgent()

	if !strings.Contains(ctx, "<workspace_state>") {
		t.Error("context should contain opening tag")
	}
	if !strings.Contains(ctx, "</workspace_state>") {
		t.Error("context should contain closing tag")
	}
	if !strings.Contains(ctx, "project_dir:") {
		t.Error("context should contain project_dir")
	}
	if !strings.Contains(ctx, "modified_files:") {
		t.Error("context should contain modified_files")
	}
	if !strings.Contains(ctx, "open_files:") {
		t.Error("context should contain open_files")
	}
	if !strings.Contains(ctx, "staged_files:") {
		t.Error("context should contain staged_files")
	}
	if !strings.Contains(ctx, "Go") {
		t.Error("context should contain language Go")
	}
}

func TestReset(t *testing.T) {
	dir := t.TempDir()

	testFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ws := NewWorkspaceState(dir)
	ws.MarkOpened(testFile)
	ws.MarkModified(testFile)
	ws.MarkStaged("main.go")
	ws.LastScan = time.Now()

	ws.Reset()

	if len(ws.OpenFiles) != 0 {
		t.Error("OpenFiles should be empty after reset")
	}
	if len(ws.ModifiedFiles) != 0 {
		t.Error("ModifiedFiles should be empty after reset")
	}
	if ws.StagedFiles != nil {
		t.Error("StagedFiles should be nil after reset")
	}
	if len(ws.fileHashes) != 0 {
		t.Error("fileHashes should be empty after reset")
	}
	if !ws.LastScan.IsZero() {
		t.Error("LastScan should be zero after reset")
	}
}

func TestConcurrentAccess(t *testing.T) {
	dir := t.TempDir()

	// Create several test files
	for i := 0; i < 10; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file%d.go", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("package f%d\n", i)), 0644); err != nil {
			t.Fatal(err)
		}
	}

	ws := NewWorkspaceState(dir)

	var wg sync.WaitGroup
	wg.Add(4)

	// Concurrent opens
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			ws.MarkOpened(filepath.Join(dir, fmt.Sprintf("file%d.go", i)))
		}
	}()

	// Concurrent modifications
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			ws.MarkModified(filepath.Join(dir, fmt.Sprintf("file%d.go", i)))
		}
	}()

	// Concurrent reads
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			_ = ws.GetOpened()
			_ = ws.GetModified()
		}
	}()

	// Concurrent staging
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			ws.MarkStaged(fmt.Sprintf("file%d.go", i))
		}
	}()

	wg.Wait()

	// Should not panic or deadlock
	_ = ws.Summary()
	_ = ws.BuildContextForAgent()
}

func TestRelativeAndAbsolutePaths(t *testing.T) {
	dir := t.TempDir()

	testFile := filepath.Join(dir, "pkg", "service.go")
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile, []byte("package pkg\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ws := NewWorkspaceState(dir)

	// Open with absolute path
	ws.MarkOpened(testFile)

	// Should be stored as relative
	opened := ws.GetOpened()
	if len(opened) != 1 {
		t.Fatalf("expected 1 opened file, got %d", len(opened))
	}
	expected := filepath.Join("pkg", "service.go")
	if opened[0] != expected {
		t.Errorf("expected %s, got %s", expected, opened[0])
	}

	// Open with relative path
	ws.MarkOpened("pkg/service.go")
	// Should deduplicate
	opened = ws.GetOpened()
	if len(opened) != 1 {
		t.Fatalf("expected 1 opened file after dedup, got %d", len(opened))
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"main.go", "Go"},
		{"app.py", "Python"},
		{"index.ts", "TypeScript"},
		{"component.tsx", "TypeScript"},
		{"lib.rs", "Rust"},
		{"App.java", "Java"},
		{"script.sh", "Shell"},
		{"config.yaml", "YAML"},
		{"data.json", "JSON"},
		{"unknown.xyz", ""},
	}

	for _, tt := range tests {
		got := detectLanguage(tt.path)
		if got != tt.expected {
			t.Errorf("detectLanguage(%s) = %s, want %s", tt.path, got, tt.expected)
		}
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"main_test.go", true},
		{"main.go", false},
		{"test_handler.py", true},
		{"handler_test.py", true},
		{"handler.py", false},
		{"app.test.js", true},
		{"app.spec.ts", true},
		{"app.js", false},
		{"UserTest.java", true},
		{"User.java", false},
		{"tests/integration.rs", true},
	}

	for _, tt := range tests {
		got := isTestFile(tt.path)
		if got != tt.expected {
			t.Errorf("isTestFile(%s) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestIsGeneratedFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"message.pb.go", true},
		{"mock_service.go", true},
		{"generated_types.go", true},
		{"go.sum", true},
		{"package-lock.json", true},
		{"handler.go", false},
		{"main.go", false},
	}

	for _, tt := range tests {
		got := isGeneratedFile(tt.path)
		if got != tt.expected {
			t.Errorf("isGeneratedFile(%s) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestGetModifiedSorted(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"c.go", "a.go", "b.go"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("package x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	ws := NewWorkspaceState(dir)
	ws.MarkModified(filepath.Join(dir, "c.go"))
	ws.MarkModified(filepath.Join(dir, "a.go"))
	ws.MarkModified(filepath.Join(dir, "b.go"))

	modified := ws.GetModified()
	if len(modified) != 3 {
		t.Fatalf("expected 3 modified files, got %d", len(modified))
	}
	if modified[0] != "a.go" || modified[1] != "b.go" || modified[2] != "c.go" {
		t.Errorf("expected sorted [a.go b.go c.go], got %v", modified)
	}
}

func TestSummaryWithNoData(t *testing.T) {
	ws := NewWorkspaceState("/tmp/empty")
	summary := ws.Summary()

	if !strings.Contains(summary, "Modified: 0 files") {
		t.Error("summary should show 0 modified files")
	}
	if !strings.Contains(summary, "Opened: 0 files") {
		t.Error("summary should show 0 opened files")
	}
	if !strings.Contains(summary, "Staged: 0 files") {
		t.Error("summary should show 0 staged files")
	}
	if !strings.Contains(summary, "Last scan: never") {
		t.Error("summary should show 'never' for last scan")
	}
}

func TestExternalChangesWithDeletedFile(t *testing.T) {
	dir := t.TempDir()

	testFile := filepath.Join(dir, "temp.go")
	if err := os.WriteFile(testFile, []byte("package temp\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ws := NewWorkspaceState(dir)
	ws.MarkOpened(testFile)

	// Delete the file externally
	if err := os.Remove(testFile); err != nil {
		t.Fatal(err)
	}

	changes := ws.DetectExternalChanges()
	if len(changes) != 1 {
		t.Fatalf("expected 1 external change for deleted file, got %d", len(changes))
	}
	if changes[0] != "temp.go" {
		t.Errorf("expected temp.go, got %s", changes[0])
	}
}
