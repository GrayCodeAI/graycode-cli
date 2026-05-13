package tool

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ChangeValidator runs a battery of validation checks before allowing changes
// to be committed. It supports parallel execution and configurable check sets.
type ChangeValidator struct {
	Checks      []ValidationCheck
	StopOnFirst bool
	mu          sync.RWMutex
}

// ValidationCheck represents a single validation step in the pre-commit pipeline.
type ValidationCheck struct {
	Name     string
	Category string // "syntax", "lint", "test", "security", "style", "size"
	RunFn    func(files []string) *CheckResult
	Required bool
}

// CheckResult holds the outcome of a single validation check.
type CheckResult struct {
	Passed    bool
	CheckName string
	Message   string
	Details   []string
	Duration  time.Duration
	Severity  string // "error", "warning", "info"
}

// ValidationReport aggregates the results of all checks in a validation run.
type ValidationReport struct {
	Checks           []CheckResult
	AllPassed        bool
	Duration         time.Duration
	BlockingFailures int
	Warnings         int
}

// NewChangeValidator creates a ChangeValidator with built-in checks covering
// syntax, format, lint, test, security, and size categories.
func NewChangeValidator() *ChangeValidator {
	cv := &ChangeValidator{}

	cv.Checks = []ValidationCheck{
		{
			Name:     "syntax",
			Category: "syntax",
			Required: true,
			RunFn:    checkSyntax,
		},
		{
			Name:     "format",
			Category: "style",
			Required: true,
			RunFn:    checkFormat,
		},
		{
			Name:     "lint",
			Category: "lint",
			Required: true,
			RunFn:    checkLint,
		},
		{
			Name:     "test",
			Category: "test",
			Required: false,
			RunFn:    checkTests,
		},
		{
			Name:     "security",
			Category: "security",
			Required: true,
			RunFn:    checkSecurity,
		},
		{
			Name:     "size",
			Category: "size",
			Required: false,
			RunFn:    checkSize,
		},
	}

	return cv
}

