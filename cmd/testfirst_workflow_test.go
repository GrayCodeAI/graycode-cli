package cmd

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultTestFirstConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultTestFirstConfig()
	if cfg.TestCmd == "" {
		t.Error("default test command should not be empty")
	}
	if cfg.MaxRounds <= 0 {
		t.Error("default max rounds should be positive")
	}
	if cfg.TestCmd != "go test ./..." {
		t.Errorf("expected 'go test ./...', got %q", cfg.TestCmd)
	}
}

func TestBuildTestFixPrompt(t *testing.T) {
	t.Parallel()
	prompt := buildTestFixPrompt("FAIL: TestFoo\n--- FAIL: TestBar", 1, 5)
	if !strings.Contains(prompt, "round 1/5") {
		t.Error("prompt should mention round number")
	}
	if !strings.Contains(prompt, "TestFoo") {
		t.Error("prompt should contain test output")
	}
	if !strings.Contains(prompt, "minimal changes") {
		t.Error("prompt should instruct minimal changes")
	}
}

func TestBuildTestFixPrompt_TruncatesLongOutput(t *testing.T) {
	t.Parallel()
	// Create output longer than 4000 chars
	longOutput := strings.Repeat("FAIL: some test output\n", 300)
	prompt := buildTestFixPrompt(longOutput, 2, 3)
	if !strings.Contains(prompt, "omitted") {
		t.Error("long output should be truncated")
	}
}

func TestRunTestFirstWorkflow_NilChatFn(t *testing.T) {
	t.Parallel()
	// With nil chatFn, should stop after first failure detection
	cfg := TestFirstConfig{
		TestCmd:   "echo 'test output'",
		MaxRounds: 3,
	}
	// This will actually run the command, which passes (exit 0)
	result := RunTestFirstWorkflow(cfg, nil)
	if !result.Passed {
		t.Error("echo command should pass")
	}
	if result.Rounds != 1 {
		t.Errorf("expected 1 round, got %d", result.Rounds)
	}
}

func TestRunTestFirstWorkflow_WithMockLLM(t *testing.T) {
	t.Parallel()
	callCount := 0
	mockChat := func(_ context.Context, prompt string) (string, error) {
		callCount++
		if !strings.Contains(prompt, "Fix") {
			t.Error("prompt should ask to fix failures")
		}
		return "Fixed the issue", nil
	}

	// Use a command that always fails
	cfg := TestFirstConfig{
		TestCmd:   "false", // always exits 1
		MaxRounds: 2,
	}
	result := RunTestFirstWorkflow(cfg, mockChat)
	if result.Passed {
		t.Error("false command should not pass")
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls, got %d", callCount)
	}
	if len(result.FixPrompts) != 2 {
		t.Errorf("expected 2 fix prompts, got %d", len(result.FixPrompts))
	}
}

func TestRunTestFirstWorkflow_PassesFirst(t *testing.T) {
	t.Parallel()
	mockChat := func(_ context.Context, _ string) (string, error) {
		t.Error("LLM should not be called when tests pass")
		return "", nil
	}

	cfg := TestFirstConfig{
		TestCmd:   "true", // always exits 0
		MaxRounds: 3,
	}
	result := RunTestFirstWorkflow(cfg, mockChat)
	if !result.Passed {
		t.Error("true command should pass")
	}
	if result.Rounds != 1 {
		t.Errorf("expected 1 round, got %d", result.Rounds)
	}
}
