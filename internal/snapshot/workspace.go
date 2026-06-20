package snapshot

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// WorkspaceSnapshot captures the full state of a project at a point in time.
type WorkspaceSnapshot struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Files       map[string]FileState `json:"files"`
	GitBranch   string               `json:"git_branch"`
	GitCommit   string               `json:"git_commit"`
	CreatedAt   time.Time            `json:"created_at"`
	Size        int64                `json:"size"`
	FileCount   int                  `json:"file_count"`
}

// FileState captures the state of a single file.
type FileState struct {
	Path    string      `json:"path"`
	Content []byte      `json:"content"`
	Mode    os.FileMode `json:"mode"`
	ModTime time.Time   `json:"mod_time"`
	Hash    string      `json:"hash"`
}

// SnapshotDiff represents the difference between a snapshot and the current workspace.
type SnapshotDiff struct {
	Added     []string `json:"added"`
	Modified  []string `json:"modified"`
	Deleted   []string `json:"deleted"`
	Unchanged int      `json:"unchanged"`
}

// SnapshotStore manages workspace snapshots on disk.
type SnapshotStore struct {
	Dir          string
	MaxSnapshots int
	mu           sync.Mutex
}

// ignoredDirs are directories that should be skipped during capture.
var ignoredDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	".hawk":        true,
	"__pycache__":  true,
	".venv":        true,
	"dist":         true,
	"build":        true,
}

// NewSnapshotStore creates a new SnapshotStore with the given directory.
// If dir is empty, defaults to Hawk's user state snapshots directory so
// that state does not leak into <cwd>/.hawk/ when hawk is run from inside
// a Go project root.
func NewSnapshotStore(dir string) *SnapshotStore {
	if dir == "" {
		dir = storage.WorkspaceSnapshotsDir()
	}
	return &SnapshotStore{
		Dir:          dir,
		MaxSnapshots: 20,
	}
}

// Capture takes a snapshot of the given project directory.
func (s *SnapshotStore) Capture(projectDir, name, description string) (*WorkspaceSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := &WorkspaceSnapshot{
		ID:          generateID(),
		Name:        name,
		Description: description,
		Files:       make(map[string]FileState),
		CreatedAt:   time.Now(),
	}

	// Capture git state
	snap.GitBranch = gitCurrentBranch(projectDir)
	snap.GitCommit = gitCurrentCommit(projectDir)

	// Walk the project directory
	var totalSize int64
	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // propagate errors
		}

		// Get relative path
		rel, relErr := filepath.Rel(projectDir, path)
		if relErr != nil {
			return nil
		}

		// Skip root
		if rel == "." {
			return nil
		}

		// Check if this directory should be ignored
		if d.IsDir() {
			base := filepath.Base(path)
			if ignoredDirs[base] {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip non-regular files (symlinks, etc.)
		if !d.Type().IsRegular() {
			return nil
		}

		// Get file info for metadata
		fi, fiErr := d.Info()
		if fiErr != nil {
			return fiErr
		}

		// Read file content
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil // skip unreadable files
		}

		// Compute SHA-256
		hash := sha256.Sum256(content)
		hashStr := hex.EncodeToString(hash[:])

		snap.Files[rel] = FileState{
			Path:    rel,
			Content: content,
			Mode:    fi.Mode(),
			ModTime: fi.ModTime(),
			Hash:    hashStr,
		}

		totalSize += int64(len(content))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking project: %w", err)
	}

	snap.Size = totalSize
	snap.FileCount = len(snap.Files)

	// Save to disk
	if err := s.save(snap); err != nil {
		return nil, fmt.Errorf("saving snapshot: %w", err)
	}

	// Prune if over limit
	if pruneErr := s.prune(); pruneErr != nil {
		// Non-fatal: log but don't fail the capture
		fmt.Fprintf(os.Stderr, "warning: snapshot prune failed: %v\n", pruneErr)
	}

	return snap, nil
}

