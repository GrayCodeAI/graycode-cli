package engine

import (
	"fmt"
	"strings"
)

// Diff3Result holds the outcome of a three-way merge.
type Diff3Result struct {
	Merged       string
	Conflicts    []Diff3Conflict
	HasConflicts bool
	Stats        Diff3Stats
}

// Diff3Conflict represents a single conflict region in the merge.
type Diff3Conflict struct {
	BaseLines   []string
	OursLines   []string
	TheirsLines []string
	StartLine   int
}

// Diff3Stats provides statistics about the merge operation.
type Diff3Stats struct {
	TotalLines    int
	ConflictCount int
	AutoMerged    int
	OursOnly      int
	TheirsOnly    int
}

// Diff3Region represents a classified region in the three-way diff.
type Diff3Region struct {
	Type        string // "unchanged", "ours", "theirs", "conflict"
	BaseStart   int
	BaseEnd     int
	OursLines   []string
	TheirsLines []string
}

// Edit represents a single edit operation in an edit script.
type Edit struct {
	Type  string // "keep", "insert", "delete"
	Line  string
	Index int
}

// Merge3 performs a three-way merge using the diff3 algorithm.
// It splits base, ours, and theirs into lines, computes diff regions,
// and merges non-conflicting changes automatically while marking conflicts.
func Merge3(base, ours, theirs string) *Diff3Result {
	baseLines := diff3Split(base)
	oursLines := diff3Split(ours)
	theirsLines := diff3Split(theirs)

	regions := diff3Regions(baseLines, oursLines, theirsLines)

	result := &Diff3Result{
		Stats: Diff3Stats{},
	}

	var merged []string
	currentLine := 1

	for _, region := range regions {
		switch region.Type {
		case "unchanged":
			// Both sides agree with base
			lines := baseLines[region.BaseStart:region.BaseEnd]
			merged = append(merged, lines...)
			result.Stats.AutoMerged += len(lines)
			currentLine += len(lines)

		case "ours":
			// Only ours changed
			merged = append(merged, region.OursLines...)
			result.Stats.OursOnly += len(region.OursLines)
			result.Stats.AutoMerged += len(region.OursLines)
			currentLine += len(region.OursLines)

		case "theirs":
			// Only theirs changed
			merged = append(merged, region.TheirsLines...)
			result.Stats.TheirsOnly += len(region.TheirsLines)
			result.Stats.AutoMerged += len(region.TheirsLines)
			currentLine += len(region.TheirsLines)

		case "conflict":
			// Both sides changed the same region differently
			conflict := Diff3Conflict{
				BaseLines:   baseLines[region.BaseStart:region.BaseEnd],
				OursLines:   region.OursLines,
				TheirsLines: region.TheirsLines,
				StartLine:   currentLine,
			}
			result.Conflicts = append(result.Conflicts, conflict)
			result.HasConflicts = true
			result.Stats.ConflictCount++

			// Add conflict markers to merged output
			markers := formatConflictLines(conflict)
			merged = append(merged, markers...)
			currentLine += len(markers)
		}
	}

	result.Merged = diff3Join(merged)
	result.Stats.TotalLines = len(merged)
	return result
}

