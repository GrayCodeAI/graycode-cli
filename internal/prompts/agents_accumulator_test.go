package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewAgentsAccumulator(t *testing.T) {
	dir := t.TempDir()
	acc := NewAgentsAccumulator(dir)
	if acc == nil {
		t.Fatal("expected non-nil accumulator")
	}
	if acc.filePath != filepath.Join(dir, ".hawk", "agents.md") {
		t.Errorf("unexpected path: %s", acc.filePath)
	}
}

func TestRecordAndFlush(t *testing.T) {
	dir := t.TempDir()
	acc := NewAgentsAccumulator(dir)

	acc.Record("Fixing auth bug", "Always validate JWT before checking permissions", []string{"auth.go", "auth_test.go"})
	acc.Record("Adding logging", "Use structured logging with context", []string{"logger.go"})

	if err := acc.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Verify file was created
	data, err := os.ReadFile(acc.filePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Agent Learnings") {
		t.Error("missing header")
	}
	if !strings.Contains(content, "Fixing auth bug") {
		t.Error("missing first learning context")
	}
	if !strings.Contains(content, "Always validate JWT") {
		t.Error("missing first learning pattern")
	}
	if !strings.Contains(content, "Adding logging") {
		t.Error("missing second learning context")
	}
}

func TestFlushEmpty(t *testing.T) {
	dir := t.TempDir()
	acc := NewAgentsAccumulator(dir)

	// Flush with no learnings should be a no-op
	if err := acc.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// File should not exist
	if _, err := os.Stat(acc.filePath); !os.IsNotExist(err) {
		t.Error("file should not exist when no learnings")
	}
}

func TestFlushAppend(t *testing.T) {
	dir := t.TempDir()
	acc := NewAgentsAccumulator(dir)

	// First flush
	acc.Record("First task", "Pattern 1", nil)
	if err := acc.Flush(); err != nil {
		t.Fatal(err)
	}

	// Second flush
	acc.Record("Second task", "Pattern 2", nil)
	if err := acc.Flush(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(acc.filePath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "First task") {
		t.Error("missing first learning after append")
	}
	if !strings.Contains(content, "Second task") {
		t.Error("missing second learning after append")
	}
}

func TestForPrompt(t *testing.T) {
	dir := t.TempDir()
	acc := NewAgentsAccumulator(dir)

	// No file yet
	if got := acc.ForPrompt(5); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	// Add some learnings
	acc.Record("Task 1", "Pattern 1", nil)
	acc.Record("Task 2", "Pattern 2", nil)
	acc.Flush()

	got := acc.ForPrompt(5)
	if !strings.Contains(got, "Project Learnings") {
		t.Error("missing learnings header")
	}
	if !strings.Contains(got, "Pattern 1") {
		t.Error("missing Pattern 1")
	}
}

func TestForPromptLimit(t *testing.T) {
	dir := t.TempDir()
	acc := NewAgentsAccumulator(dir)

	for i := 0; i < 10; i++ {
		acc.Record("Task", "Pattern", nil)
	}
	acc.Flush()

	got := acc.ForPrompt(3)
	// Should contain at most 3 learning sections (plus header)
	sections := strings.Split(got, "## 20")
	// sections[0] is the header, rest are dated sections
	if len(sections) > 4 { // header + 3 sections
		t.Errorf("expected at most 4 sections (header + 3), got %d", len(sections))
	}
}

func TestExtractPattern(t *testing.T) {
	tests := []struct {
		tool    string
		file    string
		diff    string
		wantSub string
	}{
		{"Write", "auth_test.go", "", "test file"},
		{"Edit", "auth.go", "func Test", "test"},
		{"Write", "README.md", "", "documentation"},
		{"Edit", "main.go", "import", "imports"},
		{"Edit", "main.go", "some change", "main.go"},
	}
	for _, tc := range tests {
		got := ExtractPattern(tc.tool, tc.file, tc.diff)
		if !strings.Contains(got, tc.wantSub) {
			t.Errorf("ExtractPattern(%q, %q, %q) = %q, want substring %q", tc.tool, tc.file, tc.diff, got, tc.wantSub)
		}
	}
}
