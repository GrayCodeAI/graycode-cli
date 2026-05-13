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

// StagingArea provides a local staging area for agent edits, allowing review
// before changes are applied to disk — like a git staging area for the agent's modifications.
type StagingArea struct {
	Staged map[string]*StagedChange
	mu     sync.RWMutex
}

// StagedChange represents a file modification held in the staging area.
type StagedChange struct {
	File        string
	Original    string
	Modified    string
	Hunks       []StagedHunk
	Status      string // "staged", "applied", "rejected"
	StagedAt    time.Time
	Description string
}

// StagedHunk represents a contiguous block of changes within a staged file.
type StagedHunk struct {
	StartLine int
	EndLine   int
	OldLines  []string
	NewLines  []string
	Approved  bool
}

// NewStagingArea creates a new empty StagingArea.
func NewStagingArea() *StagingArea {
	return &StagingArea{
		Staged: make(map[string]*StagedChange),
	}
}

// Stage computes a diff between original and modified content and adds it to the staging area.
func (sa *StagingArea) Stage(file, original, modified, description string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	hunks := computeStagedHunks(original, modified)

	sa.Staged[file] = &StagedChange{
		File:        file,
		Original:    original,
		Modified:    modified,
		Hunks:       hunks,
		Status:      "staged",
		StagedAt:    time.Now(),
		Description: description,
	}
}

// ApplyAll writes all staged changes to disk and returns the list of files modified.
func (sa *StagingArea) ApplyAll() ([]string, error) {
	sa.mu.Lock()
	// Snapshot staged changes
	snapshot := make(map[string]*StagedChange, len(sa.Staged))
	for k, v := range sa.Staged {
		if v.Status == "staged" {
			snapshot[k] = v
		}
	}
	sa.mu.Unlock()

	var applied []string
	for file, change := range snapshot {
		content := sa.buildApprovedContent(change)

		dir := filepath.Dir(file)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return applied, fmt.Errorf("create directory %s: %w", dir, err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			return applied, fmt.Errorf("write %s: %w", file, err)
		}
		applied = append(applied, file)

		sa.mu.Lock()
		if sc, ok := sa.Staged[file]; ok {
			sc.Status = "applied"
		}
		sa.mu.Unlock()
	}

	sort.Strings(applied)
	return applied, nil
}

// ApplyFile applies a single file's staged changes to disk.
func (sa *StagingArea) ApplyFile(file string) error {
	sa.mu.RLock()
	change, ok := sa.Staged[file]
	if !ok {
		sa.mu.RUnlock()
		return fmt.Errorf("no staged change for %s", file)
	}
	if change.Status != "staged" {
		sa.mu.RUnlock()
		return fmt.Errorf("change for %s is not in staged status (current: %s)", file, change.Status)
	}
	sa.mu.RUnlock()

	content := sa.buildApprovedContent(change)

	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", file, err)
	}

	sa.mu.Lock()
	change.Status = "applied"
	sa.mu.Unlock()

	return nil
}

// Reject removes a file from staging without applying its changes.
func (sa *StagingArea) Reject(file string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if change, ok := sa.Staged[file]; ok {
		change.Status = "rejected"
	}
}

// RejectHunk rejects a specific hunk within a staged file (partial staging).
func (sa *StagingArea) RejectHunk(file string, hunkIndex int) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	change, ok := sa.Staged[file]
	if !ok {
		return
	}
	if hunkIndex < 0 || hunkIndex >= len(change.Hunks) {
		return
	}
	change.Hunks[hunkIndex].Approved = false
}

// ApproveHunk marks a specific hunk as approved within a staged file.
func (sa *StagingArea) ApproveHunk(file string, hunkIndex int) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	change, ok := sa.Staged[file]
	if !ok {
		return
	}
	if hunkIndex < 0 || hunkIndex >= len(change.Hunks) {
		return
	}
	change.Hunks[hunkIndex].Approved = true
}

