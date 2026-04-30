package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// PowerShellTool executes PowerShell commands (Windows/cross-platform pwsh).
type PowerShellTool struct{}

func (PowerShellTool) Name() string        { return "PowerShell" }
func (PowerShellTool) Aliases() []string   { return []string{"powershell"} }
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

	cmd := exec.CommandContext(ctx, shell, "-NoProfile", "-NonInteractive", "-Command", p.Command)
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
