package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/eyrie/client"
)

// setupTestProject creates a temporary directory with test files.
func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create test files.
	dirs := []string{
		"src",
		"lib",
		"internal",
		"pkg/middleware",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	files := []string{
		"src/main.go",
		"src/auth.go",
		"lib/auth.ts",
		"internal/handler.go",
		"pkg/middleware/rate.go",
		"README.md",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("// "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func TestDetectMentions_Backticks(t *testing.T) {
	dir := setupTestProject(t)
	d := NewFileMentionDetector(dir)

	text := "You should check `src/main.go` for the entry point."
	mentions := d.DetectMentions(text)

	assertContains(t, mentions, "src/main.go")
}

func TestDetectMentions_Quotes(t *testing.T) {
	dir := setupTestProject(t)
	d := NewFileMentionDetector(dir)

	text := `The authentication logic is in "lib/auth.ts" and needs updating.`
	mentions := d.DetectMentions(text)

	assertContains(t, mentions, "lib/auth.ts")
}

func TestDetectMentions_BarePaths(t *testing.T) {
	dir := setupTestProject(t)
	d := NewFileMentionDetector(dir)

	text := "Look at ./internal/handler.go for the implementation."
	mentions := d.DetectMentions(text)

	assertContains(t, mentions, "./internal/handler.go")
}

func TestDetectMentions_FiltersURLs(t *testing.T) {
	dir := setupTestProject(t)
	d := NewFileMentionDetector(dir)

	text := "See http://example.com/path/to/file.go and https://github.com/repo/main.go"
	mentions := d.DetectMentions(text)

	for _, m := range mentions {
		if m == "http://example.com/path/to/file.go" || m == "https://github.com/repo/main.go" {
			t.Errorf("should not match URL, got: %s", m)
		}
	}
}

func TestDetectMentions_FiltersNonExistent(t *testing.T) {
	dir := setupTestProject(t)
	d := NewFileMentionDetector(dir)

	text := "Check `nonexistent/file.go` for details."
	mentions := d.DetectMentions(text)

	for _, m := range mentions {
		if m == "nonexistent/file.go" {
			t.Error("should not include non-existent files")
		}
	}
}

func TestDetectMentions_Deduplicates(t *testing.T) {
	dir := setupTestProject(t)
	d := NewFileMentionDetector(dir)

	text := "Look at `src/main.go` and also src/main.go for reference."
	mentions := d.DetectMentions(text)

	count := 0
	for _, m := range mentions {
		if m == "src/main.go" {
			count++
		}
	}
	if count > 1 {
		t.Errorf("expected src/main.go to appear once, got %d times", count)
	}
}

func TestDetectMentions_FileLineFormat(t *testing.T) {
	dir := setupTestProject(t)
	d := NewFileMentionDetector(dir)

	text := "Error at src/main.go:42 in the handler."
	mentions := d.DetectMentions(text)

	assertContains(t, mentions, "src/main.go")

	// Should NOT contain the version with line number.
	for _, m := range mentions {
		if m == "src/main.go:42" {
			t.Error("should strip line number from path")
		}
	}
}

func TestBuildSuggestion(t *testing.T) {
	d := NewFileMentionDetector("/tmp")

	result := d.BuildSuggestion([]string{"src/auth.go", "pkg/middleware/rate.go"})
	expected := "Files mentioned but not in context: src/auth.go, pkg/middleware/rate.go\nConsider reading these for better context."

	if result != expected {
		t.Errorf("unexpected suggestion:\ngot:  %q\nwant: %q", result, expected)
	}
}

func TestBuildSuggestion_Empty(t *testing.T) {
	d := NewFileMentionDetector("/tmp")

	result := d.BuildSuggestion(nil)
	if result != "" {
		t.Errorf("expected empty string for nil input, got: %q", result)
	}

	result = d.BuildSuggestion([]string{})
	if result != "" {
		t.Errorf("expected empty string for empty input, got: %q", result)
	}
}

func TestFilterNew(t *testing.T) {
	d := NewFileMentionDetector("/tmp")

	paths := []string{"src/main.go", "lib/auth.ts", "internal/handler.go"}
	alreadyInContext := map[string]bool{
		"src/main.go": true,
	}

	result := d.FilterNew(paths, alreadyInContext)

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(result), result)
	}
	assertContains(t, result, "lib/auth.ts")
	assertContains(t, result, "internal/handler.go")

	// Should not contain the already-known file.
	for _, r := range result {
		if r == "src/main.go" {
			t.Error("FilterNew should remove already-known file src/main.go")
		}
	}
}

func TestInjectFileMentionContext_NewFiles(t *testing.T) {
	dir := setupTestProject(t)
	d := NewFileMentionDetector(dir)

	text := "You need to update `src/auth.go` and `pkg/middleware/rate.go`."
	messages := []client.EyrieMessage{} // no prior messages

	result := d.InjectFileMentionContext(text, messages)

	if result == "" {
		t.Error("expected non-empty context suggestion")
	}
	if !fileMentionsContainsStr(result, "src/auth.go") {
		t.Error("expected suggestion to mention src/auth.go")
	}
}

func TestInjectFileMentionContext_AllAlreadyDiscussed(t *testing.T) {
	dir := setupTestProject(t)
	d := NewFileMentionDetector(dir)

	text := "You need to update `src/auth.go`."
	messages := []client.EyrieMessage{
		{Role: "user", Content: "I'm working on src/auth.go"},
	}

	result := d.InjectFileMentionContext(text, messages)

	if result != "" {
		t.Errorf("expected empty string when all files already discussed, got: %q", result)
	}
}

func TestDetectMentions_FiltersFalsePositives(t *testing.T) {
	dir := setupTestProject(t)
	d := NewFileMentionDetector(dir)

	text := "Write to /dev/null and check /etc/passwd for users."
	mentions := d.DetectMentions(text)

	for _, m := range mentions {
		if m == "/dev/null" || m == "/etc/passwd" {
			t.Errorf("should filter out false positive: %s", m)
		}
	}
}

func TestStripLineNumber(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"main.go:42", "main.go"},
		{"src/handler.go:100", "src/handler.go"},
		{"plain.go", "plain.go"},
		{"no-number.go:", "no-number.go:"},
	}

	for _, tt := range tests {
		got := stripLineNumber(tt.input)
		if got != tt.want {
			t.Errorf("stripLineNumber(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// assertContains checks that the slice contains the expected value.
func assertContains(t *testing.T, slice []string, expected string) {
	t.Helper()
	for _, s := range slice {
		if s == expected {
			return
		}
	}
	t.Errorf("expected slice to contain %q, got: %v", expected, slice)
}

// fileMentionsContainsStr checks if a string contains a substring.
func fileMentionsContainsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
