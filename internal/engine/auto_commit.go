package engine

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// AutoCommitter automatically commits changes after every successful edit.
// Never lose work — every change is a git commit you can undo.
type AutoCommitter struct {
	Enabled bool
	RepoDir string
}

// NewAutoCommitter creates an auto-committer for the given repo.
func NewAutoCommitter(repoDir string) *AutoCommitter {
	return &AutoCommitter{Enabled: true, RepoDir: repoDir}
}

// CommitIfChanged stages and commits any uncommitted changes with a smart message.
func (ac *AutoCommitter) CommitIfChanged(description string) error {
	if !ac.Enabled {
		return nil
	}
	// Check if there are changes
	cmd := exec.CommandContext(context.Background(), "git", "status", "--porcelain")
	cmd.Dir = ac.RepoDir
	out, err := cmd.Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return nil // no changes
	}

	// Stage all changes
	stage := exec.CommandContext(context.Background(), "git", "add", "-A")
	stage.Dir = ac.RepoDir
	if err := stage.Run(); err != nil {
		return err
	}

	// Generate commit message
	msg := ac.generateMessage(description)

	// Commit
	commit := exec.CommandContext(context.Background(), "git", "commit", "-m", msg, "--no-verify")
	commit.Dir = ac.RepoDir
	return commit.Run()
}

// Undo reverts the last auto-commit.
func (ac *AutoCommitter) Undo() error {
	cmd := exec.CommandContext(context.Background(), "git", "reset", "--soft", "HEAD~1")
	cmd.Dir = ac.RepoDir
	return cmd.Run()
}

func (ac *AutoCommitter) generateMessage(description string) string {
	if description != "" {
		// Truncate to conventional commit length
		if len(description) > 72 {
			description = description[:69] + "..."
		}
		return "hawk: " + description
	}
	return fmt.Sprintf("hawk: auto-commit %s", time.Now().Format("15:04:05"))
}
