package tool

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

var lastAutoCommitHash string

// IsGitRepo checks if the current directory is inside a git repository.
func IsGitRepo() bool {
	return exec.CommandContext(context.Background(), "git", "rev-parse", "--git-dir").Run() == nil
}

func autoCommitEnabled(ctx context.Context) bool {
	tc := GetToolContext(ctx)
	if tc == nil {
		return false
	}
	return tc.AutoCommit
}

func AutoCommit(ctx context.Context, path, toolName, description string) error {
	if !IsGitRepo() {
		return fmt.Errorf("not a git repository")
	}

	add := exec.CommandContext(context.Background(), "git", "add", "--", path)
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	base := filepath.Base(path)
	msg := fmt.Sprintf("hawk: %s %s — %s", toolName, base, description)

	if tc := GetToolContext(ctx); tc != nil && tc.Attribution != nil {
		attr := tc.Attribution
		switch attr.TrailerStyle {
		case "co-authored-by":
			msg += "\n\nCo-authored-by: Hawk <hawk@graycode.ai>"
		case "assisted-by", "":
			msg += "\n\nAssisted-by: Hawk <hawk@graycode.ai>"
		case "none":
		}
		if attr.GeneratedWith {
			msg += "\nGenerated-with: Hawk"
		}
	}

	commit := exec.CommandContext(context.Background(), "git", "commit", "-m", msg)
	if out, err := commit.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	hash, err := gitHeadHash()
	if err == nil {
		lastAutoCommitHash = hash
	}
	return nil
}

func RevertLastAutoCommit() error {
	if lastAutoCommitHash == "" {
		return fmt.Errorf("no auto-commit to revert")
	}
	reset := exec.CommandContext(context.Background(), "git", "reset", "--soft", "HEAD~1")
	if out, err := reset.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	unstage := exec.CommandContext(context.Background(), "git", "restore", "--staged", ".")
	if out, err := unstage.CombinedOutput(); err != nil {
		return fmt.Errorf("git restore: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func LastAutoCommitHash() string {
	return lastAutoCommitHash
}

func gitHeadHash() (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "rev-parse", "--short", "HEAD").CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitHeadMessage() (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "log", "-1", "--format=%s").CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
