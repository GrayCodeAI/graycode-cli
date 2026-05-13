package engine

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LintLoop implements the "reflected_message" pattern from Aider: when an edit
// tool produces a lint/syntax error, the error is automatically fed back as
// context for the next iteration so the agent can self-correct.
type LintLoop struct {
	// MaxReflections is the maximum number of lint-fix retries per file per session.
	// Default is 3.
	MaxReflections int

	// LintCommands maps file extensions to lint commands. The placeholder {file}
	// in the command string is replaced with the actual file path.
	LintCommands map[string]string

	// Enabled controls whether the lint loop is active.
	Enabled bool

	// reflectionCounts tracks how many lint reflections have been issued per file
	// in the current session.
	reflectionCounts map[string]int
	mu               sync.Mutex
}

// LintResult holds the output of a lint command execution.
type LintResult struct {
	// File is the path that was linted.
	File string

	// Errors contains the individual error/warning lines from the linter.
	Errors []string

	// ExitCode is the process exit code (0 = pass, non-zero = failure).
	ExitCode int
}

// DefaultLintCommands returns the standard set of lint commands for common languages.
func DefaultLintCommands() map[string]string {
	return map[string]string{
		".go":   "go vet {file}",
		".py":   "python -m py_compile {file}",
		".ts":   "tsc --noEmit {file}",
		".tsx":  "tsc --noEmit {file}",
		".js":   "node --check {file}",
		".jsx":  "node --check {file}",
		".rs":   "rustc --edition 2021 --crate-type lib -Z parse-only {file}",
		".rb":   "ruby -c {file}",
		".sh":   "bash -n {file}",
		".bash": "bash -n {file}",
	}
}

// NewLintLoop creates a LintLoop with sensible defaults.
func NewLintLoop() *LintLoop {
	return &LintLoop{
		MaxReflections:   3,
		LintCommands:     DefaultLintCommands(),
		Enabled:          true,
		reflectionCounts: make(map[string]int),
	}
}

// RunLint executes the appropriate lint command for the given file path.
// It returns nil if no lint command is configured for the file type, or if
// the lint passes cleanly. Returns a LintResult with errors if lint fails.
func (ll *LintLoop) RunLint(path string) (*LintResult, error) {
	if !ll.Enabled {
		return nil, nil
	}

	ext := filepath.Ext(path)
	if ext == "" {
		return nil, nil
	}

	cmdTemplate, ok := ll.LintCommands[ext]
	if !ok {
		return nil, nil
	}

	// Replace {file} placeholder with the actual path
	cmdStr := strings.ReplaceAll(cmdTemplate, "{file}", path)

	// Parse command into executable and arguments
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return nil, fmt.Errorf("lint_loop: empty command for extension %s", ext)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		// Lint passed
		return nil, nil
	}

	// Lint failed — parse errors from output
	exitCode := 1
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}

	// Combine stdout and stderr for error output
	combined := stderr.String()
	if combined == "" {
		combined = stdout.String()
	}

	errors := parseLintErrors(combined)
	if len(errors) == 0 {
		// If we got an exit code but no parseable errors, include raw output
		errors = []string{strings.TrimSpace(combined)}
	}

	return &LintResult{
		File:     path,
		Errors:   errors,
		ExitCode: exitCode,
	}, nil
}

// BuildReflectedMessage formats a LintResult into a message the agent can understand
// and act upon in the next iteration.
func (ll *LintLoop) BuildReflectedMessage(result *LintResult) string {
	if result == nil || len(result.Errors) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("The edit to %s introduced lint errors:\n", result.File))
	for _, e := range result.Errors {
		b.WriteString(fmt.Sprintf("  %s\n", e))
	}
	b.WriteString("Please fix these issues.")
	return b.String()
}

// ShouldRetry returns true if the number of lint reflections for this file
// has not yet reached MaxReflections.
func (ll *LintLoop) ShouldRetry(reflectionCount int) bool {
	return reflectionCount < ll.MaxReflections
}

// RecordReflection increments the reflection counter for a file and returns
// the new count. Thread-safe.
func (ll *LintLoop) RecordReflection(path string) int {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	if ll.reflectionCounts == nil {
		ll.reflectionCounts = make(map[string]int)
	}
	ll.reflectionCounts[path]++
	return ll.reflectionCounts[path]
}

// ReflectionCount returns the current reflection count for a file. Thread-safe.
func (ll *LintLoop) ReflectionCount(path string) int {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	if ll.reflectionCounts == nil {
		return 0
	}
	return ll.reflectionCounts[path]
}

// ResetFile clears the reflection count for a specific file.
func (ll *LintLoop) ResetFile(path string) {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	delete(ll.reflectionCounts, path)
}

// Reset clears all reflection counts.
func (ll *LintLoop) Reset() {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	ll.reflectionCounts = make(map[string]int)
}

// parseLintErrors splits lint output into individual error lines, filtering
// out empty lines and common noise.
func parseLintErrors(output string) []string {
	lines := strings.Split(output, "\n")
	var errors []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Skip common noise lines
		if strings.HasPrefix(trimmed, "Compiling") ||
			strings.HasPrefix(trimmed, "Finished") ||
			strings.HasPrefix(trimmed, "warning: unused") {
			continue
		}
		errors = append(errors, trimmed)
	}
	return errors
}
