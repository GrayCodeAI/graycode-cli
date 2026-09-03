// Package gitworktree creates and removes isolated git worktrees for subagents.
package gitworktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Create makes a new git worktree under the repo's .graycode/worktrees directory.
// branch is created from HEAD if non-empty; otherwise a unique branch name is used.
// Returns absolute path and a cleanup function (best-effort remove).
func Create(ctx context.Context, repoDir, branch string) (path string, cleanup func(), err error) {
	repoDir, err = filepath.Abs(repoDir)
	if err != nil {
		return "", nil, err
	}
	// Ensure we are in a git repo.
	if out, e := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "--is-inside-work-tree").CombinedOutput(); e != nil { // #nosec G204 -- fixed git executable
		return "", nil, fmt.Errorf("not a git repository: %s", strings.TrimSpace(string(out)))
	}
	base := filepath.Join(repoDir, ".graycode", "worktrees")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", nil, err
	}
	if branch == "" {
		branch = fmt.Sprintf("graycode-subagent-%d", time.Now().UnixNano())
	}
	path = filepath.Join(base, branch)
	// Remove leftover path if present.
	_ = os.RemoveAll(path)

	// #nosec G204 -- fixed git binary; path/branch derived internally
	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "worktree", "add", "-b", branch, path, "HEAD") // #nosec G204 -- fixed git executable
	if out, e := cmd.CombinedOutput(); e != nil {
		return "", nil, fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), e)
	}
	cleanup = func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = exec.CommandContext(cctx, "git", "-C", repoDir, "worktree", "remove", "--force", path).Run() // #nosec G204 -- fixed git executable
		_ = os.RemoveAll(path)
		// Best-effort branch delete (ignore if checked out elsewhere).
		_ = exec.CommandContext(cctx, "git", "-C", repoDir, "branch", "-D", branch).Run() // #nosec G204 -- fixed git executable
	}
	return path, cleanup, nil
}
