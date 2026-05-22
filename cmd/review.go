package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Continuous AI code review on commits",
	Long: `hawk review provides continuous background code review.

Run 'hawk review init' to install a post-commit hook, then every commit
is automatically reviewed using sight. View findings with 'hawk review tui'
or fix them with 'hawk review fix'.`,
}

var reviewInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Install post-commit hook for automatic reviews",
	RunE:  runReviewInit,
}

var reviewInitForce bool

func init() {
	reviewInitCmd.Flags().BoolVarP(&reviewInitForce, "force", "f", false, "Overwrite existing post-commit hook")
	reviewCmd.AddCommand(reviewInitCmd)
	rootCmd.AddCommand(reviewCmd)
}

const hookScript = `#!/bin/sh
# hawk review — continuous code review hook
# Installed by 'hawk review init'
SHA=$(git rev-parse HEAD)
hawk review run "$SHA" --background &
`

func runReviewInit(_ *cobra.Command, _ []string) error {
	gitDir, err := findGitDir()
	if err != nil {
		return err
	}

	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	hookPath := filepath.Join(hooksDir, "post-commit")

	// Check for existing hook.
	if _, err := os.Stat(hookPath); err == nil && !reviewInitForce {
		existing, _ := os.ReadFile(hookPath)
		if strings.Contains(string(existing), "hawk review") {
			fmt.Println("✓ hawk review hook already installed")
			return nil
		}
		return fmt.Errorf("post-commit hook already exists at %s\nUse --force to overwrite, or manually add:\n  %s", hookPath, strings.TrimSpace(hookScript))
	}

	if err := os.WriteFile(hookPath, []byte(hookScript), 0o755); err != nil {
		return fmt.Errorf("write hook: %w", err)
	}

	// Ensure .hawk directory exists for the review DB.
	projectDir, _ := os.Getwd()
	_ = os.MkdirAll(filepath.Join(projectDir, ".hawk"), 0o755)

	fmt.Printf("✓ Installed post-commit hook at %s\n", hookPath)
	fmt.Println("  Every commit will now be reviewed automatically.")
	fmt.Println("  View reviews: hawk review status")
	fmt.Println("  Interactive:  hawk review tui")
	return nil
}

func findGitDir() (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "rev-parse", "--git-dir").Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository (run from inside a git repo)")
	}
	dir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(dir) {
		cwd, _ := os.Getwd()
		dir = filepath.Join(cwd, dir)
	}
	return dir, nil
}
