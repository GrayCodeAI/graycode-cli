package parallel

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// createWorktree creates a new git worktree at a temporary path on a new branch
// derived from baseBranch. It returns the absolute path to the worktree directory.
func createWorktree(repoDir, baseBranch, branchName string) (string, error) {
	// Create a temp directory for the worktree.
	dir, err := os.MkdirTemp("", "hawk-wt-*")
	if err != nil {
		return "", fmt.Errorf("mkdtemp: %w", err)
	}

	// git worktree add requires the target directory to not exist; MkdirTemp
	// already created it, so use a subdirectory.
	wtPath := filepath.Join(dir, "work")

	// #nosec G204 -- binary is the fixed string "git"; branchName/wtPath/baseBranch come from internal caller state, not raw external input
	cmd := exec.CommandContext(context.Background(), "git", "worktree", "add", "-b", branchName, wtPath, baseBranch)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Best-effort cleanup of the temp dir; ignore error since we're already in an error path.
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return wtPath, nil
}

// removeWorktree removes a git worktree and its backing branch metadata.
// It is safe to call on a path that has already been removed.
func removeWorktree(repoDir, worktreePath string) error {
	// Remove the worktree reference from git.
	cmd := exec.CommandContext(context.Background(), "git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// If the directory is already gone, git may complain. Try pruning instead.
		prune := exec.CommandContext(context.Background(), "git", "worktree", "prune")
		prune.Dir = repoDir
		_ = prune.Run() // best-effort; ignore error

		// Also try to clean up the filesystem path directly.
		_ = os.RemoveAll(worktreePath) // best-effort; ignore error
		// Clean up the parent temp dir if it is now empty.
		parent := filepath.Dir(worktreePath)
		_ = os.Remove(parent) // best-effort; may not be empty

		// If the original error was just "not a working tree" that's fine.
		outStr := strings.TrimSpace(string(out))
		if strings.Contains(outStr, "is not a working tree") ||
			strings.Contains(outStr, "No such file or directory") {
			return nil
		}
		return fmt.Errorf("git worktree remove: %s: %w", outStr, err)
	}

	// Clean up the parent temp dir created by createWorktree.
	parent := filepath.Dir(worktreePath)
	_ = os.Remove(parent) // best-effort; may not be empty

	return nil
}

// mergeWorktree merges the task branch back into the base branch.
// The caller must ensure no uncommitted changes exist in the main repo.
func mergeWorktree(repoDir, baseBranch, taskBranch string) error {
	// Checkout the base branch.
	checkout := exec.CommandContext(context.Background(), "git", "checkout", baseBranch)
	checkout.Dir = repoDir
	if out, err := checkout.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %s: %w", baseBranch, strings.TrimSpace(string(out)), err)
	}

	// Merge the task branch.
	// #nosec G204 -- binary is the fixed string "git"; taskBranch comes from internal caller state, not raw external input
	merge := exec.CommandContext(context.Background(), "git", "merge", "--no-ff", taskBranch, "-m",
		fmt.Sprintf("Merge parallel task branch %s", taskBranch))
	merge.Dir = repoDir
	if out, err := merge.CombinedOutput(); err != nil {
		return fmt.Errorf("git merge %s: %s: %w", taskBranch, strings.TrimSpace(string(out)), err)
	}

	return nil
}
