package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// PendingChange represents a staged file modification that has not yet been applied to disk.
type PendingChange struct {
	Path       string
	Action     string // "create", "edit", "overwrite"
	OldContent string
	NewContent string
	Diff       string
	CreatedAt  time.Time
}

// DiffSandbox holds pending file changes so the user can review diffs before applying.
type DiffSandbox struct {
	mu      sync.RWMutex
	changes map[string]*PendingChange
	enabled bool
}

// NewDiffSandbox creates a new, enabled DiffSandbox.
func NewDiffSandbox() *DiffSandbox {
	return &DiffSandbox{
		changes: make(map[string]*PendingChange),
		enabled: true,
	}
}

// IsEnabled returns whether the sandbox is active.
func (ds *DiffSandbox) IsEnabled() bool {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.enabled
}

// Enable activates the sandbox.
func (ds *DiffSandbox) Enable() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.enabled = true
}

// Disable deactivates the sandbox.
func (ds *DiffSandbox) Disable() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.enabled = false
}

// Stage records a pending file change and computes a unified diff.
func (ds *DiffSandbox) Stage(path, action, oldContent, newContent string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	diff := unifiedDiff(oldContent, newContent, path)
	ds.changes[path] = &PendingChange{
		Path:       path,
		Action:     action,
		OldContent: oldContent,
		NewContent: newContent,
		Diff:       diff,
		CreatedAt:  time.Now(),
	}
}

// List returns all pending changes sorted by path.
func (ds *DiffSandbox) List() []*PendingChange {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	out := make([]*PendingChange, 0, len(ds.changes))
	for _, c := range ds.changes {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out
}

// Get returns the pending change for a specific path, or nil.
func (ds *DiffSandbox) Get(path string) *PendingChange {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.changes[path]
}

// Apply writes one pending change to disk and removes it from the sandbox.
func (ds *DiffSandbox) Apply(path string) error {
	ds.mu.Lock()
	change, ok := ds.changes[path]
	if !ok {
		ds.mu.Unlock()
		return fmt.Errorf("no pending change for %s", path)
	}
	// Copy what we need and release lock before I/O
	newContent := change.NewContent
	delete(ds.changes, path)
	ds.mu.Unlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	return os.WriteFile(path, []byte(newContent), 0o644)
}

// ApplyAll writes all pending changes to disk and clears the sandbox.
func (ds *DiffSandbox) ApplyAll() (int, error) {
	ds.mu.Lock()
	// Snapshot changes and clear under lock
	snapshot := make(map[string]*PendingChange, len(ds.changes))
	for k, v := range ds.changes {
		snapshot[k] = v
	}
	ds.changes = make(map[string]*PendingChange)
	ds.mu.Unlock()

	applied := 0
	for path, change := range snapshot {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return applied, fmt.Errorf("create directory %s: %w", dir, err)
		}
		if err := os.WriteFile(path, []byte(change.NewContent), 0o644); err != nil {
			return applied, fmt.Errorf("write %s: %w", path, err)
		}
		applied++
	}
	return applied, nil
}

// Reject discards the pending change for one file.
func (ds *DiffSandbox) Reject(path string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	delete(ds.changes, path)
}

// RejectAll discards all pending changes.
func (ds *DiffSandbox) RejectAll() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.changes = make(map[string]*PendingChange)
}

