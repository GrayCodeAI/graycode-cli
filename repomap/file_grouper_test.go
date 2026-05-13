package repomap

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupGrouperProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create a Go project structure
	files := map[string]string{
		"go.mod": "module example.com/testproject\n\ngo 1.21\n",
		"main.go": `package main

import "example.com/testproject/auth"

func main() {
	auth.Init()
}
`,
		"auth/auth.go": `package auth

import "fmt"

func Init() {
	fmt.Println("auth initialized")
}

func ValidateToken(token string) bool {
	return token != ""
}
`,
		"auth/auth_test.go": `package auth

import "testing"

func TestValidateToken(t *testing.T) {
	if !ValidateToken("abc") {
		t.Fatal("expected true")
	}
}
`,
		"auth/middleware.go": `package auth

func Middleware() {
	// auth middleware
}
`,
		"auth/claims.go": `package auth

type Claims struct {
	UserID string
	Role   string
}
`,
		"config/config.go": `package config

type Config struct {
	Port int
	Host string
}
`,
		"config/config_test.go": `package config

import "testing"

func TestConfig(t *testing.T) {}
`,
		"handler/handler.go": `package handler

func Handle() {}
`,
		"handler/handler_test.go": `package handler

import "testing"

func TestHandle(t *testing.T) {}
`,
		".env.example":    "PORT=8080\nHOST=localhost\n",
		".env":            "PORT=8080\nHOST=localhost\n",
		"Makefile":        "build:\n\tgo build .\n",
		"Dockerfile":      "FROM golang:1.21\n",
		".gitignore":      "*.exe\n",
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func TestNewFileGrouper(t *testing.T) {
	dir := setupGrouperProject(t)
	fg := NewFileGrouper(dir)

	if fg == nil {
		t.Fatal("expected non-nil FileGrouper")
	}
	if fg.ProjectDir != dir {
		t.Fatalf("expected ProjectDir=%s, got %s", dir, fg.ProjectDir)
	}
}

func TestAnalyzeGroups(t *testing.T) {
	dir := setupGrouperProject(t)
	fg := NewFileGrouper(dir)

	groups, err := fg.AnalyzeGroups()
	if err != nil {
		t.Fatalf("AnalyzeGroups error: %v", err)
	}

	if len(groups) == 0 {
		t.Fatal("expected at least one group")
	}

	// Should have package groups
	var hasPackageGroup bool
	for _, g := range groups {
		if g.Type == "package" {
			hasPackageGroup = true
			break
		}
	}
	if !hasPackageGroup {
		t.Error("expected at least one package group")
	}

	// Should have test pair groups
	var hasTestPair bool
	for _, g := range groups {
		if g.Type == "test_pair" {
			hasTestPair = true
			break
		}
	}
	if !hasTestPair {
		t.Error("expected at least one test_pair group")
	}

	// Should have config groups
	var hasConfig bool
	for _, g := range groups {
		if g.Type == "config" {
			hasConfig = true
			break
		}
	}
	if !hasConfig {
		t.Error("expected at least one config group")
	}
}

func TestAnalyzeGroupsPackage(t *testing.T) {
	dir := setupGrouperProject(t)
	fg := NewFileGrouper(dir)

	groups, _ := fg.AnalyzeGroups()

	// auth/ should be a package group with multiple files
	var authGroup *FileGroup
	for i, g := range groups {
		if g.Type == "package" && strings.Contains(g.Name, "auth") {
			authGroup = &groups[i]
			break
		}
	}
	if authGroup == nil {
		t.Fatal("expected auth package group")
	}
	if len(authGroup.Files) < 3 {
		t.Errorf("expected auth group to have at least 3 files, got %d", len(authGroup.Files))
	}
	if authGroup.Confidence != 1.0 {
		t.Errorf("expected package confidence=1.0, got %.2f", authGroup.Confidence)
	}
}

func TestFindTestPairGo(t *testing.T) {
	dir := setupGrouperProject(t)
	fg := NewFileGrouper(dir)

	// Source -> test
	pair := fg.FindTestPair("auth/auth.go")
	if pair != "auth/auth_test.go" {
		t.Errorf("expected auth/auth_test.go, got %q", pair)
	}

	// Test -> source
	pair = fg.FindTestPair("auth/auth_test.go")
	if pair != "auth/auth.go" {
		t.Errorf("expected auth/auth.go, got %q", pair)
	}

	// No pair exists
	pair = fg.FindTestPair("auth/middleware.go")
	if pair != "" {
		t.Errorf("expected empty pair for middleware.go, got %q", pair)
	}
}

func TestFindTestPairPython(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "handler.py"), []byte("def handle(): pass\n"), 0644)
	os.WriteFile(filepath.Join(dir, "test_handler.py"), []byte("def test_handle(): pass\n"), 0644)

	fg := NewFileGrouper(dir)

	pair := fg.FindTestPair("handler.py")
	if pair != "test_handler.py" {
		t.Errorf("expected test_handler.py, got %q", pair)
	}

	pair = fg.FindTestPair("test_handler.py")
	if pair != "handler.py" {
		t.Errorf("expected handler.py, got %q", pair)
	}
}

