// Package gitsnapshot captures point-in-time file-tree snapshots using git's
// content-addressed object database, adopting the approach opencode uses for
// durable, cheap snapshots.
//
// A Manager keeps a linked snapshot repository whose object database is seeded
// from the source repository via objects/info/alternates, so capturing a tree
// reuses hashes already computed by the source repo instead of re-hashing large
// checkouts. Snapshots are content-addressed git tree IDs, which makes diffing,
// previewing, and restoring cheap and unambiguous.
package gitsnapshot

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TreeID is a content-addressed git tree hash.
type TreeID string

// StaleContentError reports a compare-and-swap write that would clobber a file
// changed by another writer since it was read.
type StaleContentError struct {
	Path string
}

func (e *StaleContentError) Error() string {
	return "gitsnapshot: stale content at " + e.Path + " (file changed since read)"
}

// FileChange describes a single file's change between two trees.
type FileChange struct {
	Path      string
	Status    string // "added", "modified", "deleted"
	Additions int
	Deletions int
	Patch     string
}

// Manager snapshots a source repository's file tree into a linked snapshot repo.
type Manager struct {
	// SourceDir is the working directory of the source repo.
	SourceDir string
	// SnapDir is the directory holding the linked snapshot repo.
	SnapDir string
}

// New creates a Manager and prepares the linked snapshot repo, seeding its
// object storage from the source repo. SourceDir must be inside a git repo.
func New(sourceDir, snapDir string) (*Manager, error) {
	abs, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("gitsnapshot: resolve source dir: %w", err)
	}
	m := &Manager{SourceDir: abs, SnapDir: snapDir}
	if err := m.initLinkedRepo(); err != nil {
		return nil, err
	}
	return m, nil
}

// initLinkedRepo creates the snapshot repo (if needed) and wires its object
// database to the source repo via objects/info/alternates.
func (m *Manager) initLinkedRepo() error {
	if m.SnapDir == "" {
		return fmt.Errorf("gitsnapshot: empty snapshot dir")
	}
	if err := os.MkdirAll(m.SnapDir, 0o750); err != nil {
		return fmt.Errorf("gitsnapshot: create snapshot dir: %w", err)
	}
	gitDir := filepath.Join(m.SnapDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := m.gitRun(m.SnapDir, "init", "--bare"); err != nil {
			return fmt.Errorf("gitsnapshot: init linked repo: %w", err)
		}
	}

	// Point the snapshot repo's object db at the source repo's object db.
	altFile := filepath.Join(gitDir, "objects", "info", "alternates")
	srcObjDir, err := m.sourceObjectDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(altFile), 0o750); err != nil {
		return fmt.Errorf("gitsnapshot: mkdir objects/info: %w", err)
	}
	if err := os.WriteFile(altFile, []byte(srcObjDir+"\n"), 0o600); err != nil {
		return fmt.Errorf("gitsnapshot: write alternates: %w", err)
	}
	return nil
}

// sourceObjectDir returns the absolute path to the source repo's object db.
func (m *Manager) sourceObjectDir() (string, error) {
	out, err := m.gitOut(m.SourceDir, "rev-parse", "--git-dir")
	if err != nil {
		return "", fmt.Errorf("gitsnapshot: locate source git dir: %w", err)
	}
	gitDir := strings.TrimSpace(out)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(m.SourceDir, gitDir)
	}
	objDir := filepath.Join(gitDir, "objects")
	if st, err := os.Stat(objDir); err != nil || !st.IsDir() {
		return "", fmt.Errorf("gitsnapshot: source object db not found at %s", objDir)
	}
	return objDir, nil
}