// Validate runs all configured checks against the given changed files, executing
// independent checks in parallel where possible. Returns a full validation report.
func (cv *ChangeValidator) Validate(changedFiles []string) *ValidationReport {
	cv.mu.RLock()
	checks := make([]ValidationCheck, len(cv.Checks))
	copy(checks, cv.Checks)
	stopOnFirst := cv.StopOnFirst
	cv.mu.RUnlock()

	start := time.Now()
	report := &ValidationReport{AllPassed: true}

	type indexedResult struct {
		idx    int
		result *CheckResult
	}

	results := make(chan indexedResult, len(checks))
	var wg sync.WaitGroup

	for i, check := range checks {
		wg.Add(1)
		go func(idx int, c ValidationCheck) {
			defer wg.Done()
			checkStart := time.Now()
			result := c.RunFn(changedFiles)
			if result == nil {
				result = &CheckResult{Passed: true, CheckName: c.Name}
			}
			result.CheckName = c.Name
			result.Duration = time.Since(checkStart)
			results <- indexedResult{idx: idx, result: result}
		}(i, check)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	ordered := make([]*CheckResult, len(checks))
	for ir := range results {
		ordered[ir.idx] = ir.result
	}

	for i, result := range ordered {
		if result == nil {
			continue
		}
		report.Checks = append(report.Checks, *result)

		if !result.Passed {
			if checks[i].Required {
				report.AllPassed = false
				report.BlockingFailures++
			} else {
				report.Warnings++
			}
			if stopOnFirst && checks[i].Required {
				break
			}
		}
	}

	report.Duration = time.Since(start)
	return report
}

// ValidateQuick runs only fast checks (syntax and format) for rapid feedback.
func (cv *ChangeValidator) ValidateQuick(changedFiles []string) *ValidationReport {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	start := time.Now()
	report := &ValidationReport{AllPassed: true}

	for _, check := range cv.Checks {
		if check.Category != "syntax" && check.Category != "style" {
			continue
		}

		checkStart := time.Now()
		result := check.RunFn(changedFiles)
		if result == nil {
			result = &CheckResult{Passed: true, CheckName: check.Name}
		}
		result.CheckName = check.Name
		result.Duration = time.Since(checkStart)
		report.Checks = append(report.Checks, *result)

		if !result.Passed {
			if check.Required {
				report.AllPassed = false
				report.BlockingFailures++
			} else {
				report.Warnings++
			}
		}
	}

	report.Duration = time.Since(start)
	return report
}

// AddCheck registers a new validation check to the validator.
func (cv *ChangeValidator) AddCheck(check ValidationCheck) {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	cv.Checks = append(cv.Checks, check)
}

// FormatReport produces a human-readable formatted report of validation results.
func FormatReport(report *ValidationReport) string {
	var sb strings.Builder

	sb.WriteString("Pre-Commit Validation:\n")
	sb.WriteString(strings.Repeat("═", 23) + "\n")

	for _, check := range report.Checks {
		if check.Passed {
			sb.WriteString(fmt.Sprintf("✓ %s: %s\n", check.CheckName, check.Message))
		} else if check.Severity == "warning" {
			sb.WriteString(fmt.Sprintf("⚠ %s: %s\n", check.CheckName, check.Message))
			for _, detail := range check.Details {
				sb.WriteString(fmt.Sprintf("  - %s\n", detail))
			}
		} else {
			sb.WriteString(fmt.Sprintf("✗ %s: %s\n", check.CheckName, check.Message))
			for _, detail := range check.Details {
				sb.WriteString(fmt.Sprintf("  - %s\n", detail))
			}
		}
	}

	sb.WriteString("\n")

	if report.AllPassed && report.Warnings == 0 {
		sb.WriteString(fmt.Sprintf("Result: PASSED (%s)\n", report.Duration.Round(time.Millisecond)))
	} else if report.AllPassed && report.Warnings > 0 {
		sb.WriteString(fmt.Sprintf("Result: PASSED with %d warning(s) (%s)\n", report.Warnings, report.Duration.Round(time.Millisecond)))
	} else {
		sb.WriteString(fmt.Sprintf("Result: BLOCKED (%d required check failed, %d warning)\n", report.BlockingFailures, report.Warnings))
	}

	return sb.String()
}

// ShouldBlock returns true if the validation report indicates the commit
// should not proceed due to required check failures.
func ShouldBlock(report *ValidationReport) bool {
	return !report.AllPassed
}

// AutoFix attempts to automatically fix issues found in the validation report.
// It returns a list of descriptions of fixes that were applied or suggested.
func AutoFix(report *ValidationReport) []string {
	var fixes []string

	for _, check := range report.Checks {
		if check.Passed {
			continue
		}

		switch check.CheckName {
		case "format":
			fixes = append(fixes, autoFixFormat(check)...)
		case "lint":
			fixes = append(fixes, autoFixLint(check)...)
		}
	}

	return fixes
}

// --- Built-in check implementations ---

func checkSyntax(files []string) *CheckResult {
	result := &CheckResult{
		Passed:   true,
		Severity: "error",
		Message:  "all files compile",
	}

	goFiles := filterByExtension(files, ".go")
	pyFiles := filterByExtension(files, ".py")
	tsFiles := filterByExtension(files, ".ts", ".tsx")

	if len(goFiles) > 0 {
		cmd := exec.Command("go", "build", "./...")
		if output, err := cmd.CombinedOutput(); err != nil {
			result.Passed = false
			result.Message = "compilation failed"
			result.Details = extractLines(string(output))
			return result
		}
	}

	if len(pyFiles) > 0 {
		for _, f := range pyFiles {
			cmd := exec.Command("python", "-m", "py_compile", f)
			if output, err := cmd.CombinedOutput(); err != nil {
				result.Passed = false
				result.Message = "syntax error in Python file"
				result.Details = append(result.Details, fmt.Sprintf("%s: %s", f, strings.TrimSpace(string(output))))
			}
		}
		if !result.Passed {
			return result
		}
	}

	if len(tsFiles) > 0 {
		args := append([]string{"--noEmit"}, tsFiles...)
		cmd := exec.Command("tsc", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			result.Passed = false
			result.Message = "TypeScript compilation failed"
			result.Details = extractLines(string(output))
			return result
		}
	}

	return result
}

func checkFormat(files []string) *CheckResult {
	result := &CheckResult{
		Passed:   true,
		Severity: "error",
		Message:  "properly formatted",
	}

	goFiles := filterByExtension(files, ".go")
	jsFiles := filterByExtension(files, ".js", ".ts", ".tsx", ".jsx", ".json", ".css", ".html")

	if len(goFiles) > 0 {
		args := append([]string{"-l"}, goFiles...)
		cmd := exec.Command("gofmt", args...)
		if output, err := cmd.Output(); err == nil && len(strings.TrimSpace(string(output))) > 0 {
			result.Passed = false
			result.Message = "files need formatting"
			for _, line := range extractLines(string(output)) {
				if line != "" {
					result.Details = append(result.Details, fmt.Sprintf("%s needs gofmt", line))
				}
			}
		}
	}

	if len(jsFiles) > 0 {
		args := append([]string{"--check"}, jsFiles...)
		cmd := exec.Command("prettier", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			result.Passed = false
			result.Message = "files need formatting"
			result.Details = extractLines(string(out))
		}
	}

	return result
}

func checkLint(files []string) *CheckResult {
	result := &CheckResult{
		Passed:   true,
		Severity: "error",
		Message:  "no issues found",
	}

	goFiles := filterByExtension(files, ".go")
	jsFiles := filterByExtension(files, ".js", ".jsx")
	pyFiles := filterByExtension(files, ".py")

	if len(goFiles) > 0 {
		cmd := exec.Command("go", "vet", "./...")
		if out, err := cmd.CombinedOutput(); err != nil {
			result.Passed = false
			result.Message = fmt.Sprintf("%d issues found", countNonEmptyLines(string(out)))
			result.Details = extractLines(string(out))
		}
	}

	if len(jsFiles) > 0 {
		args := append([]string{"--format", "compact"}, jsFiles...)
		cmd := exec.Command("eslint", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			result.Passed = false
			issues := extractLines(string(out))
			result.Message = fmt.Sprintf("%d issues found", len(issues))
			result.Details = append(result.Details, issues...)
		}
	}

	if len(pyFiles) > 0 {
		args := append([]string{"--errors-only"}, pyFiles...)
		cmd := exec.Command("pylint", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			result.Passed = false
			issues := extractLines(string(out))
			result.Message = fmt.Sprintf("%d issues found", len(issues))
			result.Details = append(result.Details, issues...)
		}
	}

	return result
}

func checkTests(files []string) *CheckResult {
	result := &CheckResult{
		Passed:   true,
		Severity: "warning",
		Message:  "related tests pass",
	}

	goFiles := filterByExtension(files, ".go")
	if len(goFiles) == 0 {
		result.Message = "no testable files"
		return result
	}

	// Determine unique packages to test.
	packages := make(map[string]struct{})
	for _, f := range goFiles {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		dir := filepath.Dir(f)
		packages[dir] = struct{}{}
	}

	if len(packages) == 0 {
		result.Message = "no packages to test"
		return result
	}

	pkgList := make([]string, 0, len(packages))
	for pkg := range packages {
		pkgList = append(pkgList, "./"+pkg+"/...")
	}

	args := append([]string{"test", "-short", "-count=1"}, pkgList...)
	cmd := exec.Command("go", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		result.Passed = false
		result.Severity = "warning"
		result.Message = "some tests failed"
		result.Details = extractLines(string(out))
	}

	return result
}

func checkSecurity(files []string) *CheckResult {
	result := &CheckResult{
		Passed:   true,
		Severity: "error",
		Message:  "no secrets detected",
	}

	// Patterns that indicate potential secrets or dangerous code.
	secretPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(password|secret|token|api_key|apikey|private_key)\s*[:=]\s*["'][^"']{8,}["']`),
		regexp.MustCompile(`(?i)AKIA[0-9A-Z]{16}`),                   // AWS Access Key
		regexp.MustCompile(`(?i)-----BEGIN (RSA |EC )?PRIVATE KEY-----`), // Private key
		regexp.MustCompile(`(?i)(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{36,}`), // GitHub token
		regexp.MustCompile(`(?i)sk-[A-Za-z0-9]{20,}`),                 // OpenAI-style key
	}

	dangerousPatterns := []*regexp.Regexp{
		regexp.MustCompile(`os\.Remove(All)?\s*\(\s*"/`),
		regexp.MustCompile(`exec\.Command\s*\(\s*"rm"\s*,\s*"-rf"`),
		regexp.MustCompile(`eval\s*\(`),
	}

	for _, f := range files {
		lines := readFileLines(f)
		for lineNum, line := range lines {
			for _, pat := range secretPatterns {
				if pat.MatchString(line) {
					result.Passed = false
					result.Severity = "error"
					result.Message = "potential secrets detected"
					result.Details = append(result.Details,
						fmt.Sprintf("%s:%d: possible hardcoded secret", f, lineNum+1))
				}
			}
			for _, pat := range dangerousPatterns {
				if pat.MatchString(line) {
					result.Details = append(result.Details,
						fmt.Sprintf("%s:%d: dangerous pattern detected", f, lineNum+1))
					if result.Severity != "error" || result.Passed {
						result.Severity = "warning"
						result.Message = "dangerous patterns found"
					}
				}
			}
		}
	}

	return result
}

func checkSize(files []string) *CheckResult {
	result := &CheckResult{
		Passed:   true,
		Severity: "warning",
		Message:  "file sizes acceptable",
	}

	const maxNewLines = 500
	var largeFiles []string

	for _, f := range files {
		lines := readFileLines(f)
		if len(lines) > maxNewLines {
			largeFiles = append(largeFiles, fmt.Sprintf("%s adds %d lines (consider splitting)", f, len(lines)))
		}
	}

	if len(largeFiles) > 0 {
		result.Passed = false
		result.Message = "large files detected"
		result.Details = largeFiles
	}

	return result
}

// --- Auto-fix implementations ---

func autoFixFormat(check CheckResult) []string {
	var fixes []string

	for _, detail := range check.Details {
		if strings.Contains(detail, "needs gofmt") {
			// Extract filename.
			parts := strings.SplitN(detail, " ", 2)
			if len(parts) > 0 {
				file := parts[0]
				cmd := exec.Command("gofmt", "-w", file)
				if err := cmd.Run(); err == nil {
					fixes = append(fixes, fmt.Sprintf("applied gofmt to %s", file))
				} else {
					fixes = append(fixes, fmt.Sprintf("suggest: gofmt -w %s", file))
				}
			}
		} else if strings.Contains(detail, "prettier") || strings.Contains(detail, ".js") || strings.Contains(detail, ".ts") {
			fixes = append(fixes, "suggest: prettier --write <files>")
		}
	}

	if len(fixes) == 0 {
		fixes = append(fixes, "suggest: run formatter on affected files")
	}

	return fixes
}

func autoFixLint(check CheckResult) []string {
	var fixes []string

	for _, detail := range check.Details {
		if strings.Contains(detail, "unused") {
			fixes = append(fixes, fmt.Sprintf("suggest: remove unused declaration in %s", detail))
		} else if strings.Contains(detail, "import") {
			fixes = append(fixes, fmt.Sprintf("suggest: remove unused import in %s", detail))
		}
	}

	return fixes
}

// --- Utility functions ---

func filterByExtension(files []string, exts ...string) []string {
	var filtered []string
	for _, f := range files {
		ext := filepath.Ext(f)
		for _, e := range exts {
			if ext == e {
				filtered = append(filtered, f)
				break
			}
		}
	}
	return filtered
}

func extractLines(s string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func countNonEmptyLines(s string) int {
	count := 0
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	return count
}

func readFileLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(string(data), "\n")
}