func TestFindTestPairTypeScript(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "component.tsx"), []byte("export const Comp = () => {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "component.test.tsx"), []byte("test('comp', () => {})\n"), 0644)

	fg := NewFileGrouper(dir)

	pair := fg.FindTestPair("component.tsx")
	if pair != "component.test.tsx" {
		t.Errorf("expected component.test.tsx, got %q", pair)
	}

	pair = fg.FindTestPair("component.test.tsx")
	if pair != "component.tsx" {
		t.Errorf("expected component.tsx, got %q", pair)
	}
}

func TestFindByFeature(t *testing.T) {
	dir := setupGrouperProject(t)
	fg := NewFileGrouper(dir)
	fg.AnalyzeGroups() // populate internal state

	files := fg.FindByFeature("auth")
	if len(files) == 0 {
		t.Fatal("expected files related to 'auth' feature")
	}

	// Should include files from the auth directory
	var hasAuth bool
	for _, f := range files {
		if strings.Contains(f, "auth") {
			hasAuth = true
			break
		}
	}
	if !hasAuth {
		t.Errorf("expected auth-related files, got %v", files)
	}
}

func TestFindByFeatureConfig(t *testing.T) {
	dir := setupGrouperProject(t)
	fg := NewFileGrouper(dir)
	fg.AnalyzeGroups()

	files := fg.FindByFeature("config")
	if len(files) == 0 {
		t.Fatal("expected files related to 'config' feature")
	}

	var hasConfigGo bool
	for _, f := range files {
		if strings.Contains(f, "config.go") {
			hasConfigGo = true
		}
	}
	if !hasConfigGo {
		t.Error("expected config.go in config feature files")
	}
}

func TestFindRelated(t *testing.T) {
	dir := setupGrouperProject(t)
	fg := NewFileGrouper(dir)
	fg.AnalyzeGroups()

	related := fg.FindRelated("auth/auth.go")
	if len(related) == 0 {
		t.Fatal("expected related files for auth/auth.go")
	}

	// Should include the test file
	var hasTest bool
	for _, f := range related {
		if strings.Contains(f, "auth_test.go") {
			hasTest = true
		}
	}
	if !hasTest {
		t.Error("expected auth_test.go in related files")
	}
}

func TestSuggestEditGroup(t *testing.T) {
	dir := setupGrouperProject(t)
	fg := NewFileGrouper(dir)
	fg.AnalyzeGroups()

	suggestions := fg.SuggestEditGroup("auth/auth.go")
	if len(suggestions) == 0 {
		t.Fatal("expected edit group suggestions for auth/auth.go")
	}

	// Should include test pair
	var hasTest bool
	for _, f := range suggestions {
		if strings.Contains(f, "auth_test.go") {
			hasTest = true
		}
	}
	if !hasTest {
		t.Error("expected auth_test.go in edit group suggestions")
	}
}

