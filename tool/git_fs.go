package tool

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GitState holds parsed git repository state read directly from the
// filesystem without spawning a git subprocess.
type GitState struct {
	Branch   string
	Commit   string
	IsDirty  bool
	Worktree bool
}

// ReadGitState reads .git/HEAD and refs to determine the current branch and
// commit hash without spawning a git subprocess.
func ReadGitState(dir string) (*GitState, error) {
	gitDir := filepath.Join(dir, ".git")

	info, err := os.Stat(gitDir)
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}

	// If .git is a file, it's a worktree reference
	worktree := false
	if !info.IsDir() {
		data, err := os.ReadFile(gitDir)
		if err != nil {
			return nil, err
		}
		line := strings.TrimSpace(string(data))
		if strings.HasPrefix(line, "gitdir: ") {
			gitDir = strings.TrimPrefix(line, "gitdir: ")
			if !filepath.IsAbs(gitDir) {
				gitDir = filepath.Join(dir, gitDir)
			}
			worktree = true
		} else {
			return nil, fmt.Errorf("unexpected .git file content")
		}
	}

	headContent, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return nil, fmt.Errorf("cannot read HEAD: %w", err)
	}
	head := strings.TrimSpace(string(headContent))

	state := &GitState{Worktree: worktree}

	if strings.HasPrefix(head, "ref: ") {
		ref := strings.TrimPrefix(head, "ref: ")
		state.Branch = strings.TrimPrefix(ref, "refs/heads/")
		commit, err := resolveRef(gitDir, ref)
		if err == nil {
			state.Commit = commit
		}
	} else {
		// Detached HEAD — commit hash directly
		state.Commit = head
		state.Branch = "(detached)"
	}

	// Check for dirty state by looking at index modification time vs HEAD
	indexPath := filepath.Join(gitDir, "index")
	if indexInfo, err := os.Stat(indexPath); err == nil {
		headInfo, headErr := os.Stat(filepath.Join(gitDir, "HEAD"))
		if headErr == nil && indexInfo.ModTime().After(headInfo.ModTime()) {
			state.IsDirty = true
		}
	}

	return state, nil
}

// resolveRef follows a ref chain to its final commit hash.
func resolveRef(gitDir, ref string) (string, error) {
	// Try loose ref first
	refPath := filepath.Join(gitDir, ref)
	data, err := os.ReadFile(refPath)
	if err == nil {
		resolved := strings.TrimSpace(string(data))
		if strings.HasPrefix(resolved, "ref: ") {
			return resolveRef(gitDir, strings.TrimPrefix(resolved, "ref: "))
		}
		return resolved, nil
	}

	// Fall back to packed-refs
	packed, err := parsePackedRefs(gitDir)
	if err != nil {
		return "", fmt.Errorf("ref %s not found: %w", ref, err)
	}

	if hash, ok := packed[ref]; ok {
		return hash, nil
	}

	return "", fmt.Errorf("ref %s not found in packed-refs", ref)
}

// parsePackedRefs reads .git/packed-refs and returns a map of ref to commit hash.
func parsePackedRefs(gitDir string) (map[string]string, error) {
	packedPath := filepath.Join(gitDir, "packed-refs")
	f, err := os.Open(packedPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	refs := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip comments and peeled refs
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 2 {
			refs[parts[1]] = parts[0]
		}
	}

	return refs, scanner.Err()
}
