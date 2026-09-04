package engine

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Default branch names treated as "don't commit agent work here".
var defaultBranchNames = map[string]bool{
	"main":        true,
	"master":      true,
	"trunk":       true,
	"develop":     true,
	"development": true,
}

// GitBranchInfo describes the current branch safety posture.
type GitBranchInfo struct {
	RepoDir   string
	Branch    string
	OnDefault bool
	Detached  bool
	HasRepo   bool
	Dirty     bool
	Suggested string // graycode/agent-<timestamp> when OnDefault
}

// InspectGitBranch reads branch and dirty state for repoDir ("" = cwd).
func InspectGitBranch(repoDir string) GitBranchInfo {
	info := GitBranchInfo{RepoDir: repoDir}
	if repoDir == "" {
		info.RepoDir = "."
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Inside a git work tree?
	chk := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	chk.Dir = info.RepoDir
	if out, err := chk.Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
		return info
	}
	info.HasRepo = true

	br := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	br.Dir = info.RepoDir
	bout, err := br.Output()
	if err != nil {
		return info
	}
	info.Branch = strings.TrimSpace(string(bout))
	if info.Branch == "HEAD" {
		info.Detached = true
		short := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
		short.Dir = info.RepoDir
		if s, err := short.Output(); err == nil {
			info.Branch = strings.TrimSpace(string(s))
		}
	}
	info.OnDefault = !info.Detached && defaultBranchNames[info.Branch]
	if info.OnDefault {
		info.Suggested = fmt.Sprintf("graycode/agent-%s", time.Now().Format("20060102-150405"))
	}

	st := exec.CommandContext(ctx, "git", "status", "--porcelain")
	st.Dir = info.RepoDir
	if out, err := st.Output(); err == nil && len(strings.TrimSpace(string(out))) > 0 {
		info.Dirty = true
	}
	return info
}

// EnsureAgentBranch creates and checks out a graycode/agent-* branch when currently
// on a default branch. No-op if already on a feature branch or not a git repo.
// Returns the branch name after the operation.
func EnsureAgentBranch(repoDir string) (string, error) {
	info := InspectGitBranch(repoDir)
	if !info.HasRepo {
		return "", fmt.Errorf("not a git repository")
	}
	if info.Detached {
		return info.Branch, fmt.Errorf("detached HEAD — checkout a branch first")
	}
	if !info.OnDefault {
		return info.Branch, nil
	}
	name := info.Suggested
	if name == "" {
		name = fmt.Sprintf("graycode/agent-%d", time.Now().Unix())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// #nosec G204 -- fixed git subcommand; branch name is generated internally (graycode/agent-*)
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", name)
	cmd.Dir = info.RepoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return info.Branch, fmt.Errorf("create branch %s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	return name, nil
}

// GitSafetyAdvice is a one-line warning for onboarding / status.
func GitSafetyAdvice(info GitBranchInfo) string {
	if !info.HasRepo {
		return ""
	}
	if info.OnDefault {
		return fmt.Sprintf("On %s — consider /branch-agent before large edits (suggested: %s)", info.Branch, info.Suggested)
	}
	if info.Dirty {
		return fmt.Sprintf("Branch %s has uncommitted changes", info.Branch)
	}
	return fmt.Sprintf("Branch %s", info.Branch)
}
