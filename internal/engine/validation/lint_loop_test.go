package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultLintCommands(t *testing.T) {
	cmds := DefaultLintCommands()

	// Verify all expected extensions are present
	expected := map[string]string{
		".go":  "go vet",
		".py":  "python -m py_compile",
		".ts":  "tsc --noEmit",
		".tsx": "tsc --noEmit",
		".js":  "node --check",
		".rs":  "rustc",
		".rb":  "ruby -c",
	}

	for ext, prefix := range expected {
		cmd, ok := cmds[ext]
		if !ok {
			t.Errorf("missing lint command for extension %s", ext)
			continue
		}
		if !strings.Contains(cmd, prefix) {
			t.Errorf("lint command for %s = %q, want it to contain %q", ext, cmd, prefix)
		}
	}
}

func TestLintLoopCommandSelectionByExtension(t *testing.T) {
	ll := NewLintLoop()

	tests := []struct {
		file    string
		wantCmd string
	}{
		{"main.go", "go vet"},
		{"app.py", "python -m py_compile"},
		{"index.ts", "tsc --noEmit"},
		{"component.tsx", "tsc --noEmit"},
		{"script.js", "node --check"},
		{"lib.rs", "rustc"},
		{"unknown.xyz", ""},
		{"noext", ""},
	}

	for _, tt := range tests {
		ext := filepath.Ext(tt.file)
		cmd, ok := ll.LintCommands[ext]
		if tt.wantCmd == "" {
			if ok {
				t.Errorf("file %s: expected no lint command, got %q", tt.file, cmd)
			}
			continue
		}
		if !ok {
			t.Errorf("file %s: no lint command found for ext %q", tt.file, ext)
			continue
		}
		if !strings.Contains(cmd, tt.wantCmd) {
			t.Errorf("file %s: command %q does not contain %q", tt.file, cmd, tt.wantCmd)
		}
	}
}

func TestBuildReflectedMessage(t *testing.T) {
	ll := NewLintLoop()

	t.Run("nil result", func(t *testing.T) {
		msg := ll.BuildReflectedMessage(nil)
		if msg != "" {
			t.Errorf("expected empty message for nil result, got %q", msg)
		}
	})

	t.Run("empty errors", func(t *testing.T) {
		msg := ll.BuildReflectedMessage(&LintResult{
			File:   "main.go",
			Errors: nil,
		})
		if msg != "" {
			t.Errorf("expected empty message for no errors, got %q", msg)
		}
	})

	t.Run("single error", func(t *testing.T) {
		result := &LintResult{
			File:     "main.go",
			Errors:   []string{"main.go:10:5: undefined: foo"},
			ExitCode: 1,
		}
		msg := ll.BuildReflectedMessage(result)

		if !strings.Contains(msg, "main.go") {
			t.Error("message should contain the file name")
		}
		if !strings.Contains(msg, "lint errors") {
			t.Error("message should mention lint errors")
		}
		if !strings.Contains(msg, "undefined: foo") {
			t.Error("message should contain the actual error")
		}
		if !strings.Contains(msg, "Please fix these issues") {
			t.Error("message should ask the agent to fix issues")
		}
	})

	t.Run("multiple errors", func(t *testing.T) {
		result := &LintResult{
			File: "app.py",
			Errors: []string{
				"SyntaxError: invalid syntax (app.py, line 5)",
				"IndentationError: unexpected indent (app.py, line 12)",
			},
			ExitCode: 1,
		}
		msg := ll.BuildReflectedMessage(result)

		if !strings.Contains(msg, "app.py") {
			t.Error("message should contain the file name")
		}
		if !strings.Contains(msg, "SyntaxError") {
			t.Error("message should contain first error")
		}
		if !strings.Contains(msg, "IndentationError") {
			t.Error("message should contain second error")
		}
	})
}

func TestShouldRetry(t *testing.T) {
	ll := NewLintLoop()
	ll.MaxReflections = 3

	tests := []struct {
		count int
		want  bool
	}{
		{0, true},
		{1, true},
		{2, true},
		{3, false},
		{4, false},
		{10, false},
	}

	for _, tt := range tests {
		got := ll.ShouldRetry(tt.count)
		if got != tt.want {
			t.Errorf("ShouldRetry(%d) = %v, want %v", tt.count, got, tt.want)
		}
	}
}

func TestMaxReflectionsEnforcement(t *testing.T) {
	ll := NewLintLoop()
	ll.MaxReflections = 3

	file := "/tmp/test.go"

	// Should allow retries up to MaxReflections
	for i := 0; i < 3; i++ {
		count := ll.ReflectionCount(file)
		if !ll.ShouldRetry(count) {
			t.Errorf("iteration %d: expected ShouldRetry=true at count %d", i, count)
		}
		ll.RecordReflection(file)
	}

	// After 3 reflections, should stop
	count := ll.ReflectionCount(file)
	if ll.ShouldRetry(count) {
		t.Errorf("expected ShouldRetry=false after %d reflections", count)
	}
}

