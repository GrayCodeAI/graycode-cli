package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDocLinter(t *testing.T) {
	dl := NewDocLinter()
	if dl == nil {
		t.Fatal("NewDocLinter returned nil")
	}
	if dl.MinCommentLength != 10 {
		t.Errorf("expected MinCommentLength=10, got %d", dl.MinCommentLength)
	}
	if !dl.RequireExported {
		t.Error("expected RequireExported=true")
	}
}

func TestLintFile_WellDocumented(t *testing.T) {
	dl := NewDocLinter()
	content := `package example

// Handler processes incoming HTTP requests and dispatches to routes.
type Handler struct {
	routes map[string]func()
}

// NewHandler creates a new Handler with default route configuration.
func NewHandler() *Handler {
	return &Handler{}
}

// ServeHTTP handles an incoming HTTP request and writes the response.
func (h *Handler) ServeHTTP(w, r string) {}
`

	result, err := dl.LintFile("handler.go", content)
	if err != nil {
		t.Fatalf("LintFile error: %v", err)
	}

	if result.Score < 70 {
		t.Errorf("expected high score for well-documented file, got %.0f", result.Score)
	}
	if result.Stats.TotalExported != 3 {
		t.Errorf("expected 3 exported symbols, got %d", result.Stats.TotalExported)
	}
	if result.Stats.Documented != 3 {
		t.Errorf("expected 3 documented symbols, got %d", result.Stats.Documented)
	}
	if result.Stats.Coverage != 100 {
		t.Errorf("expected 100%% coverage, got %.1f%%", result.Stats.Coverage)
	}
}

func TestLintFile_MissingDocs(t *testing.T) {
	dl := NewDocLinter()
	content := `package example

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func ProcessRequest(r string) error {
	return nil
}
`

	result, err := dl.LintFile("handler.go", content)
	if err != nil {
		t.Fatalf("LintFile error: %v", err)
	}

	if result.Stats.MissingCount != 3 {
		t.Errorf("expected 3 missing docs, got %d", result.Stats.MissingCount)
	}
	if result.Stats.Coverage != 0 {
		t.Errorf("expected 0%% coverage, got %.1f%%", result.Stats.Coverage)
	}
	if result.Score >= 50 {
		t.Errorf("expected low score for undocumented file, got %.0f", result.Score)
	}
}

func TestLintFile_ParseError(t *testing.T) {
	dl := NewDocLinter()
	content := `not valid go code {{{{`

	_, err := dl.LintFile("bad.go", content)
	if err == nil {
		t.Fatal("expected parse error for invalid Go code")
	}
}

func TestCheckGoDoc_Missing(t *testing.T) {
	dl := NewDocLinter()
	issues := dl.CheckGoDoc("Handler", "")
	if len(issues) == 0 {
		t.Fatal("expected issue for missing doc")
	}
	if issues[0].Type != "missing" {
		t.Errorf("expected type 'missing', got %q", issues[0].Type)
	}
	if issues[0].Severity != "error" {
		t.Errorf("expected severity 'error', got %q", issues[0].Severity)
	}
}

func TestCheckGoDoc_BadPrefix(t *testing.T) {
	dl := NewDocLinter()
	issues := dl.CheckGoDoc("Handler", "This handles HTTP requests and processes them.")
	found := false
	for _, issue := range issues {
		if issue.Type == "incomplete" && strings.Contains(issue.Message, "does not start with symbol name") {
			found = true
		}
	}
	if !found {
		t.Error("expected bad prefix issue")
	}
}

func TestCheckGoDoc_TooShort(t *testing.T) {
	dl := NewDocLinter()
	issues := dl.CheckGoDoc("Handler", "Handler x")
	found := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "too short") {
			found = true
		}
	}
	if !found {
		t.Error("expected too-short issue")
	}
}

func TestCheckGoDoc_Trivial(t *testing.T) {
	dl := NewDocLinter()
	issues := dl.CheckGoDoc("Handler", "Handler handles")
	found := false
	for _, issue := range issues {
		if issue.Type == "unclear" {
			found = true
		}
	}
	if !found {
		t.Error("expected trivial comment issue")
	}
}

func TestCheckGoDoc_GoodComment(t *testing.T) {
	dl := NewDocLinter()
	issues := dl.CheckGoDoc("Handler", "Handler processes incoming HTTP requests and dispatches them to the appropriate route.")
	if len(issues) != 0 {
		t.Errorf("expected no issues for good comment, got %d: %+v", len(issues), issues)
	}
}

func TestSuggestDocComment_Func(t *testing.T) {
	dl := NewDocLinter()
	suggestion := dl.SuggestDocComment("ValidateToken", "func", "func ValidateToken(token string) (Claims, error)")
	if !strings.HasPrefix(suggestion, "// ValidateToken") {
		t.Errorf("suggestion should start with '// ValidateToken', got: %s", suggestion)
	}
	if !strings.Contains(suggestion, "validate") {
		t.Errorf("suggestion should contain verb 'validate', got: %s", suggestion)
	}
}