// diff3Regions computes the classified regions for a three-way merge.
// It finds the edit scripts from base to ours and base to theirs,
// then walks through them to identify unchanged, ours-only, theirs-only,
// and conflicting regions.
func diff3Regions(base, ours, theirs []string) []Diff3Region {
	oursEdits := EditScript(base, ours)
	theirsEdits := EditScript(base, theirs)

	// Build maps of which base lines are changed by each side
	oursChanges := classifyEdits(oursEdits, len(base))
	theirsChanges := classifyEdits(theirsEdits, len(base))

	var regions []Diff3Region
	i := 0

	for i < len(base) {
		oursChanged := oursChanges[i].changed
		theirsChanged := theirsChanges[i].changed

		if !oursChanged && !theirsChanged {
			// Find extent of unchanged region
			start := i
			for i < len(base) && !oursChanges[i].changed && !theirsChanges[i].changed {
				i++
			}
			regions = append(regions, Diff3Region{
				Type:      "unchanged",
				BaseStart: start,
				BaseEnd:   i,
			})
		} else if oursChanged && !theirsChanged {
			// Only ours changed this region
			start := i
			for i < len(base) && oursChanges[i].changed && !theirsChanges[i].changed {
				i++
			}
			oursResult := applyRegionEdits(base, start, i, oursEdits)
			regions = append(regions, Diff3Region{
				Type:      "ours",
				BaseStart: start,
				BaseEnd:   i,
				OursLines: oursResult,
			})
		} else if !oursChanged && theirsChanged {
			// Only theirs changed this region
			start := i
			for i < len(base) && !oursChanges[i].changed && theirsChanges[i].changed {
				i++
			}
			theirsResult := applyRegionEdits(base, start, i, theirsEdits)
			regions = append(regions, Diff3Region{
				Type:        "theirs",
				BaseStart:   start,
				BaseEnd:     i,
				TheirsLines: theirsResult,
			})
		} else {
			// Both changed - potential conflict
			start := i
			for i < len(base) && oursChanges[i].changed && theirsChanges[i].changed {
				i++
			}
			oursResult := applyRegionEdits(base, start, i, oursEdits)
			theirsResult := applyRegionEdits(base, start, i, theirsEdits)

			// If both made identical changes, it's not a conflict
			if linesEqual(oursResult, theirsResult) {
				regions = append(regions, Diff3Region{
					Type:      "ours",
					BaseStart: start,
					BaseEnd:   i,
					OursLines: oursResult,
				})
			} else {
				regions = append(regions, Diff3Region{
					Type:        "conflict",
					BaseStart:   start,
					BaseEnd:     i,
					OursLines:   oursResult,
					TheirsLines: theirsResult,
				})
			}
		}
	}

	// Handle insertions at end of file
	oursTrailing := getTrailingInserts(oursEdits, len(base))
	theirsTrailing := getTrailingInserts(theirsEdits, len(base))

	if len(oursTrailing) > 0 && len(theirsTrailing) > 0 {
		if linesEqual(oursTrailing, theirsTrailing) {
			regions = append(regions, Diff3Region{
				Type:      "ours",
				BaseStart: len(base),
				BaseEnd:   len(base),
				OursLines: oursTrailing,
			})
		} else {
			regions = append(regions, Diff3Region{
				Type:        "conflict",
				BaseStart:   len(base),
				BaseEnd:     len(base),
				OursLines:   oursTrailing,
				TheirsLines: theirsTrailing,
			})
		}
	} else if len(oursTrailing) > 0 {
		regions = append(regions, Diff3Region{
			Type:      "ours",
			BaseStart: len(base),
			BaseEnd:   len(base),
			OursLines: oursTrailing,
		})
	} else if len(theirsTrailing) > 0 {
		regions = append(regions, Diff3Region{
			Type:        "theirs",
			BaseStart:   len(base),
			BaseEnd:     len(base),
			TheirsLines: theirsTrailing,
		})
	}

	return regions
}

// FormatConflictMarkers formats a conflict with standard diff3 markers.
func FormatConflictMarkers(conflict Diff3Conflict) string {
	var b strings.Builder
	b.WriteString("<<<<<<< ours\n")
	for _, line := range conflict.OursLines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("||||||| base\n")
	for _, line := range conflict.BaseLines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("=======\n")
	for _, line := range conflict.TheirsLines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(">>>>>>> theirs")
	return b.String()
}

// LCS computes the Longest Common Subsequence of two string slices.
func LCS(a, b []string) []string {
	m := len(a)
	n := len(b)

	if m == 0 || n == 0 {
		return nil
	}

	// Build DP table
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

	// Backtrack to find LCS
	length := dp[m][n]
	result := make([]string, length)
	i, j := m, n
	idx := length - 1
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			result[idx] = a[i-1]
			idx--
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	return result
}

// EditScript computes a minimal edit script to transform 'from' into 'to'.
func EditScript(from, to []string) []Edit {
	lcs := LCS(from, to)

	var edits []Edit
	fi, ti, li := 0, 0, 0

	for li < len(lcs) {
		// Delete lines from 'from' that are not in LCS
		for fi < len(from) && from[fi] != lcs[li] {
			edits = append(edits, Edit{Type: "delete", Line: from[fi], Index: fi})
			fi++
		}
		// Insert lines from 'to' that are not in LCS
		for ti < len(to) && to[ti] != lcs[li] {
			edits = append(edits, Edit{Type: "insert", Line: to[ti], Index: fi})
			ti++
		}
		// Keep the matching line
		edits = append(edits, Edit{Type: "keep", Line: lcs[li], Index: fi})
		fi++
		ti++
		li++
	}

	// Handle remaining lines after LCS is exhausted
	for fi < len(from) {
		edits = append(edits, Edit{Type: "delete", Line: from[fi], Index: fi})
		fi++
	}
	for ti < len(to) {
		edits = append(edits, Edit{Type: "insert", Line: to[ti], Index: fi})
		ti++
	}

	return edits
}