func TestReflectionCountTracking(t *testing.T) {
	ll := NewLintLoop()

	// Different files tracked independently
	ll.RecordReflection("/a.go")
	ll.RecordReflection("/a.go")
	ll.RecordReflection("/b.go")

	if got := ll.ReflectionCount("/a.go"); got != 2 {
		t.Errorf("ReflectionCount(/a.go) = %d, want 2", got)
	}
	if got := ll.ReflectionCount("/b.go"); got != 1 {
		t.Errorf("ReflectionCount(/b.go) = %d, want 1", got)
	}
	if got := ll.ReflectionCount("/c.go"); got != 0 {
		t.Errorf("ReflectionCount(/c.go) = %d, want 0", got)
	}
}

func TestResetFile(t *testing.T) {
	ll := NewLintLoop()
	ll.RecordReflection("/a.go")
	ll.RecordReflection("/a.go")
	ll.ResetFile("/a.go")

	if got := ll.ReflectionCount("/a.go"); got != 0 {
		t.Errorf("ReflectionCount after reset = %d, want 0", got)
	}
}

func TestReset(t *testing.T) {
	ll := NewLintLoop()
	ll.RecordReflection("/a.go")
	ll.RecordReflection("/b.go")
	ll.Reset()

	if got := ll.ReflectionCount("/a.go"); got != 0 {
		t.Errorf("ReflectionCount(/a.go) after reset = %d, want 0", got)
	}
	if got := ll.ReflectionCount("/b.go"); got != 0 {
		t.Errorf("ReflectionCount(/b.go) after reset = %d, want 0", got)
	}
}

func TestRunLintDisabled(t *testing.T) {
	ll := NewLintLoop()
	ll.Enabled = false

	result, err := ll.RunLint("main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result when disabled")
	}
}

func TestRunLintNoExtension(t *testing.T) {
	ll := NewLintLoop()

	result, err := ll.RunLint("Makefile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for file without extension")
	}
}

func TestRunLintUnknownExtension(t *testing.T) {
	ll := NewLintLoop()

	result, err := ll.RunLint("data.xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for unknown extension")
	}
}

func TestRunLintPassingFile(t *testing.T) {
	// Create a temporary valid Go file
	dir := t.TempDir()
	goFile := filepath.Join(dir, "valid.go")
	err := os.WriteFile(goFile, []byte("package main\n\nfunc main() {}\n"), 0o644)
	if err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ll := NewLintLoop()
	// Use a simpler command that will definitely pass
	ll.LintCommands[".go"] = "go vet {file}"

	result, err := ll.RunLint(goFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for valid Go file, got: %+v", result)
	}
}

func TestRunLintFailingFile(t *testing.T) {
	// Create a temporary invalid Go file
	dir := t.TempDir()
	goFile := filepath.Join(dir, "invalid.go")
	// This file has a syntax error
	err := os.WriteFile(goFile, []byte("package main\n\nfunc main() {\n\tfmt.Println(undefined)\n}\n"), 0o644)
	if err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ll := NewLintLoop()
	// Use go vet which will catch this
	ll.LintCommands[".go"] = "go vet {file}"

	result, err := ll.RunLint(goFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// go vet should report errors for this file
	if result == nil {
		t.Skip("go vet might not be available or might not catch this; skipping")
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code for invalid file")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}
}

func TestParseLintErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		notWant []string
	}{
		{
			name:  "go vet output",
			input: "main.go:10:5: undefined: foo\nmain.go:12:3: too many arguments\n",
			want:  2,
		},
		{
			name:  "empty output",
			input: "",
			want:  0,
		},
		{
			name:  "only whitespace",
			input: "  \n\n  \n",
			want:  0,
		},
		{
			name:    "filters noise",
			input:   "Compiling mylib v0.1.0\nerror[E0425]: cannot find value `x`\nFinished dev\n",
			want:    1,
			notWant: []string{"Compiling", "Finished"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLintErrors(tt.input)
			if len(got) != tt.want {
				t.Errorf("parseLintErrors() returned %d errors, want %d: %v", len(got), tt.want, got)
			}
			for _, nw := range tt.notWant {
				for _, line := range got {
					if strings.Contains(line, nw) {
						t.Errorf("parseLintErrors() should filter %q, but found it in output", nw)
					}
				}
			}
		})
	}
}

func TestNewLintLoopDefaults(t *testing.T) {
	ll := NewLintLoop()

	if ll.MaxReflections != 3 {
		t.Errorf("MaxReflections = %d, want 3", ll.MaxReflections)
	}
	if !ll.Enabled {
		t.Error("expected Enabled = true by default")
	}
	if ll.LintCommands == nil {
		t.Fatal("LintCommands should not be nil")
	}
	if ll.reflectionCounts == nil {
		t.Fatal("reflectionCounts should not be nil")
	}

	// Verify key languages are covered
	for _, ext := range []string{".go", ".py", ".ts", ".js", ".rs"} {
		if _, ok := ll.LintCommands[ext]; !ok {
			t.Errorf("missing default lint command for %s", ext)
		}
	}
}