// Restore restores a snapshot to the given project directory.
func (s *SnapshotStore) Restore(snapshotID string, projectDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap, err := s.load(snapshotID)
	if err != nil {
		return fmt.Errorf("loading snapshot: %w", err)
	}

	// Collect current files (to know what to delete)
	currentFiles := make(map[string]bool)
	walkErr := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(projectDir, path)
		if relErr != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if ignoredDirs[base] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			currentFiles[rel] = true
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("walking current files: %w", walkErr)
	}

	// Delete files that weren't in the snapshot
	for relPath := range currentFiles {
		if _, exists := snap.Files[relPath]; !exists {
			fullPath := filepath.Join(projectDir, relPath)
			if removeErr := os.Remove(fullPath); removeErr != nil && !os.IsNotExist(removeErr) {
				fmt.Fprintf(os.Stderr, "warning: failed to remove %s: %v\n", relPath, removeErr)
			}
			// Clean up empty parent dirs
			removeEmptyParents(filepath.Dir(fullPath), projectDir)
		}
	}

	// Restore all files from snapshot
	for rel, fs := range snap.Files {
		fullPath := filepath.Join(projectDir, rel)

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", rel, err)
		}

		// Write file content
		if err := os.WriteFile(fullPath, fs.Content, fs.Mode); err != nil {
			return fmt.Errorf("writing %s: %w", rel, err)
		}

		// Restore modification time
		if chtimesErr := os.Chtimes(fullPath, fs.ModTime, fs.ModTime); chtimesErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to restore mtime for %s: %v\n", rel, chtimesErr)
		}
	}

	return nil
}

// List returns all snapshots sorted by date (newest first).
func (s *SnapshotStore) List() ([]*WorkspaceSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.list()
}

// list is the internal unlocked version of List.
func (s *SnapshotStore) list() ([]*WorkspaceSnapshot, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var snapshots []*WorkspaceSnapshot
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json.gz") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".json.gz")
		snap, loadErr := s.load(id)
		if loadErr != nil {
			continue // skip corrupted snapshots
		}
		// Clear file content from listing to save memory
		listing := &WorkspaceSnapshot{
			ID:          snap.ID,
			Name:        snap.Name,
			Description: snap.Description,
			GitBranch:   snap.GitBranch,
			GitCommit:   snap.GitCommit,
			CreatedAt:   snap.CreatedAt,
			Size:        snap.Size,
			FileCount:   snap.FileCount,
		}
		snapshots = append(snapshots, listing)
	}

	// Sort by date, newest first
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	return snapshots, nil
}

// Get retrieves a specific snapshot by ID.
func (s *SnapshotStore) Get(id string) (*WorkspaceSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.load(id)
}

// Delete removes a snapshot by ID.
func (s *SnapshotStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.Dir, id+".json.gz")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("snapshot %s not found", id)
	}
	return os.Remove(path)
}

// Diff compares a snapshot to the current workspace state.
func (s *SnapshotStore) Diff(snapshotID string, projectDir string) (*SnapshotDiff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap, err := s.load(snapshotID)
	if err != nil {
		return nil, fmt.Errorf("loading snapshot: %w", err)
	}

	diff := &SnapshotDiff{}

	// Collect current file hashes
	currentFiles := make(map[string]string) // path -> hash
	_ = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(projectDir, path)
		if relErr != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if ignoredDirs[base] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		hash := sha256.Sum256(content)
		currentFiles[rel] = hex.EncodeToString(hash[:])
		return nil
	})

	// Check files in snapshot vs current
	for relPath, fs := range snap.Files {
		currentHash, exists := currentFiles[relPath]
		if !exists {
			diff.Deleted = append(diff.Deleted, relPath)
		} else if currentHash != fs.Hash {
			diff.Modified = append(diff.Modified, relPath)
		} else {
			diff.Unchanged++
		}
	}

	// Check for files added since snapshot
	for relPath := range currentFiles {
		if _, exists := snap.Files[relPath]; !exists {
			diff.Added = append(diff.Added, relPath)
		}
	}

	// Sort slices for deterministic output
	sort.Strings(diff.Added)
	sort.Strings(diff.Modified)
	sort.Strings(diff.Deleted)

	return diff, nil
}

// Prune removes the oldest snapshots when over MaxSnapshots.
func (s *SnapshotStore) Prune() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.prune()
}

