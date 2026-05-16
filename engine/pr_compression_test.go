package engine

import (
	"strings"
	"testing"
)

func TestNewPRCompressor(t *testing.T) {
	pc := NewPRCompressor(8000)
	if pc.MaxTokens != 8000 {
		t.Errorf("expected MaxTokens=8000, got %d", pc.MaxTokens)
	}
	if len(pc.LanguagePriority) == 0 {
		t.Error("expected non-empty LanguagePriority")
	}
}

func TestScoreFile(t *testing.T) {
	pc := NewPRCompressor(8000)

	tests := []struct {
		path     string
		expected float64
	}{
		{"src/auth.go", 1.0},
		{"pkg/handler.py", 1.0},
		{"components/App.tsx", 1.0},
		{"src/main.rs", 1.0},
		{"src/auth_test.go", 0.7},
		{"tests/test_handler.py", 0.7},
		{"src/app.spec.ts", 0.7},
		{"config.yaml", 0.5},
		{"settings.toml", 0.5},
		{"package.json", 0.5},
		{"README.md", 0.4},
		{"docs/guide.rst", 0.4},
		{"go.sum", 0.1},
		{"package-lock.json", 0.1},
		{"yarn.lock", 0.1},
		{"api/service.pb.go", 0.1},
		{"dist/bundle.js", 0.1},
		{"build/output.js", 0.1},
		{"vendor/lib/util.go", 0.0},
		{"node_modules/pkg/index.js", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			score := pc.ScoreFile(tt.path)
			if score != tt.expected {
				t.Errorf("ScoreFile(%q) = %f, want %f", tt.path, score, tt.expected)
			}
		})
	}
}

func TestDetectGenerated(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"go.sum", true},
		{"package-lock.json", true},
		{"yarn.lock", true},
		{"pnpm-lock.yaml", true},
		{"api/service.pb.go", true},
		{"src/types.generated.ts", true},
		{"dist/app.js", true},
		{"build/main.go", true},
		{"src/main.go", false},
		{"handler.py", false},
		{"config.yaml", false},
		{"internal/auth.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := DetectGenerated(tt.path)
			if got != tt.expected {
				t.Errorf("DetectGenerated(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestEstimateDiffTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"a", 1},
		{"abcd", 1},
		{"abcdefgh", 2},
		{strings.Repeat("x", 100), 25},
		{strings.Repeat("x", 4000), 1000},
	}

	for _, tt := range tests {
		t.Run(tt.input[:prMin(len(tt.input), 10)], func(t *testing.T) {
			got := EstimateDiffTokens(tt.input)
			if got != tt.expected {
				t.Errorf("EstimateDiffTokens(%d chars) = %d, want %d", len(tt.input), got, tt.expected)
			}
		})
	}
}

func TestTruncateHunks(t *testing.T) {
	// Build a diff with 4 hunks
	hunks := []string{
		"@@ -1,5 +1,6 @@\n context\n+added line 1\n context",
		"@@ -10,5 +11,6 @@\n context\n+added line 2\n context",
		"@@ -20,5 +22,6 @@\n context\n+added line 3\n context",
		"@@ -30,5 +33,6 @@\n context\n+added line 4\n context",
	}
	diff := strings.Join(hunks, "\n")

	// Large budget should keep everything
	result := TruncateHunks(diff, 10000)
	if !strings.Contains(result, "added line 1") {
		t.Error("expected first hunk preserved with large budget")
	}
	if !strings.Contains(result, "added line 4") {
		t.Error("expected last hunk preserved with large budget")
	}

	// Small budget should trigger truncation
	result = TruncateHunks(diff, 30)
	if !strings.Contains(result, "added line 1") {
		t.Error("expected first hunk preserved after truncation")
	}
	if strings.Contains(result, "added line 2") && strings.Contains(result, "added line 3") {
		t.Error("expected middle hunks to be omitted")
	}
}

func TestCompressDiff(t *testing.T) {
	pc := NewPRCompressor(8000)

	// Build a realistic diff with multiple files
	diff := buildTestDiff(map[string]string{
		"src/auth.go":        generateDiffContent("src/auth.go", 50),
		"src/handler.go":     generateDiffContent("src/handler.go", 30),
		"src/auth_test.go":   generateDiffContent("src/auth_test.go", 20),
		"go.sum":             generateDiffContent("go.sum", 200),
		"package-lock.json":  generateDiffContent("package-lock.json", 100),
		"vendor/lib/util.go": generateDiffContent("vendor/lib/util.go", 40),
	})

	result := pc.CompressDiff(diff, 500)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Source files should be included before lock/generated files
	if len(result.Files) == 0 {
		t.Fatal("expected at least one file included")
	}

	// First file should be highest priority source file
	if result.Files[0].Priority < 0.7 {
		t.Errorf("expected first file to be high priority, got %f", result.Files[0].Priority)
	}

	// Vendor files should be excluded (priority 0.0)
	for _, f := range result.Files {
		if strings.Contains(f.Path, "vendor/") {
			t.Error("vendor files should not be included")
		}
	}

	// Total tokens should not exceed budget
	if result.TotalTokens > 500 {
		t.Errorf("total tokens %d exceeds budget 500", result.TotalTokens)
	}
}

func TestCompressDiffEmpty(t *testing.T) {
	pc := NewPRCompressor(8000)
	result := pc.CompressDiff("", 1000)
	if result == nil {
		t.Fatal("expected non-nil result for empty diff")
	}
	if len(result.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(result.Files))
	}
}

