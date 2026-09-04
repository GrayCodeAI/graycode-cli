package tool

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

func TestNewChangeValidator(t *testing.T) {
	cv := NewChangeValidator()
	if cv == nil {
		t.Fatal("NewChangeValidator returned nil")
	}
	if len(cv.Checks) != 6 {
		t.Errorf("expected 6 built-in checks, got %d", len(cv.Checks))
	}

	expectedNames := []string{"syntax", "format", "lint", "test", "security", "size"}
	for i, name := range expectedNames {
		if cv.Checks[i].Name != name {
			t.Errorf("check[%d]: expected name %q, got %q", i, name, cv.Checks[i].Name)
		}
	}
}

func TestAddCheck(t *testing.T) {
	cv := NewChangeValidator()
	initial := len(cv.Checks)

	cv.AddCheck(ValidationCheck{
		Name:     "custom",
		Category: "lint",
		Required: true,
		RunFn: func(files []string) *CheckResult {
			return &CheckResult{Passed: true, Message: "custom check passed"}
		},
	})

	if len(cv.Checks) != initial+1 {
		t.Errorf("expected %d checks after AddCheck, got %d", initial+1, len(cv.Checks))
	}
	if cv.Checks[len(cv.Checks)-1].Name != "custom" {
		t.Errorf("last check should be 'custom', got %q", cv.Checks[len(cv.Checks)-1].Name)
	}
}

func TestValidate_AllPass(t *testing.T) {
	cv := &ChangeValidator{
		Checks: []ValidationCheck{
			{
				Name:     "pass1",
				Category: "syntax",
				Required: true,
				RunFn: func(files []string) *CheckResult {
					return &CheckResult{Passed: true, Message: "ok", Severity: "error"}
				},
			},
			{
				Name:     "pass2",
				Category: "lint",
				Required: true,
				RunFn: func(files []string) *CheckResult {
					return &CheckResult{Passed: true, Message: "ok", Severity: "error"}
				},
			},
		},
	}

	report := cv.Validate([]string{"file.go"})
	if !report.AllPassed {
		t.Error("expected AllPassed to be true")
	}
	if report.BlockingFailures != 0 {
		t.Errorf("expected 0 blocking failures, got %d", report.BlockingFailures)
	}
	if report.Warnings != 0 {
		t.Errorf("expected 0 warnings, got %d", report.Warnings)
	}
	if len(report.Checks) != 2 {
		t.Errorf("expected 2 check results, got %d", len(report.Checks))
	}
}

func TestValidate_RequiredCheckFails(t *testing.T) {
	cv := &ChangeValidator{
		Checks: []ValidationCheck{
			{
				Name:     "failing",
				Category: "lint",
				Required: true,
				RunFn: func(files []string) *CheckResult {
					return &CheckResult{
						Passed:   false,
						Message:  "2 issues found",
						Details:  []string{"file.go:10: unused variable", "file.go:20: error not checked"},
						Severity: "error",
					}
				},
			},
		},
	}

	report := cv.Validate([]string{"file.go"})
	if report.AllPassed {
		t.Error("expected AllPassed to be false")
	}
	if report.BlockingFailures != 1 {
		t.Errorf("expected 1 blocking failure, got %d", report.BlockingFailures)
	}
}

func TestValidate_OptionalCheckFails(t *testing.T) {
	cv := &ChangeValidator{
		Checks: []ValidationCheck{
			{
				Name:     "optional-warn",
				Category: "size",
				Required: false,
				RunFn: func(files []string) *CheckResult {
					return &CheckResult{
						Passed:   false,
						Message:  "large file",
						Severity: "warning",
					}
				},
			},
		},
	}

	report := cv.Validate([]string{"big.go"})
	if !report.AllPassed {
		t.Error("optional check failure should not block")
	}
	if report.Warnings != 1 {
		t.Errorf("expected 1 warning, got %d", report.Warnings)
	}
}

