package engine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DiffPreview holds all pending changes for review before they are committed/applied.
type DiffPreview struct {
	Changes   []FileChange
	CreatedAt time.Time
	SessionID string
	mu        sync.RWMutex
}

// FileChange represents a single file modification in the workspace.
type FileChange struct {
	Path       string
	Type       string // "modified", "created", "deleted", "renamed"
	OldContent string
	NewContent string
	Hunks      []DiffHunk
	Stats      ChangeStats
	Approved   bool
	Rejected   bool
	Comment    string
}

// DiffHunk represents a contiguous block of changes with surrounding context.
type DiffHunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []DiffLine
}

// DiffLine represents a single line in a diff hunk.
type DiffLine struct {
	Type      string // "add", "remove", "context"
	Content   string
	OldLineNo int
	NewLineNo int
}

// ChangeStats summarizes additions and deletions for a file change.
type ChangeStats struct {
	Additions int
	Deletions int
	NetChange int
}

// NewDiffPreview creates a new DiffPreview with a generated session ID.
func NewDiffPreview() *DiffPreview {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	sessionID := hex.EncodeToString(b)
	return &DiffPreview{
		Changes:   make([]FileChange, 0),
		CreatedAt: time.Now(),
		SessionID: sessionID,
	}
}

// RecordChange computes a unified diff between old and new content and records it.
func (dp *DiffPreview) RecordChange(path, oldContent, newContent string) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	hunks := ComputeDiff(oldContent, newContent)
	stats := computeStats(hunks)

	changeType := "modified"
	if oldContent == "" && newContent != "" {
		changeType = "created"
	} else if oldContent != "" && newContent == "" {
		changeType = "deleted"
	}

	change := FileChange{
		Path:       path,
		Type:       changeType,
		OldContent: oldContent,
		NewContent: newContent,
		Hunks:      hunks,
		Stats:      stats,
	}

	dp.Changes = append(dp.Changes, change)
}

// ComputeDiff computes hunks between old and new content using Myers diff with 3 lines of context.
func ComputeDiff(oldContent, newContent string) []DiffHunk {
	oldLines := diffSplitLines(oldContent)
	newLines := diffSplitLines(newContent)

	diffLines := ComputeMyersDiff(oldLines, newLines)

	return groupIntoHunks(diffLines, 3)
}