func TestFormatCompressedPR(t *testing.T) {
	pr := &CompressedPR{
		Files: []CompressedFile{
			{Path: "src/auth.go", Diff: "+new line\n", Tokens: 100, Priority: 1.0},
			{Path: "src/handler.go", Diff: "+handler\n", Tokens: 80, Priority: 1.0, Truncated: true},
		},
		TotalTokens:   180,
		OverflowFiles: []string{"go.sum", "package-lock.json", "yarn.lock"},
		Summary:       "2 files included (1 truncated), 3 excluded, 180/8000 tokens used",
	}

	output := FormatCompressedPR(pr)

	if !strings.Contains(output, "compressed to 180 tokens") {
		t.Errorf("expected token count in output, got: %s", output)
	}
	if !strings.Contains(output, "Included: 2 files") {
		t.Errorf("expected included count, got: %s", output)
	}
	if !strings.Contains(output, "Truncated: 1 files") {
		t.Errorf("expected truncated count, got: %s", output)
	}
	if !strings.Contains(output, "Excluded: 3 files") {
		t.Errorf("expected excluded count, got: %s", output)
	}
	if !strings.Contains(output, "+new line") {
		t.Errorf("expected diff content in output, got: %s", output)
	}
}

func TestFormatCompressedLargeTokens(t *testing.T) {
	pr := &CompressedPR{
		Files:       []CompressedFile{{Path: "a.go", Diff: "x", Tokens: 8000}},
		TotalTokens: 8000,
	}
	output := FormatCompressedPR(pr)
	if !strings.Contains(output, "8,000") {
		t.Errorf("expected formatted number with comma, got: %s", output)
	}
}

func TestParseDiffIntoFiles(t *testing.T) {
	diff := `diff --git a/src/main.go b/src/main.go
index abc123..def456 100644
--- a/src/main.go
+++ b/src/main.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 func main() {}
diff --git a/src/util.go b/src/util.go
index 111111..222222 100644
--- a/src/util.go
+++ b/src/util.go
@@ -1,2 +1,3 @@
 package main
+func helper() {}
`

	files := parseDiffIntoFiles(diff)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if _, ok := files["src/main.go"]; !ok {
		t.Error("expected src/main.go in parsed files")
	}
	if _, ok := files["src/util.go"]; !ok {
		t.Error("expected src/util.go in parsed files")
	}
}

func TestCompressDiffPrioritization(t *testing.T) {
	pc := NewPRCompressor(8000)

	// Create diff where source files should come before config
	diff := buildTestDiff(map[string]string{
		"config.yaml":     generateDiffContent("config.yaml", 20),
		"src/core.go":     generateDiffContent("src/core.go", 20),
		"README.md":       generateDiffContent("README.md", 20),
		"src/api_test.go": generateDiffContent("src/api_test.go", 20),
	})

	result := pc.CompressDiff(diff, 10000)

	if len(result.Files) < 4 {
		t.Fatalf("expected 4 files included, got %d", len(result.Files))
	}

	// Verify ordering: source > test > config > docs
	priorities := make([]float64, len(result.Files))
	for i, f := range result.Files {
		priorities[i] = f.Priority
	}
	for i := 1; i < len(priorities); i++ {
		if priorities[i] > priorities[i-1] {
			t.Errorf("files not sorted by priority: %v", priorities)
			break
		}
	}
}

func TestPRDetectLanguage(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.ts", "typescript"},
		{"script.js", "javascript"},
		{"lib.rs", "rust"},
		{"unknown.xyz", ""},
	}
	for _, tt := range tests {
		got := prDetectLanguage(tt.path)
		if got != tt.expected {
			t.Errorf("prDetectLanguage(%q) = %q, want %q", tt.path, got, tt.expected)
		}
	}
}

func TestPRIsTestFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"main_test.go", true},
		{"test_handler.py", true},
		{"app.spec.ts", true},
		{"component.test.js", true},
		{"__tests__/util.js", true},
		{"main.go", false},
		{"handler.py", false},
	}
	for _, tt := range tests {
		got := prIsTestFile(tt.path)
		if got != tt.expected {
			t.Errorf("prIsTestFile(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestPRFormatNumber(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{100, "100"},
		{1000, "1,000"},
		{8000, "8,000"},
		{12345, "12,345"},
		{1000000, "1,000,000"},
	}
	for _, tt := range tests {
		got := prFormatNumber(tt.input)
		if got != tt.expected {
			t.Errorf("prFormatNumber(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSummarizePaths(t *testing.T) {
	short := []string{"a.go", "b.go"}
	got := summarizePaths(short)
	if got != "a.go, b.go" {
		t.Errorf("expected 'a.go, b.go', got %q", got)
	}

	long := []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go"}
	got = summarizePaths(long)
	if !strings.Contains(got, "+2 more") {
		t.Errorf("expected truncation indicator, got %q", got)
	}
}

// --- test helpers ---

func buildTestDiff(files map[string]string) string {
	var sb strings.Builder
	for _, content := range files {
		sb.WriteString(content)
	}
	return sb.String()
}

func generateDiffContent(path string, lines int) string {
	var sb strings.Builder
	sb.WriteString("diff --git a/" + path + " b/" + path + "\n")
	sb.WriteString("index abc123..def456 100644\n")
	sb.WriteString("--- a/" + path + "\n")
	sb.WriteString("+++ b/" + path + "\n")
	sb.WriteString("@@ -1,5 +1,6 @@\n")
	for i := 0; i < lines; i++ {
		sb.WriteString("+added line " + strings.Repeat("x", 20) + "\n")
	}
	return sb.String()
}

func prMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
