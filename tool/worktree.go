package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EnterWorktreeTool switches to a git worktree.
type EnterWorktreeTool struct{}

func (EnterWorktreeTool) Name() string      { return "EnterWorktree" }
func (EnterWorktreeTool) Aliases() []string { return nil }
func (EnterWorktreeTool) Description() string {
	return "Switch to a git worktree directory."
}
func (EnterWorktreeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{"type": "string", "description": "Path to the worktree directory"},
		},
		"required": []string{"path"},
	}
}

func (EnterWorktreeTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	// Validate it's a git worktree
	gitDir := filepath.Join(p.Path, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return "", fmt.Errorf("%s is not a git worktree (no .git directory)", p.Path)
	}

	// Check if it's actually a worktree (not the main repo)
	out, err := exec.CommandContext(ctx, "git", "-C", p.Path, "rev-parse", "--git-dir").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("not a valid git repository: %s", string(out))
	}
	gitDirPath := strings.TrimSpace(string(out))

	// Check if it's a worktree (git dir will be different from .git)
	if gitDirPath == ".git" {
		return "", fmt.Errorf("%s appears to be the main repository, not a worktree", p.Path)
	}

	// Change to the worktree directory
	if err := os.Chdir(p.Path); err != nil {
		return "", fmt.Errorf("failed to change to worktree: %w", err)
	}

	return fmt.Sprintf("Switched to worktree: %s", p.Path), nil
}

// ExitWorktreeTool returns to the main repository from a worktree.
type ExitWorktreeTool struct{}

func (ExitWorktreeTool) Name() string      { return "ExitWorktree" }
func (ExitWorktreeTool) Aliases() []string { return nil }
func (ExitWorktreeTool) Description() string {
	return "Return to the main repository from a git worktree."
}
func (ExitWorktreeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (ExitWorktreeTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	// Find the main repository by looking at git config
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %s", string(out))
	}
	currentTop := strings.TrimSpace(string(out))

	// Get the main worktree
	out, err = exec.CommandContext(ctx, "git", "worktree", "list", "--porcelain").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to list worktrees: %s", string(out))
	}

	// Find the main worktree (first one without a branch)
	lines := strings.Split(string(out), "\n")
	var mainWorktree string
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			mainWorktree = strings.TrimPrefix(line, "worktree ")
		}
	}

	if mainWorktree == "" {
		return "", fmt.Errorf("could not find main worktree")
	}

	if mainWorktree == currentTop {
		return "Already in main repository: " + mainWorktree, nil
	}

	if err := os.Chdir(mainWorktree); err != nil {
		return "", fmt.Errorf("failed to change to main repository: %w", err)
	}

	return fmt.Sprintf("Returned to main repository: %s", mainWorktree), nil
}
