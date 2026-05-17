package diffsandbox

import (
	"fmt"
	"strings"
)

// unifiedDiff generates a unified diff between old and new content.
// It produces standard unified diff format with @@ hunk headers.
func unifiedDiff(oldPath, newPath, oldContent, newContent string) string {
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- %s\n", oldPath))
	b.WriteString(fmt.Sprintf("+++ %s\n", newPath))

	// Compute LCS-based edit script
	lcs := computeLCS(oldLines, newLines)

	var edits []edit
	oi, ni, li := 0, 0, 0
	for li < len(lcs) {
		for oi < len(oldLines) && oldLines[oi] != lcs[li] {
			edits = append(edits, edit{op: '-', line: oldLines[oi]})
			oi++
		}
		for ni < len(newLines) && newLines[ni] != lcs[li] {
			edits = append(edits, edit{op: '+', line: newLines[ni]})
			ni++
		}
		edits = append(edits, edit{op: ' ', line: lcs[li]})
		oi++
		ni++
		li++
	}
	for oi < len(oldLines) {
		edits = append(edits, edit{op: '-', line: oldLines[oi]})
		oi++
	}
	for ni < len(newLines) {
		edits = append(edits, edit{op: '+', line: newLines[ni]})
		ni++
	}

	// Group edits into hunks with 3 lines of context
	const contextLines = 3
	hunks := groupHunks(edits, contextLines)
	for _, h := range hunks {
		oldStart, oldCount, newStart, newCount := hunkHeader(h, edits)
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

// splitLines splits content into lines. An empty string returns nil.
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
		return nil
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
func hunkHeader(hunk []edit, allEdits []edit) (int, int, int, int) {
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
