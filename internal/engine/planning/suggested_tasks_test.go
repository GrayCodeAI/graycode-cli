package planning

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewTaskQueue(t *testing.T) {
	tq := NewTaskQueue()
	if tq == nil {
		t.Fatal("expected non-nil TaskQueue")
	}
	if len(tq.Tasks) != 0 {
		t.Fatalf("expected empty tasks, got %d", len(tq.Tasks))
	}
	if len(tq.Dismissed) != 0 {
		t.Fatalf("expected empty dismissed, got %d", len(tq.Dismissed))
	}
}

func TestDismiss(t *testing.T) {
	tq := NewTaskQueue()
	tq.Tasks = []*SuggestedTask{
		{ID: "task-1", Title: "Fix bug", Priority: 1, Category: "fix"},
		{ID: "task-2", Title: "Add docs", Priority: 4, Category: "docs"},
		{ID: "task-3", Title: "Clean up", Priority: 3, Category: "cleanup"},
	}

	tq.Dismiss("task-2")

	if len(tq.Dismissed) != 1 {
		t.Fatalf("expected 1 dismissed, got %d", len(tq.Dismissed))
	}
	if tq.Dismissed[0] != "task-2" {
		t.Fatalf("expected task-2 dismissed, got %s", tq.Dismissed[0])
	}

	// GetTop should exclude dismissed
	top := tq.GetTop(10)
	for _, task := range top {
		if task.ID == "task-2" {
			t.Fatal("dismissed task should not appear in GetTop")
		}
	}
}

func TestGetTopPriority(t *testing.T) {
	tq := NewTaskQueue()
	tq.Tasks = []*SuggestedTask{
		{ID: "low", Title: "Low priority", Priority: 5, Category: "docs"},
		{ID: "high", Title: "High priority", Priority: 1, Category: "fix"},
		{ID: "med", Title: "Medium priority", Priority: 3, Category: "test"},
	}

	top := tq.GetTop(2)
	if len(top) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(top))
	}
	if top[0].ID != "high" {
		t.Fatalf("expected highest priority first, got %s (priority %d)", top[0].ID, top[0].Priority)
	}
	if top[1].ID != "med" {
		t.Fatalf("expected medium priority second, got %s (priority %d)", top[1].ID, top[1].Priority)
	}
}

func TestGetTopLimitExceedsCount(t *testing.T) {
	tq := NewTaskQueue()
	tq.Tasks = []*SuggestedTask{
		{ID: "one", Title: "Task one", Priority: 1, Category: "fix"},
	}

	top := tq.GetTop(10)
	if len(top) != 1 {
		t.Fatalf("expected 1 task when limit exceeds count, got %d", len(top))
	}
}

func TestGetTopEmpty(t *testing.T) {
	tq := NewTaskQueue()
	top := tq.GetTop(5)
	if len(top) != 0 {
		t.Fatalf("expected 0 tasks for empty queue, got %d", len(top))
	}
}

func TestFormatTasksEmpty(t *testing.T) {
	result := FormatTasks(nil)
	if result != "No suggested tasks." {
		t.Fatalf("unexpected output for empty tasks: %s", result)
	}
}

func TestFormatTasks(t *testing.T) {
	tasks := []*SuggestedTask{
		{
			ID:       "t1",
			Title:    "Resolve merge conflict in src/auth.go",
			Priority: 1,
			Category: "fix",
			Command:  `hawk exec "resolve the merge conflict"`,
		},
		{
			ID:       "t2",
			Title:    "Fix failing TestValidateToken",
			Priority: 2,
			Category: "test",
			Command:  `hawk exec "fix the failing test"`,
		},
		{
			ID:       "t3",
			Title:    "Document exported function ParseConfig",
			Priority: 4,
			Category: "docs",
			Command:  `hawk exec "add godoc to ParseConfig"`,
		},
	}

	result := FormatTasks(tasks)

	// Verify structure
	if !strings.Contains(result, "Suggested Tasks (3):") {
		t.Fatal("expected header with count")
	}
	if !strings.Contains(result, "───") {
		t.Fatal("expected separator line")
	}
	if !strings.Contains(result, "1.") {
		t.Fatal("expected numbered items")
	}
	if !strings.Contains(result, "[fix]") {
		t.Fatal("expected category labels")
	}
	if !strings.Contains(result, "[test]") {
		t.Fatal("expected test category")
	}
	if !strings.Contains(result, "[docs]") {
		t.Fatal("expected docs category")
	}
	if !strings.Contains(result, "Run: hawk exec") {
		t.Fatal("expected Run: command")
	}
	// Red circle for priority 1
	if !strings.Contains(result, "\U0001f534") {
		t.Fatal("expected red indicator for priority 1")
	}
	// Yellow circle for priority 2-3
	if !strings.Contains(result, "\U0001f7e1") {
		t.Fatal("expected yellow indicator for priority 2-3")
	}
	// Blue circle for priority 4-5
	if !strings.Contains(result, "\U0001f535") {
		t.Fatal("expected blue indicator for priority 4-5")
	}
}

