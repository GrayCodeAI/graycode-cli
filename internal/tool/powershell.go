package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/sandbox"
)

// PowerShellTool executes PowerShell commands (Windows/cross-platform pwsh).
type PowerShellTool struct{}

func (PowerShellTool) Name() string      { return "PowerShell" }
func (PowerShellTool) RiskLevel() string { return "high" }
func (PowerShellTool) Aliases() []string { return []string{"powershell"} }
func (PowerShellTool) Description() string {
	return "Execute a PowerShell command. Use this instead of Bash when running on Windows or when PowerShell-specific cmdlets are needed."
}

func (PowerShellTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The PowerShell command to execute",
			},
			"timeout": map[string]interface{}{
				"type":        "number",
				"description": "Timeout in milliseconds (max 600000, default 120000)",
			},
		},
		"required": []string{"command"},
	}
}

func (PowerShellTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Command string `json:"command"`
		Timeout int64  `json:"timeout"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Command == "" {
		return "", fmt.Errorf("command is required")
	}
	// Safety: check for destructive commands (same as Bash tool)
	if IsDestructiveCommand(p.Command) {
		return "", fmt.Errorf("command blocked: contains a destructive pattern")
	}
	if IsSuspicious(p.Command) {
		return "", fmt.Errorf("command blocked: flagged as suspicious")
	}

	timeout := 120 * time.Second
	if p.Timeout > 0 {
		if p.Timeout > 600_000 {
			p.Timeout = 600_000
		}
		timeout = time.Duration(p.Timeout) * time.Millisecond
	}

	shell := findPowerShell()
	if shell == "" {
		return "", fmt.Errorf("PowerShell not found (install pwsh for cross-platform support)")
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Sandbox wrapping: same fail-closed behavior as the Bash tool. When a
	// sandbox mode is configured, wrap the command with the platform sandbox.
	// If no backend is available, fail closed with an error instead of
	// running unsandboxed on the host (C3 fix).
	execName := shell
	execArgs := []string{"-NoProfile", "-NonInteractive", "-Command", p.Command}
	if sbMode := sandbox.ModeFromContext(ctx); sbMode != sandbox.ModeOff {
		workDir, _ := os.Getwd()
		cfg := sandbox.SandboxConfig{Mode: sbMode, WorkspaceDir: workDir, AllowNetwork: sandbox.ModeAllowsNetwork(sbMode)}
		switch sbMode {
		case sandbox.ModeStrict:
			cfg.Security = sandbox.SecurityStrict
		case sandbox.ModeWorkspace:
			cfg.Security = sandbox.SecurityWorkspace
		}
		// WrapCommand wraps the command as "bash -c <full command>", so we
		// pass the full pwsh invocation as the command string. The sandbox
		// isolates the bash process, which in turn launches pwsh.
		pwshInvocation := fmt.Sprintf("%s -NoProfile -NonInteractive -Command %s", shell, p.Command)
		var wrapErr error
		execName, execArgs, wrapErr = sandbox.WrapCommand(pwshInvocation, cfg)
		if wrapErr != nil {
			return "", fmt.Errorf("sandbox unavailable (mode=%s): %w", sbMode, wrapErr)
		}
	}

	cmd := exec.CommandContext(ctx, execName, execArgs...) // #nosec G204 -- shell invocation with user-supplied command, sandbox-wrapped when mode is set
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := stdout.String()
	if stderr.Len() > 0 {
		if result != "" {
			result += "\n"
		}
		result += stderr.String()
	}

	if ctx.Err() == context.DeadlineExceeded {
		return result + "\n(command timed out)", nil
	}
	if err != nil && result == "" {
		return "", fmt.Errorf("powershell error: %w", err)
	}

	const maxOutput = 200_000
	if len(result) > maxOutput {
		half := maxOutput / 2
		result = result[:half] + "\n...(output truncated)...\n" + result[len(result)-half:]
	}

	return strings.TrimRight(result, "\n"), nil
}

func findPowerShell() string {
	// Prefer pwsh (PowerShell Core / cross-platform)
	if path, err := exec.LookPath("pwsh"); err == nil {
		return path
	}
	// Fall back to Windows PowerShell
	if runtime.GOOS == "windows" {
		if path, err := exec.LookPath("powershell.exe"); err == nil {
			return path
		}
	}
	return ""
}

// IsPowerShellAvailable returns whether a PowerShell runtime is available.
func IsPowerShellAvailable() bool {
	return findPowerShell() != ""
}
