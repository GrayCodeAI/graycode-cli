package validation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/fsutil"
)

// TestLoop implements an auto-test loop similar to Aider's auto_test feature.
// After edits, it automatically runs the project's test suite and feeds
// failures back to the agent for self-correction.
type TestLoop struct {
	// Enabled controls whether the test loop is active.
	Enabled bool

	// MaxRetries is the maximum number of test-fix retries per file.
	// Default is 3.
	MaxRetries int

	// Commands maps project type identifiers to test commands.
	// Auto-detected from project files if not explicitly set.
	Commands map[string]string

	// Timeout is the maximum duration allowed for a test run.
	// Default is 120s.
	Timeout time.Duration

	// retryCount tracks how many retries have been issued per file.
	retryCount map[string]int
	mu         sync.Mutex
}

// TestResult holds the structured output from a test run.
type TestResult struct {
	// Passed indicates whether all tests passed.
	Passed bool

	// Output is the combined stdout+stderr from the test command.
	Output string

	// FailedTests is a list of individual test names that failed.
	FailedTests []string

	// Duration is how long the test run took.
	Duration time.Duration

	// ExitCode is the process exit code (0 = pass).
	ExitCode int
}

// NewTestLoop creates a TestLoop with sensible defaults.
func NewTestLoop() *TestLoop {
	return &TestLoop{
		Enabled:    true,
		MaxRetries: 3,
		Commands:   DefaultTestCommands(),
		Timeout:    120 * time.Second,
		retryCount: make(map[string]int),
	}
}

// DefaultTestCommands returns the standard set of test commands for common project types.
func DefaultTestCommands() map[string]string {
	return map[string]string{
		"go":          "go test ./...",
		"python":      "pytest",
		"javascript":  "npm test",
		"typescript":  "npm test",
		"rust":        "cargo test",
		"ruby":        "bundle exec rspec",
		"java-gradle": "./gradlew test",
		"java-maven":  "mvn test",
	}
}

// DetectTestCommand inspects the project directory for known build/config files
// and returns the appropriate test command. Returns "" if no project type is detected.
func DetectTestCommand(projectDir string) string {
	// Go
	if fileExists(filepath.Join(projectDir, "go.mod")) {
		return "go test -count=1 ./..."
	}

	// Node.js / JavaScript / TypeScript
	pkgJSON := filepath.Join(projectDir, "package.json")
	if fileExists(pkgJSON) {
		cmd := readPackageJSONTestScript(pkgJSON)
		if cmd != "" {
			return cmd
		}
		return "npm test"
	}

	// Rust
	if fileExists(filepath.Join(projectDir, "Cargo.toml")) {
		return "cargo test"
	}

	// Python
	if fileExists(filepath.Join(projectDir, "pytest.ini")) ||
		fileExists(filepath.Join(projectDir, "setup.py")) ||
		fileExists(filepath.Join(projectDir, "pyproject.toml")) {
		return "pytest -x"
	}

	// Ruby
	if fileExists(filepath.Join(projectDir, "Gemfile")) {
		return "bundle exec rspec"
	}

	// Java (Gradle)
	if fileExists(filepath.Join(projectDir, "build.gradle")) ||
		fileExists(filepath.Join(projectDir, "build.gradle.kts")) {
		return "./gradlew test"
	}

	// Java (Maven)
	if fileExists(filepath.Join(projectDir, "pom.xml")) {
		return "mvn test -q"
	}

	return ""
}

// RunTests executes the detected or configured test command for the given project
// directory and returns a structured TestResult.
func (tl *TestLoop) RunTests(ctx context.Context, projectDir string) (*TestResult, error) {
	if !tl.Enabled {
		return nil, nil
	}

	cmdStr := DetectTestCommand(projectDir)
	if cmdStr == "" {
		return nil, nil
	}

	timeout := tl.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return nil, fmt.Errorf("test_loop: empty test command")
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...) // #nosec G204 -- command parsed from tool-configured command string (lint/test command)
	cmd.Dir = projectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	combined := stdout.String() + stderr.String()

	exitCode := 0
	passed := true
	if err != nil {
		passed = false
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	language := detectLanguage(projectDir)
	failedTests := ParseFailedTests(combined, language)

	return &TestResult{
		Passed:      passed,
		Output:      combined,
		FailedTests: failedTests,
		Duration:    duration,
		ExitCode:    exitCode,
	}, nil
}