// ComputeMyersDiff implements Myers' O(ND) diff algorithm returning an edit script.
// It finds a minimal edit distance between sequences a and b, then returns the
// result as a series of DiffLine entries (context, add, remove).
func ComputeMyersDiff(a, b []string) []DiffLine {
	n := len(a)
	m := len(b)

	if n == 0 && m == 0 {
		return nil
	}

	// Handle trivial cases
	if n == 0 {
		result := make([]DiffLine, m)
		for i, line := range b {
			result[i] = DiffLine{Type: "add", Content: line, OldLineNo: 0, NewLineNo: i + 1}
		}
		return result
	}
	if m == 0 {
		result := make([]DiffLine, n)
		for i, line := range a {
			result[i] = DiffLine{Type: "remove", Content: line, OldLineNo: i + 1, NewLineNo: 0}
		}
		return result
	}

	// Myers algorithm - forward pass
	// max possible edit distance
	max := n + m
	offset := max + 1
	vSize := 2*max + 3

	// Store the V array at each step for backtracking
	type trace struct {
		v []int
	}
	traces := make([]trace, 0)

	v := make([]int, vSize)
	v[1+offset] = 0

	var finalD int
	done := false
	for d := 0; d <= max && !done; d++ {
		// Save V before modifications for this d
		vCopy := make([]int, vSize)
		copy(vCopy, v)
		traces = append(traces, trace{v: vCopy})

		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[k-1+offset] < v[k+1+offset]) {
				x = v[k+1+offset] // move down (insert)
			} else {
				x = v[k-1+offset] + 1 // move right (delete)
			}
			y := x - k

			// Follow diagonal (matches)
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}

			v[k+offset] = x

			if x >= n && y >= m {
				finalD = d
				done = true
				break
			}
		}
	}

	// Backtrack through traces to find the edit path
	// traces[d] stores v BEFORE step d ran, which is the same as v AFTER step d-1 ran.
	// So to find where we were after step d-1, we look at traces[d].
	type snake struct {
		startX, startY int
		midX, midY     int // after the non-diagonal move
		endX, endY     int // after following diagonal
	}
	snakes := make([]snake, finalD)

	x := n
	y := m
	for d := finalD; d > 0; d-- {
		k := x - y
		// traces[d] = v state after step d-1, i.e., the state from which step d starts
		prevV := traces[d].v

		var prevK int
		if k == -d || (k != d && prevV[k-1+offset] < prevV[k+1+offset]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}

		prevX := prevV[prevK+offset]
		prevY := prevX - prevK

		// The snake starts at (prevX, prevY), then we make one non-diagonal move,
		// then follow diagonals to (x, y)
		var midX, midY int
		if k == prevK+1 {
			// Deletion (move right): prevX -> prevX+1 on same y
			midX = prevX + 1
			midY = prevY
		} else {
			// Insertion (move down): same x, prevY -> prevY+1
			midX = prevX
			midY = prevY + 1
		}

		snakes[d-1] = snake{
			startX: prevX, startY: prevY,
			midX: midX, midY: midY,
			endX: x, endY: y,
		}

		x = prevX
		y = prevY
	}

	// Build the result from the snakes
	result := make([]DiffLine, 0, n+m)
	curX := 0
	curY := 0

	// If d == 0, everything is context (identical)
	if finalD == 0 {
		for i := 0; i < n; i++ {
			result = append(result, DiffLine{
				Type:      "context",
				Content:   a[i],
				OldLineNo: i + 1,
				NewLineNo: i + 1,
			})
		}
		return result
	}

	for _, s := range snakes {
		// Emit context lines from current position to start of this snake
		for curX < s.startX && curY < s.startY {
			result = append(result, DiffLine{
				Type:      "context",
				Content:   a[curX],
				OldLineNo: curX + 1,
				NewLineNo: curY + 1,
			})
			curX++
			curY++
		}

		// Emit the non-diagonal move
		if s.midX == s.startX+1 && s.midY == s.startY {
			// Deletion
			result = append(result, DiffLine{
				Type:      "remove",
				Content:   a[s.startX],
				OldLineNo: s.startX + 1,
				NewLineNo: 0,
			})
			curX = s.midX
			curY = s.midY
		} else {
			// Insertion
			result = append(result, DiffLine{
				Type:      "add",
				Content:   b[s.startY],
				OldLineNo: 0,
				NewLineNo: s.startY + 1,
			})
			curX = s.midX
			curY = s.midY
		}

		// Emit diagonal context lines after the edit to reach the end of the snake
		for curX < s.endX && curY < s.endY {
			result = append(result, DiffLine{
				Type:      "context",
				Content:   a[curX],
				OldLineNo: curX + 1,
				NewLineNo: curY + 1,
			})
			curX++
			curY++
		}
	}

	// Emit any remaining context
	for curX < n && curY < m {
		result = append(result, DiffLine{
			Type:      "context",
			Content:   a[curX],
			OldLineNo: curX + 1,
			NewLineNo: curY + 1,
		})
		curX++
		curY++
	}

	return result
}

// RenderUnified renders a single FileChange in standard unified diff format.
func RenderUnified(change *FileChange) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- a/%s\n", change.Path))
	sb.WriteString(fmt.Sprintf("+++ b/%s\n", change.Path))

	for _, hunk := range change.Hunks {
		sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount))
		for _, line := range hunk.Lines {
			switch line.Type {
			case "add":
				sb.WriteString(fmt.Sprintf("+%s\n", line.Content))
			case "remove":
				sb.WriteString(fmt.Sprintf("-%s\n", line.Content))
			case "context":
				sb.WriteString(fmt.Sprintf(" %s\n", line.Content))
			}
		}
	}

	return sb.String()
}

// RenderAll renders all changes as a combined unified diff.
func (dp *DiffPreview) RenderAll() string {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	var sb strings.Builder
	for i := range dp.Changes {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(RenderUnified(&dp.Changes[i]))
	}
	return sb.String()
}

// RenderSummary produces a compact summary of all pending changes.
func (dp *DiffPreview) RenderSummary() string {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("Pending Changes:\n")

	totalFiles := 0
	totalAdd := 0
	totalDel := 0

	for _, change := range dp.Changes {
		totalFiles++
		totalAdd += change.Stats.Additions
		totalDel += change.Stats.Deletions

		prefix := "M"
		switch change.Type {
		case "created":
			prefix = "A"
		case "deleted":
			prefix = "D"
		case "renamed":
			prefix = "R"
		}

		statsStr := ""
		if change.Stats.Additions > 0 && change.Stats.Deletions > 0 {
			statsStr = fmt.Sprintf("(+%d, -%d)", change.Stats.Additions, change.Stats.Deletions)
		} else if change.Stats.Additions > 0 {
			statsStr = fmt.Sprintf("(+%d)", change.Stats.Additions)
		} else if change.Stats.Deletions > 0 {
			statsStr = fmt.Sprintf("(-%d)", change.Stats.Deletions)
		}

		sb.WriteString(fmt.Sprintf("%s %s %s\n", prefix, change.Path, statsStr))
	}

	sb.WriteString(fmt.Sprintf("\nTotal: %d files, +%d, -%d\n", totalFiles, totalAdd, totalDel))
	return sb.String()
}

