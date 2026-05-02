package diffsandbox

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ChangeType identifies the kind of file modification.
type ChangeType int

const (
	// ChangeCreate represents creating a new file.
	ChangeCreate ChangeType = iota
	// ChangeModify represents modifying an existing file.
	ChangeModify
	// ChangeDelete represents deleting a file.
	ChangeDelete
)

// String returns a human-readable name for the change type.
func (ct ChangeType) String() string {
	switch ct {
	case ChangeCreate:
		return "create"
	case ChangeModify:
		return "modify"
	case ChangeDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// Change represents a single proposed file modification.
type Change struct {
	ID        string
	Path      string // relative file path
	Type      ChangeType
	Content   string    // new content (for Create/Modify)
	Original  string    // original content (for Modify, used for diff)
	Timestamp time.Time
}

// SandboxStats holds statistics about pending changes.
type SandboxStats struct {
	FilesCreated  int
	FilesModified int
	FilesDeleted  int
	LinesAdded    int
	LinesRemoved  int
}

// Sandbox accumulates proposed changes without modifying the filesystem.
type Sandbox struct {
	rootDir string
	changes map[string]*Change // path -> latest change
	order   []string           // insertion order
	mu      sync.RWMutex
}

// New creates a sandbox rooted at the given directory.
func New(rootDir string) *Sandbox {
	return &Sandbox{
		rootDir: rootDir,
		changes: make(map[string]*Change),
		order:   nil,
	}
}

// generateID produces an 8 hex-char random ID using crypto/rand.
func generateID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback: should never happen in practice.
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xFFFFFFFF)
	}
	return hex.EncodeToString(b)
}

// ProposeCreate stages a new file creation.
func (s *Sandbox) ProposeCreate(path string, content string) *Change {
	s.mu.Lock()
	defer s.mu.Unlock()

	c := &Change{
		ID:        generateID(),
		Path:      path,
		Type:      ChangeCreate,
		Content:   content,
		Timestamp: time.Now(),
	}

	if _, exists := s.changes[path]; !exists {
		s.order = append(s.order, path)
	}
	s.changes[path] = c
	return c
}

// ProposeModify stages a file modification. Reads the original content from disk.
func (s *Sandbox) ProposeModify(path string, newContent string) (*Change, error) {
	absPath := s.absPath(path)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read original %s: %w", path, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	c := &Change{
		ID:        generateID(),
		Path:      path,
		Type:      ChangeModify,
		Content:   newContent,
		Original:  string(data),
		Timestamp: time.Now(),
	}

	if _, exists := s.changes[path]; !exists {
		s.order = append(s.order, path)
	}
	s.changes[path] = c
	return c, nil
}

// ProposeDelete stages a file deletion.
func (s *Sandbox) ProposeDelete(path string) *Change {
	s.mu.Lock()
	defer s.mu.Unlock()

	c := &Change{
		ID:        generateID(),
		Path:      path,
		Type:      ChangeDelete,
		Timestamp: time.Now(),
	}

	if _, exists := s.changes[path]; !exists {
		s.order = append(s.order, path)
	}
	s.changes[path] = c
	return c
}

// Diff returns the unified diff of all accumulated changes.
func (s *Sandbox) Diff() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.changes) == 0 {
		return ""
	}

	var b strings.Builder
	for _, path := range s.order {
		c, ok := s.changes[path]
		if !ok {
			continue
		}
		d := s.diffForChange(c)
		if d != "" {
			b.WriteString(d)
		}
	}
	return b.String()
}

// DiffFile returns the diff for a single file.
func (s *Sandbox) DiffFile(path string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.changes[path]
	if !ok {
		return ""
	}
	return s.diffForChange(c)
}

// diffForChange computes the unified diff for a single change (caller must hold lock).
func (s *Sandbox) diffForChange(c *Change) string {
	switch c.Type {
	case ChangeCreate:
		return unifiedDiff("a/"+c.Path, "b/"+c.Path, "", c.Content)
	case ChangeModify:
		return unifiedDiff("a/"+c.Path, "b/"+c.Path, c.Original, c.Content)
	case ChangeDelete:
		return unifiedDiff("a/"+c.Path, "b/"+c.Path, c.Original, "")
	default:
		return ""
	}
}

// Changes returns all pending changes in insertion order.
func (s *Sandbox) Changes() []*Change {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Change, 0, len(s.order))
	for _, path := range s.order {
		if c, ok := s.changes[path]; ok {
			result = append(result, c)
		}
	}
	return result
}

// HasChanges returns true if there are any pending changes.
func (s *Sandbox) HasChanges() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.changes) > 0
}