// MergeClean performs a three-way merge and returns the merged string
// and whether the merge was clean (no conflicts).
func MergeClean(base, ours, theirs string) (string, bool) {
	result := Merge3(base, ours, theirs)
	return result.Merged, !result.HasConflicts
}

// FormatDiff3Result produces a human-readable summary of a Diff3Result.
func FormatDiff3Result(result *Diff3Result) string {
	var b strings.Builder
	b.WriteString("Three-way merge:\n")
	b.WriteString(fmt.Sprintf("  Auto-merged: %d lines\n", result.Stats.AutoMerged))

	if result.Stats.ConflictCount > 0 {
		b.WriteString(fmt.Sprintf("  Conflicts: %d (", result.Stats.ConflictCount))
		for i, c := range result.Conflicts {
			if i > 0 {
				b.WriteString(", ")
			}
			endLine := c.StartLine + len(c.OursLines) + len(c.TheirsLines) + len(c.BaseLines) + 3
			b.WriteString(fmt.Sprintf("lines %d-%d", c.StartLine, endLine))
		}
		b.WriteString(")\n")
	} else {
		b.WriteString("  Conflicts: 0\n")
	}

	b.WriteString(fmt.Sprintf("  Ours-only changes: %d lines\n", result.Stats.OursOnly))
	b.WriteString(fmt.Sprintf("  Theirs-only changes: %d lines\n", result.Stats.TheirsOnly))
	return b.String()
}

// --- Internal helpers ---

// baseChangeInfo tracks whether a base line is part of a change.
type baseChangeInfo struct {
	changed bool
}

// classifyEdits determines which base lines are affected by the given edit script.
func classifyEdits(edits []Edit, baseLen int) []baseChangeInfo {
	info := make([]baseChangeInfo, baseLen)

	for _, e := range edits {
		switch e.Type {
		case "delete":
			if e.Index >= 0 && e.Index < baseLen {
				info[e.Index] = baseChangeInfo{changed: true}
			}
		case "insert":
			// An insert before a base line marks the preceding base line as changed
			// if it exists, to ensure the region is captured
			idx := e.Index
			if idx > 0 && idx <= baseLen {
				info[idx-1] = baseChangeInfo{changed: true}
			} else if idx == 0 && baseLen > 0 {
				info[0] = baseChangeInfo{changed: true}
			}
		}
	}

	return info
}

// applyRegionEdits applies the edit script to a specific region of base lines,
// returning the resulting lines for that region.
func applyRegionEdits(base []string, start, end int, edits []Edit) []string {
	var result []string

	for _, e := range edits {
		switch e.Type {
		case "insert":
			if e.Index >= start && e.Index <= end {
				result = append(result, e.Line)
			}
		case "keep":
			if e.Index >= start && e.Index < end {
				result = append(result, e.Line)
			}
		case "delete":
			// deleted lines are simply not included
		}
	}

	return result
}

// getTrailingInserts returns any insertions at the end of the file.
func getTrailingInserts(edits []Edit, baseLen int) []string {
	var result []string
	for _, e := range edits {
		if e.Type == "insert" && e.Index >= baseLen {
			result = append(result, e.Line)
		}
	}
	return result
}

// formatConflictLines produces the conflict marker lines as a slice.
func formatConflictLines(conflict Diff3Conflict) []string {
	var lines []string
	lines = append(lines, "<<<<<<< ours")
	lines = append(lines, conflict.OursLines...)
	lines = append(lines, "||||||| base")
	lines = append(lines, conflict.BaseLines...)
	lines = append(lines, "=======")
	lines = append(lines, conflict.TheirsLines...)
	lines = append(lines, ">>>>>>> theirs")
	return lines
}

// diff3Split splits a string into lines. An empty string yields an empty slice.
func diff3Split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// diff3Join joins lines with newline separators.
func diff3Join(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// linesEqual checks if two string slices are identical.
func linesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
