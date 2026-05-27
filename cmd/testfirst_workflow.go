package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// TestFirstConfig controls the test-first fix workflow.
type TestFirstConfig struct {
	TestCmd     string // test command to run (e.g. "go test ./...")
	MaxRounds   int    // maximum fix iterations before giving up
	FailPattern string // pattern to match in test output for failures
}

// DefaultTestFirstConfig returns sensible defaults for Go projects.
func DefaultTestFirstConfig() TestFirstConfig {
	return TestFirstConfig{
		TestCmd:     "go test ./...",
		MaxRounds:   5,
		FailPattern: "FAIL",
	}
}

// TestFirstResult holds the outcome of a test-first workflow run.
type TestFirstResult struct {
	Rounds      int
	FinalOutput string
	Passed      bool
	FixPrompts  []string // prompts sent to the LLM for each round
}

// RunTestFirstWorkflow executes tests, then feeds failures as context to an
// LLM fix cycle. Each round runs tests, checks for failures, and if failures
// exist, asks the LLM to fix them. Returns when tests pass or max rounds hit.
func RunTestFirstWorkflow(cfg TestFirstConfig, chatFn ReviewChatFn) TestFirstResult {
	if cfg.MaxRounds <= 0 {
		cfg.MaxRounds = 5
	}
	if cfg.TestCmd == "" {
		cfg.TestCmd = "go test ./..."
	}

	result := TestFirstResult{}

	for round := 0; round < cfg.MaxRounds; round++ {
		result.Rounds = round + 1

		// Run tests
		output, passed := runTests(cfg.TestCmd)
		result.FinalOutput = output

		if passed {
			result.Passed = true
			return result
		}

		// Build fix prompt from failures
		prompt := buildTestFixPrompt(output, round+1, cfg.MaxRounds)
		result.FixPrompts = append(result.FixPrompts, prompt)

		if chatFn == nil {
			// No LLM available; stop after first failure
			return result
		}

		// Ask LLM to fix
		response, err := chatFn(nil, prompt)
		if err != nil {
			result.FinalOutput = fmt.Sprintf("LLM error on round %d: %v\n\nTest output:\n%s", round+1, err, output)
			return result
		}

		// The LLM response is the fix instruction. In a full integration,
		// this would apply code changes. Here we report what was suggested.
		_ = response
	}

	// Max rounds exhausted
	result.FinalOutput = fmt.Sprintf("Exceeded %d fix rounds. Last test output:\n%s", cfg.MaxRounds, result.FinalOutput)
	return result
}

// runTests executes the test command and returns output + pass status.
func runTests(testCmd string) (string, bool) {
	cmd := exec.Command("sh", "-c", testCmd)
	cmd.Dir, _ = os.Getwd()
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return output, false
	}
	return output, true
}

// buildTestFixPrompt creates a prompt for the LLM to fix test failures.
func buildTestFixPrompt(testOutput string, round, maxRounds int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Tests failed (round %d/%d). Fix the failures:\n\n", round, maxRounds))
	b.WriteString("```\n")
	// Truncate very long test output
	if len(testOutput) > 4000 {
		lines := strings.Split(testOutput, "\n")
		if len(lines) > 80 {
			testOutput = strings.Join(lines[:40], "\n") +
				fmt.Sprintf("\n... (%d lines omitted) ...\n", len(lines)-80) +
				strings.Join(lines[len(lines)-40:], "\n")
		}
	}
	b.WriteString(testOutput)
	b.WriteString("\n```\n\n")
	b.WriteString("Fix the failing tests. Make minimal changes. Do not remove tests — fix the code they test.")
	return b.String()
}