// BuildTestFeedback formats test failures into agent-readable feedback that can
// be injected into the next agent iteration for self-correction.
func BuildTestFeedback(result *TestResult, editedFiles []string) string {
	if result == nil || result.Passed {
		return ""
	}

	var b strings.Builder

	filesStr := strings.Join(editedFiles, ", ")
	b.WriteString(fmt.Sprintf("Tests failed after your edits to %s:\n\n", filesStr))

	filtered := FilterRelevantOutput(result.Output, 2000)
	if filtered != "" {
		b.WriteString(filtered)
		b.WriteString("\n\n")
	}

	if len(result.FailedTests) > 0 {
		b.WriteString("Failed tests: ")
		b.WriteString(strings.Join(result.FailedTests, ", "))
		b.WriteString("\n\n")
	}

	b.WriteString("Please fix the failing tests.")
	return b.String()
}

// ShouldRetry returns true if the number of retries for the given file
// has not yet reached MaxRetries.
func (tl *TestLoop) ShouldRetry(file string) bool {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if tl.retryCount == nil {
		return true
	}
	return tl.retryCount[file] < tl.MaxRetries
}

// RecordRetry increments the retry counter for a file. Thread-safe.
func (tl *TestLoop) RecordRetry(file string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if tl.retryCount == nil {
		tl.retryCount = make(map[string]int)
	}
	tl.retryCount[file]++
}

// ResetFile clears the retry count for a specific file.
func (tl *TestLoop) ResetFile(file string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	delete(tl.retryCount, file)
}

// RetryCount returns the current retry count for a file. Thread-safe.
func (tl *TestLoop) RetryCount(file string) int {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if tl.retryCount == nil {
		return 0
	}
	return tl.retryCount[file]
}

// ParseFailedTests extracts failed test names from test output based on language.
func ParseFailedTests(output string, language string) []string {
	switch language {
	case "go":
		return parseGoFailedTests(output)
	case "python":
		return parsePythonFailedTests(output)
	case "javascript", "typescript":
		return parseJSFailedTests(output)
	case "rust":
		return parseRustFailedTests(output)
	default:
		return parseGenericFailedTests(output)
	}
}

// FilterRelevantOutput keeps only failure-related lines from test output,
// removes noise (progress bars, timing info, success messages), and truncates
// to maxChars.
func FilterRelevantOutput(output string, maxChars int) string {
	if output == "" {
		return ""
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	var relevant []string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines
		if trimmed == "" {
			continue
		}

		// Skip noise lines
		if isNoiseLine(trimmed) {
			continue
		}

		// Keep failure-related lines
		if isFailureLine(trimmed) {
			relevant = append(relevant, line)
		}
	}

	result := strings.Join(relevant, "\n")

	if maxChars > 0 && len(result) > maxChars {
		result = result[:maxChars]
		// Try to break at a newline to avoid cutting mid-line
		lastNewline := strings.LastIndex(result, "\n")
		if lastNewline > maxChars/2 {
			result = result[:lastNewline]
		}
		result += "\n... (truncated)"
	}

	return result
}

// --- internal helpers ---

func fileExists(path string) bool {
	return fsutil.Exists(path)
}

func readPackageJSONTestScript(path string) string {
	data, err := os.ReadFile(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return ""
	}

	var pkg struct {
		Scripts struct {
			Test string `json:"test"`
		} `json:"scripts"`
	}

	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}

	if pkg.Scripts.Test != "" && pkg.Scripts.Test != "echo \"Error: no test specified\" && exit 1" {
		return "npm test"
	}
	return ""
}