func TestScanTODOs(t *testing.T) {
	// Create a temporary directory with source files containing TODOs
	dir := t.TempDir()

	// Create a Go file with TODO comments
	goFile := filepath.Join(dir, "main.go")
	content := `package main

// TODO: implement proper error handling
func main() {
	// FIXME: this is broken
	doSomething()
	// HACK: temporary workaround
	hackAround()
}
`
	if err := os.WriteFile(goFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a file in a skip directory (should be ignored)
	vendorDir := filepath.Join(dir, "vendor")
	os.MkdirAll(vendorDir, 0o755)
	vendorFile := filepath.Join(vendorDir, "dep.go")
	os.WriteFile(vendorFile, []byte("// TODO: vendor todo"), 0o644)

	tasks := ScanTODOs(dir)

	if len(tasks) != 3 {
		t.Fatalf("expected 3 TODO tasks, got %d", len(tasks))
	}

	// Verify categories/priorities
	foundFixme := false
	foundHack := false
	foundTodo := false
	for _, task := range tasks {
		if task.Source != "todo" {
			t.Fatalf("expected source 'todo', got %s", task.Source)
		}
		if strings.Contains(task.Description, "FIXME") {
			foundFixme = true
			if task.Priority != 2 {
				t.Fatalf("expected FIXME priority 2, got %d", task.Priority)
			}
			if task.Category != "fix" {
				t.Fatalf("expected FIXME category 'fix', got %s", task.Category)
			}
		}
		if strings.Contains(task.Description, "HACK") {
			foundHack = true
			if task.Priority != 2 {
				t.Fatalf("expected HACK priority 2, got %d", task.Priority)
			}
		}
		if strings.Contains(task.Description, "TODO") {
			foundTodo = true
			if task.Priority != 3 {
				t.Fatalf("expected TODO priority 3, got %d", task.Priority)
			}
		}
	}
	if !foundFixme || !foundHack || !foundTodo {
		t.Fatal("expected to find FIXME, HACK, and TODO tasks")
	}
}

func TestScanTODOsSkipsVendor(t *testing.T) {
	dir := t.TempDir()

	// Only vendor file
	vendorDir := filepath.Join(dir, "vendor")
	os.MkdirAll(vendorDir, 0o755)
	os.WriteFile(filepath.Join(vendorDir, "lib.go"), []byte("// TODO: should skip"), 0o644)

	tasks := ScanTODOs(dir)
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks for vendor-only project, got %d", len(tasks))
	}
}

func TestScanGitTasksNonGitDir(t *testing.T) {
	// Non-git directory should return empty (git commands will fail)
	dir := t.TempDir()
	tasks := ScanGitTasks(dir)
	// Should not panic, just return empty or tasks based on error handling
	_ = tasks
}

func TestScanGitTasksUncommittedChanges(t *testing.T) {
	dir := t.TempDir()

	// Initialize a git repo with uncommitted changes
	runGit := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Run()
	}

	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "Test")

	// Create a committed file
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0o644)
	runGit("add", ".")
	runGit("commit", "-m", "initial")

	// Create uncommitted changes
	os.WriteFile(filepath.Join(dir, "new_file.go"), []byte("package main"), 0o644)

	tasks := ScanGitTasks(dir)

	foundUncommitted := false
	for _, task := range tasks {
		if strings.Contains(task.Title, "pending changes") {
			foundUncommitted = true
			if task.Priority != 2 {
				t.Fatalf("expected priority 2, got %d", task.Priority)
			}
			if task.Category != "cleanup" {
				t.Fatalf("expected category cleanup, got %s", task.Category)
			}
			if task.Source != "git" {
				t.Fatalf("expected source git, got %s", task.Source)
			}
		}
	}
	if !foundUncommitted {
		t.Fatal("expected to find uncommitted changes task")
	}
}

func TestScanTestFailuresNoTestProject(t *testing.T) {
	dir := t.TempDir()
	// No go.mod, package.json, or Cargo.toml — should return empty
	tasks := ScanTestFailures(dir)
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks for non-test project, got %d", len(tasks))
	}
}

func TestTaskQueueConcurrency(t *testing.T) {
	tq := NewTaskQueue()
	tq.Tasks = []*SuggestedTask{
		{ID: "t1", Title: "Task 1", Priority: 1, Category: "fix"},
		{ID: "t2", Title: "Task 2", Priority: 2, Category: "test"},
		{ID: "t3", Title: "Task 3", Priority: 3, Category: "docs"},
	}

	var wg sync.WaitGroup
	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tq.GetTop(2)
		}()
	}
	// Concurrent dismissals
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tq.Dismiss("t1")
		}()
	}
	wg.Wait()
}

