package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewSummaryGenerator(t *testing.T) {
	sg := NewSummaryGenerator("/tmp/test", 1024)
	if sg == nil {
		t.Fatal("expected non-nil SummaryGenerator")
	}
	if sg.ProjectDir != "/tmp/test" {
		t.Errorf("expected ProjectDir /tmp/test, got %s", sg.ProjectDir)
	}
	if sg.MaxTokens != 1024 {
		t.Errorf("expected MaxTokens 1024, got %d", sg.MaxTokens)
	}
}

func TestNewSummaryGeneratorDefaultTokens(t *testing.T) {
	sg := NewSummaryGenerator("/tmp/test", 0)
	if sg.MaxTokens != 2048 {
		t.Errorf("expected default MaxTokens 2048, got %d", sg.MaxTokens)
	}
}

func TestInferArchitectureFlat(t *testing.T) {
	packages := []SummaryPackageInfo{
		{Name: "main", Path: ".", LOC: 100},
		{Name: "utils", Path: "utils", LOC: 50},
	}
	arch := InferArchitecture(packages)
	if arch != "flat" {
		t.Errorf("expected 'flat', got %q", arch)
	}
}

func TestInferArchitectureLayered(t *testing.T) {
	packages := []SummaryPackageInfo{
		{Name: "cmd", Path: "cmd", LOC: 200},
		{Name: "service", Path: "service", LOC: 500},
		{Name: "store", Path: "store", LOC: 300},
		{Name: "model", Path: "model", LOC: 100},
		{Name: "config", Path: "config", LOC: 80},
		{Name: "util", Path: "util", LOC: 50},
	}
	arch := InferArchitecture(packages)
	if arch != "layered" {
		t.Errorf("expected 'layered', got %q", arch)
	}
}

func TestInferArchitectureMonorepo(t *testing.T) {
	packages := []SummaryPackageInfo{
		{Name: "web", Path: "packages/web", LOC: 1000},
		{Name: "api", Path: "packages/api", LOC: 800},
		{Name: "shared", Path: "packages/shared", LOC: 500},
		{Name: "cli", Path: "packages/cli", LOC: 300},
	}
	arch := InferArchitecture(packages)
	if arch != "monorepo" {
		t.Errorf("expected 'monorepo', got %q", arch)
	}
}

func TestInferArchitectureMicroservices(t *testing.T) {
	packages := []SummaryPackageInfo{
		{Name: "auth", Path: "cmd/auth", LOC: 300},
		{Name: "gateway", Path: "cmd/gateway", LOC: 400},
		{Name: "users", Path: "cmd/users", LOC: 350},
		{Name: "orders", Path: "cmd/orders", LOC: 500},
		{Name: "shared", Path: "shared", LOC: 200},
		{Name: "pkg", Path: "pkg", LOC: 150},
	}
	arch := InferArchitecture(packages)
	if arch != "microservices" {
		t.Errorf("expected 'microservices', got %q", arch)
	}
}

func TestInferArchitectureHexagonal(t *testing.T) {
	packages := []SummaryPackageInfo{
		{Name: "domain", Path: "domain", LOC: 500},
		{Name: "ports", Path: "ports", LOC: 200},
		{Name: "adapters", Path: "adapters/http", LOC: 300},
		{Name: "infra", Path: "infra", LOC: 400},
		{Name: "app", Path: "app", LOC: 250},
		{Name: "cmd", Path: "cmd", LOC: 100},
	}
	arch := InferArchitecture(packages)
	if arch != "hexagonal" {
		t.Errorf("expected 'hexagonal', got %q", arch)
	}
}

func TestInferArchitectureEmpty(t *testing.T) {
	arch := InferArchitecture(nil)
	if arch != "flat" {
		t.Errorf("expected 'flat' for empty packages, got %q", arch)
	}
}

func TestInferPurposeAuth(t *testing.T) {
	purpose := InferPurpose("pkg/auth", []string{"ValidateToken", "NewAuthService", "Login"})
	if !strings.Contains(purpose, "Authentication") {
		t.Errorf("expected purpose about authentication, got %q", purpose)
	}
}

func TestInferPurposeHandler(t *testing.T) {
	purpose := InferPurpose("internal/handler", []string{"ServeHTTP", "HandleRequest"})
	if !strings.Contains(purpose, "HTTP") {
		t.Errorf("expected purpose about HTTP handling, got %q", purpose)
	}
}

