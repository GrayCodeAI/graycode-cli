package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// dangerousCommands are commands that should ALWAYS be blocked.
var dangerousCommands = map[string]bool{
	"rm": true, "rmdir": true, "mkfs": true, "dd": true,
	"shred": true, "wipefs": true,
}

// dangerousPatterns catches structural patterns that bypass simple word matching.
var dangerousSubstrings = []string{
	"rm -rf /", "rm -rf ~", "rm -rf .",
	":(){ :|:& };:", // fork bomb
	"chmod -r 777 /",
	"> /dev/sd", "> /dev/nv",
}

// suspiciousPatterns indicate commands that need extra scrutiny (force permission prompt).
var suspiciousPatterns = []string{
	"eval ", "exec ", "$(",  "`",     // command substitution / eval
	"| sh", "| bash", "| zsh",       // pipe to shell
	"|sh", "|bash", "|zsh",          // pipe to shell (no space)
	"sudo ", "su -",                  // privilege escalation
	"curl ", "wget ",                 // network downloads (when piped)
	"> /", ">> /",                    // writing to absolute paths
	"git push --force", "git reset --hard",
	"DROP ", "DELETE FROM", "TRUNCATE ", // SQL
}

type BashTool struct{}

func (BashTool) Name() string        { return "bash" }
func (BashTool) Description() string { return "Run a shell command." }
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

// IsSuspicious returns true if the command needs a permission prompt.
// This is fail-closed: anything we can't confidently classify as safe gets flagged.
func IsSuspicious(command string) bool {
	lower := strings.ToLower(command)

	// Check dangerous substrings
	for _, pat := range dangerousSubstrings {
		if strings.Contains(lower, pat) {
			return true
		}
	}

	// Check suspicious patterns
	for _, pat := range suspiciousPatterns {
		if strings.Contains(lower, strings.ToLower(pat)) {
			return true
		}
	}

	// Check if first command word is dangerous
	words := strings.Fields(command)
	if len(words) > 0 {
		base := words[0]
		// Strip path prefix
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		// Strip leading backslash (bypass attempt)
		base = strings.TrimLeft(base, "\\")
		if dangerousCommands[base] {
			return true
		}
	}

	// Multi-command detection: ;, &&, || with dangerous commands
	for _, sep := range []string{";", "&&", "||"} {
		parts := strings.Split(command, sep)
		for _, part := range parts {
			part = strings.TrimSpace(part)
			w := strings.Fields(part)
			if len(w) > 0 {
				base := strings.TrimLeft(w[0], "\\/")
				if dangerousCommands[base] {
					return true
				}
			}
		}
	}

	return false
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

	// Hard block: always-dangerous patterns
	lower := strings.ToLower(p.Command)
	for _, pat := range dangerousSubstrings {
		if strings.Contains(lower, pat) {
			return "", fmt.Errorf("blocked: dangerous command pattern detected")
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