// GetStaged returns a copy of all staged changes.
func (sa *StagingArea) GetStaged() map[string]*StagedChange {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	result := make(map[string]*StagedChange, len(sa.Staged))
	for k, v := range sa.Staged {
		result[k] = v
	}
	return result
}

// GetDiff returns a unified diff for a staged file.
func (sa *StagingArea) GetDiff(file string) string {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	change, ok := sa.Staged[file]
	if !ok {
		return ""
	}

	return unifiedDiffStaging(change.Original, change.Modified, file)
}

// FormatStaging returns a human-readable summary of the staging area.
func (sa *StagingArea) FormatStaging() string {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	// Collect only staged entries
	var staged []*StagedChange
	for _, change := range sa.Staged {
		if change.Status == "staged" {
			staged = append(staged, change)
		}
	}

	if len(staged) == 0 {
		return "Staging Area (0 files):\nNo staged changes."
	}

	sort.Slice(staged, func(i, j int) bool {
		return staged[i].File < staged[j].File
	})

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Staging Area (%d files):\n", len(staged)))
	b.WriteString("─────────────────────────\n")

	readyCount := 0
	pendingCount := 0

	for _, change := range staged {
		adds, dels := countAddsDels(change.Original, change.Modified)
		changeType := detectChangeType(change.Original, change.Modified)

		statsStr := formatStats(adds, dels)
		b.WriteString(fmt.Sprintf("\n%s %s (%s) [%d hunks]\n", changeType, change.File, statsStr, len(change.Hunks)))

		allApproved := true
		for i, hunk := range change.Hunks {
			status := "○" // open circle - pending
			if hunk.Approved {
				status = "✓" // checkmark - approved
			} else {
				allApproved = false
			}

			desc := hunkDescription(hunk, i)
			b.WriteString(fmt.Sprintf("  Hunk %d: %s %s\n", i+1, desc, status))
		}

		if allApproved && len(change.Hunks) > 0 {
			readyCount++
		} else {
			pendingCount++
		}
	}

	b.WriteString("\n")
	if pendingCount > 0 {
		b.WriteString(fmt.Sprintf("Ready to apply: %d files (%d pending review)\n", readyCount, pendingCount))
	} else {
		b.WriteString(fmt.Sprintf("Ready to apply: %d files\n", readyCount))
	}

	return b.String()
}

// Clear discards all staged changes.
func (sa *StagingArea) Clear() {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	sa.Staged = make(map[string]*StagedChange)
}

// HasPending returns true if there are any changes in "staged" status.
func (sa *StagingArea) HasPending() bool {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	for _, change := range sa.Staged {
		if change.Status == "staged" {
			return true
		}
	}
	return false
}

// buildApprovedContent constructs the final file content by applying only approved hunks.
// If no hunks are explicitly approved/rejected (all approved), the full modified content is used.
func (sa *StagingArea) buildApprovedContent(change *StagedChange) string {
	// Check if any hunk selection is active (i.e., at least one hunk is not approved)
	hasRejected := false
	for _, h := range change.Hunks {
		if !h.Approved {
			hasRejected = true
			break
		}
	}

	// If all hunks are approved (or no explicit rejection), use full modified content
	if !hasRejected {
		return change.Modified
	}

	// Partial apply: reconstruct content applying only approved hunks
	originalLines := splitLinesStaging(change.Original)
	var result []string
	origIdx := 0

	for _, hunk := range change.Hunks {
		if hunk.StartLine > 0 {
			// Add lines before this hunk
			endBefore := hunk.StartLine - 1
			if endBefore > len(originalLines) {
				endBefore = len(originalLines)
			}
			for origIdx < endBefore {
				result = append(result, originalLines[origIdx])
				origIdx++
			}
		}

		if hunk.Approved {
			// Apply new lines
			result = append(result, hunk.NewLines...)
		} else {
			// Keep old lines
			result = append(result, hunk.OldLines...)
		}

		// Advance past the old lines covered by this hunk
		hunkOldEnd := hunk.EndLine
		if hunkOldEnd > len(originalLines) {
			hunkOldEnd = len(originalLines)
		}
		origIdx = hunkOldEnd
	}

	// Append remaining original lines
	for origIdx < len(originalLines) {
		result = append(result, originalLines[origIdx])
		origIdx++
	}

	if len(result) == 0 {
		return ""
	}
	return strings.Join(result, "\n") + "\n"
}

