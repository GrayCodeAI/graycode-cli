package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectAffectedTests_NoGoFiles(t *testing.T) {
	t.Parallel()
	result := DetectAffectedTests([]string{"README.md", "Makefile"})
	if len(result.AffectedTests) != 0 {
		t.Errorf("expected 0 affected tests for non-Go files, got %d", len(result.AffectedTests))
	}
}

func TestDetectAffectedTests_TestFileIgnored(t *testing.T) {
	t.Parallel()
	result := DetectAffectedTests([]string{"cmd/foo_test.go"})
	if len(result.AffectedTests) != 0 {
		t.Errorf("test files should not be tracked as changed source, got %d", len(result.AffectedTests))
	}
}

func TestDetectAffectedTests_WithRealFiles(t *testing.T) {
	// Create a temp directory structure
	dir := t.TempDir()
	subDir := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create source file
	srcFile := filepath.Join(subDir, "handler.go")
	if err := os.WriteFile(srcFile, []byte("package pkg\nfunc Handle() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create test file in same directory
	testFile := filepath.Join(subDir, "handler_test.go")
	if err := os.WriteFile(testFile, []byte("package pkg\nimport \"testing\"\nfunc TestHandle(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := DetectAffectedTests([]string{srcFile})
	if len(result.AffectedTests) == 0 {
		t.Error("expected at least one affected test")
	}

	found := false
	for _, tf := range result.AffectedTests {
		if tf == testFile {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %s in affected tests, got %v", testFile, result.AffectedTests)
	}
}

func TestFormatAffectedTests_Empty(t *testing.T) {
	t.Parallel()
	at := AffectedTests{}
	output := FormatAffectedTests(at)
	if output == "" {
		t.Error("should produce output for empty input")
	}
}

func TestFormatAffectedTests_WithResults(t *testing.T) {
	t.Parallel()
	at := AffectedTests{
		ChangedFiles:  []string{"cmd/main.go", "internal/handler.go"},
		AffectedTests: []string{"cmd/main_test.go", "internal/handler_test.go"},
		TestFileMap: map[string][]string{
			"cmd/main.go":         {"cmd/main_test.go"},
			"internal/handler.go": {"internal/handler_test.go"},
		},
	}
	output := FormatAffectedTests(at)
	if output == "" {
		t.Error("should produce output")
	}
}