func TestValidate_StopOnFirst(t *testing.T) {
	var mu sync.Mutex
	callCount := 0
	cv := &ChangeValidator{
		StopOnFirst: true,
		Checks: []ValidationCheck{
			{
				Name:     "fail-first",
				Category: "syntax",
				Required: true,
				RunFn: func(files []string) *CheckResult {
					mu.Lock()
					callCount++
					mu.Unlock()
					return &CheckResult{Passed: false, Message: "fail", Severity: "error"}
				},
			},
			{
				Name:     "second-check",
				Category: "lint",
				Required: true,
				RunFn: func(files []string) *CheckResult {
					mu.Lock()
					callCount++
					mu.Unlock()
					return &CheckResult{Passed: true, Message: "ok", Severity: "error"}
				},
			},
		},
	}

	report := cv.Validate([]string{"file.go"})
	if report.AllPassed {
		t.Error("expected AllPassed false")
	}
	// With StopOnFirst and parallel execution, both may run but only first failure blocks result collection.
	if report.BlockingFailures < 1 {
		t.Errorf("expected at least 1 blocking failure, got %d", report.BlockingFailures)
	}
}

func TestValidateQuick(t *testing.T) {
	cv := &ChangeValidator{
		Checks: []ValidationCheck{
			{
				Name:     "syntax",
				Category: "syntax",
				Required: true,
				RunFn: func(files []string) *CheckResult {
					return &CheckResult{Passed: true, Message: "ok", Severity: "error"}
				},
			},
			{
				Name:     "format",
				Category: "style",
				Required: true,
				RunFn: func(files []string) *CheckResult {
					return &CheckResult{Passed: true, Message: "ok", Severity: "error"}
				},
			},
			{
				Name:     "lint",
				Category: "lint",
				Required: true,
				RunFn: func(files []string) *CheckResult {
					t.Error("lint should not run in quick mode")
					return &CheckResult{Passed: true}
				},
			},
			{
				Name:     "test",
				Category: "test",
				Required: false,
				RunFn: func(files []string) *CheckResult {
					t.Error("test should not run in quick mode")
					return &CheckResult{Passed: true}
				},
			},
		},
	}

	report := cv.ValidateQuick([]string{"file.go"})
	if !report.AllPassed {
		t.Error("expected quick validation to pass")
	}
	if len(report.Checks) != 2 {
		t.Errorf("expected 2 checks in quick mode, got %d", len(report.Checks))
	}
}

func TestFormatReport_AllPassed(t *testing.T) {
	report := &ValidationReport{
		AllPassed: true,
		Duration:  150 * time.Millisecond,
		Checks: []CheckResult{
			{Passed: true, CheckName: "syntax", Message: "all files compile", Severity: "error"},
			{Passed: true, CheckName: "format", Message: "properly formatted", Severity: "error"},
		},
	}

	formatted := FormatReport(report)
	if !strings.Contains(formatted, "Pre-Commit Validation:") {
		t.Error("missing header")
	}
	if !strings.Contains(formatted, icons.CheckBold()+" syntax: all files compile") {
		t.Error("missing syntax check line")
	}
	if !strings.Contains(formatted, icons.CheckBold()+" format: properly formatted") {
		t.Error("missing format check line")
	}
	if !strings.Contains(formatted, "Result: PASSED") {
		t.Error("missing PASSED result")
	}
}

