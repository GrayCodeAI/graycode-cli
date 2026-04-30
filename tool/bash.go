package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type BashTool struct{}

func (BashTool) Name() string        { return "bash" }
func (BashTool) Description() string { return "Run a shell command. Use for running tests, installing packages, or any shell operation." }
func (BashTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{"type": "string", "description": "The shell command to run"},
			"timeout": map[string]interface{}{"type": "integer", "description": "Timeout in seconds (default 30)"},
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
	timeout := time.Duration(p.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", p.Command)
	out, err := cmd.CombinedOutput()
	result := strings.TrimRight(string(out), "\n")
	if err != nil {
		return fmt.Sprintf("%s\n\nexit code: %s", result, err.Error()), nil
	}
	return result, nil
}
