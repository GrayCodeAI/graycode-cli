// Package hunktracker tracks line-level edits ("hunks") per file with stable
// hunk identity across re-computation and author attribution that survives
// external edits, adopted from grok-build's xai-hunk-tracker.
//
// The core problem: naive re-diffing after every edit assigns fresh positions
// and IDs, so "the change the agent made" cannot be followed across subsequent
// modifications. Here each hunk carries an ID derived from its content, and
// when file content changes later, previously tracked hunks are matched by
// content-and-overlap so their identity — and their author attribution — is
// preserved even when the newest edit came from someone else.
package hunktracker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Author classifies who produced a hunk.
type Author string

const (
	// AuthorAgent marks hunks written by the coding agent.
	AuthorAgent Author = "agent"
	// AuthorExternal marks hunks written by anything else (user, other tools).
	AuthorExternal Author = "external"
)

// Hunk is one contiguous changed region.
type Hunk struct {
	// ID is stable across re-computation while content overlaps.
	ID string `json:"id"`
	// StartLine is 1-based in the *current* content.
	StartLine int    `json:"start_line"`
	Lines     int    `json:"lines"`
	Text      string `json:"text"`
	// Author records who last wrote this region.
	Author Author `json:"author"`
}

// FileState is the tracked state of one file.
type FileState struct {
	Path     string `json:"path"`
	Baseline string `json:"baseline"`
	Hunks    []Hunk `json:"hunks"`
}

// Tracker holds per-file hunk states.
type Tracker struct {
	files map[string]*FileState
}

// NewTracker returns an empty tracker.
func NewTracker() *Tracker { return &Tracker{files: map[string]*FileState{}} }

// Track registers a file whose future changes should be attributed. The
// current content becomes the baseline.
func (t *Tracker) Track(path, currentContent string) {
	t.files[path] = &FileState{Path: path, Baseline: currentContent}
}

// Forget removes a file from tracking.
func (t *Tracker) Forget(path string) { delete(t.files, path) }

// Tracked reports whether path is being tracked.
func (t *Tracker) Tracked(path string) bool { _, ok := t.files[path]; return ok }

// Update records an edit of a tracked file and returns its hunks after
// identity reconciliation. Untracked paths are ignored (call Track first).
func (t *Tracker) Update(path, newContent string, author Author) ([]Hunk, bool) {
	st, ok := t.files[path]
	if !ok {
		return nil, false
	}
	newHunks := computeHunks(st.Baseline, newContent)
	st.Hunks = reconcile(st.Hunks, newHunks, author)
	st.Baseline = newContent
	return st.Hunks, true
}

// Hunks returns a copy of the tracked hunks for a file.
func (t *Tracker) Hunks(path string) []Hunk {
	st, ok := t.files[path]
	if !ok {
		return nil
	}
	out := make([]Hunk, len(st.Hunks))
	copy(out, st.Hunks)
	return out
}

// AgentTouchedFiles lists tracked files with at least one agent-authored hunk.
func (t *Tracker) AgentTouchedFiles() []string {
	var out []string
	for p, st := range t.files {
		for _, h := range st.Hunks {
			if h.Author == AuthorAgent {
				out = append(out, p)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// computeHunks diffs baseline vs current line-wise, returning changed regions
// positioned in current. It uses an LCS over lines.
func computeHunks(baseline, current string) []Hunk {
	a := splitLines(baseline)
	b := splitLines(current)
	ops := lcsOps(a, b) // sequence of ops over b indices: keep/delete(insert)

	var hunks []Hunk
	i := 0
	for i < len(ops) {
		if ops[i] {
			// Kept line (part of the LCS with baseline): not a change.
			i++
			continue
		}
		start := i
		for i < len(ops) && !ops[i] {
			i++
		}
		h := Hunk{
			ID:        hunkID(b[start:i]),
			StartLine: start + 1,
			Lines:     i - start,
			Text:      strings.Join(b[start:i], "\n"),
		}
		hunks = append(hunks, h)
	}
	return hunks
}

// reconcile merges newly computed hunks with previously known ones:
//   - a new hunk matching an old hunk's ID keeps the old identity and author;
//   - a new hunk overlapping an old one keeps the old author (an external edit
//     touching an agent-authored region does not strip attribution);
//   - brand-new hunks take the incoming author.
func reconcile(old, next []Hunk, author Author) []Hunk {
	oldByID := map[string]Hunk{}
	for _, h := range old {
		oldByID[h.ID] = h
	}
	out := make([]Hunk, 0, len(next))
	for _, h := range next {
		if prev, ok := oldByID[h.ID]; ok {
			h.Author = prev.Author
			out = append(out, h)
			continue
		}
		attributed := Author("")
		for _, prev := range old {
			if hunksOverlap(prev, h) {
				attributed = prev.Author
				break
			}
		}
		if attributed == "" {
			attributed = author
		}
		// Note: an external edit overlapping a previously agent-authored
		// region intentionally keeps the prior attribution.
		h.Author = attributed
		out = append(out, h)
	}
	return out
}

func hunksOverlap(a, b Hunk) bool {
	aStart, aEnd := a.StartLine, a.StartLine+a.Lines
	bStart, bEnd := b.StartLine, b.StartLine+b.Lines
	return aStart < bEnd && bStart < aEnd
}

func hunkID(lines []string) string {
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return fmt.Sprintf("h%s", hex.EncodeToString(sum[:6]))
}

func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// lcsOps returns, for each index of b, whether that line is part of the LCS
// with a (true = kept/present in both, false = inserted relative to a).
func lcsOps(a, b []string) []bool {
	n, m := len(a), len(b)
	// Guard against pathological sizes; fall back to "all inserted".
	if n*m > 4_000_000 {
		all := make([]bool, m)
		for i := range all {
			all[i] = true
		}
		return all
	}
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	ops := make([]bool, m) // true = line kept from baseline (LCS member)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops[j] = true
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	return ops
}
