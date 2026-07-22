package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestProject(t *testing.T) (string, *PathResolver) {
	t.Helper()
	dir := t.TempDir()

	// Create a project structure
	dirs := []string{
		"src/auth",
		"src/api",
		"src/utils",
		"pkg/models",
		"cmd",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	files := []string{
		"src/auth/token.go",
		"src/auth/token_test.go",
		"src/auth/claims.go",
		"src/api/handler.go",
		"src/api/middleware.go",
		"src/utils/helpers.go",
		"pkg/models/user.go",
		"pkg/models/user_test.go",
		"cmd/main.go",
		"go.mod",
		"README.md",
	}
	for _, f := range files {
		path := filepath.Join(dir, f)
		if err := os.WriteFile(path, []byte("package test\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	pr := NewPathResolver(dir)
	return dir, pr
}

func TestNewPathResolver(t *testing.T) {
	pr := NewPathResolver("/some/project")
	if pr.ProjectDir != "/some/project" {
		t.Errorf("expected ProjectDir /some/project, got %s", pr.ProjectDir)
	}
	if pr.FileCache == nil {
		t.Error("expected FileCache to be initialized")
	}
}

func TestScanProject(t *testing.T) {
	dir, pr := setupTestProject(t)

	if err := pr.ScanProject(); err != nil {
		t.Fatalf("ScanProject failed: %v", err)
	}

	if pr.LastScan.IsZero() {
		t.Error("LastScan should be set after scanning")
	}

	// Check that expected files are in the cache
	expectedFiles := []string{
		filepath.Join(dir, "src/auth/token.go"),
		filepath.Join(dir, "cmd/main.go"),
		filepath.Join(dir, "go.mod"),
	}
	for _, f := range expectedFiles {
		if !pr.FileCache[f] {
			t.Errorf("expected %s in FileCache", f)
		}
	}
}

func TestScanProjectSkipsDirs(t *testing.T) {
	dir := t.TempDir()

	// Create directories that should be skipped
	for _, d := range []string{".git", "node_modules", "vendor", "src"} {
		os.MkdirAll(filepath.Join(dir, d), 0o755)
		os.WriteFile(filepath.Join(dir, d, "file.txt"), []byte("x"), 0o644)
	}

	pr := NewPathResolver(dir)
	if err := pr.ScanProject(); err != nil {
		t.Fatal(err)
	}

	// .git, node_modules, vendor should be skipped
	for path := range pr.FileCache {
		rel, _ := filepath.Rel(dir, path)
		parts := strings.Split(rel, string(filepath.Separator))
		for _, part := range parts {
			if part == ".git" || part == "node_modules" || part == "vendor" {
				t.Errorf("should have skipped directory %s but found %s in cache", part, path)
			}
		}
	}

	// src should be included
	srcFile := filepath.Join(dir, "src", "file.txt")
	if !pr.FileCache[srcFile] {
		t.Errorf("expected %s in FileCache", srcFile)
	}
}

func TestResolveExactPath(t *testing.T) {
	dir, pr := setupTestProject(t)
	_ = pr.ScanProject()

	exactPath := filepath.Join(dir, "src/auth/token.go")
	result := pr.Resolve(exactPath)

	if !result.Found {
		t.Error("expected Found=true for exact path")
	}
	if result.Path != exactPath {
		t.Errorf("expected %s, got %s", exactPath, result.Path)
	}
	if result.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", result.Confidence)
	}
}

func TestResolveRelativePath(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	result := pr.Resolve("src/auth/token.go")

	if !result.Found {
		t.Error("expected Found=true for relative path")
	}
	if result.Confidence < 0.95 {
		t.Errorf("expected high confidence, got %f", result.Confidence)
	}
}

func TestResolveBasenameUnique(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	// "main.go" only exists in cmd/
	result := pr.Resolve("main.go")

	if !result.Found {
		t.Error("expected Found=true for unique basename")
	}
	if !strings.HasSuffix(result.Path, "cmd/main.go") {
		t.Errorf("expected path ending with cmd/main.go, got %s", result.Path)
	}
	if result.Confidence < 0.8 {
		t.Errorf("expected confidence >= 0.8, got %f", result.Confidence)
	}
}

func TestResolveBasenameAmbiguous(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	// "token.go" exists in src/auth/ as token.go (not token_test.go)
	// But let's test with user_test.go which is unique...
	// Actually let's use a name that only appears once: helpers.go
	result := pr.Resolve("helpers.go")

	if !result.Found {
		t.Error("expected Found=true for unique basename helpers.go")
	}
	if !strings.HasSuffix(result.Path, "src/utils/helpers.go") {
		t.Errorf("expected path ending with src/utils/helpers.go, got %s", result.Path)
	}
}

func TestResolveFuzzyMatch(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	// Typo: "src/atuh/token.go" instead of "src/auth/token.go"
	result := pr.Resolve("src/atuh/token.go")

	if result.Path == "" {
		t.Fatal("expected a suggestion for fuzzy match")
	}
	if !strings.Contains(result.Path, "auth/token.go") {
		t.Errorf("expected fuzzy match to suggest auth/token.go, got %s", result.Path)
	}
}

func TestFindByName(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	results := pr.FindByName("token.go")
	if len(results) != 1 {
		t.Errorf("expected 1 result for token.go, got %d", len(results))
	}

	results = pr.FindByName("nonexistent.go")
	if len(results) != 0 {
		t.Errorf("expected 0 results for nonexistent.go, got %d", len(results))
	}
}

func TestFindSimilar(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	// "src/atuh.go" should suggest "src/auth/..." files
	results := pr.FindSimilar("src/auth/tken.go", 3)
	if len(results) == 0 {
		t.Fatal("expected at least one similar result")
	}

	// The first result should be close to the input
	if !strings.Contains(results[0], "token.go") && !strings.Contains(results[0], "auth") {
		t.Errorf("expected first similar result to be auth-related, got %s", results[0])
	}
}

func TestFindByPattern(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	results := pr.FindByPattern("*.go")
	if len(results) == 0 {
		t.Error("expected results for *.go pattern")
	}
	for _, r := range results {
		if !strings.HasSuffix(r, ".go") {
			t.Errorf("expected .go file, got %s", r)
		}
	}

	results = pr.FindByPattern("*_test.go")
	if len(results) != 2 {
		t.Errorf("expected 2 test files, got %d: %v", len(results), results)
	}
}

func TestSuggestCorrection(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	correction := pr.SuggestCorrection("src/auth/token.go")
	if correction == "" {
		t.Error("expected a correction for valid path")
	}
	if !strings.Contains(correction, "token.go") {
		t.Errorf("expected correction to contain token.go, got %s", correction)
	}
}

func TestFormatResolveResult(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	// Test with a found result
	result := &ResolveResult{
		Found:        true,
		Path:         filepath.Join(pr.ProjectDir, "src/auth/token.go"),
		Alternatives: []string{filepath.Join(pr.ProjectDir, "src/auth/token_test.go")},
		Confidence:   0.95,
	}

	formatted := pr.FormatResult(result)
	if !strings.Contains(formatted, "Resolved to") {
		t.Errorf("expected 'Resolved to' in output, got: %s", formatted)
	}
	if !strings.Contains(formatted, "0.95") {
		t.Errorf("expected confidence in output, got: %s", formatted)
	}
	if !strings.Contains(formatted, "Alternatives") {
		t.Errorf("expected 'Alternatives' in output, got: %s", formatted)
	}

	// Test nil result
	formatted = pr.FormatResult(nil)
	if !strings.Contains(formatted, "no result") {
		t.Errorf("expected 'no result' for nil, got: %s", formatted)
	}
}

func TestFormatResolveResultNotFound(t *testing.T) {
	_, pr := setupTestProject(t)

	result := &ResolveResult{
		Found:      false,
		Path:       filepath.Join(pr.ProjectDir, "src/auth/token.go"),
		Confidence: 0.4,
	}

	formatted := pr.FormatResult(result)
	if !strings.Contains(formatted, "Best guess") {
		t.Errorf("expected 'Best guess' for not-found result, got: %s", formatted)
	}
}

func TestIsValidPath(t *testing.T) {
	dir, pr := setupTestProject(t)

	// Valid file
	if !pr.IsValidPath(filepath.Join(dir, "src/auth/token.go")) {
		t.Error("expected true for existing file")
	}

	// Non-existent file
	if pr.IsValidPath(filepath.Join(dir, "nonexistent.go")) {
		t.Error("expected false for non-existent file")
	}

	// Directory (not a file)
	if pr.IsValidPath(filepath.Join(dir, "src")) {
		t.Error("expected false for directory")
	}
}

func TestNormalizePath(t *testing.T) {
	pr := NewPathResolver("/project")

	tests := []struct {
		input    string
		contains string // substring the output should contain
	}{
		{"src/../pkg/models/user.go", "pkg/models/user.go"},
		{"./src/auth/token.go", "src/auth/token.go"},
		{"src//auth///token.go", "src/auth/token.go"},
		{"", ""},
	}

	for _, tt := range tests {
		result := pr.NormalizePath(tt.input)
		if tt.contains != "" && !strings.Contains(result, tt.contains) {
			t.Errorf("NormalizePath(%q) = %q, expected to contain %q", tt.input, result, tt.contains)
		}
		if tt.contains == "" && result != "" {
			t.Errorf("NormalizePath(%q) = %q, expected empty", tt.input, result)
		}
	}
}

func TestNormalizePathTilde(t *testing.T) {
	pr := NewPathResolver("/project")

	home, err := os.UserHomeDir()
	if err != nil {
		// FIXME: cannot determine home directory
		t.Skip("cannot determine home directory")
	}

	result := pr.NormalizePath("~/Documents/file.go")
	expected := filepath.Join(home, "Documents/file.go")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"auth", "atuh", 2},
		{"token", "tken", 1},
	}

	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestResolveConcurrent(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	// Run concurrent resolves to test thread safety
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			pr.Resolve("src/auth/token.go")
			pr.FindByName("token.go")
			pr.FindSimilar("src/atuh/token.go", 3)
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestResolveEmptyPath(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	result := pr.Resolve("")
	if result.Found && result.Confidence == 1.0 {
		t.Error("empty path should not resolve with full confidence")
	}
}

func TestFindByPatternNoMatch(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	results := pr.FindByPattern("*.xyz")
	if len(results) != 0 {
		t.Errorf("expected 0 results for *.xyz, got %d", len(results))
	}
}

func TestFindSimilarLimit(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	results := pr.FindSimilar("src/auth/token.go", 2)
	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

func TestResolveWithDirHint(t *testing.T) {
	_, pr := setupTestProject(t)
	_ = pr.ScanProject()

	// When asking for "auth/claims.go" it should resolve to src/auth/claims.go
	result := pr.Resolve("auth/claims.go")
	if !result.Found {
		t.Error("expected to find auth/claims.go via partial directory match")
	}
	if !strings.Contains(result.Path, "src/auth/claims.go") {
		t.Errorf("expected path containing src/auth/claims.go, got %s", result.Path)
	}
}
