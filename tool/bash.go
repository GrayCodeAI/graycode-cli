package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Dangerous command patterns that should never run without explicit user request.
var dangerousPatterns = []string{
	"rm -rf /", "rm -rf ~", "rm -rf .",
	"mkfs", "dd if=", "> /dev/",
	":(){ :|:& };:", // fork bomb
	"chmod -R 777 /",
	"curl|sh", "curl|bash", "wget|sh", "wget|bash",
}

type BashTool struct{}

func (BashTool) Name() string        { return "bash" }
func (BashTool) Description() string { return "Run a shell command. Use for running tests, installing packages, or any shell operation." }
func (BashTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{"type": "string", "description": "The shell command to run"},
			"timeout": map[string]interface{}{"type": "integer", "description": "Timeout in seconds (default 120)"},
		},
		"required": []string{"command"},
	}
}

func (BashTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Command == "" {
		return "", fmt.Errorf("command is required")
	}

	// Security check
	lower := strings.ToLower(p.Command)
	for _, pat := range dangerousPatterns {
		if strings.Contains(lower, pat) {
			return "", fmt.Errorf("blocked dangerous command pattern: %s", pat)
		}
	}

	timeout := time.Duration(p.Timeout) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", p.Command)
	out, err := cmd.CombinedOutput()
	result := string(out)

	// Cap output size
	if len(result) > 100000 {
		result = result[:100000] + "\n... (output truncated at 100KB)"
	}
	result = strings.TrimRight(result, "\n")

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result + "\n\n(command timed out)", nil
		}
		return fmt.Sprintf("%s\n\nexit code: %s", result, err.Error()), nil
	}
	return result, nil
}
