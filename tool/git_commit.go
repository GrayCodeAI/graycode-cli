package tool

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
)

// lastAutoCommitHash stores the hash of the most recent auto-commit so it
// can be reverted if needed.
var lastAutoCommitHash string

func getAttribution() *hawkconfig.Attribution {
	s := hawkconfig.LoadSettings()
	if s.Attribution == nil {
		return &hawkconfig.Attribution{TrailerStyle: "assisted-by"}
	}
	return s.Attribution
}

// AutoCommit stages the file at path and creates a commit with a
// conventional hawk message.  toolName and description are used to
// build the commit message.
func AutoCommit(path, toolName, description string) error {
	if !IsGitRepo() {
		return fmt.Errorf("not a git repository")
	}

	// Stage the specific file.
	add := exec.Command("git", "add", "--", path)
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	base := filepath.Base(path)
	msg := fmt.Sprintf("hawk: %s %s — %s", toolName, base, description)

	// Attribution trailer from config.
	if attr := getAttribution(); attr != nil {
		switch attr.TrailerStyle {
		case "co-authored-by":
			msg += "\n\nCo-authored-by: Hawk <hawk@graycode.ai>"
		case "assisted-by", "":
			msg += "\n\nAssisted-by: Hawk <hawk@graycode.ai>"
		case "none":
			// no trailer
		}
		if attr.GeneratedWith {
			msg += "\nGenerated-with: Hawk"
		}
	}

	commit := exec.Command("git", "commit", "-m", msg)
	if out, err := commit.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	// Record the new HEAD for possible revert.
	hash, err := gitHeadHash()
	if err == nil {
		lastAutoCommitHash = hash
	}
	return nil
}

// RevertLastAutoCommit reverts the most recent auto-commit, but only
// if the current HEAD message starts with "hawk:".
func RevertLastAutoCommit() error {
	if !IsGitRepo() {
		return fmt.Errorf("not a git repository")
	}

	msg, err := gitHeadMessage()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(msg, "hawk:") {
		return fmt.Errorf("HEAD commit is not a hawk auto-commit")
	}

	revert := exec.Command("git", "revert", "HEAD", "--no-edit")
	if out, err := revert.CombinedOutput(); err != nil {
		return fmt.Errorf("git revert: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// IsGitRepo returns true when the current working directory is inside a
// git repository.
func IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	out, err := cmd.CombinedOutput()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// LastAutoCommitHash returns the hash of the most recent hawk auto-commit,
// or "" if none has been made in this process.
func LastAutoCommitHash() string {
	return lastAutoCommitHash
}

// autoCommitEnabled checks the ToolContext for the AutoCommit flag.
func autoCommitEnabled(ctx context.Context) bool {
	tc := GetToolContext(ctx)
	if tc == nil {
		return false
	}
	return tc.AutoCommit
}

// gitHeadHash returns the abbreviated hash of HEAD.
func gitHeadHash() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// gitHeadMessage returns the subject line of the HEAD commit.
func gitHeadMessage() (string, error) {
	out, err := exec.Command("git", "log", "-1", "--format=%s").CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
