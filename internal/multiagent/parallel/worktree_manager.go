package parallel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

// WorktreeStatus represents the lifecycle state of a managed worktree.
type WorktreeStatus string

const (
	WorktreeActive   WorktreeStatus = "active"
	WorktreeComplete WorktreeStatus = "complete"
	WorktreeFailed   WorktreeStatus = "failed"
	WorktreeMerged   WorktreeStatus = "merged"
)

// Worktree represents a single managed git worktree instance.
type Worktree struct {
	ID              string
	Path            string
	Branch          string
	BaseBranch      string
	CreatedAt       time.Time
	Status          WorktreeStatus
	TaskDescription string
}

// WorktreeManager manages the lifecycle of git worktrees for parallel execution.
type WorktreeManager struct {
	BaseDir      string
	Worktrees    map[string]*Worktree
	MaxWorktrees int
	mu           sync.RWMutex
}

// NewWorktreeManager creates a new WorktreeManager rooted at the given base directory.
// The base directory must be inside a git repository.
func NewWorktreeManager(baseDir string) *WorktreeManager {
	return &WorktreeManager{
		BaseDir:      baseDir,
		Worktrees:    make(map[string]*Worktree),
		MaxWorktrees: 8,
	}
}

// generateID produces a short random hex identifier for a worktree.
func generateID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails.
		return fmt.Sprintf("wt-%d", time.Now().UnixNano())
	}
	return "wt-" + hex.EncodeToString(b)
}

// Create creates a new git worktree on a new branch derived from baseBranch.
// The worktree is stored under Graycode's project state directory.
func (wm *WorktreeManager) Create(branch, baseBranch, taskDescription string) (*Worktree, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// Check capacity.
	activeCount := 0
	for _, wt := range wm.Worktrees {
		if wt.Status == WorktreeActive {
			activeCount++
		}
	}
	if activeCount >= wm.MaxWorktrees {
		return nil, fmt.Errorf("maximum worktrees reached (%d/%d)", activeCount, wm.MaxWorktrees)
	}

	id := generateID()
	wtDir := filepath.Join(storage.ProjectStateDir(wm.BaseDir), "worktrees", id)

	// #nosec G204 -- binary is the fixed string "git"; branch/wtDir/baseBranch come from internal caller state, not raw external input
	cmd := exec.CommandContext(context.Background(), "git", "worktree", "add", "-b", branch, wtDir, baseBranch)
	cmd.Dir = wm.BaseDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}

	wt := &Worktree{
		ID:              id,
		Path:            wtDir,
		Branch:          branch,
		BaseBranch:      baseBranch,
		CreatedAt:       time.Now(),
		Status:          WorktreeActive,
		TaskDescription: taskDescription,
	}
	wm.Worktrees[id] = wt
	return wt, nil
}

// Remove removes a worktree by ID, cleaning it from git and the internal map.
func (wm *WorktreeManager) Remove(id string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wt, ok := wm.Worktrees[id]
	if !ok {
		return fmt.Errorf("worktree %q not found", id)
	}

	// #nosec G204 -- binary is the fixed string "git"; wt.Path comes from the manager's own tracked Worktree state, not raw external input
	cmd := exec.CommandContext(context.Background(), "git", "worktree", "remove", "--force", wt.Path)
	cmd.Dir = wm.BaseDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(out))
		if !strings.Contains(outStr, "is not a working tree") &&
			!strings.Contains(outStr, "No such file or directory") {
			return fmt.Errorf("git worktree remove: %s: %w", outStr, err)
		}
	}

	delete(wm.Worktrees, id)
	return nil
}