func TestInferPurposeConfig(t *testing.T) {
	purpose := InferPurpose("config", []string{"Load", "Validate", "Settings"})
	if !strings.Contains(purpose, "Configuration") {
		t.Errorf("expected purpose about configuration, got %q", purpose)
	}
}

func TestInferPurposeEngine(t *testing.T) {
	purpose := InferPurpose("engine", []string{"Run", "Execute", "Start"})
	if !strings.Contains(purpose, "engine") && !strings.Contains(purpose, "execution") {
		t.Errorf("expected purpose about engine/execution, got %q", purpose)
	}
}

func TestInferPurposeFallback(t *testing.T) {
	purpose := InferPurpose("foobar", []string{"DoStuff"})
	if purpose == "" {
		t.Error("expected non-empty fallback purpose")
	}
	if !strings.Contains(purpose, "Foobar") {
		t.Errorf("expected fallback to include formatted name, got %q", purpose)
	}
}

func TestFindEntryPointsGo(t *testing.T) {
	dir := t.TempDir()
	// Create a Go main file
	mainContent := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	cmdDir := filepath.Join(dir, "cmd")
	os.MkdirAll(cmdDir, 0o755)
	os.WriteFile(filepath.Join(cmdDir, "main.go"), []byte(mainContent), 0o644)

	// Create a non-main Go file
	libContent := `package lib

func Helper() string {
	return "help"
}
`
	libDir := filepath.Join(dir, "lib")
	os.MkdirAll(libDir, 0o755)
	os.WriteFile(filepath.Join(libDir, "helper.go"), []byte(libContent), 0o644)

	entries := FindEntryPoints(dir)
	if len(entries) == 0 {
		t.Fatal("expected at least one entry point")
	}
	found := false
	for _, e := range entries {
		if strings.Contains(e, "main.go") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected cmd/main.go in entry points, got %v", entries)
	}
}

func TestFindEntryPointsPython(t *testing.T) {
	dir := t.TempDir()
	content := `#!/usr/bin/env python3

def run():
    print("running")

if __name__ == "__main__":
    run()
`
	os.WriteFile(filepath.Join(dir, "app.py"), []byte(content), 0o644)

	entries := FindEntryPoints(dir)
	if len(entries) == 0 {
		t.Fatal("expected at least one entry point")
	}
	found := false
	for _, e := range entries {
		if e == "app.py" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected app.py in entry points, got %v", entries)
	}
}

