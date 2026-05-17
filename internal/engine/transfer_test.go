package engine

import (
	"os"
	"testing"
)

func TestTransferLearning_LearnAndApply(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.hawk", 0o755)

	tl := NewTransferLearning()
	tl.Learn("go", "error_handling", "wrap errors with context", "use fmt.Errorf with %w")
	tl.Learn("go", "testing", "table-driven tests", "use []struct with t.Run")
	tl.Learn("python", "error_handling", "exception hierarchy", "define custom exceptions")

	patterns := tl.Apply("go", "fix error handling")
	_ = patterns
}

func TestTransferLearning_FormatForPrompt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.hawk", 0o755)

	tl := NewTransferLearning()
	tl.Learn("go", "concurrency", "use errgroup", "bounded goroutines")

	result := tl.FormatForPrompt("go", "add parallel processing")
	_ = result
}

func TestTransferLearning_Empty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.hawk", 0o755)

	tl := NewTransferLearning()
	patterns := tl.Apply("rust", "memory management")
	if len(patterns) != 0 {
		t.Errorf("empty store should return 0 patterns, got %d", len(patterns))
	}
}
