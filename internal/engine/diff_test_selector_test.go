package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDiffFiles(t *testing.T) {
	diff := `diff --git a/pkg/auth/token.go b/pkg/auth/token.go
index abc123..def456 100644
--- a/pkg/auth/token.go
+++ b/pkg/auth/token.go
@@ -10,6 +10,8 @@ func ValidateToken(t string) bool {
+    // new validation logic
+    return checkExpiry(t)
diff --git a/pkg/handler/api.go b/pkg/handler/api.go
index 111222..333444 100644
--- a/pkg/handler/api.go
+++ b/pkg/handler/api.go
@@ -5,3 +5,5 @@ func HandleRequest(r *Request) {
+    log.Info("handling request")
`

	files := dtsParseDiffFiles(diff)
	if len(files) == 0 {
		t.Fatal("expected parsed files, got none")
	}

	expected := map[string]bool{
		"pkg/auth/token.go":  true,
		"pkg/handler/api.go": true,
	}
	for _, f := range files {
		if !expected[f] {
			t.Errorf("unexpected file in diff: %s", f)
		}
		delete(expected, f)
	}
	for f := range expected {
		t.Errorf("missing expected file: %s", f)
	}
}

func TestParseDiffFilesEmpty(t *testing.T) {
	files := dtsParseDiffFiles("")
	if len(files) != 0 {
		t.Errorf("expected 0 files from empty diff, got %d", len(files))
	}
}

func TestDetectLanguage(t *testing.T) {
	// Create temp dir with go.mod.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n"), 0o644)

	lang := dtsDetectLanguage(dir)
	if lang != "go" {
		t.Errorf("expected 'go', got %q", lang)
	}

	// Python project.
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir2, "setup.py"), []byte(""), 0o644)

	lang2 := dtsDetectLanguage(dir2)
	if lang2 != "python" {
		t.Errorf("expected 'python', got %q", lang2)
	}

	// JS project.
	dir3 := t.TempDir()
	os.WriteFile(filepath.Join(dir3, "package.json"), []byte("{}"), 0o644)

	lang3 := dtsDetectLanguage(dir3)
	if lang3 != "javascript" {
		t.Errorf("expected 'javascript', got %q", lang3)
	}

	// TS project.
	dir4 := t.TempDir()
	os.WriteFile(filepath.Join(dir4, "tsconfig.json"), []byte("{}"), 0o644)

	lang4 := dtsDetectLanguage(dir4)
	if lang4 != "typescript" {
		t.Errorf("expected 'typescript', got %q", lang4)
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		file     string
		lang     string
		expected bool
	}{
		{"foo_test.go", "go", true},
		{"foo.go", "go", false},
		{"test_auth.py", "python", true},
		{"auth.py", "python", false},
		{"auth.test.ts", "typescript", true},
		{"auth.spec.js", "javascript", true},
		{"auth.ts", "typescript", false},
	}

	for _, tc := range tests {
		got := isTestFile(tc.file, tc.lang)
		if got != tc.expected {
			t.Errorf("isTestFile(%q, %q) = %v, want %v", tc.file, tc.lang, got, tc.expected)
		}
	}
}

func TestNewTestSelector(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n"), 0o644)

	ts := NewTestSelector(dir)
	if ts.ProjectDir != dir {
		t.Errorf("expected ProjectDir %q, got %q", dir, ts.ProjectDir)
	}
	if ts.Language != "go" {
		t.Errorf("expected Language 'go', got %q", ts.Language)
	}
	if ts.DepGraph == nil {
		t.Error("expected non-nil DepGraph")
	}
}

func TestSelectFromDiffEmpty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n"), 0o644)

	ts := NewTestSelector(dir)
	selected, err := ts.SelectFromDiff("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected.Tests) != 0 {
		t.Errorf("expected 0 tests from empty diff, got %d", len(selected.Tests))
	}
}

