package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/sandbox"
)

func TestPowerShellTool_EmptyCommand(t *testing.T) {
	ps := PowerShellTool{}
	_, err := ps.Execute(context.Background(), json.RawMessage(`{"command":""}`))
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestPowerShellTool_DestructiveBlocked(t *testing.T) {
	ps := PowerShellTool{}
	_, err := ps.Execute(context.Background(), json.RawMessage(`{"command":"rm -rf /"}`))
	if err == nil || err.Error() != "command blocked: contains a destructive pattern" {
		t.Fatalf("expected destructive block error, got: %v", err)
	}
}

func TestPowerShellTool_SuspiciousBlocked(t *testing.T) {
	ps := PowerShellTool{}
	_, err := ps.Execute(context.Background(), json.RawMessage(`{"command":"eval bad"}`))
	if err == nil || err.Error() != "command blocked: flagged as suspicious" {
		t.Fatalf("expected suspicious block error, got: %v", err)
	}
}

func TestPowerShellTool_Name(t *testing.T) {
	ps := PowerShellTool{}
	if ps.Name() != "PowerShell" {
		t.Fatalf("expected PowerShell, got %s", ps.Name())
	}
}

// TestPowerShellTool_SandboxFailClosed verifies that when a sandbox mode is
// set but no backend is available, PowerShell commands fail closed with a
// sandbox error instead of running unsandboxed on the host (C3 fix).
func TestPowerShellTool_SandboxFailClosed(t *testing.T) {
	// Skip if pwsh is not installed — the sandbox check happens after the
	// pwsh-availability check, so we can't test it without pwsh.
	if findPowerShell() == "" {
		// TODO: track pwsh availability on Linux/macOS CI runners.
		t.Skip("pwsh not installed")
	}
	ctx := sandbox.ContextWithMode(context.Background(), sandbox.ModeWorkspace)

	ps := PowerShellTool{}
	input, _ := json.Marshal(map[string]any{
		"command": "Write-Output hello",
	})

	_, err := ps.Execute(ctx, input)
	if err == nil {
		// If no error, a sandbox backend is available on this system.
		if !sandbox.Available() {
			t.Fatal("expected error when sandbox mode set but no backend available (fail-closed), got nil — fail-open bug")
		}
		return
	}
	if !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("expected sandbox-related error, got: %v", err)
	}
}

// TestPowerShellTool_SandboxOffExecutes verifies no regression when sandbox
// is off.
func TestPowerShellTool_SandboxOffExecutes(t *testing.T) {
	// Skip if pwsh is not installed.
	if findPowerShell() == "" {
		// TODO: track pwsh availability on Linux/macOS CI runners.
		t.Skip("pwsh not installed")
	}
	ctx := context.Background()

	ps := PowerShellTool{}
	input, _ := json.Marshal(map[string]any{
		"command": "Write-Output ps-test",
	})

	result, err := ps.Execute(ctx, input)
	if err != nil {
		t.Fatalf("expected success with sandbox off, got error: %v", err)
	}
	if !strings.Contains(result, "ps-test") {
		t.Fatalf("expected output to contain 'ps-test', got: %q", result)
	}
}
