package cmd

import (
	"fmt"
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

func TestFormatDirectivePrompt(t *testing.T) {
	tests := []struct {
		name        string
		directive   AIDirective
		wantSubstrs []string
	}{
		{
			name:        "act directive",
			directive:   AIDirective{Path: "main.go", Line: 4, Instruction: "add a doc comment", Mode: "!"},
			wantSubstrs: []string{"main.go:4", "Instruction: add a doc comment", "Implement this change now"},
		},
		{
			name:        "ask directive",
			directive:   AIDirective{Path: "util.go", Line: 9, Instruction: "is this thread-safe?", Mode: "?"},
			wantSubstrs: []string{"util.go:9", "Question: is this thread-safe?", "Answer the question"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDirectivePrompt(tt.directive)
			for _, sub := range tt.wantSubstrs {
				if !strings.Contains(got, sub) {
					t.Errorf("prompt missing %q:\n%s", sub, got)
				}
			}
		})
	}
}

func TestProcessAIDirectives_DispatchAndStrip(t *testing.T) {
	dir := t.TempDir()

	content := "package main\n\n// AI! add a doc comment\nfunc hello() {}\n"
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Mock the LLM/agent dispatch path.
	var dispatched []AIDirective
	orig := aiDispatchFn
	aiDispatchFn = func(d AIDirective) error {
		dispatched = append(dispatched, d)
		return nil
	}
	defer func() { aiDispatchFn = orig }()

	n := processAIDirectives(dir, nil)
	if n != 1 {
		t.Fatalf("expected 1 processed directive, got %d", n)
	}
	if len(dispatched) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(dispatched))
	}
	if dispatched[0].Mode != "!" {
		t.Errorf("expected mode '!', got %q", dispatched[0].Mode)
	}
	if dispatched[0].Instruction != "add a doc comment" {
		t.Errorf("unexpected instruction: %q", dispatched[0].Instruction)
	}

	// The AI token must be stripped from the file after a successful dispatch.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "AI!") {
		t.Errorf("AI token should have been stripped:\n%s", string(data))
	}
	if !strings.Contains(string(data), "func hello() {}") {
		t.Errorf("surrounding code should be preserved:\n%s", string(data))
	}
}

func TestProcessAIDirectives_DispatchFailureKeepsToken(t *testing.T) {
	dir := t.TempDir()
	content := "package main\n\n// AI! do the thing\n"
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := aiDispatchFn
	aiDispatchFn = func(d AIDirective) error { return fmt.Errorf("boom") }
	defer func() { aiDispatchFn = orig }()

	n := processAIDirectives(dir, nil)
	if n != 0 {
		t.Fatalf("expected 0 processed directives on failure, got %d", n)
	}

	// On dispatch failure the token must remain so it is retried later.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "AI!") {
		t.Errorf("AI token should be preserved when dispatch fails:\n%s", string(data))
	}
}

func TestProcessAIDirectives_MultipleInSameFileBottomUp(t *testing.T) {
	dir := t.TempDir()
	// Two directives in one file; removing the lower one first must not shift the
	// upper one's line number before it is handled.
	content := "package main\n\n// AI! first\nfunc a() {}\n\n// AI? second?\nfunc b() {}\n"
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var seen []string
	orig := aiDispatchFn
	aiDispatchFn = func(d AIDirective) error {
		seen = append(seen, d.Instruction)
		return nil
	}
	defer func() { aiDispatchFn = orig }()

	n := processAIDirectives(dir, nil)
	if n != 2 {
		t.Fatalf("expected 2 processed directives, got %d", n)
	}
	if len(seen) != 2 {
		t.Fatalf("expected 2 dispatches, got %d", len(seen))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "AI!") || strings.Contains(string(data), "AI?") {
		t.Errorf("all AI tokens should be stripped:\n%s", string(data))
	}
	if !strings.Contains(string(data), "func a()") || !strings.Contains(string(data), "func b()") {
		t.Errorf("code should be preserved:\n%s", string(data))
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
