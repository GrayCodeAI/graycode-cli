package snapshot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Tracker maintains a shadow git repository that records every file change
// the agent makes. Supports point-in-time restore and selective revert.
type Tracker struct {
	projectDir string
	shadowDir  string
	mu         sync.Mutex
}

// Patch represents a recorded snapshot with its changed files.
type Patch struct {
	Hash      string    `json:"hash"`
	Message   string    `json:"message"`
	Files     []string  `json:"files"`
	Timestamp time.Time `json:"timestamp"`
}

// FileDiff represents a diff for a single file between two snapshots.
type FileDiff struct {
	File      string `json:"file"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Status    string `json:"status"` // added, deleted, modified
}

// New creates a Tracker for the given project directory.
func New(projectDir string) *Tracker {
	return &Tracker{
		projectDir: projectDir,
		shadowDir:  filepath.Join(projectDir, ".hawk", "snapshots"),
	}
}

// Init initializes the shadow git repository if it doesn't exist.
func (t *Tracker) Init() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := os.MkdirAll(t.shadowDir, 0o755); err != nil {
		return err
	}

	// Check if already initialized
	if _, err := os.Stat(filepath.Join(t.shadowDir, ".git")); err == nil {
		return nil
	}

	// Initialize as a regular repo with the project as work tree
	if err := t.gitWork("init"); err != nil {
		return err
	}
	_ = t.gitWork("config", "user.email", "hawk@snapshot")
	_ = t.gitWork("config", "user.name", "hawk-snapshot")
	return nil
}

// Track takes a snapshot of the current project state. Returns the commit hash.
func (t *Tracker) Track(message string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if message == "" {
		message = "snapshot " + time.Now().Format("15:04:05")
	}

	// Add all files from project dir
	if err := t.gitWork("add", "--all", t.projectDir); err != nil {
		return "", fmt.Errorf("add: %w", err)
	}

	// Check if there are changes to commit
	if err := t.gitWork("diff", "--cached", "--quiet"); err == nil {
		// No changes — return current HEAD
		out, _ := t.gitWorkOutput("rev-parse", "--short", "HEAD")
		return strings.TrimSpace(out), nil
	}

	// Commit
	if err := t.gitWork("commit", "-m", message, "--allow-empty"); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	out, err := t.gitWorkOutput("rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Restore resets the project directory to the state at the given snapshot.
func (t *Tracker) Restore(hash string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.gitWork("checkout", hash, "--", t.projectDir); err != nil {
		return fmt.Errorf("checkout: %w", err)
	}
	return nil
}

// Diff returns the list of changed files between two snapshot hashes.
func (t *Tracker) Diff(from, to string) ([]FileDiff, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	out, err := t.gitWorkOutput("diff", "--numstat", from, to)
	if err != nil {
		return nil, err
	}

	var diffs []FileDiff
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		var adds, dels int
		_, _ = fmt.Sscanf(parts[0], "%d", &adds)
		_, _ = fmt.Sscanf(parts[1], "%d", &dels)

		status := "modified"
		if adds > 0 && dels == 0 {
			status = "added"
		}

		diffs = append(diffs, FileDiff{
			File:      parts[2],
			Additions: adds,
			Deletions: dels,
			Status:    status,
		})
	}
	return diffs, nil
}

// History returns the last N snapshots.
func (t *Tracker) History(limit int) ([]Patch, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if limit <= 0 {
		limit = 20
	}

	out, err := t.gitWorkOutput("log", "--format=%h|%s|%ai", fmt.Sprintf("-%d", limit))
	if err != nil {
		return nil, nil // no commits yet
	}

	var patches []Patch
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}
		ts, _ := time.Parse("2006-01-02 15:04:05 -0700", parts[2])
		patches = append(patches, Patch{
			Hash:      parts[0],
			Message:   parts[1],
			Timestamp: ts,
		})
	}
	return patches, nil
}

// Cleanup runs garbage collection on the shadow repo.
func (t *Tracker) Cleanup(maxAge time.Duration) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	cutoff := time.Now().Add(-maxAge).Format(time.RFC3339)
	_ = t.gitWork("reflog", "expire", "--expire="+cutoff, "--all")
	_ = t.gitWork("gc", "--prune="+cutoff)
	return nil
}

func (t *Tracker) gitWork(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = t.shadowDir
	cmd.Env = append(os.Environ(),
		"GIT_WORK_TREE="+t.projectDir,
		"GIT_DIR="+filepath.Join(t.shadowDir, ".git"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (t *Tracker) gitWorkOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = t.shadowDir
	cmd.Env = append(os.Environ(),
		"GIT_WORK_TREE="+t.projectDir,
		"GIT_DIR="+filepath.Join(t.shadowDir, ".git"),
	)
	out, err := cmd.Output()
	return string(out), err
}
