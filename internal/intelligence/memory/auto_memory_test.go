package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShouldRemember(t *testing.T) {
	am := &AutoMemory{dir: t.TempDir()}
	tests := []struct {
		input string
		want  bool
	}{
		{"don't use tabs, use spaces", true},
		{"use spaces instead of tabs", true},
		{"correction: the port is 8080", true},
		{"actually, I meant the other file", true},
		{"remember to always run tests", true},
		{"hello world", false},
		{"please read the file", false},
	}
	for _, tt := range tests {
		got := am.ShouldRemember(tt.input)
		if got != tt.want {
			t.Errorf("ShouldRemember(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestWriteAndSearch(t *testing.T) {
	dir := t.TempDir()
	am := &AutoMemory{dir: dir}

	if err := am.Write("prefs", "use 4 spaces for indent"); err != nil {
		t.Fatal(err)
	}
	if err := am.Write("prefs", "always run go vet before commit"); err != nil {
		t.Fatal(err)
	}

	results := am.Search("indent")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(results), results)
	}
	if !strings.Contains(results[0], "indent") {
		t.Fatalf("result should contain 'indent': %s", results[0])
	}
}

func TestLoadStartup(t *testing.T) {
	dir := t.TempDir()
	am := &AutoMemory{dir: dir}

	// No file → empty string
	if got := am.LoadStartup(); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}

	// Write a MEMORY.md
	content := "# Memory\n- preference 1\n- preference 2\n"
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := am.LoadStartup()
	if got != content {
		t.Fatalf("expected %q, got %q", content, got)
	}
}

func TestLoadStartupMaxLines(t *testing.T) {
	dir := t.TempDir()
	am := &AutoMemory{dir: dir}

	// Write more than 200 lines
	var b strings.Builder
	for i := 0; i < 250; i++ {
		b.WriteString("line content\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	got := am.LoadStartup()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) > 200 {
		t.Fatalf("expected at most 200 lines, got %d", len(lines))
	}
}

func TestFormat(t *testing.T) {
	dir := t.TempDir()
	am := &AutoMemory{dir: dir}

	// No memory → empty
	if got := am.Format(); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}

	// With memory
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("- pref 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := am.Format()
	if !strings.HasPrefix(got, "## Project Memory") {
		t.Fatalf("expected formatted header, got %q", got)
	}
	if !strings.Contains(got, "pref 1") {
		t.Fatalf("expected content in format output, got %q", got)
	}
}
