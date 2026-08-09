package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DependencyAuditTool inspects dependency integrity and available updates. It
// never installs, upgrades, or edits dependency files; network access is only
// performed by the package manager when its native audit/outdated command
// requires it and remains subject to the session's network policy.
type DependencyAuditTool struct{}

func (DependencyAuditTool) Name() string      { return "DependencyAudit" }
func (DependencyAuditTool) RiskLevel() string { return "medium" }
func (DependencyAuditTool) Aliases() []string { return []string{"dependency-audit", "deps"} }
func (DependencyAuditTool) Description() string {
	return "Audit dependency integrity and report outdated packages without installing or changing anything. Supports Go, npm, Python, and Cargo projects with structured results."
}

func (DependencyAuditTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"check", "outdated", "all"},
				"description": "check validates dependency integrity; outdated reports available updates; all runs both.",
			},
			"path":            map[string]interface{}{"type": "string", "description": "Project directory (default: session working directory)."},
			"timeout_seconds": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 300, "description": "Per-command timeout (default 60 seconds)."},
		},
		"required": []string{"action"},
	}
}

func (DependencyAuditTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Action         string `json:"action"`
		Path           string `json:"path"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	params.Action = strings.ToLower(strings.TrimSpace(params.Action))
	if params.Action != "check" && params.Action != "outdated" && params.Action != "all" {
		return "", fmt.Errorf("unsupported action %q (use check, outdated, or all)", params.Action)
	}
	if params.TimeoutSeconds <= 0 {
		params.TimeoutSeconds = 60
	}
	if params.TimeoutSeconds > 300 {
		params.TimeoutSeconds = 300
	}
	root := params.Path
	if root == "" {
		if tc := GetToolContext(ctx); tc != nil && tc.WorkingDir != "" {
			root = tc.WorkingDir
		} else {
			root, _ = os.Getwd()
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}
	if err := validatePathAllowed(ctx, root); err != nil {
		return "", err
	}
	stack := detectProjectStack(root)
	commands := dependencyCommands(stack, params.Action)
	results := make([]verificationResult, 0, len(commands))
	for _, command := range commands {
		results = append(results, runVerificationCommand(ctx, root, command, time.Duration(params.TimeoutSeconds)*time.Second))
	}
	if results == nil {
		results = []verificationResult{}
	}
	return encodeJSON(map[string]interface{}{"project": stack, "results": results})
}

func dependencyCommands(stack projectStack, action string) []verificationCommand {
	var result []verificationCommand
	add := func(c verificationCommand) {
		if _, err := exec.LookPath(c.bin); err == nil {
			result = append(result, c)
		}
	}
	for _, phase := range []string{"check", "outdated"} {
		if action != "all" && action != phase {
			continue
		}
		switch {
		case containsString(stack.Stacks, "go"):
			if phase == "check" {
				add(verificationCommand{action: phase, args: []string{"mod", "verify"}, label: "go mod verify", bin: "go"})
			} else {
				add(verificationCommand{action: phase, args: []string{"list", "-m", "-u", "all"}, label: "go list -m -u all", bin: "go"})
			}
		case containsString(stack.Stacks, "node"):
			if phase == "check" {
				add(verificationCommand{action: phase, args: []string{"audit", "--omit=dev", "--json"}, label: "npm audit --omit=dev --json", bin: "npm"})
			} else {
				add(verificationCommand{action: phase, args: []string{"outdated", "--json"}, label: "npm outdated --json", bin: "npm"})
			}
		case containsString(stack.Stacks, "python"):
			if phase == "check" {
				add(verificationCommand{action: phase, args: []string{"-m", "pip", "check"}, label: "python3 -m pip check", bin: "python3"})
			} else if _, err := exec.LookPath("pip-audit"); err == nil {
				add(verificationCommand{action: phase, args: []string{"-f", "json"}, label: "pip-audit -f json", bin: "pip-audit"})
			}
		case containsString(stack.Stacks, "rust"):
			if phase == "check" {
				if _, err := exec.LookPath("cargo"); err == nil {
					add(verificationCommand{action: phase, args: []string{"tree", "--edges", "normal"}, label: "cargo tree --edges normal", bin: "cargo"})
				}
			} else if _, err := exec.LookPath("cargo"); err == nil {
				add(verificationCommand{action: phase, args: []string{"update", "--dry-run"}, label: "cargo update --dry-run", bin: "cargo"})
			}
		}
	}
	return result
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
