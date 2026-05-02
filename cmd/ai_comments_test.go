package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanForAIComments(t *testing.T) {
	dir := t.TempDir()

	// Create a Go file with AI directives
	goContent := `package main

func main() {
	// AI! implement error handling
	x := doSomething()
	// AI? should we use a different approach here?
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(goContent), 0o644); err != nil {
		t.Fatal(err)
	}

	directives := scanForAIComments(dir, nil)
	if len(directives) != 2 {
		t.Fatalf("expected 2 directives, got %d", len(directives))
	}

	if directives[0].Mode != "!" {
		t.Errorf("expected mode '!', got %q", directives[0].Mode)
	}
	if directives[0].Instruction != "implement error handling" {
		t.Errorf("unexpected instruction: %q", directives[0].Instruction)
	}
	if directives[0].Line != 4 {
		t.Errorf("expected line 4, got %d", directives[0].Line)
	}

	if directives[1].Mode != "?" {
		t.Errorf("expected mode '?', got %q", directives[1].Mode)
	}
}

func TestScanForAICommentsIgnore(t *testing.T) {
	dir := t.TempDir()

	// Create a file in an ignored directory
	ignored := filepath.Join(dir, "vendor")
	if err := os.MkdirAll(ignored, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ignored, "lib.go"), []byte("// AI! do something\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	directives := scanForAIComments(dir, []string{"vendor"})
	if len(directives) != 0 {
		t.Fatalf("expected 0 directives (vendor ignored), got %d", len(directives))
	}
}

func TestFormatDirectivesAsPrompt(t *testing.T) {
	directives := []AIDirective{
		{Path: "main.go", Line: 10, Instruction: "add logging", Mode: "!"},
		{Path: "util.go", Line: 5, Instruction: "is this correct?", Mode: "?"},
	}

	result := formatDirectivesAsPrompt(directives)
	if !strings.Contains(result, "main.go:10") {
		t.Errorf("expected file reference in output: %s", result)
	}
	if !strings.Contains(result, "[DO]") {
		t.Errorf("expected [DO] tag in output: %s", result)
	}
	if !strings.Contains(result, "[ASK]") {
		t.Errorf("expected [ASK] tag in output: %s", result)
	}

	// Empty directives
	if got := formatDirectivesAsPrompt(nil); got != "" {
		t.Errorf("expected empty string for nil directives, got %q", got)
	}
}

func TestRemoveAIComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")

	content := "package main\n\n// AI! implement this\nfunc hello() {}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := removeAIComment(path, 3); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "AI!") {
		t.Errorf("AI comment should have been removed: %s", string(data))
	}
}
