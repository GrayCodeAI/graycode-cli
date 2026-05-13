package engine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// UndoEntry represents a single undoable operation that modified one or more files.
type UndoEntry struct {
	ID          string
	Timestamp   time.Time
	Description string
	Files       []FileSnapshot
	ToolName    string
	ToolArgs    map[string]interface{}
}

// FileSnapshot captures the state of a file before and after modification.
type FileSnapshot struct {
	Path            string
	OriginalContent []byte
	ModifiedContent []byte
	OriginalMode    os.FileMode
	WasNew          bool
}

// UndoManager tracks file modifications and allows granular rollback of changes.
type UndoManager struct {
	Stack      []UndoEntry
	MaxEntries int
	mu         sync.Mutex

	// pending holds file states captured by BeforeModify, keyed by absolute path.
	pending map[string]fileCapture
}

// fileCapture is the internal representation of a pre-modification file state.
type fileCapture struct {
	path         string
	content      []byte
	mode         os.FileMode
	wasNew       bool
	capturedAt   time.Time
}

// NewUndoManager creates a new UndoManager with a default capacity of 100 entries.
func NewUndoManager() *UndoManager {
	return &UndoManager{
		Stack:      make([]UndoEntry, 0),
		MaxEntries: 100,
		pending:    make(map[string]fileCapture),
	}
}

// BeforeModify captures the current file state before modification. If the file
// does not exist, it marks the capture as WasNew so that undo will delete it.
func (um *UndoManager) BeforeModify(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("undo: resolve path %q: %w", path, err)
	}

	um.mu.Lock()
	defer um.mu.Unlock()

	info, statErr := os.Stat(absPath)
	if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("undo: stat %q: %w", absPath, statErr)
	}

	capture := fileCapture{
		path:       absPath,
		capturedAt: time.Now(),
	}

	if os.IsNotExist(statErr) {
		capture.wasNew = true
		capture.mode = 0644
	} else {
		content, readErr := os.ReadFile(absPath)
		if readErr != nil {
			return fmt.Errorf("undo: read %q: %w", absPath, readErr)
		}
		capture.content = content
		capture.mode = info.Mode()
	}

	um.pending[absPath] = capture
	return nil
}

// RecordChange creates an UndoEntry from previously captured states, reads the
// new file content for each path, and appends the entry to the stack. It returns
// the generated entry ID. The stack is trimmed if it exceeds MaxEntries.
func (um *UndoManager) RecordChange(description, toolName string, toolArgs map[string]interface{}, paths []string) string {
	um.mu.Lock()
	defer um.mu.Unlock()

	id := generateUndoID()
	entry := UndoEntry{
		ID:          id,
		Timestamp:   time.Now(),
		Description: description,
		ToolName:    toolName,
		ToolArgs:    toolArgs,
		Files:       make([]FileSnapshot, 0, len(paths)),
	}

	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			continue
		}

		capture, ok := um.pending[absPath]
		if !ok {
			// No pre-capture for this path; skip it.
			continue
		}

		snapshot := FileSnapshot{
			Path:            absPath,
			OriginalContent: capture.content,
			OriginalMode:    capture.mode,
			WasNew:          capture.wasNew,
		}

		// Read current (modified) content.
		modContent, err := os.ReadFile(absPath)
		if err == nil {
			snapshot.ModifiedContent = modContent
		}

		entry.Files = append(entry.Files, snapshot)
		delete(um.pending, absPath)
	}

	um.Stack = append(um.Stack, entry)

	// Trim oldest entries if over capacity.
	if len(um.Stack) > um.MaxEntries {
		excess := len(um.Stack) - um.MaxEntries
		um.Stack = um.Stack[excess:]
	}

	return id
}

