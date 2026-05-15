package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// allowedGitSubcommands is the set of git subcommands the agent may run.
var allowedGitSubcommands = map[string]bool{
	"status":   true,
	"diff":     true,
	"log":      true,
	"show":     true,
	"branch":   true,
	"checkout": true,
	"add":      true,
	"commit":   true,
	"pull":     true,
	"push":     true,
	"fetch":    true,
	"stash":    true,
	"rebase":   true,
	"merge":    true,
	"reset":    true,
	"tag":      true,
}

// GitTool executes structured git commands. Inspired by herm's GitTool.
type GitTool struct {
	// WorkDir is the git repository working directory. If empty, uses CWD.
	WorkDir string
}

func (GitTool) Name() string      { return "Git" }
func (GitTool) RiskLevel() string { return "medium" }
func (GitTool) Aliases() []string { return []string{"git"} }
func (GitTool) Description() string {
	return "Run git commands in the project worktree. Supports: status, diff, log, show, branch, checkout, add, commit, pull, push, fetch, stash, rebase, merge, reset, tag."
}

func (GitTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"subcommand": map[string]interface{}{
				"type":        "string",
				"description": "Git subcommand to run (e.g. status, diff, add, commit)",
			},
			"args": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Arguments for the subcommand (e.g. [\"-m\", \"fix bug\"])",
			},
		},
		"required": []string{"subcommand"},
	}
}

func (t GitTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Subcommand string   `json:"subcommand"`
		Args       []string `json:"args,omitempty"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}

	if in.Subcommand == "" {
		return "", fmt.Errorf("subcommand is required")
	}

	if !allowedGitSubcommands[in.Subcommand] {
		return "", fmt.Errorf("git subcommand %q is not allowed", in.Subcommand)
	}

	args := append([]string{in.Subcommand}, in.Args...)
	cmd := exec.CommandContext(ctx, "git", args...)
	if t.WorkDir != "" {
		cmd.Dir = t.WorkDir
	}

	out, err := cmd.CombinedOutput()
	output := string(out)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Sprintf("exit code: %d\n%s", exitErr.ExitCode(), output), nil
		}
		return "", fmt.Errorf("git exec: %w", err)
	}

	return output, nil
}

func (t GitTool) RequiresApproval(input json.RawMessage) bool {
	var in struct {
		Subcommand string   `json:"subcommand"`
		Args       []string `json:"args,omitempty"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return false
	}

	if in.Subcommand == "push" {
		return true
	}
	if gitArgsContainForce(in.Args) {
		return true
	}
	if in.Subcommand == "reset" {
		for _, arg := range in.Args {
			if arg == "--hard" {
				return true
			}
		}
	}
	return false
}

// gitArgsContainForce checks if git args contain --force or -f.
func gitArgsContainForce(args []string) bool {
	for _, a := range args {
		if a == "--force" || a == "-f" || strings.HasPrefix(a, "--force=") {
			return true
		}
	}
	return false
}