func TestFindEntryPointsJS(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := `{"name": "myapp", "main": "src/index.js"}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0o644)

	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0o755)
	os.WriteFile(filepath.Join(srcDir, "index.js"), []byte("console.log('hi')"), 0o644)

	entries := FindEntryPoints(dir)
	found := false
	for _, e := range entries {
		if strings.Contains(e, "src/index.js") || strings.Contains(e, "src"+string(filepath.Separator)+"index.js") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected src/index.js in entry points, got %v", entries)
	}
}

func TestFindKeyFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a file with many exported symbols
	bigFile := `package main

func ExportedOne() {}
func ExportedTwo() {}
func ExportedThree() {}
func ExportedFour() {}
func ExportedFive() {}
func unexported() {}
`
	os.WriteFile(filepath.Join(dir, "big.go"), []byte(bigFile), 0o644)

	// Create a small file
	smallFile := `package main

func tiny() {}
`
	os.WriteFile(filepath.Join(dir, "small.go"), []byte(smallFile), 0o644)

	keys := FindKeyFiles(dir, 5)
	if len(keys) == 0 {
		t.Fatal("expected at least one key file")
	}
	// big.go should rank higher due to more public symbols
	if keys[0] != "big.go" {
		t.Errorf("expected big.go as top key file, got %q", keys[0])
	}
}

func TestRenderForPrompt(t *testing.T) {
	summary := &CodebaseSummary{
		ProjectName: "hawk",
		Language:    "Go",
		Packages: []SummaryPackageInfo{
			{Path: "engine", Purpose: "Agent loop, compaction, streaming", LOC: 14000},
			{Path: "tool", Purpose: "40 built-in tools with safety layer", LOC: 8000},
			{Path: "config", Purpose: "Settings, validation, budget tracking", LOC: 3000},
			{Path: "repomap", Purpose: "Code intelligence (PageRank, BM25)", LOC: 5000},
		},
		EntryPoints:  []string{"cmd/main.go"},
		KeyFiles:     []string{"engine/stream.go", "engine/session.go", "tool/tool.go"},
		Architecture: "layered",
		TotalLOC:     478000,
		TotalFiles:   1476,
		GeneratedAt:  time.Now(),
	}

	result := RenderForPrompt(summary, 1024)

	checks := []string{
		"## Codebase: hawk",
		"Go project",
		"1,476 files",
		"478K LOC",
		"Architecture:",
		"Key packages:",
		"engine/",
		"Entry points: cmd/main.go",
		"Key files:",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected output to contain %q, got:\n%s", check, result)
		}
	}
}

func TestRenderCompact(t *testing.T) {
	summary := &CodebaseSummary{
		ProjectName: "hawk",
		Language:    "Go",
		Packages: []SummaryPackageInfo{
			{Path: "engine", Purpose: "Core engine", LOC: 14000},
			{Path: "tool", Purpose: "Tools", LOC: 8000},
		},
		EntryPoints:  []string{"cmd/main.go"},
		Architecture: "layered",
		TotalLOC:     50000,
		TotalFiles:   200,
	}

	result := RenderCompact(summary)
	if result == "" {
		t.Fatal("expected non-empty compact render")
	}
	if !strings.Contains(result, "hawk") {
		t.Errorf("expected 'hawk' in compact output, got %q", result)
	}
	if !strings.Contains(result, "Go") {
		t.Errorf("expected 'Go' in compact output, got %q", result)
	}
	if !strings.Contains(result, "layered") {
		t.Errorf("expected 'layered' in compact output, got %q", result)
	}
	if !strings.Contains(result, "Entry:") {
		t.Errorf("expected entry points in compact output, got %q", result)
	}
}

func TestGenerateOnTempProject(t *testing.T) {
	dir := t.TempDir()

	// Set up a small Go project
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0o644)

	cmdDir := filepath.Join(dir, "cmd")
	os.MkdirAll(cmdDir, 0o755)
	os.WriteFile(filepath.Join(cmdDir, "main.go"), []byte(`package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`), 0o644)

	pkgDir := filepath.Join(dir, "service")
	os.MkdirAll(pkgDir, 0o755)
	os.WriteFile(filepath.Join(pkgDir, "service.go"), []byte(`package service

type UserService struct{}

func NewUserService() *UserService {
	return &UserService{}
}

func (s *UserService) GetUser(id string) string {
	return id
}
`), 0o644)

	sg := NewSummaryGenerator(dir, 2048)
	summary, err := sg.Generate()
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	if summary.ProjectName == "" {
		t.Error("expected non-empty ProjectName")
	}
	if summary.Language != "Go" {
		t.Errorf("expected language 'Go', got %q", summary.Language)
	}
	if summary.TotalFiles == 0 {
		t.Error("expected TotalFiles > 0")
	}
	if summary.TotalLOC == 0 {
		t.Error("expected TotalLOC > 0")
	}
	if len(summary.Packages) == 0 {
		t.Error("expected at least one package")
	}
	if len(summary.EntryPoints) == 0 {
		t.Error("expected at least one entry point")
	}
	if summary.GeneratedAt.IsZero() {
		t.Error("expected non-zero GeneratedAt")
	}
}

func TestSummaryFormatNumber(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1476, "1,476"},
		{12345, "12,345"},
	}
	for _, tc := range tests {
		got := summaryFormatNumber(tc.input)
		if got != tc.expected {
			t.Errorf("summaryFormatNumber(%d) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestSummaryFormatLOC(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{500, "500"},
		{1000, "1K"},
		{14000, "14K"},
		{478000, "478K"},
		{1500000, "1.5M"},
	}
	for _, tc := range tests {
		got := summaryFormatLOC(tc.input)
		if got != tc.expected {
			t.Errorf("summaryFormatLOC(%d) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestSummaryEstimateTokens(t *testing.T) {
	// 4 chars per token approximately
	text := "hello world" // 11 chars -> ~2 tokens
	tokens := summaryEstimateTokens(text)
	if tokens < 1 || tokens > 5 {
		t.Errorf("unexpected token estimate for %q: %d", text, tokens)
	}
}

func TestSummaryIsPublicSymbol(t *testing.T) {
	tests := []struct {
		name     string
		lang     string
		expected bool
	}{
		{"ExportedFunc", "Go", true},
		{"unexported", "Go", false},
		{"public_func", "Python", true},
		{"_private", "Python", false},
		{"__dunder__", "Python", false},
		{"Component", "TypeScript", true},
	}
	for _, tc := range tests {
		got := summaryIsPublicSymbol(tc.name, tc.lang)
		if got != tc.expected {
			t.Errorf("summaryIsPublicSymbol(%q, %q) = %v, want %v", tc.name, tc.lang, got, tc.expected)
		}
	}
}

func TestSummarySkipDir(t *testing.T) {
	skipped := []string{".git", "node_modules", "vendor", "__pycache__", "dist", "build"}
	for _, dir := range skipped {
		if !summarySkipDir(dir) {
			t.Errorf("expected %q to be skipped", dir)
		}
	}

	allowed := []string{"src", "pkg", "internal", "cmd", "lib"}
	for _, dir := range allowed {
		if summarySkipDir(dir) {
			t.Errorf("expected %q to NOT be skipped", dir)
		}
	}
}

func TestSummaryIsSupportedFile(t *testing.T) {
	supported := []string{"main.go", "app.py", "index.ts", "module.tsx", "script.js", "comp.jsx", "lib.rs"}
	for _, f := range supported {
		if !summaryIsSupportedFile(f) {
			t.Errorf("expected %q to be supported", f)
		}
	}

	unsupported := []string{"readme.md", "config.yaml", "image.png", "data.json"}
	for _, f := range unsupported {
		if summaryIsSupportedFile(f) {
			t.Errorf("expected %q to NOT be supported", f)
		}
	}
}

func TestSummaryPackageInfoFields(t *testing.T) {
	pkg := SummaryPackageInfo{
		Name:          "engine",
		Path:          "engine",
		Purpose:       "Core execution engine",
		PublicSymbols: 25,
		Files:         10,
		LOC:           5000,
		Dependencies:  []string{"config", "tool"},
		Dependents:    []string{"cmd"},
	}

	if pkg.Name != "engine" {
		t.Errorf("unexpected Name: %s", pkg.Name)
	}
	if len(pkg.Dependencies) != 2 {
		t.Errorf("expected 2 dependencies, got %d", len(pkg.Dependencies))
	}
	if len(pkg.Dependents) != 1 {
		t.Errorf("expected 1 dependent, got %d", len(pkg.Dependents))
	}
}

func TestCodebaseSummaryFields(t *testing.T) {
	summary := CodebaseSummary{
		ProjectName:  "test",
		Description:  "A test project",
		Language:     "Go",
		Architecture: "flat",
		TotalLOC:     1000,
		TotalFiles:   10,
		GeneratedAt:  time.Now(),
	}

	if summary.ProjectName != "test" {
		t.Errorf("unexpected ProjectName: %s", summary.ProjectName)
	}
	if summary.GeneratedAt.IsZero() {
		t.Error("expected non-zero GeneratedAt")
	}
}

func TestRenderForPromptBudgetRespected(t *testing.T) {
	// Create a summary with many packages
	packages := make([]SummaryPackageInfo, 50)
	for i := range packages {
		packages[i] = SummaryPackageInfo{
			Path:    strings.Repeat("pkg", 10) + string(rune('a'+i%26)),
			Purpose: "Does something important with long description that takes tokens",
			LOC:     1000 * (50 - i),
		}
	}

	summary := &CodebaseSummary{
		ProjectName:  "bigproject",
		Language:     "Go",
		Packages:     packages,
		Architecture: "layered",
		TotalLOC:     500000,
		TotalFiles:   2000,
	}

	// Small budget should limit output
	result := RenderForPrompt(summary, 200)
	tokens := summaryEstimateTokens(result)
	// Should not massively exceed budget (some overflow is OK for headers)
	if tokens > 400 {
		t.Errorf("render exceeded budget significantly: %d estimated tokens for budget 200", tokens)
	}
}