// Format returns a human-readable summary of all pending changes.
func (ds *DiffSandbox) Format() string {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if len(ds.changes) == 0 {
		return "No pending changes."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Pending changes (%d files):\n", len(ds.changes)))
	for _, c := range ds.List() {
		lines := strings.Count(c.Diff, "\n")
		b.WriteString(fmt.Sprintf("  [%s] %s (%d diff lines)\n", c.Action, c.Path, lines))
	}
	return b.String()
}

// DiffFor returns the unified diff for a single pending file.
func (ds *DiffSandbox) DiffFor(path string) string {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	c, ok := ds.changes[path]
	if !ok {
		return ""
	}
	return c.Diff
}

// DiffAll returns all diffs combined.
func (ds *DiffSandbox) DiffAll() string {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if len(ds.changes) == 0 {
		return ""
	}

	var b strings.Builder
	for _, c := range ds.List() {
		b.WriteString(c.Diff)
		b.WriteString("\n")
	}
	return b.String()
}

// unifiedDiff produces a git-style unified diff between old and new content for the given path.
func unifiedDiff(old, new, path string) string {
	oldLines := splitLines(old)
	newLines := splitLines(new)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- a/%s\n", path))
	b.WriteString(fmt.Sprintf("+++ b/%s\n", path))

	// Simple Myers-like approach: compute LCS then emit hunks
	lcs := computeLCS(oldLines, newLines)

	// Build edit script
	var edits []edit
	oi, ni, li := 0, 0, 0
	for li < len(lcs) {
		for oi < len(oldLines) && oldLines[oi] != lcs[li] {
			edits = append(edits, edit{'-', oldLines[oi]})
			oi++
		}
		for ni < len(newLines) && newLines[ni] != lcs[li] {
			edits = append(edits, edit{'+', newLines[ni]})
			ni++
		}
		edits = append(edits, edit{' ', lcs[li]})
		oi++
		ni++
		li++
	}
	for oi < len(oldLines) {
		edits = append(edits, edit{'-', oldLines[oi]})
		oi++
	}
	for ni < len(newLines) {
		edits = append(edits, edit{'+', newLines[ni]})
		ni++
	}

	// Group edits into hunks with 3 lines of context
	const contextLines = 3
	hunks := groupHunks(edits, contextLines)
	for _, h := range hunks {
		oldStart, oldCount, newStart, newCount := hunkHeader(h, edits, oldLines, newLines, contextLines)
		b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount))
		for _, e := range h {
			b.WriteString(fmt.Sprintf("%c%s\n", e.op, e.line))
		}
	}

	return b.String()
}

type edit struct {
	op   byte
	line string
}

// splitLines splits content into lines. An empty string returns an empty slice.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// Remove trailing empty string from a final newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// computeLCS returns the longest common subsequence of two string slices.
func computeLCS(a, b []string) []string {
	m, n := len(a), len(b)
	// DP table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Backtrack
	lcs := make([]string, 0, dp[m][n])
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			lcs = append(lcs, a[i-1])
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}
	// Reverse
	for left, right := 0, len(lcs)-1; left < right; left, right = left+1, right-1 {
		lcs[left], lcs[right] = lcs[right], lcs[left]
	}
	return lcs
}

// groupHunks groups consecutive edits into hunks separated by context lines.
func groupHunks(edits []edit, ctx int) [][]edit {
	if len(edits) == 0 {
		return nil
	}

	// Find changed regions (non-context edits)
	type region struct{ start, end int }
	var regions []region
	for i, e := range edits {
		if e.op != ' ' {
			if len(regions) == 0 || i > regions[len(regions)-1].end+1 {
				regions = append(regions, region{i, i})
			} else {
				regions[len(regions)-1].end = i
			}
		}
	}

	if len(regions) == 0 {
		return nil // no changes
	}

	// Merge regions that are within 2*ctx of each other, then add context
	var hunks [][]edit
	hunkStart := regions[0].start - ctx
	if hunkStart < 0 {
		hunkStart = 0
	}
	hunkEnd := regions[0].end + ctx
	if hunkEnd >= len(edits) {
		hunkEnd = len(edits) - 1
	}

	for i := 1; i < len(regions); i++ {
		nextStart := regions[i].start - ctx
		if nextStart < 0 {
			nextStart = 0
		}
		if nextStart <= hunkEnd+1 {
			// Merge
			hunkEnd = regions[i].end + ctx
			if hunkEnd >= len(edits) {
				hunkEnd = len(edits) - 1
			}
		} else {
			hunks = append(hunks, edits[hunkStart:hunkEnd+1])
			hunkStart = nextStart
			hunkEnd = regions[i].end + ctx
			if hunkEnd >= len(edits) {
				hunkEnd = len(edits) - 1
			}
		}
	}
	hunks = append(hunks, edits[hunkStart:hunkEnd+1])
	return hunks
}

// hunkHeader computes the old/new start line and count for a hunk.
func hunkHeader(hunk []edit, allEdits []edit, oldLines, newLines []string, ctx int) (int, int, int, int) {
	// Find position of this hunk in the full edit list
	hunkStart := -1
	for i := range allEdits {
		if len(hunk) > 0 && &allEdits[i] == &hunk[0] {
			hunkStart = i
			break
		}
	}

	// Count old and new lines before the hunk to get starting line numbers
	oldLine := 1
	newLine := 1
	for i := 0; i < hunkStart && i < len(allEdits); i++ {
		switch allEdits[i].op {
		case ' ':
			oldLine++
			newLine++
		case '-':
			oldLine++
		case '+':
			newLine++
		}
	}

	oldCount := 0
	newCount := 0
	for _, e := range hunk {
		switch e.op {
		case ' ':
			oldCount++
			newCount++
		case '-':
			oldCount++
		case '+':
			newCount++
		}
	}

	return oldLine, oldCount, newLine, newCount
}