// Apply writes all pending changes to the actual filesystem.
// Each file write is atomic: content is written to a temp file then renamed.
func (s *Sandbox) Apply() error {
	s.mu.Lock()
	// Snapshot and clear under lock
	snapshot := make([]*Change, 0, len(s.order))
	for _, path := range s.order {
		if c, ok := s.changes[path]; ok {
			snapshot = append(snapshot, c)
		}
	}
	s.changes = make(map[string]*Change)
	s.order = nil
	s.mu.Unlock()

	for _, c := range snapshot {
		if err := s.applyChange(c); err != nil {
			return err
		}
	}
	return nil
}

// ApplyFile applies only a single file's changes.
func (s *Sandbox) ApplyFile(path string) error {
	s.mu.Lock()
	c, ok := s.changes[path]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("no pending change for %s", path)
	}
	delete(s.changes, path)
	// Remove from order
	for i, p := range s.order {
		if p == path {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	s.mu.Unlock()

	return s.applyChange(c)
}

// applyChange writes a single change to disk atomically.
func (s *Sandbox) applyChange(c *Change) error {
	absPath := s.absPath(c.Path)

	switch c.Type {
	case ChangeCreate, ChangeModify:
		dir := filepath.Dir(absPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
		// Write to temp then rename for atomicity.
		tmp, err := os.CreateTemp(dir, ".diffsandbox-*")
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		tmpName := tmp.Name()
		if _, err := tmp.WriteString(c.Content); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return fmt.Errorf("write temp file: %w", err)
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmpName)
			return fmt.Errorf("close temp file: %w", err)
		}
		if err := os.Chmod(tmpName, 0o644); err != nil {
			os.Remove(tmpName)
			return fmt.Errorf("chmod temp file: %w", err)
		}
		if err := os.Rename(tmpName, absPath); err != nil {
			os.Remove(tmpName)
			return fmt.Errorf("rename temp to %s: %w", absPath, err)
		}

	case ChangeDelete:
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete %s: %w", absPath, err)
		}
	}
	return nil
}

// Discard removes all pending changes without applying.
func (s *Sandbox) Discard() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.changes = make(map[string]*Change)
	s.order = nil
}

// DiscardFile removes a single file's pending changes.
func (s *Sandbox) DiscardFile(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.changes, path)
	for i, p := range s.order {
		if p == path {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
}

// Summary returns a brief description of pending changes.
func (s *Sandbox) Summary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.changes) == 0 {
		return "No pending changes."
	}

	var b strings.Builder
	stats := s.statsLocked()
	b.WriteString(fmt.Sprintf("Pending changes (%d file(s)):\n", len(s.changes)))

	for _, path := range s.order {
		c, ok := s.changes[path]
		if !ok {
			continue
		}
		b.WriteString(fmt.Sprintf("  [%s] %s\n", c.Type.String(), c.Path))
	}

	b.WriteString(fmt.Sprintf("Stats: +%d -%d lines | %d created, %d modified, %d deleted\n",
		stats.LinesAdded, stats.LinesRemoved,
		stats.FilesCreated, stats.FilesModified, stats.FilesDeleted))

	return b.String()
}

// Stats returns change statistics.
func (s *Sandbox) Stats() SandboxStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statsLocked()
}

// statsLocked computes stats (caller must hold at least RLock).
func (s *Sandbox) statsLocked() SandboxStats {
	var st SandboxStats
	for _, c := range s.changes {
		switch c.Type {
		case ChangeCreate:
			st.FilesCreated++
		case ChangeModify:
			st.FilesModified++
		case ChangeDelete:
			st.FilesDeleted++
		}

		oldLines := splitLines(c.Original)
		newLines := splitLines(c.Content)

		// Count added/removed by diffing
		lcs := computeLCS(oldLines, newLines)
		// Walk LCS against old/new to count adds/removes
		oi, ni, li := 0, 0, 0
		for li < len(lcs) {
			for oi < len(oldLines) && oldLines[oi] != lcs[li] {
				st.LinesRemoved++
				oi++
			}
			for ni < len(newLines) && newLines[ni] != lcs[li] {
				st.LinesAdded++
				ni++
			}
			oi++
			ni++
			li++
		}
		for oi < len(oldLines) {
			st.LinesRemoved++
			oi++
		}
		for ni < len(newLines) {
			st.LinesAdded++
			ni++
		}
	}
	return st
}

// absPath resolves a path relative to the sandbox root.
func (s *Sandbox) absPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.rootDir, path)
}

// SortedPaths returns all pending paths in sorted order.
func (s *Sandbox) SortedPaths() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	paths := make([]string, 0, len(s.changes))
	for p := range s.changes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}