// computeStagedHunks computes hunks between original and modified content.
func computeStagedHunks(original, modified string) []StagedHunk {
	oldLines := splitLinesStaging(original)
	newLines := splitLinesStaging(modified)

	lcs := computeLCSStaging(oldLines, newLines)

	// Build edit script
	type editOp struct {
		op   byte // ' ', '-', '+'
		line string
		oldN int // 1-based old line number (for '-' and ' ')
		newN int // 1-based new line number (for '+' and ' ')
	}

	var edits []editOp
	oi, ni, li := 0, 0, 0
	for li < len(lcs) {
		for oi < len(oldLines) && oldLines[oi] != lcs[li] {
			edits = append(edits, editOp{'-', oldLines[oi], oi + 1, 0})
			oi++
		}
		for ni < len(newLines) && newLines[ni] != lcs[li] {
			edits = append(edits, editOp{'+', newLines[ni], 0, ni + 1})
			ni++
		}
		edits = append(edits, editOp{' ', lcs[li], oi + 1, ni + 1})
		oi++
		ni++
		li++
	}
	for oi < len(oldLines) {
		edits = append(edits, editOp{'-', oldLines[oi], oi + 1, 0})
		oi++
	}
	for ni < len(newLines) {
		edits = append(edits, editOp{'+', newLines[ni], 0, ni + 1})
		ni++
	}

	// Group changed regions into hunks
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

	var hunks []StagedHunk
	for _, r := range regions {
		var oldL, newL []string
		startLine := 0
		endLine := 0

		for i := r.start; i <= r.end; i++ {
			e := edits[i]
			switch e.op {
			case '-':
				oldL = append(oldL, e.line)
				if startLine == 0 {
					startLine = e.oldN
				}
				endLine = e.oldN
			case '+':
				newL = append(newL, e.line)
				if startLine == 0 {
					// For pure additions, find nearest old line from preceding context
					for back := i - 1; back >= 0; back-- {
						if edits[back].oldN > 0 {
							startLine = edits[back].oldN + 1
							break
						}
					}
					if startLine == 0 {
						startLine = 1
					}
				}
			}
		}

		if startLine == 0 {
			startLine = 1
		}
		if endLine == 0 {
			endLine = startLine
		}

		hunks = append(hunks, StagedHunk{
			StartLine: startLine,
			EndLine:   endLine,
			OldLines:  oldL,
			NewLines:  newL,
			Approved:  true, // default to approved
		})
	}

	return hunks
}