func TestFormatGroups(t *testing.T) {
	groups := []FileGroup{
		{
			Name:       "auth/",
			Files:      []string{"auth/token.go", "auth/middleware.go", "auth/claims.go", "auth/auth_test.go"},
			Reason:     "Same Go package",
			Confidence: 1.0,
			Type:       "package",
		},
		{
			Name:       "config",
			Files:      []string{"config/config.go", "config/config_test.go", ".env.example"},
			Reason:     "Feature group",
			Confidence: 0.85,
			Type:       "feature",
		},
		{
			Name:       "handler",
			Files:      []string{"handler/handler.go", "handler/handler_test.go"},
			Reason:     "Test pair",
			Confidence: 1.0,
			Type:       "test_pair",
		},
	}

	output := FormatGroups(groups)

	if !strings.Contains(output, "File Groups (3)") {
		t.Errorf("expected 'File Groups (3)' header, got:\n%s", output)
	}
	if !strings.Contains(output, "[package] auth/") {
		t.Errorf("expected '[package] auth/' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "[feature] config") {
		t.Errorf("expected '[feature] config' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "[test_pair] handler") {
		t.Errorf("expected '[test_pair] handler' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "confidence: 1.00") {
		t.Errorf("expected 'confidence: 1.00' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "confidence: 0.85") {
		t.Errorf("expected 'confidence: 0.85' in output, got:\n%s", output)
	}
}

func TestFormatGroupsEmpty(t *testing.T) {
	output := FormatGroups(nil)
	if output != "No file groups found." {
		t.Errorf("expected empty message, got: %s", output)
	}
}

func TestFormatGroupsManyFiles(t *testing.T) {
	groups := []FileGroup{
		{
			Name:       "large",
			Files:      []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go", "h.go"},
			Reason:     "Large group",
			Confidence: 0.9,
			Type:       "package",
		},
	}
	output := FormatGroups(groups)
	if !strings.Contains(output, "+3 more") {
		t.Errorf("expected truncation indicator, got:\n%s", output)
	}
}

func TestCoChangeAnalysisMap(t *testing.T) {
	// Create a git repo with some co-change history
	dir := t.TempDir()

	// Initialize git repo
	runGrouperGit(t, dir, "init")
	runGrouperGit(t, dir, "config", "user.email", "test@test.com")
	runGrouperGit(t, dir, "config", "user.name", "Test")

	// Create and commit files together multiple times
	writeGrouperFile(t, dir, "auth.go", "package main\nfunc Auth() {}\n")
	writeGrouperFile(t, dir, "auth_test.go", "package main\nfunc TestAuth() {}\n")
	runGrouperGit(t, dir, "add", ".")
	runGrouperGit(t, dir, "commit", "-m", "add auth")

	writeGrouperFile(t, dir, "auth.go", "package main\nfunc Auth() { /* v2 */ }\n")
	writeGrouperFile(t, dir, "auth_test.go", "package main\nfunc TestAuth() { /* v2 */ }\n")
	runGrouperGit(t, dir, "add", ".")
	runGrouperGit(t, dir, "commit", "-m", "update auth")

	writeGrouperFile(t, dir, "auth.go", "package main\nfunc Auth() { /* v3 */ }\n")
	writeGrouperFile(t, dir, "auth_test.go", "package main\nfunc TestAuth() { /* v3 */ }\n")
	runGrouperGit(t, dir, "add", ".")
	runGrouperGit(t, dir, "commit", "-m", "fix auth")

	result := CoChangeAnalysisMap(dir)

	// auth.go and auth_test.go should co-change (3 times >= threshold of 2)
	related, ok := result["auth.go"]
	if !ok {
		t.Fatal("expected auth.go in co-change map")
	}
	var hasTestFile bool
	for _, r := range related {
		if r == "auth_test.go" {
			hasTestFile = true
		}
	}
	if !hasTestFile {
		t.Errorf("expected auth_test.go in co-change for auth.go, got %v", related)
	}
}

func TestCoChangeAnalysisMapNoGit(t *testing.T) {
	dir := t.TempDir()
	result := CoChangeAnalysisMap(dir)
	if len(result) != 0 {
		t.Errorf("expected empty map for non-git dir, got %v", result)
	}
}

func TestFileGroupConcurrency(t *testing.T) {
	dir := setupGrouperProject(t)
	fg := NewFileGrouper(dir)
	fg.AnalyzeGroups()

	// Run concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = fg.FindRelated("auth/auth.go")
			_ = fg.FindByFeature("auth")
			_ = fg.SuggestEditGroup("auth/auth.go")
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestExtractFeatureRoot(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"auth", "auth"},
		{"auth_test", "auth"},
		{"auth_middleware", "auth"},
		{"test_handler", "handler"},
		{"config", "config"},
	}
	for _, tc := range tests {
		got := extractFeatureRoot(tc.input)
		if got != tc.expected {
			t.Errorf("extractFeatureRoot(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestFileBaseName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"auth.go", "auth"},
		{"auth_test.go", "auth_test"},
		{"component.test.tsx", "component"},
		{"handler.spec.js", "handler"},
		{"path/to/file.go", "file"},
	}
	for _, tc := range tests {
		got := fileBaseName(tc.input)
		if got != tc.expected {
			t.Errorf("fileBaseName(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestConfigGrouping(t *testing.T) {
	dir := setupGrouperProject(t)
	fg := NewFileGrouper(dir)

	groups, _ := fg.AnalyzeGroups()

	var envGroup *FileGroup
	var buildGroup *FileGroup
	for i, g := range groups {
		if g.Type == "config" && g.Name == "environment" {
			envGroup = &groups[i]
		}
		if g.Type == "config" && g.Name == "build" {
			buildGroup = &groups[i]
		}
	}

	if envGroup == nil {
		t.Error("expected environment config group")
	} else if len(envGroup.Files) < 2 {
		t.Errorf("expected at least 2 env files, got %d", len(envGroup.Files))
	}

	if buildGroup == nil {
		t.Error("expected build config group")
	} else if len(buildGroup.Files) < 2 {
		t.Errorf("expected at least 2 build files, got %d", len(buildGroup.Files))
	}
}

// ── Test helpers ──

func runGrouperGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeGrouperFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