func TestSelectFromFiles_Go(t *testing.T) {
	// Set up a project with source and test files.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/myapp\n"), 0o644)

	// Create pkg/auth directory with source and test files.
	authDir := filepath.Join(dir, "pkg", "auth")
	os.MkdirAll(authDir, 0o755)
	os.WriteFile(filepath.Join(authDir, "token.go"), []byte(`package auth

func ValidateToken(t string) bool {
	return t != ""
}
`), 0o644)
	os.WriteFile(filepath.Join(authDir, "token_test.go"), []byte(`package auth

import "testing"

func TestValidateToken(t *testing.T) {
	if !ValidateToken("abc") {
		t.Error("expected true")
	}
}
`), 0o644)

	ts := NewTestSelector(dir)
	selected, err := ts.SelectFromFiles([]string{"pkg/auth/token.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(selected.Tests) == 0 {
		t.Fatal("expected at least one test to be selected")
	}

	found := false
	for _, test := range selected.Tests {
		if strings.Contains(test, "token_test.go") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected token_test.go in selected tests, got: %v", selected.Tests)
	}

	if len(selected.Packages) == 0 {
		t.Error("expected at least one package")
	}
}

func TestSelectFromFiles_Python(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "setup.py"), []byte(""), 0o644)

	// Create source and test files.
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "auth.py"), []byte("def login(): pass\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "app", "test_auth.py"), []byte("def test_login(): pass\n"), 0o644)

	ts := NewTestSelector(dir)
	selected, err := ts.SelectFromFiles([]string{"app/auth.py"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, test := range selected.Tests {
		if strings.Contains(test, "test_auth.py") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected test_auth.py in selected tests, got: %v", selected.Tests)
	}
}

func TestSelectFromFiles_JS(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644)

	// Create source and test files.
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "auth.ts"), []byte("export function login() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "src", "auth.test.ts"), []byte("test('login', () => {})\n"), 0o644)

	ts := NewTestSelector(dir)
	selected, err := ts.SelectFromFiles([]string{"src/auth.ts"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, test := range selected.Tests {
		if strings.Contains(test, "auth.test.ts") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected auth.test.ts in selected tests, got: %v", selected.Tests)
	}
}

func TestBuildDependencyGraph(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/myapp\n"), 0o644)

	// Create packages with imports.
	authDir := filepath.Join(dir, "pkg", "auth")
	serverDir := filepath.Join(dir, "pkg", "server")
	os.MkdirAll(authDir, 0o755)
	os.MkdirAll(serverDir, 0o755)

	os.WriteFile(filepath.Join(authDir, "auth.go"), []byte(`package auth

func Check() bool { return true }
`), 0o644)

	os.WriteFile(filepath.Join(serverDir, "server.go"), []byte(`package server

import "example.com/myapp/pkg/auth"

func Start() {
	auth.Check()
}
`), 0o644)

	graph := BuildDependencyGraph(dir)

	// pkg/server depends on pkg/auth, so pkg/auth's dependents should include pkg/server.
	dependents, ok := graph["pkg/auth"]
	if !ok {
		t.Fatalf("expected pkg/auth in graph, got: %v", graph)
	}

	found := false
	for _, dep := range dependents {
		if dep == "pkg/server" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected pkg/server to depend on pkg/auth, graph[pkg/auth] = %v", dependents)
	}
}

func TestGenerateTestCommand_Go(t *testing.T) {
	selected := &SelectedTests{
		Tests:    []string{"pkg/auth/token_test.go", "pkg/handler/api_test.go"},
		Packages: []string{"pkg/auth", "pkg/handler"},
		Reason:   map[string]string{},
	}

	cmd := GenerateTestCommand(selected, "go")
	if cmd == "" {
		t.Fatal("expected non-empty command")
	}
	if !strings.Contains(cmd, "go test") {
		t.Errorf("expected 'go test' in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "./pkg/auth/...") {
		t.Errorf("expected './pkg/auth/...' in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "./pkg/handler/...") {
		t.Errorf("expected './pkg/handler/...' in command, got: %s", cmd)
	}
}

func TestGenerateTestCommand_Python(t *testing.T) {
	selected := &SelectedTests{
		Tests:    []string{"tests/test_auth.py", "tests/test_handler.py"},
		Packages: []string{"tests"},
		Reason:   map[string]string{},
	}

	cmd := GenerateTestCommand(selected, "python")
	if !strings.Contains(cmd, "pytest") {
		t.Errorf("expected 'pytest' in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "test_auth.py") {
		t.Errorf("expected 'test_auth.py' in command, got: %s", cmd)
	}
}

func TestGenerateTestCommand_JS(t *testing.T) {
	selected := &SelectedTests{
		Tests:    []string{"src/auth.test.ts", "src/handler.spec.ts"},
		Packages: []string{"src"},
		Reason:   map[string]string{},
	}

	cmd := GenerateTestCommand(selected, "typescript")
	if !strings.Contains(cmd, "jest") {
		t.Errorf("expected 'jest' in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "testPathPattern") {
		t.Errorf("expected 'testPathPattern' in command, got: %s", cmd)
	}
}

func TestGenerateTestCommand_Empty(t *testing.T) {
	selected := &SelectedTests{
		Tests:    []string{},
		Packages: []string{},
		Reason:   map[string]string{},
	}

	cmd := GenerateTestCommand(selected, "go")
	if cmd != "" {
		t.Errorf("expected empty command for no packages, got: %s", cmd)
	}
}

func TestFormatSelection(t *testing.T) {
	selected := &SelectedTests{
		Tests: []string{
			"pkg/auth/token_test.go",
			"pkg/auth/middleware_test.go",
			"pkg/handler/api_test.go",
		},
		Packages: []string{"pkg/auth", "pkg/handler"},
		Reason: map[string]string{
			"pkg/auth/token_test.go":      "direct: tests token.go",
			"pkg/auth/middleware_test.go": "direct: tests middleware.go",
			"pkg/handler/api_test.go":     "direct: tests api.go",
		},
	}

	changedFiles := []string{"pkg/auth/token.go", "pkg/auth/middleware.go", "pkg/handler/api.go"}
	output := FormatSelection(selected, changedFiles, "go", 200)

	if !strings.Contains(output, "Diff-Aware Test Selection:") {
		t.Error("expected header in output")
	}
	if !strings.Contains(output, "Changed: 3 files") {
		t.Errorf("expected 'Changed: 3 files' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Selected tests:") {
		t.Error("expected 'Selected tests:' in output")
	}
	if !strings.Contains(output, "pkg/auth/") {
		t.Error("expected 'pkg/auth/' in output")
	}
	if !strings.Contains(output, "go test") {
		t.Error("expected 'go test' in output")
	}
	if !strings.Contains(output, "Estimated: 3 tests (vs 200+ in full suite)") {
		t.Errorf("expected estimate in output, got:\n%s", output)
	}
}

func TestEstimateTimeSaved(t *testing.T) {
	result := EstimateTimeSaved(200, 12)
	if !strings.Contains(result, "saved") {
		t.Errorf("expected 'saved' in result, got: %s", result)
	}
	if !strings.Contains(result, "12/200") {
		t.Errorf("expected '12/200' in result, got: %s", result)
	}

	// Test edge cases.
	result2 := EstimateTimeSaved(0, 0)
	if !strings.Contains(result2, "No time estimate") {
		t.Errorf("expected 'No time estimate' for zero tests, got: %s", result2)
	}

	result3 := EstimateTimeSaved(10, 10)
	if !strings.Contains(result3, "No time saved") {
		t.Errorf("expected 'No time saved' when running full suite, got: %s", result3)
	}
}

func TestEstimateTimeSavedMinutes(t *testing.T) {
	// 500 total tests, 10 selected -> (500-10)*0.5 = 245s -> ~4.1m
	result := EstimateTimeSaved(500, 10)
	if !strings.Contains(result, "m saved") {
		t.Errorf("expected minutes format for large savings, got: %s", result)
	}
}

func TestDedup(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b", "d"}
	result := dtsDedup(input)
	expected := []string{"a", "b", "c", "d"}

	if len(result) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(result))
	}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("index %d: expected %q, got %q", i, expected[i], v)
		}
	}
}

func TestAppendUnique(t *testing.T) {
	slice := []string{"a", "b"}
	result := appendUnique(slice, "c")
	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}

	result2 := appendUnique(result, "b")
	if len(result2) != 3 {
		t.Errorf("expected 3 items (no dup), got %d", len(result2))
	}
}

func TestParseGoImports(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	content := `package main

import (
	"fmt"
	"os"

	"example.com/myapp/pkg/auth"
	"example.com/myapp/pkg/handler"
)

func main() {
	fmt.Println("hello")
}
`
	os.WriteFile(file, []byte(content), 0o644)

	imports := parseGoImports(file)
	expected := map[string]bool{
		"fmt":                           true,
		"os":                            true,
		"example.com/myapp/pkg/auth":    true,
		"example.com/myapp/pkg/handler": true,
	}

	for _, imp := range imports {
		if !expected[imp] {
			t.Errorf("unexpected import: %s", imp)
		}
		delete(expected, imp)
	}
	for imp := range expected {
		t.Errorf("missing expected import: %s", imp)
	}
}

func TestParseGoImportsSingleLine(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "single.go")
	content := `package main

import "fmt"

func main() {}
`
	os.WriteFile(file, []byte(content), 0o644)

	imports := parseGoImports(file)
	if len(imports) != 1 || imports[0] != "fmt" {
		t.Errorf("expected [fmt], got %v", imports)
	}
}

func TestSelectFromDiff_Integration(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/myapp\n"), 0o644)

	// Create source + test.
	authDir := filepath.Join(dir, "pkg", "auth")
	os.MkdirAll(authDir, 0o755)
	os.WriteFile(filepath.Join(authDir, "token.go"), []byte(`package auth
func Validate() bool { return true }
`), 0o644)
	os.WriteFile(filepath.Join(authDir, "token_test.go"), []byte(`package auth
import "testing"
func TestValidate(t *testing.T) {}
`), 0o644)

	diff := `diff --git a/pkg/auth/token.go b/pkg/auth/token.go
index abc..def 100644
--- a/pkg/auth/token.go
+++ b/pkg/auth/token.go
@@ -1,2 +1,3 @@
 package auth
+// added comment
 func Validate() bool { return true }
`

	ts := NewTestSelector(dir)
	selected, err := ts.SelectFromDiff(diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(selected.Tests) == 0 {
		t.Fatal("expected tests to be selected from diff")
	}

	found := false
	for _, test := range selected.Tests {
		if strings.Contains(test, "token_test.go") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected token_test.go in results, got: %v", selected.Tests)
	}
}

func TestFindRelatedTests_Go(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n"), 0o644)

	pkgDir := filepath.Join(dir, "pkg", "core")
	os.MkdirAll(pkgDir, 0o755)
	os.WriteFile(filepath.Join(pkgDir, "core.go"), []byte("package core\n"), 0o644)
	os.WriteFile(filepath.Join(pkgDir, "core_test.go"), []byte("package core\n"), 0o644)
	os.WriteFile(filepath.Join(pkgDir, "helper_test.go"), []byte("package core\n"), 0o644)

	ts := NewTestSelector(dir)
	related := ts.FindRelatedTests("pkg/core/core.go")

	if len(related) == 0 {
		t.Fatal("expected related tests")
	}

	hasCore := false
	hasHelper := false
	for _, r := range related {
		if strings.Contains(r, "core_test.go") {
			hasCore = true
		}
		if strings.Contains(r, "helper_test.go") {
			hasHelper = true
		}
	}
	if !hasCore {
		t.Error("expected core_test.go in related tests")
	}
	if !hasHelper {
		t.Error("expected helper_test.go in related tests (same package)")
	}
}

func TestDetectModulePath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/example/myproject\n\ngo 1.21\n"), 0o644)

	mod := detectModulePath(dir)
	if mod != "github.com/example/myproject" {
		t.Errorf("expected 'github.com/example/myproject', got %q", mod)
	}
}

func TestDetectModulePathMissing(t *testing.T) {
	dir := t.TempDir()
	mod := detectModulePath(dir)
	if mod != "" {
		t.Errorf("expected empty string for missing go.mod, got %q", mod)
	}
}

func TestSelectedTestsCoverageField(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n"), 0o644)

	// Create multiple test files.
	pkgDir := filepath.Join(dir, "pkg", "a")
	os.MkdirAll(pkgDir, 0o755)
	os.WriteFile(filepath.Join(pkgDir, "a.go"), []byte("package a\n"), 0o644)
	os.WriteFile(filepath.Join(pkgDir, "a_test.go"), []byte("package a\n"), 0o644)

	pkg2Dir := filepath.Join(dir, "pkg", "b")
	os.MkdirAll(pkg2Dir, 0o755)
	os.WriteFile(filepath.Join(pkg2Dir, "b.go"), []byte("package b\n"), 0o644)
	os.WriteFile(filepath.Join(pkg2Dir, "b_test.go"), []byte("package b\n"), 0o644)

	ts := NewTestSelector(dir)
	selected, err := ts.SelectFromFiles([]string{"pkg/a/a.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Coverage should be less than 1.0 since we only selected one package's tests.
	if selected.Coverage >= 1.0 {
		t.Errorf("expected coverage < 1.0, got %f", selected.Coverage)
	}
	if selected.Coverage <= 0 {
		t.Errorf("expected coverage > 0, got %f", selected.Coverage)
	}
}