func TestSuggestDocComment_Type(t *testing.T) {
	dl := NewDocLinter()
	suggestion := dl.SuggestDocComment("AuthService", "type", "struct")
	if !strings.HasPrefix(suggestion, "// AuthService") {
		t.Errorf("suggestion should start with '// AuthService', got: %s", suggestion)
	}
	if !strings.Contains(suggestion, "represents") {
		t.Errorf("suggestion for struct should contain 'represents', got: %s", suggestion)
	}
}

func TestSuggestDocComment_Interface(t *testing.T) {
	dl := NewDocLinter()
	suggestion := dl.SuggestDocComment("TokenValidator", "type", "interface")
	if !strings.Contains(suggestion, "interface") {
		t.Errorf("suggestion for interface should contain 'interface', got: %s", suggestion)
	}
}

func TestSuggestDocComment_Var(t *testing.T) {
	dl := NewDocLinter()
	suggestion := dl.SuggestDocComment("DefaultTimeout", "var", "")
	if !strings.HasPrefix(suggestion, "// DefaultTimeout") {
		t.Errorf("suggestion should start with '// DefaultTimeout', got: %s", suggestion)
	}
}

func TestDocLintFormatReport(t *testing.T) {
	dl := NewDocLinter()
	results := []*DocLintResult{
		{
			File:  "src/auth/token.go",
			Score: 72,
			Issues: []DocIssue{
				{Line: 15, Symbol: "ValidateToken", Type: "missing", Message: "ValidateToken — missing doc comment", Severity: "error"},
				{Line: 42, Symbol: "Claims", Type: "incomplete", Message: "Claims — comment too short", Severity: "warning"},
			},
			Stats: DocStats{TotalExported: 5, Documented: 3, Coverage: 60},
		},
		{
			File:  "src/handler/api.go",
			Score: 45,
			Issues: []DocIssue{
				{Line: 8, Symbol: "HandleRequest", Type: "missing", Message: "HandleRequest — missing doc comment", Severity: "error"},
				{Line: 25, Symbol: "ProcessResponse", Type: "missing", Message: "ProcessResponse — missing doc comment", Severity: "error"},
				{Line: 40, Symbol: "ErrorHandler", Type: "unclear", Message: "ErrorHandler — trivial comment that restates the name", Severity: "warning"},
			},
			Stats: DocStats{TotalExported: 4, Documented: 1, Coverage: 25},
		},
	}

	report := dl.FormatReport(results)

	if !strings.Contains(report, "Documentation Lint:") {
		t.Error("report missing header")
	}
	if !strings.Contains(report, "═══════════════════════════════") {
		t.Error("report missing separator")
	}
	if !strings.Contains(report, "src/auth/token.go") {
		t.Error("report missing first file")
	}
	if !strings.Contains(report, "src/handler/api.go") {
		t.Error("report missing second file")
	}
	if !strings.Contains(report, "Summary:") {
		t.Error("report missing summary")
	}
	if !strings.Contains(report, "3 missing") {
		t.Error("report missing count incorrect")
	}
	if !strings.Contains(report, "1 trivial") {
		t.Error("report trivial count incorrect")
	}
}

func TestDocLintFormatReport_Empty(t *testing.T) {
	dl := NewDocLinter()
	report := dl.FormatReport(nil)
	if !strings.Contains(report, "No files analyzed") {
		t.Error("empty report should indicate no files analyzed")
	}
}

func TestCoverageReport(t *testing.T) {
	dl := NewDocLinter()
	results := []*DocLintResult{
		{
			File:  "auth.go",
			Stats: DocStats{TotalExported: 10, Documented: 8, Coverage: 80},
		},
		{
			File:  "handler.go",
			Stats: DocStats{TotalExported: 5, Documented: 2, Coverage: 40},
		},
	}

	report := dl.CoverageReport(results)
	if !strings.Contains(report, "Documentation Coverage Report") {
		t.Error("coverage report missing header")
	}
	if !strings.Contains(report, "Overall:") {
		t.Error("coverage report missing overall line")
	}
	if !strings.Contains(report, "auth.go") {
		t.Error("coverage report missing auth.go")
	}
	if !strings.Contains(report, "handler.go") {
		t.Error("coverage report missing handler.go")
	}
	// Overall should be 10/15 = 66.7%
	if !strings.Contains(report, "66.7%") {
		t.Error("coverage report should show 66.7% overall coverage")
	}
}

func TestCoverageReport_Empty(t *testing.T) {
	dl := NewDocLinter()
	report := dl.CoverageReport(nil)
	if !strings.Contains(report, "No files analyzed") {
		t.Error("empty coverage report should indicate no files analyzed")
	}
}