func TestFormatReport_Blocked(t *testing.T) {
	report := &ValidationReport{
		AllPassed:        false,
		BlockingFailures: 1,
		Warnings:         1,
		Duration:         200 * time.Millisecond,
		Checks: []CheckResult{
			{Passed: true, CheckName: "syntax", Message: "all files compile", Severity: "error"},
			{
				Passed:    false,
				CheckName: "lint",
				Message:   "2 issues found",
				Severity:  "error",
				Details:   []string{"src/auth.go:42: error return value not checked", "src/handler.go:15: unused variable"},
			},
			{
				Passed:    false,
				CheckName: "size",
				Message:   "src/big_file.go adds 600 lines (consider splitting)",
				Severity:  "warning",
			},
		},
	}

	formatted := FormatReport(report)
	if !strings.Contains(formatted, icons.CloseThick()+" lint: 2 issues found") {
		t.Errorf("missing lint failure line, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "src/auth.go:42: error return value not checked") {
		t.Error("missing lint detail")
	}
	if !strings.Contains(formatted, icons.Alert()+" size:") {
		t.Error("missing size warning")
	}
	if !strings.Contains(formatted, "BLOCKED") {
		t.Errorf("missing BLOCKED result, got:\n%s", formatted)
	}
}

func TestShouldBlock(t *testing.T) {
	tests := []struct {
		name     string
		report   *ValidationReport
		expected bool
	}{
		{
			name:     "all passed",
			report:   &ValidationReport{AllPassed: true},
			expected: false,
		},
		{
			name:     "has blocking failures",
			report:   &ValidationReport{AllPassed: false, BlockingFailures: 1},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldBlock(tt.report)
			if got != tt.expected {
				t.Errorf("ShouldBlock() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAutoFix_Format(t *testing.T) {
	report := &ValidationReport{
		Checks: []CheckResult{
			{
				Passed:    false,
				CheckName: "format",
				Message:   "files need formatting",
				Details:   []string{"main.go needs gofmt", "utils.go needs gofmt"},
			},
		},
	}

	fixes := AutoFix(report)
	if len(fixes) == 0 {
		t.Error("expected fix suggestions for format issues")
	}

	hasGofmt := false
	for _, fix := range fixes {
		if strings.Contains(fix, "gofmt") {
			hasGofmt = true
			break
		}
	}
	if !hasGofmt {
		t.Errorf("expected gofmt suggestion, got: %v", fixes)
	}
}

func TestAutoFix_Lint(t *testing.T) {
	report := &ValidationReport{
		Checks: []CheckResult{
			{
				Passed:    false,
				CheckName: "lint",
				Message:   "2 issues found",
				Details:   []string{"file.go:10: unused variable x", "file.go:5: unused import fmt"},
			},
		},
	}

	fixes := AutoFix(report)
	if len(fixes) == 0 {
		t.Error("expected fix suggestions for lint issues")
	}

	hasUnused := false
	hasImport := false
	for _, fix := range fixes {
		if strings.Contains(fix, "unused declaration") {
			hasUnused = true
		}
		if strings.Contains(fix, "unused import") {
			hasImport = true
		}
	}
	if !hasUnused {
		t.Errorf("expected unused declaration suggestion, got: %v", fixes)
	}
	if !hasImport {
		t.Errorf("expected unused import suggestion, got: %v", fixes)
	}
}

func TestAutoFix_PassedChecks(t *testing.T) {
	report := &ValidationReport{
		AllPassed: true,
		Checks: []CheckResult{
			{Passed: true, CheckName: "format", Message: "properly formatted"},
			{Passed: true, CheckName: "lint", Message: "no issues"},
		},
	}

	fixes := AutoFix(report)
	if len(fixes) != 0 {
		t.Errorf("expected no fixes for passing checks, got: %v", fixes)
	}
}

func TestCheckSecurity_DetectsSecrets(t *testing.T) {
	// Create a temp file with a fake secret.
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "config.go")
	content := `package config

var apiKey = "sk-abcdefghijklmnopqrstuvwxyz123456"
var password = "super_secret_password_123"
`
	if err := os.WriteFile(secretFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result := checkSecurity([]string{secretFile})
	if result.Passed {
		t.Error("expected security check to fail with secrets present")
	}
	if !strings.Contains(result.Message, "secret") {
		t.Errorf("expected message about secrets, got: %s", result.Message)
	}
	if len(result.Details) == 0 {
		t.Error("expected details about detected secrets")
	}
}

func TestCheckSecurity_NoSecrets(t *testing.T) {
	dir := t.TempDir()
	safeFile := filepath.Join(dir, "main.go")
	content := `package main

import "fmt"

func main() {
	fmt.Println("hello world")
}
`
	if err := os.WriteFile(safeFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result := checkSecurity([]string{safeFile})
	if !result.Passed {
		t.Errorf("expected security check to pass, details: %v", result.Details)
	}
}

func TestCheckSize_LargeFile(t *testing.T) {
	dir := t.TempDir()
	bigFile := filepath.Join(dir, "big.go")

	var sb strings.Builder
	sb.WriteString("package big\n\n")
	for i := 0; i < 600; i++ {
		sb.WriteString("// line of code\n")
	}
	if err := os.WriteFile(bigFile, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	result := checkSize([]string{bigFile})
	if result.Passed {
		t.Error("expected size check to fail for large file")
	}
	if len(result.Details) == 0 {
		t.Error("expected details about large file")
	}
}

func TestCheckSize_SmallFile(t *testing.T) {
	dir := t.TempDir()
	smallFile := filepath.Join(dir, "small.go")
	content := "package small\n\nfunc Hello() string { return \"hi\" }\n"
	if err := os.WriteFile(smallFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result := checkSize([]string{smallFile})
	if !result.Passed {
		t.Error("expected size check to pass for small file")
	}
}

func TestFilterByExtension(t *testing.T) {
	files := []string{
		"main.go",
		"utils.go",
		"app.js",
		"style.css",
		"index.ts",
		"readme.md",
	}

	goFiles := filterByExtension(files, ".go")
	if len(goFiles) != 2 {
		t.Errorf("expected 2 .go files, got %d: %v", len(goFiles), goFiles)
	}

	jsFiles := filterByExtension(files, ".js", ".ts")
	if len(jsFiles) != 2 {
		t.Errorf("expected 2 JS/TS files, got %d: %v", len(jsFiles), jsFiles)
	}

	noFiles := filterByExtension(files, ".rb")
	if len(noFiles) != 0 {
		t.Errorf("expected 0 .rb files, got %d", len(noFiles))
	}
}

func TestExtractLines(t *testing.T) {
	input := "line one\n  \nline two\n\nline three\n"
	lines := extractLines(input)
	if len(lines) != 3 {
		t.Errorf("expected 3 non-empty lines, got %d: %v", len(lines), lines)
	}
}

func TestCountNonEmptyLines(t *testing.T) {
	input := "a\n\nb\nc\n  \nd\n"
	count := countNonEmptyLines(input)
	if count != 4 {
		t.Errorf("expected 4 non-empty lines, got %d", count)
	}
}

func TestValidate_Duration(t *testing.T) {
	cv := &ChangeValidator{
		Checks: []ValidationCheck{
			{
				Name:     "slow",
				Category: "test",
				Required: false,
				RunFn: func(files []string) *CheckResult {
					time.Sleep(10 * time.Millisecond)
					return &CheckResult{Passed: true, Message: "ok"}
				},
			},
		},
	}

	report := cv.Validate([]string{"file.go"})
	if report.Duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", report.Duration)
	}
}

func TestValidate_ParallelExecution(t *testing.T) {
	cv := &ChangeValidator{
		Checks: []ValidationCheck{
			{
				Name:     "slow1",
				Category: "test",
				Required: false,
				RunFn: func(files []string) *CheckResult {
					time.Sleep(50 * time.Millisecond)
					return &CheckResult{Passed: true, Message: "ok"}
				},
			},
			{
				Name:     "slow2",
				Category: "test",
				Required: false,
				RunFn: func(files []string) *CheckResult {
					time.Sleep(50 * time.Millisecond)
					return &CheckResult{Passed: true, Message: "ok"}
				},
			},
		},
	}

	start := time.Now()
	report := cv.Validate([]string{"file.go"})
	elapsed := time.Since(start)

	if !report.AllPassed {
		t.Error("expected all checks to pass")
	}
	// If running in parallel, total time should be ~50ms, not ~100ms.
	if elapsed > 90*time.Millisecond {
		t.Errorf("checks appear to run sequentially: elapsed %v (expected < 90ms for parallel)", elapsed)
	}
}

func TestChangeValidator_ConcurrentAccess(t *testing.T) {
	cv := &ChangeValidator{
		Checks: []ValidationCheck{
			{
				Name:     "base",
				Category: "syntax",
				Required: true,
				RunFn: func(files []string) *CheckResult {
					return &CheckResult{Passed: true, Message: "ok"}
				},
			},
		},
	}

	// Run validation and add checks concurrently.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 10; i++ {
			cv.AddCheck(ValidationCheck{
				Name:     "concurrent",
				Category: "lint",
				Required: false,
				RunFn: func(files []string) *CheckResult {
					return &CheckResult{Passed: true, Message: "ok"}
				},
			})
		}
	}()

	// Validate while adding checks.
	for i := 0; i < 5; i++ {
		_ = cv.Validate([]string{"file.go"})
	}

	<-done
}