func detectLanguage(projectDir string) string {
	if fileExists(filepath.Join(projectDir, "go.mod")) {
		return "go"
	}
	if fileExists(filepath.Join(projectDir, "Cargo.toml")) {
		return "rust"
	}
	if fileExists(filepath.Join(projectDir, "package.json")) {
		// Check if TypeScript
		if fileExists(filepath.Join(projectDir, "tsconfig.json")) {
			return "typescript"
		}
		return "javascript"
	}
	if fileExists(filepath.Join(projectDir, "pytest.ini")) ||
		fileExists(filepath.Join(projectDir, "setup.py")) ||
		fileExists(filepath.Join(projectDir, "pyproject.toml")) {
		return "python"
	}
	if fileExists(filepath.Join(projectDir, "Gemfile")) {
		return "ruby"
	}
	if fileExists(filepath.Join(projectDir, "build.gradle")) ||
		fileExists(filepath.Join(projectDir, "pom.xml")) {
		return "java"
	}
	return ""
}

var goFailPattern = regexp.MustCompile(`--- FAIL: (\S+)`)

func parseGoFailedTests(output string) []string {
	matches := goFailPattern.FindAllStringSubmatch(output, -1)
	var tests []string
	for _, m := range matches {
		if len(m) > 1 {
			tests = append(tests, m[1])
		}
	}
	return tests
}

var pythonFailPattern = regexp.MustCompile(`(?m)^FAILED\s+(\S+)`)

func parsePythonFailedTests(output string) []string {
	matches := pythonFailPattern.FindAllStringSubmatch(output, -1)
	var tests []string
	for _, m := range matches {
		if len(m) > 1 {
			tests = append(tests, m[1])
		}
	}
	return tests
}

var jsFailPattern = regexp.MustCompile(`(?:✗|✕|FAIL)\s+(.+)`)

func parseJSFailedTests(output string) []string {
	matches := jsFailPattern.FindAllStringSubmatch(output, -1)
	var tests []string
	for _, m := range matches {
		if len(m) > 1 {
			name := strings.TrimSpace(m[1])
			if name != "" {
				tests = append(tests, name)
			}
		}
	}
	return tests
}

var rustFailPattern = regexp.MustCompile(`test (\S+) \.\.\. FAILED`)

func parseRustFailedTests(output string) []string {
	matches := rustFailPattern.FindAllStringSubmatch(output, -1)
	var tests []string
	for _, m := range matches {
		if len(m) > 1 {
			tests = append(tests, m[1])
		}
	}
	return tests
}

func parseGenericFailedTests(output string) []string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	var tests []string
	seen := make(map[string]bool)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.Contains(upper, "FAIL") || strings.Contains(upper, "ERROR") {
			// Use the line as the "test name" for generic output
			if !seen[line] && len(line) < 200 {
				seen[line] = true
				tests = append(tests, line)
			}
		}
	}
	return tests
}

func isNoiseLine(line string) bool {
	noisePatterns := []string{
		"ok  \t", // Go passing package lines
		"PASS",
		"passed",
		"passing",
		"✓",
		"✔",
		"Running ",
		"Compiling ",
		"Finished ",
		"Downloading ",
		"Downloaded ",
		"...",
		"test result: ok",
		"tests passed",
	}

	for _, pattern := range noisePatterns {
		if strings.Contains(line, pattern) {
			return true
		}
	}

	// Skip lines that are just timing info like "  (0.5s)"
	if matched, _ := regexp.MatchString(`^\s*\(\d+\.?\d*s?\)\s*$`, line); matched {
		return true
	}

	// Skip progress bars
	if strings.Count(line, "=") > 10 || strings.Count(line, "-") > 10 {
		if !strings.Contains(strings.ToUpper(line), "FAIL") {
			return true
		}
	}

	return false
}

func isFailureLine(line string) bool {
	failureIndicators := []string{
		"FAIL",
		"FAILED",
		"ERROR",
		"Error",
		"error",
		"panic:",
		"--- FAIL",
		"✗",
		"✕",
		"FATAL",
		"assertion",
		"expected",
		"got:",
		"want:",
		"!=",
		"not equal",
		"undefined",
		"cannot find",
		"no such",
		"traceback",
		"Traceback",
		"Exception",
		"exception",
	}

	for _, indicator := range failureIndicators {
		if strings.Contains(line, indicator) {
			return true
		}
	}
	return false
}