func TestLintDirectory(t *testing.T) {
	// Create a temporary directory with Go files
	dir := t.TempDir()

	// Well documented file
	wellDoc := `package example

// Service provides the core business logic for the application.
type Service struct{}

// Run executes the main service loop and handles incoming requests.
func (s *Service) Run() {}
`
	err := os.WriteFile(filepath.Join(dir, "service.go"), []byte(wellDoc), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Undocumented file
	undoc := `package example

type Repo struct{}

func (r *Repo) Save() {}

func (r *Repo) Load() {}
`
	err = os.WriteFile(filepath.Join(dir, "repo.go"), []byte(undoc), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Test file should be skipped
	testFile := `package example

func TestSomething(t *testing.T) {}
`
	err = os.WriteFile(filepath.Join(dir, "service_test.go"), []byte(testFile), 0644)
	if err != nil {
		t.Fatal(err)
	}

	dl := NewDocLinter()
	results, lintErr := dl.LintDirectory(dir)
	if lintErr != nil {
		t.Fatalf("LintDirectory error: %v", lintErr)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results (test file skipped), got %d", len(results))
	}

	// Find the undocumented file result
	var repoResult *DocLintResult
	for _, r := range results {
		if strings.Contains(r.File, "repo.go") {
			repoResult = r
		}
	}
	if repoResult == nil {
		t.Fatal("missing result for repo.go")
	}
	if repoResult.Stats.MissingCount == 0 {
		t.Error("expected missing docs in repo.go")
	}
}

func TestLintDirectory_SkipsVendor(t *testing.T) {
	dir := t.TempDir()

	// Create vendor directory with Go file
	vendorDir := filepath.Join(dir, "vendor")
	err := os.MkdirAll(vendorDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(vendorDir, "lib.go"), []byte("package lib\n\ntype X struct{}\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create a main file
	err = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\n// Run starts the application.\nfunc Run() {}\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	dl := NewDocLinter()
	results, lintErr := dl.LintDirectory(dir)
	if lintErr != nil {
		t.Fatalf("LintDirectory error: %v", lintErr)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result (vendor skipped), got %d", len(results))
	}
}

func TestLintFile_NoExportedSymbols(t *testing.T) {
	dl := NewDocLinter()
	content := `package internal

func helper() {}
func process() {}
`
	result, err := dl.LintFile("internal.go", content)
	if err != nil {
		t.Fatalf("LintFile error: %v", err)
	}
	if result.Score != 100 {
		t.Errorf("expected score 100 for file with no exported symbols, got %.0f", result.Score)
	}
	if result.Stats.TotalExported != 0 {
		t.Errorf("expected 0 exported symbols, got %d", result.Stats.TotalExported)
	}
}

func TestLintFile_ExportedVars(t *testing.T) {
	dl := NewDocLinter()
	content := `package config

// DefaultTimeout defines the default timeout duration for connections.
var DefaultTimeout = 30

var UndocumentedVar = "oops"
`
	result, err := dl.LintFile("config.go", content)
	if err != nil {
		t.Fatalf("LintFile error: %v", err)
	}
	if result.Stats.TotalExported != 2 {
		t.Errorf("expected 2 exported vars, got %d", result.Stats.TotalExported)
	}
	if result.Stats.Documented != 1 {
		t.Errorf("expected 1 documented var, got %d", result.Stats.Documented)
	}
}

func TestIsTrivialComment(t *testing.T) {
	tests := []struct {
		testName string
		symbol   string
		comment  string
		trivial  bool
	}{
		{"Handler handles", "Handler", "Handler handles", true},
		{"Process processes", "Process", "Process processes", true},
		{"Handler is a Handler", "Handler", "Handler is a handler", true},
		{"Good comment", "Handler", "Handler processes incoming HTTP requests and routes them.", false},
		{"Different verb", "Handler", "Handler dispatches events to registered listeners.", false},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			got := isTrivialComment(tt.symbol, tt.comment)
			if got != tt.trivial {
				t.Errorf("isTrivialComment(%q, %q) = %v, want %v", tt.symbol, tt.comment, got, tt.trivial)
			}
		})
	}
}

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"HandleRequest", []string{"Handle", "Request"}},
		{"ProcessHTTPData", []string{"Process", "H", "T", "T", "P", "Data"}},
		{"Run", []string{"Run"}},
		{"lower", []string{"lower"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitCamelCase(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("splitCamelCase(%q) = %v, want %v", tt.input, got, tt.expected)
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("splitCamelCase(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestExtractReturns(t *testing.T) {
	tests := []struct {
		sig      string
		expected string
	}{
		{"func Foo() error", "an error if the operation fails"},
		{"func Foo() (string, error)", "string"},
		{"func Foo() string", "string"},
		{"func Foo()", ""},
	}

	for _, tt := range tests {
		t.Run(tt.sig, func(t *testing.T) {
			got := extractReturns(tt.sig)
			if got != tt.expected {
				t.Errorf("extractReturns(%q) = %q, want %q", tt.sig, got, tt.expected)
			}
		})
	}
}

func TestCoverageBar(t *testing.T) {
	bar0 := coverageBar(0)
	if !strings.Contains(bar0, "░") {
		t.Error("0% bar should be all empty")
	}
	bar100 := coverageBar(100)
	if !strings.Contains(bar100, "█") {
		t.Error("100% bar should be all filled")
	}
	if strings.Contains(bar100, "░") {
		t.Error("100% bar should have no empty portions")
	}
}
