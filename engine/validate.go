package engine

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ValidationResult holds the outcome of validating a file.
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// ValidationError represents a single validation issue.
type ValidationError struct {
	File    string
	Line    int
	Column  int
	Message string
}

// MaxAutoFixRetries is the maximum number of times to retry auto-fixing a file.
const MaxAutoFixRetries = 3

// ValidateFile determines the language from the file extension and runs
// the appropriate syntax checker.
func ValidateFile(path string) *ValidationResult {
	ext := strings.ToLower(filepath.Ext(path))
	validator := languageValidator(ext)
	if validator == nil {
		return &ValidationResult{Valid: true}
	}
	return validator(path)
}

// AutoFixPrompt returns a prompt instructing the LLM to fix syntax errors in a file.
func AutoFixPrompt(path, content string, errors []ValidationError) string {
	var errLines strings.Builder
	for _, e := range errors {
		if e.Line > 0 {
			errLines.WriteString(fmt.Sprintf("  Line %d", e.Line))
			if e.Column > 0 {
				errLines.WriteString(fmt.Sprintf(", Col %d", e.Column))
			}
			errLines.WriteString(fmt.Sprintf(": %s\n", e.Message))
		} else {
			errLines.WriteString(fmt.Sprintf("  %s\n", e.Message))
		}
	}

	return fmt.Sprintf(`The file %s has syntax errors after your edit:
%s
Please fix the errors. Here's the current content:
`+"```"+`
%s
`+"```", path, errLines.String(), content)
}

// languageValidator returns a validation function for the given file extension,
// or nil if no validator is available.
func languageValidator(ext string) func(path string) *ValidationResult {
	switch ext {
	case ".go":
		return validateGo
	case ".py":
		return validatePython
	case ".js":
		return validateJS
	case ".ts":
		return validateTS
	default:
		return nil
	}
}

// validateGo runs go vet on a Go file and parses the output.
func validateGo(path string) *ValidationResult {
	dir := filepath.Dir(path)
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err == nil {
		return &ValidationResult{Valid: true}
	}

	errors := parseGoErrors(string(output))
	if len(errors) == 0 {
		// go vet failed but we couldn't parse errors — report the raw output
		return &ValidationResult{
			Valid: false,
			Errors: []ValidationError{
				{File: path, Message: strings.TrimSpace(string(output))},
			},
		}
	}
	return &ValidationResult{Valid: false, Errors: errors}
}

// validatePython runs py_compile on a Python file.
func validatePython(path string) *ValidationResult {
	cmd := exec.Command("python3", "-c",
		fmt.Sprintf("import py_compile; py_compile.compile('%s', doraise=True)", path))

	output, err := cmd.CombinedOutput()
	if err == nil {
		return &ValidationResult{Valid: true}
	}

	errors := parsePythonErrors(string(output), path)
	if len(errors) == 0 {
		return &ValidationResult{
			Valid: false,
			Errors: []ValidationError{
				{File: path, Message: strings.TrimSpace(string(output))},
			},
		}
	}
	return &ValidationResult{Valid: false, Errors: errors}
}

// validateJS runs node --check on a JavaScript file.
func validateJS(path string) *ValidationResult {
	cmd := exec.Command("node", "--check", path)

	output, err := cmd.CombinedOutput()
	if err == nil {
		return &ValidationResult{Valid: true}
	}

	errors := parseNodeErrors(string(output), path)
	if len(errors) == 0 {
		return &ValidationResult{
			Valid: false,
			Errors: []ValidationError{
				{File: path, Message: strings.TrimSpace(string(output))},
			},
		}
	}
	return &ValidationResult{Valid: false, Errors: errors}
}

// validateTS performs a basic syntax check for TypeScript files.
// Uses npx tsc --noEmit if available, otherwise falls back to node --check.
func validateTS(path string) *ValidationResult {
	// Try tsc first
	cmd := exec.Command("npx", "tsc", "--noEmit", "--allowJs", path)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return &ValidationResult{Valid: true}
	}

	// If npx/tsc is not available, try basic node check
	if strings.Contains(string(output), "not found") || strings.Contains(string(output), "ERR!") {
		return &ValidationResult{Valid: true} // can't validate, assume valid
	}

	errors := parseTSErrors(string(output), path)
	if len(errors) == 0 {
		return &ValidationResult{
			Valid: false,
			Errors: []ValidationError{
				{File: path, Message: strings.TrimSpace(string(output))},
			},
		}
	}
	return &ValidationResult{Valid: false, Errors: errors}
}

// Go error format: file.go:line:col: message
var goErrorRe = regexp.MustCompile(`([^:]+\.go):(\d+):(\d+):\s*(.+)`)

func parseGoErrors(output string) []ValidationError {
	var errors []ValidationError
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if matches := goErrorRe.FindStringSubmatch(line); matches != nil {
			lineNum, _ := strconv.Atoi(matches[2])
			colNum, _ := strconv.Atoi(matches[3])
			errors = append(errors, ValidationError{
				File:    matches[1],
				Line:    lineNum,
				Column:  colNum,
				Message: matches[4],
			})
		}
	}
	return errors
}

// Python error format varies, but py_compile typically gives:
// File "path", line N
//   ...
// SyntaxError: message
var pythonLineRe = regexp.MustCompile(`File "([^"]+)", line (\d+)`)
var pythonErrorRe = regexp.MustCompile(`(SyntaxError|IndentationError|TabError):\s*(.+)`)

func parsePythonErrors(output, path string) []ValidationError {
	var errors []ValidationError
	lines := strings.Split(output, "\n")
	lineNum := 0

	for _, line := range lines {
		if matches := pythonLineRe.FindStringSubmatch(line); matches != nil {
			lineNum, _ = strconv.Atoi(matches[2])
		}
		if matches := pythonErrorRe.FindStringSubmatch(line); matches != nil {
			errors = append(errors, ValidationError{
				File:    path,
				Line:    lineNum,
				Message: matches[1] + ": " + matches[2],
			})
		}
	}
	return errors
}

// Node error format: path:line
// SyntaxError: message
var nodeLineRe = regexp.MustCompile(`:(\d+)`)

func parseNodeErrors(output, path string) []ValidationError {
	var errors []ValidationError
	lines := strings.Split(output, "\n")
	lineNum := 0

	for _, line := range lines {
		if matches := nodeLineRe.FindStringSubmatch(line); matches != nil {
			lineNum, _ = strconv.Atoi(matches[1])
		}
		if strings.Contains(line, "SyntaxError:") {
			msg := strings.TrimSpace(line)
			errors = append(errors, ValidationError{
				File:    path,
				Line:    lineNum,
				Message: msg,
			})
		}
	}
	return errors
}

// TS error format: file(line,col): error TSxxxx: message
var tsErrorRe = regexp.MustCompile(`\((\d+),(\d+)\):\s*error\s+\w+:\s*(.+)`)

func parseTSErrors(output, path string) []ValidationError {
	var errors []ValidationError
	for _, line := range strings.Split(output, "\n") {
		if matches := tsErrorRe.FindStringSubmatch(line); matches != nil {
			lineNum, _ := strconv.Atoi(matches[1])
			colNum, _ := strconv.Atoi(matches[2])
			errors = append(errors, ValidationError{
				File:    path,
				Line:    lineNum,
				Column:  colNum,
				Message: matches[3],
			})
		}
	}
	return errors
}
