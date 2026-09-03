package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// autoCommitTimeout bounds each git operation run by the auto-committer.
// These operations run after an edit completes, detached from any request
// context (the caller has none to give), so a hung git invocation — e.g. on
// a slow network filesystem — cannot block the session forever.
const autoCommitTimeout = 2 * time.Minute

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
	ctx, cancel := context.WithTimeout(context.Background(), autoCommitTimeout)
	defer cancel()

	// Check if there are changes
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = ac.RepoDir
	out, err := cmd.Output()
	if err != nil {
		// Distinguish "git status failed" from "nothing to commit" so the
		// failure is not silently swallowed as a no-op.
		slog.Warn("auto-commit: git status failed; skipping commit", "dir", ac.RepoDir, "error", err)
		return nil
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return nil // no changes
	}

	// Stage all changes
	stage := exec.CommandContext(ctx, "git", "add", "-A")
	stage.Dir = ac.RepoDir
	if err := stage.Run(); err != nil {
		return err
	}

	// Generate commit message
	msg := ac.generateMessage(description)

	// Commit
	commit := exec.CommandContext(ctx, "git", "commit", "-m", msg, "--no-verify") // #nosec G204 -- git subcommand invocation with fixed subcommand and internally-derived args
	commit.Dir = ac.RepoDir
	return commit.Run()
}

// Undo reverts the last auto-commit.
func (ac *AutoCommitter) Undo() error {
	ctx, cancel := context.WithTimeout(context.Background(), autoCommitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "reset", "--soft", "HEAD~1")
	cmd.Dir = ac.RepoDir
	return cmd.Run()
}

func (ac *AutoCommitter) generateMessage(description string) string {
	if description != "" {
		// Truncate to conventional commit length
		if len(description) > 72 {
			description = description[:69] + "..."
		}
		return "graycode: " + description
	}
	return fmt.Sprintf("graycode: auto-commit %s", time.Now().Format("15:04:05"))
}