// Undo pops the last entry from the stack and restores all files to their
// original state. If a file WasNew, it is deleted. Returns the undone entry.
func (um *UndoManager) Undo() (*UndoEntry, error) {
	um.mu.Lock()
	defer um.mu.Unlock()

	if len(um.Stack) == 0 {
		return nil, fmt.Errorf("undo: stack is empty, nothing to undo")
	}

	entry := um.Stack[len(um.Stack)-1]
	um.Stack = um.Stack[:len(um.Stack)-1]

	if err := restoreEntry(&entry); err != nil {
		return &entry, err
	}

	return &entry, nil
}

// UndoN undoes the last n changes, returning the undone entries in order from
// most recent to oldest.
func (um *UndoManager) UndoN(n int) ([]*UndoEntry, error) {
	um.mu.Lock()
	defer um.mu.Unlock()

	if n <= 0 {
		return nil, fmt.Errorf("undo: n must be positive, got %d", n)
	}
	if n > len(um.Stack) {
		return nil, fmt.Errorf("undo: requested %d undos but only %d entries on stack", n, len(um.Stack))
	}

	undone := make([]*UndoEntry, 0, n)
	for i := 0; i < n; i++ {
		entry := um.Stack[len(um.Stack)-1]
		um.Stack = um.Stack[:len(um.Stack)-1]

		if err := restoreEntry(&entry); err != nil {
			undone = append(undone, &entry)
			return undone, err
		}
		undone = append(undone, &entry)
	}

	return undone, nil
}

// UndoTo undoes everything back to (and including) the entry with the specified
// ID. Returns all undone entries from most recent to the target.
func (um *UndoManager) UndoTo(entryID string) ([]*UndoEntry, error) {
	um.mu.Lock()
	defer um.mu.Unlock()

	// Find the target entry index.
	targetIdx := -1
	for i, e := range um.Stack {
		if e.ID == entryID {
			targetIdx = i
			break
		}
	}
	if targetIdx == -1 {
		return nil, fmt.Errorf("undo: entry %q not found in stack", entryID)
	}

	count := len(um.Stack) - targetIdx
	undone := make([]*UndoEntry, 0, count)

	for i := 0; i < count; i++ {
		entry := um.Stack[len(um.Stack)-1]
		um.Stack = um.Stack[:len(um.Stack)-1]

		if err := restoreEntry(&entry); err != nil {
			undone = append(undone, &entry)
			return undone, err
		}
		undone = append(undone, &entry)
	}

	return undone, nil
}

// Peek returns the last entry without removing it, or nil if the stack is empty.
func (um *UndoManager) Peek() *UndoEntry {
	um.mu.Lock()
	defer um.mu.Unlock()

	if len(um.Stack) == 0 {
		return nil
	}

	entry := um.Stack[len(um.Stack)-1]
	return &entry
}

// History returns the most recent entries up to limit, ordered from newest to
// oldest. If limit <= 0 or exceeds the stack size, all entries are returned.
func (um *UndoManager) History(limit int) []UndoEntry {
	um.mu.Lock()
	defer um.mu.Unlock()

	if limit <= 0 || limit > len(um.Stack) {
		limit = len(um.Stack)
	}

	result := make([]UndoEntry, limit)
	for i := 0; i < limit; i++ {
		result[i] = um.Stack[len(um.Stack)-1-i]
	}
	return result
}