// List returns all registered worktrees sorted by creation time (newest first).
func (wm *WorktreeManager) List() []*Worktree {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	result := make([]*Worktree, 0, len(wm.Worktrees))
	for _, wt := range wm.Worktrees {
		result = append(result, wt)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

// Get returns a worktree by ID, or nil if not found.
func (wm *WorktreeManager) Get(id string) *Worktree {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.Worktrees[id]
}

// MergeBack merges the worktree branch back into its base branch using the
// specified strategy. Supported strategies: "fast-forward", "squash", "merge-commit".
func (wm *WorktreeManager) MergeBack(id string, strategy string) error {
	wm.mu.RLock()
	wt, ok := wm.Worktrees[id]
	if !ok {
		wm.mu.RUnlock()
		return fmt.Errorf("worktree %q not found", id)
	}
	branch := wt.Branch
	baseBranch := wt.BaseBranch
	wm.mu.RUnlock()

	// Checkout the base branch.
	// #nosec G204 -- binary is the fixed string "git"; baseBranch comes from the manager's own tracked Worktree state, not raw external input
	checkout := exec.CommandContext(context.Background(), "git", "checkout", baseBranch)
	checkout.Dir = wm.BaseDir
	if out, err := checkout.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %s: %w", baseBranch, strings.TrimSpace(string(out)), err)
	}

	// Merge with the chosen strategy.
	var mergeCmd *exec.Cmd
	switch strategy {
	case "fast-forward":
		// #nosec G204 -- binary is the fixed string "git"; branch comes from the manager's own tracked Worktree state, not raw external input
		mergeCmd = exec.CommandContext(context.Background(), "git", "merge", "--ff-only", branch)
	case "squash":
		// #nosec G204 -- binary is the fixed string "git"; branch comes from the manager's own tracked Worktree state, not raw external input
		mergeCmd = exec.CommandContext(context.Background(), "git", "merge", "--squash", branch)
	case "merge-commit":
		// #nosec G204 -- binary is the fixed string "git"; branch/baseBranch come from the manager's own tracked Worktree state, not raw external input
		mergeCmd = exec.CommandContext(context.Background(), "git", "merge", "--no-ff", branch, "-m",
			fmt.Sprintf("Merge branch '%s' into %s", branch, baseBranch))
	default:
		return fmt.Errorf("unsupported merge strategy: %q (use fast-forward, squash, or merge-commit)", strategy)
	}

	mergeCmd.Dir = wm.BaseDir
	if out, err := mergeCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git merge (%s): %s: %w", strategy, strings.TrimSpace(string(out)), err)
	}

	// For squash merges, git does not auto-commit; we need to commit.
	if strategy == "squash" {
		// #nosec G204 -- binary is the fixed string "git"; branch/baseBranch come from the manager's own tracked Worktree state, not raw external input
		commitCmd := exec.CommandContext(context.Background(), "git", "commit", "-m",
			fmt.Sprintf("Squash merge branch '%s' into %s", branch, baseBranch))
		commitCmd.Dir = wm.BaseDir
		if out, err := commitCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git commit (squash): %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	wm.mu.Lock()
	if w, exists := wm.Worktrees[id]; exists {
		w.Status = WorktreeMerged
	}
	wm.mu.Unlock()

	return nil
}

// Cleanup removes all worktrees that are in completed, failed, or merged state,
// then runs git worktree prune.
func (wm *WorktreeManager) Cleanup() error {
	wm.mu.Lock()
	var toRemove []string
	for id, wt := range wm.Worktrees {
		if wt.Status == WorktreeComplete || wt.Status == WorktreeFailed || wt.Status == WorktreeMerged {
			toRemove = append(toRemove, id)
		}
	}
	wm.mu.Unlock()

	var firstErr error
	for _, id := range toRemove {
		if err := wm.Remove(id); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Prune stale worktree references.
	prune := exec.CommandContext(context.Background(), "git", "worktree", "prune")
	prune.Dir = wm.BaseDir
	if out, err := prune.CombinedOutput(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("git worktree prune: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return firstErr
}

// StatusReport returns a formatted string showing the current state of all worktrees.
func (wm *WorktreeManager) StatusReport() string {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	total := len(wm.Worktrees)
	if total == 0 {
		return fmt.Sprintf("Worktrees (0/%d):\n  No active worktrees.", wm.MaxWorktrees)
	}

	// Sort by creation time (oldest first for display).
	worktrees := make([]*Worktree, 0, total)
	for _, wt := range wm.Worktrees {
		worktrees = append(worktrees, wt)
	}
	sort.Slice(worktrees, func(i, j int) bool {
		return worktrees[i].CreatedAt.Before(worktrees[j].CreatedAt)
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Worktrees (%d/%d):\n", total, wm.MaxWorktrees))
	sb.WriteString(strings.Repeat("─", 25))
	sb.WriteString("\n")

	for i, wt := range worktrees {
		ago := formatDuration(time.Since(wt.CreatedAt))
		sb.WriteString(fmt.Sprintf("%d. [%s] %s (%s ago)\n", i+1, wt.Status, wt.Branch, ago))
		sb.WriteString(fmt.Sprintf("   Task: %q\n", wt.TaskDescription))
		if wt.Status == WorktreeComplete {
			sb.WriteString("   Ready to merge\n")
		}
		sb.WriteString(fmt.Sprintf("   Path: %s\n", relativePath(wm.BaseDir, wt.Path)))
		if i < len(worktrees)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// IsClean returns true if the worktree has no uncommitted changes.
func (wm *WorktreeManager) IsClean(id string) (bool, error) {
	wm.mu.RLock()
	wt, ok := wm.Worktrees[id]
	wm.mu.RUnlock()

	if !ok {
		return false, fmt.Errorf("worktree %q not found", id)
	}

	cmd := exec.CommandContext(context.Background(), "git", "status", "--porcelain")
	cmd.Dir = wt.Path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git status in %s: %s: %w", wt.Path, strings.TrimSpace(string(out)), err)
	}

	return strings.TrimSpace(string(out)) == "", nil
}

// GetDiff returns the diff between the worktree branch and its base branch.
func (wm *WorktreeManager) GetDiff(id string) (string, error) {
	wm.mu.RLock()
	wt, ok := wm.Worktrees[id]
	wm.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("worktree %q not found", id)
	}

	// #nosec G204 -- binary is the fixed string "git"; wt.BaseBranch/wt.Branch come from the manager's own tracked Worktree state, not raw external input
	cmd := exec.CommandContext(context.Background(), "git", "diff", wt.BaseBranch+"..."+wt.Branch)
	cmd.Dir = wm.BaseDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// FormatMergePreview returns a human-readable preview of what merging the
// worktree branch would bring into the base branch.
func (wm *WorktreeManager) FormatMergePreview(id string) string {
	wm.mu.RLock()
	wt, ok := wm.Worktrees[id]
	wm.mu.RUnlock()

	if !ok {
		return fmt.Sprintf("worktree %q not found", id)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Merge Preview: %s -> %s\n", wt.Branch, wt.BaseBranch))
	sb.WriteString(strings.Repeat("─", 40))
	sb.WriteString("\n")

	// Get commit log between base and branch.
	// #nosec G204 -- binary is the fixed string "git"; wt.BaseBranch/wt.Branch come from the manager's own tracked Worktree state, not raw external input
	logCmd := exec.CommandContext(context.Background(), "git", "log", "--oneline", wt.BaseBranch+".."+wt.Branch)
	logCmd.Dir = wm.BaseDir
	logOut, err := logCmd.CombinedOutput()
	if err != nil {
		sb.WriteString(fmt.Sprintf("  (unable to get commit log: %v)\n", err))
	} else {
		commits := strings.TrimSpace(string(logOut))
		if commits == "" {
			sb.WriteString("  No new commits to merge.\n")
		} else {
			sb.WriteString("Commits:\n")
			for _, line := range strings.Split(commits, "\n") {
				sb.WriteString("  " + line + "\n")
			}
		}
	}

	// Get diffstat.
	// #nosec G204 -- binary is the fixed string "git"; wt.BaseBranch/wt.Branch come from the manager's own tracked Worktree state, not raw external input
	statCmd := exec.CommandContext(context.Background(), "git", "diff", "--stat", wt.BaseBranch+"..."+wt.Branch)
	statCmd.Dir = wm.BaseDir
	statOut, err := statCmd.CombinedOutput()
	if err == nil {
		stat := strings.TrimSpace(string(statOut))
		if stat != "" {
			sb.WriteString("\nFiles changed:\n")
			for _, line := range strings.Split(stat, "\n") {
				sb.WriteString("  " + line + "\n")
			}
		}
	}

	return sb.String()
}

// formatDuration returns a human-friendly duration string (e.g., "2m", "1h", "30s").
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

// relativePath returns the path relative to baseDir, or the absolute path if
// it cannot be made relative.
func relativePath(baseDir, path string) string {
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return path
	}
	return rel
}