// Capture stages the given project-relative paths into a private throwaway
// index in the source repo and writes a content-addressed tree, returning its
// TreeID. The user's real index and staging area are left untouched. An empty
// paths list captures the whole worktree (respecting .gitignore). The resulting
// tree lives in the source object db, which the linked snapshot repo shares.
func (m *Manager) Capture(ctx context.Context, paths []string) (TreeID, error) {
	idx, err := os.CreateTemp("", "gitsnapshot-capture-*.index")
	if err != nil {
		return "", fmt.Errorf("gitsnapshot: create capture index: %w", err)
	}
	idxPath := idx.Name()
	_ = idx.Close()
	defer func() { _ = os.Remove(idxPath) }()

	env := append(os.Environ(), "GIT_INDEX_FILE="+idxPath)

	// Seed the throwaway index from HEAD so tree reads/writes reflect the repo.
	if _, err := m.gitOutEnv(ctx, m.SourceDir, env, "read-tree", "HEAD"); err != nil {
		// A repo with no commits has no HEAD; tolerate by starting empty.
		if _, e2 := m.gitOutEnv(ctx, m.SourceDir, env, "read-tree", "--empty"); e2 != nil {
			return "", fmt.Errorf("gitsnapshot: seed index: %w", err)
		}
	}

	args := []string{"add", "--all"}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	if _, err := m.gitOutEnv(ctx, m.SourceDir, env, args...); err != nil {
		return "", fmt.Errorf("gitsnapshot: add: %w", err)
	}
	out, err := m.gitOutEnv(ctx, m.SourceDir, env, "write-tree")
	if err != nil {
		return "", fmt.Errorf("gitsnapshot: write-tree: %w", err)
	}
	tid := TreeID(strings.TrimSpace(out))
	if tid == "" {
		return "", fmt.Errorf("gitsnapshot: write-tree produced empty id")
	}
	return tid, nil
}

// Diff returns per-file changes between two trees. Either id may be empty to
// compare against an empty tree (all files added).
func (m *Manager) Diff(ctx context.Context, from, to TreeID) ([]FileChange, error) {
	a, b := string(from), string(to)
	if a == "" {
		a = emptyTreeID(ctx, m)
	}
	if b == "" {
		b = emptyTreeID(ctx, m)
	}
	raw, err := m.gitOutCtx(ctx, m.SourceDir, "diff-tree", "-r", "--name-status", a, b)
	if err != nil {
		return nil, fmt.Errorf("gitsnapshot: diff-tree: %w", err)
	}
	var changes []FileChange
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		status := fields[0]
		path := fields[len(fields)-1]
		ch := FileChange{Path: path}
		switch status[0] {
		case 'A':
			ch.Status = "added"
		case 'D':
			ch.Status = "deleted"
		case 'M':
			ch.Status = "modified"
		default:
			ch.Status = status
		}
		changes = append(changes, ch)
	}
	return changes, nil
}

// Preview computes a hypothetical per-file diff for a set of paths against a
// target tree without touching the source worktree. It builds a throwaway
// index (GIT_INDEX_FILE) so the operation is side-effect free.
func (m *Manager) Preview(ctx context.Context, target TreeID, paths []string) ([]FileChange, error) {
	tmp, err := os.CreateTemp("", "gitsnapshot-preview-*.index")
	if err != nil {
		return nil, fmt.Errorf("gitsnapshot: create throwaway index: %w", err)
	}
	idxPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(idxPath) }()

	env := append(os.Environ(), "GIT_INDEX_FILE="+idxPath)
	if _, err := m.gitOutEnv(ctx, m.SourceDir, env, "read-tree", string(target)); err != nil {
		return nil, fmt.Errorf("gitsnapshot: read-tree: %w", err)
	}

	var changes []FileChange
	for _, p := range paths {
		// Compare the index blob for the path against the worktree file.
		idxBlob, err := m.gitOutEnv(ctx, m.SourceDir, env, "ls-files", "-s", "--", p)
		if err != nil {
			return nil, err
		}
		worktreePath := filepath.Join(m.SourceDir, p)
		data, err := os.ReadFile(worktreePath)
		if err != nil {
			if os.IsNotExist(err) {
				changes = append(changes, FileChange{Path: p, Status: "deleted"})
				continue
			}
			return nil, err
		}
		// Hash the worktree blob and compare to the index entry hash.
		idxFields := strings.Fields(idxBlob)
		blobID := ""
		if len(idxFields) >= 2 {
			// "ls-files -s" emits "<mode> <object> <stage>\t<path>".
			blobID = idxFields[1]
		}
		if blobID == "" {
			changes = append(changes, FileChange{Path: p, Status: "added"})
			continue
		}
		workID := m.hashBlob(ctx, env, data)
		if workID == "" {
			changes = append(changes, FileChange{Path: p, Status: "modified"})
			continue
		}
		if workID != blobID {
			changes = append(changes, FileChange{Path: p, Status: "modified"})
		}
	}
	return changes, nil
}