// FormatHistory formats undo entries for terminal display with relative
// timestamps and change statistics.
func (um *UndoManager) FormatHistory(entries []UndoEntry) string {
	if len(entries) == 0 {
		return "Undo History: (empty)"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Undo History (last %d):\n", len(entries)))

	now := time.Now()
	for i, entry := range entries {
		elapsed := now.Sub(entry.Timestamp)
		timeStr := formatElapsed(elapsed)

		// Calculate line additions and deletions across all files.
		added, removed := countChanges(entry.Files)
		changeStr := formatChanges(added, removed)

		sb.WriteString(fmt.Sprintf("  %d. [%s] %s %s\n", i+1, timeStr, entry.Description, changeStr))
	}

	return sb.String()
}

// DiffEntry shows what changed in the entry using a unified diff-like format.
func (um *UndoManager) DiffEntry(entry *UndoEntry) string {
	if entry == nil {
		return ""
	}

	var sb strings.Builder
	for _, f := range entry.Files {
		sb.WriteString(fmt.Sprintf("--- %s\n", f.Path))
		sb.WriteString(fmt.Sprintf("+++ %s\n", f.Path))

		if f.WasNew {
			sb.WriteString("(new file)\n")
			lines := strings.Split(string(f.ModifiedContent), "\n")
			for _, line := range lines {
				if line != "" {
					sb.WriteString(fmt.Sprintf("+%s\n", line))
				}
			}
			continue
		}

		origLines := strings.Split(string(f.OriginalContent), "\n")
		modLines := strings.Split(string(f.ModifiedContent), "\n")

		// Simple line-by-line diff: show removed and added lines.
		origSet := make(map[string]int)
		for _, l := range origLines {
			origSet[l]++
		}
		modSet := make(map[string]int)
		for _, l := range modLines {
			modSet[l]++
		}

		for _, l := range origLines {
			if modSet[l] > 0 {
				modSet[l]--
			} else {
				sb.WriteString(fmt.Sprintf("-%s\n", l))
			}
		}

		// Reset modSet for additions.
		origSet2 := make(map[string]int)
		for _, l := range origLines {
			origSet2[l]++
		}
		for _, l := range modLines {
			if origSet2[l] > 0 {
				origSet2[l]--
			} else {
				sb.WriteString(fmt.Sprintf("+%s\n", l))
			}
		}
	}

	return sb.String()
}

// Clear empties the undo stack entirely.
func (um *UndoManager) Clear() {
	um.mu.Lock()
	defer um.mu.Unlock()

	um.Stack = make([]UndoEntry, 0)
	um.pending = make(map[string]fileCapture)
}

// Size returns the number of entries currently on the stack.
func (um *UndoManager) Size() int {
	um.mu.Lock()
	defer um.mu.Unlock()

	return len(um.Stack)
}

// restoreEntry writes original content back or deletes new files. Must be called
// with the lock held or on a local copy.
func restoreEntry(entry *UndoEntry) error {
	for _, f := range entry.Files {
		if f.WasNew {
			// File was created by the operation; delete it.
			if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("undo: remove new file %q: %w", f.Path, err)
			}
			continue
		}

		// Ensure parent directory exists.
		dir := filepath.Dir(f.Path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("undo: mkdir %q: %w", dir, err)
		}

		if err := os.WriteFile(f.Path, f.OriginalContent, f.OriginalMode); err != nil {
			return fmt.Errorf("undo: restore %q: %w", f.Path, err)
		}
	}
	return nil
}

// generateUndoID creates a short random hex ID for undo entries.
func generateUndoID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// formatElapsed converts a duration into a human-friendly relative time string.
func formatElapsed(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// countChanges calculates total lines added and removed across all file snapshots.
func countChanges(files []FileSnapshot) (added, removed int) {
	for _, f := range files {
		if f.WasNew {
			added += countLines(f.ModifiedContent)
			continue
		}
		origCount := countLines(f.OriginalContent)
		modCount := countLines(f.ModifiedContent)
		if modCount > origCount {
			added += modCount - origCount
		} else {
			removed += origCount - modCount
		}
	}
	return
}

// countLines counts the number of lines in content.
func countLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	count := 1
	for _, b := range content {
		if b == '\n' {
			count++
		}
	}
	return count
}

// formatChanges formats add/remove counts for display.
func formatChanges(added, removed int) string {
	parts := make([]string, 0, 2)
	if added > 0 {
		parts = append(parts, fmt.Sprintf("+%d", added))
	}
	if removed > 0 {
		parts = append(parts, fmt.Sprintf("-%d", removed))
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, ", ") + ")"
}