// prune is the internal unlocked version of Prune.
func (s *SnapshotStore) prune() error {
	snapshots, err := s.list()
	if err != nil {
		return err
	}

	if len(snapshots) <= s.MaxSnapshots {
		return nil
	}

	// Snapshots are sorted newest first; remove from the end
	for i := s.MaxSnapshots; i < len(snapshots); i++ {
		path := filepath.Join(s.Dir, snapshots[i].ID+".json.gz")
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			fmt.Fprintf(os.Stderr, "warning: failed to prune snapshot %s: %v\n", snapshots[i].ID, removeErr)
		}
	}

	return nil
}

// Save serializes a snapshot and writes it to disk as compressed JSON.
func (s *SnapshotStore) Save(snapshot *WorkspaceSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.save(snapshot)
}

// save is the internal unlocked version of Save.
func (s *SnapshotStore) save(snapshot *WorkspaceSnapshot) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("creating snapshot dir: %w", err)
	}

	path := filepath.Join(s.Dir, snapshot.ID+".json.gz")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to close snapshot file: %v\n", closeErr)
		}
	}()

	gw := gzip.NewWriter(f)
	defer func() {
		if closeErr := gw.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to close gzip writer: %v\n", closeErr)
		}
	}()

	enc := json.NewEncoder(gw)
	if err := enc.Encode(snapshot); err != nil {
		return fmt.Errorf("encoding snapshot: %w", err)
	}

	return nil
}

// load reads a snapshot from disk.
func (s *SnapshotStore) load(id string) (*WorkspaceSnapshot, error) {
	path := filepath.Join(s.Dir, id+".json.gz")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening snapshot %s: %w", id, err)
	}
	defer func() { _ = f.Close() }() // read-only, close error not critical

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("decompressing snapshot %s: %w", id, err)
	}
	defer func() { _ = gr.Close() }() // read-only, close error not critical

	var snap WorkspaceSnapshot
	dec := json.NewDecoder(gr)
	if err := dec.Decode(&snap); err != nil {
		return nil, fmt.Errorf("decoding snapshot %s: %w", id, err)
	}

	return &snap, nil
}

// FormatList produces a human-readable listing of snapshots.
func FormatList(snapshots []*WorkspaceSnapshot) string {
	if len(snapshots) == 0 {
		return "No snapshots found."
	}

	var sb strings.Builder
	sb.WriteString("Snapshots:\n")

	for i, snap := range snapshots {
		age := formatAge(time.Since(snap.CreatedAt))
		size := formatSize(snap.Size)
		sb.WriteString(fmt.Sprintf(
			"  %d. [%s] %q (%s, %d files, %s)\n",
			i+1,
			snap.ID[:7],
			snap.Name,
			age,
			snap.FileCount,
			size,
		))
	}

	return sb.String()
}

// FormatDiff produces a human-readable summary of a snapshot diff.
func FormatDiff(diff *SnapshotDiff, snapshotName string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Changes since snapshot %q:\n", snapshotName))
	sb.WriteString(fmt.Sprintf("  Added: %d %s\n", len(diff.Added), pluralize("file", len(diff.Added))))
	sb.WriteString(fmt.Sprintf("  Modified: %d %s\n", len(diff.Modified), pluralize("file", len(diff.Modified))))
	sb.WriteString(fmt.Sprintf("  Deleted: %d %s\n", len(diff.Deleted), pluralize("file", len(diff.Deleted))))
	sb.WriteString(fmt.Sprintf("  Unchanged: %d %s\n", diff.Unchanged, pluralize("file", diff.Unchanged)))
	return sb.String()
}

// generateID creates a unique snapshot ID using timestamp and random bytes.
func generateID() string {
	now := time.Now()
	h := sha256.New()
	_, _ = io.WriteString(h, now.String())
	_, _ = io.WriteString(h, fmt.Sprintf("%d", now.UnixNano()))
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// gitCurrentBranch returns the current git branch name.
func gitCurrentBranch(dir string) string {
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitCurrentCommit returns the current git HEAD commit hash.
func gitCurrentCommit(dir string) string {
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// removeEmptyParents removes empty directories up to the stop directory.
func removeEmptyParents(dir, stopDir string) {
	for dir != stopDir && dir != "." && dir != "/" {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		_ = os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

// formatAge converts a duration to a human-readable age string.
func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}

// formatSize converts bytes to a human-readable string.
func formatSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%dB", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%dKB", bytes/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
}

// pluralize returns the singular or plural form of a word.
func pluralize(word string, count int) string {
	if count == 1 {
		return word
	}
	return word + "s"
}