// Restore selectively checks out (or deletes) paths from a target tree into the
// source worktree. Deleted-in-tree paths are removed from the worktree.
func (m *Manager) Restore(ctx context.Context, target TreeID, paths []string) error {
	for _, p := range paths {
		worktreePath := filepath.Join(m.SourceDir, p)
		blobID, err := m.gitOutCtx(ctx, m.SourceDir, "ls-tree", string(target), "--", p)
		if err != nil {
			return err
		}
		trimmed := strings.TrimSpace(blobID)
		if trimmed == "" {
			// Path absent from tree => delete from worktree.
			if err := os.RemoveAll(worktreePath); err != nil {
				return fmt.Errorf("gitsnapshot: delete %s: %w", p, err)
			}
			continue
		}
		// blobID line: "<mode> blob <hash>\t<path>"
		fields := strings.Fields(trimmed)
		if len(fields) < 3 {
			return fmt.Errorf("gitsnapshot: unexpected ls-tree output for %s: %q", p, trimmed)
		}
		if err := m.writeBlobToFile(ctx, envPlain(), fields[2], worktreePath); err != nil {
			return err
		}
	}
	return nil
}

// WriteIfUnchanged performs a compare-and-swap write: it only writes newBytes
// to path if the current content still equals expectedBytes. It returns a
// *StaleContentError when the file changed since it was read, preventing one
// agent from clobbering another's concurrent edit.
func (m *Manager) WriteIfUnchanged(path string, expected, newContent []byte) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("gitsnapshot: read %s: %w", path, err)
	}
	if !bytes.Equal(current, expected) {
		return &StaleContentError{Path: path}
	}
	if err := os.WriteFile(path, newContent, 0o644); err != nil {
		return fmt.Errorf("gitsnapshot: write %s: %w", path, err)
	}
	return nil
}

// writeBlobToFile writes a git blob's content to path using `git cat-file`.
func (m *Manager) writeBlobToFile(ctx context.Context, env []string, blobID, path string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", m.SourceDir, "cat-file", "blob", blobID) // #nosec G204 -- fixed git subcommand, internally-derived blob id
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("gitsnapshot: cat-file %s: %w", blobID, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func (m *Manager) hashBlob(ctx context.Context, env []string, data []byte) string {
	cmd := exec.CommandContext(ctx, "git", "-C", m.SourceDir, "hash-object", "-t", "blob", "--stdin") // #nosec G204
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (m *Manager) gitRun(dir string, args ...string) error {
	return m.gitRunCtx(context.Background(), dir, args...)
}

func (m *Manager) gitRunCtx(ctx context.Context, dir string, args ...string) error {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...) // #nosec G204 -- fixed git subcommand, internally-derived args
	cmd.Env = envPlain()
	return cmd.Run()
}

func (m *Manager) gitOut(dir string, args ...string) (string, error) {
	return m.gitOutCtx(context.Background(), dir, args...)
}

func (m *Manager) gitOutCtx(ctx context.Context, dir string, args ...string) (string, error) {
	return m.gitOutEnv(ctx, dir, envPlain(), args...)
}

func (m *Manager) gitOutEnv(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...) // #nosec G204 -- fixed git subcommand, internally-derived args
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// envPlain returns the current environment (used to keep PATH etc.).
func envPlain() []string {
	return os.Environ()
}

// emptyTreeID returns the well-known empty tree hash. This is the git
// object id of the empty tree, stable across all git installations.
const emptyTreeIDValue = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

func emptyTreeID(ctx context.Context, m *Manager) string {
	_ = ctx
	_ = m
	return emptyTreeIDValue
}