func TestSuggestedTaskStruct(t *testing.T) {
	task := &SuggestedTask{
		ID:          "abc123",
		Title:       "Fix critical bug",
		Description: "A critical bug needs fixing.",
		Priority:    1,
		Category:    "fix",
		Source:      "test",
		Actionable:  true,
		Command:     `hawk exec "fix the bug"`,
	}

	if task.ID != "abc123" {
		t.Fatal("unexpected ID")
	}
	if task.Category != "fix" {
		t.Fatal("unexpected category")
	}
	if !task.Actionable {
		t.Fatal("expected actionable to be true")
	}
}

func TestGenerateTaskID(t *testing.T) {
	id1 := generateTaskID()
	id2 := generateTaskID()

	if id1 == "" {
		t.Fatal("expected non-empty ID")
	}
	if id2 == "" {
		t.Fatal("expected non-empty ID")
	}
	if id1 == id2 {
		t.Fatal("expected unique IDs")
	}
	if len(id1) != 16 {
		t.Fatalf("expected 16-char hex ID, got %d chars", len(id1))
	}
}

func TestScanSecurityTasks(t *testing.T) {
	dir := t.TempDir()

	// Create a Go file with a potential security issue
	goFile := filepath.Join(dir, "config.go")
	content := `package config

var dbPassword = "super_secret_123"
var api_key = "sk-abc123"
`
	os.WriteFile(goFile, []byte(content), 0o644)

	tasks := scanSecurityTasks(dir)

	// Should find password and api_key references
	if len(tasks) == 0 {
		t.Fatal("expected security tasks for hardcoded secrets")
	}

	foundPassword := false
	foundAPIKey := false
	for _, task := range tasks {
		if task.Category != "security" {
			t.Fatalf("expected category security, got %s", task.Category)
		}
		if task.Priority != 1 {
			t.Fatalf("expected priority 1 for security, got %d", task.Priority)
		}
		if strings.Contains(strings.ToLower(task.Title), "password") {
			foundPassword = true
		}
		if strings.Contains(strings.ToLower(task.Title), "api key") {
			foundAPIKey = true
		}
	}
	if !foundPassword {
		t.Fatal("expected to find password security issue")
	}
	if !foundAPIKey {
		t.Fatal("expected to find API key security issue")
	}
}

func TestScanDocsTasks(t *testing.T) {
	dir := t.TempDir()

	// Create go.mod so it's recognized as a Go project
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0o644)

	// Create a Go file with an undocumented exported function
	goFile := filepath.Join(dir, "handler.go")
	content := `package handler

// ParseConfig parses the configuration file.
func ParseConfig() error {
	return nil
}

func UndocumentedExport() string {
	return "no docs"
}

func unexported() {}
`
	os.WriteFile(goFile, []byte(content), 0o644)

	tasks := scanDocsTasks(dir)

	// Should find UndocumentedExport but not ParseConfig (has comment) or unexported
	foundUndoc := false
	for _, task := range tasks {
		if strings.Contains(task.Title, "UndocumentedExport") {
			foundUndoc = true
			if task.Category != "docs" {
				t.Fatalf("expected category docs, got %s", task.Category)
			}
			if task.Priority != 4 {
				t.Fatalf("expected priority 4, got %d", task.Priority)
			}
		}
		if strings.Contains(task.Title, "ParseConfig") {
			t.Fatal("ParseConfig has a doc comment and should not appear")
		}
		if strings.Contains(task.Title, "unexported") {
			t.Fatal("unexported functions should not appear")
		}
	}
	if !foundUndoc {
		t.Fatal("expected to find UndocumentedExport task")
	}
}

func TestRefresh(t *testing.T) {
	dir := t.TempDir()

	// Create a file with a TODO
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("// TODO: test refresh\npackage main\n"), 0o644)

	tq := NewTaskQueue()
	err := tq.Refresh(dir)
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	if len(tq.Tasks) == 0 {
		t.Fatal("expected tasks after refresh")
	}

	// At minimum, should have the TODO task
	foundTodo := false
	for _, task := range tq.Tasks {
		if task.Source == "todo" {
			foundTodo = true
			break
		}
	}
	if !foundTodo {
		t.Fatal("expected TODO task after refresh")
	}
}

func TestGetTopSecondarySortByCategory(t *testing.T) {
	tq := NewTaskQueue()
	tq.Tasks = []*SuggestedTask{
		{ID: "docs", Title: "Docs task", Priority: 2, Category: "docs"},
		{ID: "fix", Title: "Fix task", Priority: 2, Category: "fix"},
		{ID: "test", Title: "Test task", Priority: 2, Category: "test"},
	}

	top := tq.GetTop(3)
	if len(top) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(top))
	}
	// Same priority — should sort by category importance: fix, test, docs
	if top[0].ID != "fix" {
		t.Fatalf("expected fix first in secondary sort, got %s", top[0].ID)
	}
	if top[1].ID != "test" {
		t.Fatalf("expected test second in secondary sort, got %s", top[1].ID)
	}
	if top[2].ID != "docs" {
		t.Fatalf("expected docs third in secondary sort, got %s", top[2].ID)
	}
}