// Approve marks a specific file change as approved.
func (dp *DiffPreview) Approve(path string) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	for i := range dp.Changes {
		if dp.Changes[i].Path == path {
			dp.Changes[i].Approved = true
			dp.Changes[i].Rejected = false
			break
		}
	}
}

// Reject marks a specific file change as rejected with an optional comment.
func (dp *DiffPreview) Reject(path string, comment string) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	for i := range dp.Changes {
		if dp.Changes[i].Path == path {
			dp.Changes[i].Rejected = true
			dp.Changes[i].Approved = false
			dp.Changes[i].Comment = comment
			break
		}
	}
}

// ApproveAll marks all changes as approved.
func (dp *DiffPreview) ApproveAll() {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	for i := range dp.Changes {
		dp.Changes[i].Approved = true
		dp.Changes[i].Rejected = false
	}
}

// RejectAll marks all changes as rejected with an optional comment.
func (dp *DiffPreview) RejectAll(comment string) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	for i := range dp.Changes {
		dp.Changes[i].Rejected = true
		dp.Changes[i].Approved = false
		dp.Changes[i].Comment = comment
	}
}

// GetPending returns changes that are neither approved nor rejected.
func (dp *DiffPreview) GetPending() []FileChange {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	var pending []FileChange
	for _, change := range dp.Changes {
		if !change.Approved && !change.Rejected {
			pending = append(pending, change)
		}
	}
	return pending
}

// GetApproved returns changes that have been approved.
func (dp *DiffPreview) GetApproved() []FileChange {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	var approved []FileChange
	for _, change := range dp.Changes {
		if change.Approved {
			approved = append(approved, change)
		}
	}
	return approved
}

// Clear removes all recorded changes.
func (dp *DiffPreview) Clear() {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	dp.Changes = make([]FileChange, 0)
}

// diffSplitLines splits content into lines. Returns empty slice for empty string.
func diffSplitLines(content string) []string {
	if content == "" {
		return []string{}
	}
	lines := strings.Split(content, "\n")
	// Remove trailing empty string from trailing newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// computeStats calculates addition/deletion statistics from hunks.
func computeStats(hunks []DiffHunk) ChangeStats {
	var stats ChangeStats
	for _, hunk := range hunks {
		for _, line := range hunk.Lines {
			switch line.Type {
			case "add":
				stats.Additions++
			case "remove":
				stats.Deletions++
			}
		}
	}
	stats.NetChange = stats.Additions - stats.Deletions
	return stats
}

// groupIntoHunks takes a flat list of DiffLines and groups them into hunks
// with the specified number of context lines around changes.
func groupIntoHunks(lines []DiffLine, contextSize int) []DiffHunk {
	if len(lines) == 0 {
		return nil
	}

	// Find indices of change lines
	changeIndices := make([]int, 0)
	for i, line := range lines {
		if line.Type != "context" {
			changeIndices = append(changeIndices, i)
		}
	}

	if len(changeIndices) == 0 {
		return nil
	}

	// Group changes into ranges with context
	type rangeT struct {
		start, end int
	}
	var ranges []rangeT

	for _, idx := range changeIndices {
		start := idx - contextSize
		if start < 0 {
			start = 0
		}
		end := idx + contextSize + 1
		if end > len(lines) {
			end = len(lines)
		}

		if len(ranges) > 0 && start <= ranges[len(ranges)-1].end {
			// Merge with previous range
			ranges[len(ranges)-1].end = end
		} else {
			ranges = append(ranges, rangeT{start, end})
		}
	}

	// Build hunks from ranges
	hunks := make([]DiffHunk, 0, len(ranges))
	for _, r := range ranges {
		hunkLines := lines[r.start:r.end]

		// Calculate old/new start and count
		oldStart := 0
		oldCount := 0
		newStart := 0
		newCount := 0

		for i, line := range hunkLines {
			switch line.Type {
			case "context":
				if i == 0 {
					oldStart = line.OldLineNo
					newStart = line.NewLineNo
				}
				oldCount++
				newCount++
			case "remove":
				if i == 0 || (oldStart == 0 && line.OldLineNo > 0) {
					if oldStart == 0 {
						oldStart = line.OldLineNo
					}
				}
				oldCount++
			case "add":
				if i == 0 || (newStart == 0 && line.NewLineNo > 0) {
					if newStart == 0 {
						newStart = line.NewLineNo
					}
				}
				newCount++
			}
		}

		// Ensure start is set correctly
		if oldStart == 0 {
			oldStart = 1
		}
		if newStart == 0 {
			newStart = 1
		}

		hunks = append(hunks, DiffHunk{
			OldStart: oldStart,
			OldCount: oldCount,
			NewStart: newStart,
			NewCount: newCount,
			Lines:    hunkLines,
		})
	}

	return hunks
}