// splitLinesStaging splits content into lines, handling empty input.
func splitLinesStaging(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// computeLCSStaging returns the longest common subsequence of two string slices.
func computeLCSStaging(a, b []string) []string {
	m, n := len(a), len(b)
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
	for left, right := 0, len(lcs)-1; left < right; left, right = left+1, right-1 {
		lcs[left], lcs[right] = lcs[right], lcs[left]
	}
	return lcs
}

// unifiedDiffStaging produces a unified diff between old and new content.
func unifiedDiffStaging(old, new, path string) string {
	oldLines := splitLinesStaging(old)
	newLines := splitLinesStaging(new)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- a/%s\n", path))
	b.WriteString(fmt.Sprintf("+++ b/%s\n", path))

	lcs := computeLCSStaging(oldLines, newLines)

	type editEntry struct {
		op   byte
		line string
	}

	var edits []editEntry
	oi, ni, li := 0, 0, 0
	for li < len(lcs) {
		for oi < len(oldLines) && oldLines[oi] != lcs[li] {
			edits = append(edits, editEntry{'-', oldLines[oi]})
			oi++
		}
		for ni < len(newLines) && newLines[ni] != lcs[li] {
			edits = append(edits, editEntry{'+', newLines[ni]})
			ni++
		}
		edits = append(edits, editEntry{' ', lcs[li]})
		oi++
		ni++
		li++
	}
	for oi < len(oldLines) {
		edits = append(edits, editEntry{'-', oldLines[oi]})
		oi++
	}
	for ni < len(newLines) {
		edits = append(edits, editEntry{'+', newLines[ni]})
		ni++
	}

	// Group into hunks with 3 lines context
	const ctx = 3
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
		return ""
	}

	// Merge nearby regions and emit hunks
	type hunkRange struct{ start, end int }
	var hunkRanges []hunkRange

	hs := regions[0].start - ctx
	if hs < 0 {
		hs = 0
	}
	he := regions[0].end + ctx
	if he >= len(edits) {
		he = len(edits) - 1
	}

	for i := 1; i < len(regions); i++ {
		ns := regions[i].start - ctx
		if ns < 0 {
			ns = 0
		}
		if ns <= he+1 {
			he = regions[i].end + ctx
			if he >= len(edits) {
				he = len(edits) - 1
			}
		} else {
			hunkRanges = append(hunkRanges, hunkRange{hs, he})
			hs = ns
			he = regions[i].end + ctx
			if he >= len(edits) {
				he = len(edits) - 1
			}
		}
	}
	hunkRanges = append(hunkRanges, hunkRange{hs, he})

	for _, hr := range hunkRanges {
		// Compute header
		oldLine := 1
		newLine := 1
		for i := 0; i < hr.start; i++ {
			switch edits[i].op {
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
		for i := hr.start; i <= hr.end; i++ {
			switch edits[i].op {
			case ' ':
				oldCount++
				newCount++
			case '-':
				oldCount++
			case '+':
				newCount++
			}
		}

		b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldLine, oldCount, newLine, newCount))
		for i := hr.start; i <= hr.end; i++ {
			b.WriteString(fmt.Sprintf("%c%s\n", edits[i].op, edits[i].line))
		}
	}

	return b.String()
}

// countAddsDels counts added and deleted lines between old and new content.
func countAddsDels(old, new string) (int, int) {
	oldLines := splitLinesStaging(old)
	newLines := splitLinesStaging(new)

	lcs := computeLCSStaging(oldLines, newLines)
	dels := len(oldLines) - len(lcs)
	adds := len(newLines) - len(lcs)
	return adds, dels
}

// detectChangeType determines if a change is a Modification (M), Addition (A), or Deletion (D).
func detectChangeType(original, modified string) string {
	if original == "" {
		return "A"
	}
	if modified == "" {
		return "D"
	}
	return "M"
}

// formatStats formats add/del counts for display.
func formatStats(adds, dels int) string {
	parts := []string{}
	if adds > 0 {
		parts = append(parts, fmt.Sprintf("+%d", adds))
	}
	if dels > 0 {
		parts = append(parts, fmt.Sprintf("-%d", dels))
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, ", ")
}

// hunkDescription generates a brief description for a hunk.
func hunkDescription(hunk StagedHunk, index int) string {
	adds := len(hunk.NewLines)
	dels := len(hunk.OldLines)

	if adds > 0 && dels > 0 {
		return fmt.Sprintf("modify lines %d-%d", hunk.StartLine, hunk.EndLine)
	}
	if adds > 0 {
		return fmt.Sprintf("add %d lines at %d", adds, hunk.StartLine)
	}
	if dels > 0 {
		return fmt.Sprintf("remove %d lines at %d-%d", dels, hunk.StartLine, hunk.EndLine)
	}
	return "no-op"
}
