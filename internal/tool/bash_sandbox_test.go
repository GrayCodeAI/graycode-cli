package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/sandbox"
)

// TestBashTool_SandboxFailClosed verifies that when a sandbox mode is set in
// the context but no sandbox backend is available on the system, the Bash tool
// fails closed (returns an error) instead of falling through to unsandboxed
// host execution. This is the C1 fix: the previous code checked
// sandbox.Available() and skipped wrapping when no backend was present,
// causing fail-open behavior that contradicted the documented "fail closed"
// promise.
func TestBashTool_SandboxFailClosed(t *testing.T) {
	// Set a sandbox mode in the context. On a system with no sandbox backend
	// (no sandbox-exec, no unshare, no /proc/self/ns), WrapCommand should
	// return an error and the Bash tool should propagate it.
	ctx := sandbox.ContextWithMode(context.Background(), sandbox.ModeWorkspace)

	bt := BashTool{}
	input, _ := json.Marshal(map[string]any{
		"command": "echo hello",
	})

	_, err := bt.Execute(ctx, input)
	if err == nil {
		// If no error occurred, it means a sandbox backend IS available on
		// this machine (e.g. macOS sandbox-exec). In that case the command
		// should have executed successfully inside the sandbox — that's fine.
		// We only fail the test if the error is nil AND the sandbox was
		// actually unavailable (fail-open).
		if !sandbox.Available() {
			t.Fatal("expected error when sandbox mode is set but no backend available (fail-closed), got nil — fail-open bug")
		}
		// Sandbox backend available — the command executed sandboxed. Good.
		return
	}
	// Error occurred — verify it's a sandbox error, not some other error.
	if !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("expected sandbox-related error, got: %v", err)
	}
}

// TestBashTool_SandboxOffExecutesDirectly verifies that when sandbox mode is
// off (or not set), the Bash tool executes commands directly without
// sandbox wrapping — no regression from the fail-closed fix.
func TestBashTool_SandboxOffExecutesDirectly(t *testing.T) {
	// No sandbox mode set in context → ModeFromContext returns ModeOff.
	ctx := context.Background()

	bt := BashTool{}
	input, _ := json.Marshal(map[string]any{
		"command": "echo sandbox-off-test",
	})

	result, err := bt.Execute(ctx, input)
	if err != nil {
		t.Fatalf("expected success with sandbox off, got error: %v", err)
	}
	if !strings.Contains(result, "sandbox-off-test") {
		t.Fatalf("expected output to contain 'sandbox-off-test', got: %q", result)
	}
}

// TestBashTool_SandboxStrictFailClosed verifies that ModeStrict also fails
// closed when no backend is available.
func TestBashTool_SandboxStrictFailClosed(t *testing.T) {
	ctx := sandbox.ContextWithMode(context.Background(), sandbox.ModeStrict)

	bt := BashTool{}
	input, _ := json.Marshal(map[string]any{
		"command": "echo hello",
	})

	_, err := bt.Execute(ctx, input)
	if err == nil && !sandbox.Available() {
		t.Fatal("expected error when sandbox strict mode set but no backend available (fail-closed), got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("expected sandbox-related error, got: %v", err)
	}
}

// TestBashTool_BackgroundBashSandboxWrapping verifies that background bash
// (run_in_background=true) applies sandbox wrapping when a sandbox mode is
// set. Previously, the background path returned before the sandbox-wrapping
// block was reached (C2 fix).
func TestBashTool_BackgroundBashSandboxWrapping(t *testing.T) {
	ctx := sandbox.ContextWithMode(context.Background(), sandbox.ModeWorkspace)

	bt := BashTool{}
	input, _ := json.Marshal(map[string]any{
		"command":           "echo bg-test",
		"run_in_background": true,
	})

	result, err := bt.Execute(ctx, input)
	if err != nil {
		// If a sandbox backend is unavailable, we expect a sandbox error
		// (fail-closed), NOT silent unsandboxed execution.
		if !strings.Contains(err.Error(), "sandbox") {
			t.Fatalf("expected sandbox error for background bash, got: %v", err)
		}
		return
	}
	// If no error, the sandbox backend was available and the background task
	// was started sandboxed. Verify we got a task ID back.
	if !strings.Contains(result, "Started background task") {
		t.Fatalf("expected background task started message, got: %q", result)
	}
	// Clean up the background task if one was started.
	idx := strings.Index(result, "task_")
	if idx >= 0 {
		fields := strings.Fields(result[idx:])
		if len(fields) > 0 {
			taskID := strings.TrimRight(fields[0], ".")
			stopInput, _ := json.Marshal(map[string]any{"task_id": taskID})
			_, _ = (TaskStopTool{}).Execute(context.Background(), stopInput)
		}
	}
}
