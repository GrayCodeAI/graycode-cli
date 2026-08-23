// Package hashline implements anchor-based file editing adopted from
// grok-build's hashline system: every line carries a content-derived hash,
// reads emit anchored output the agent can reference, and edit batches
// validate ALL anchors against current content before applying anything —
// eliminating two real failure modes of positional editing:
//
//   - line-number drift between Read and Edit (another tool inserted/deleted
//     lines in between);
//   - ambiguity when search/replace old_string appears more than once.
//
// Invariants: anchors are validated against CURRENT file content; a drifted
// anchor is recovered by searching within a bounded window; if ANY edit fails
// validation the entire batch is rejected without writing.
package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// HashLen is the anchor length in hex characters (collision-safe for
// single-file editing at 32 bits).
const HashLen = 8

// AnchorWindow is how far ± from the stated position a drifted anchor may be
// recovered before the edit is rejected.
const AnchorWindow = 5

// AnchoredLine is one line's anchor + content.
type AnchoredLine struct {
	Line    int    `json:"line"` // 1-based
	Hash    string `json:"hash"` // first HashLen hex chars of SHA-256(content)
	Content string `json:"content"`
}

// EditOp is one operation in an edit batch.
type EditOp struct {
	Line int    `json:"line"`           // 1-based target line
	Hash string `json:"hash"`           // expected content hash (from read)
	Op   string `json:"op"`             // replace | insert_after | delete
	Text string `json:"text,omitempty"` // replacement / insertion text
}

// Validate checks basic op sanity without touching disk.
func (e EditOp) Validate() error {
	switch e.Op {
	case "replace", "insert_after", "delete":
	default:
		return fmt.Errorf("hashline: unknown op %q", e.Op)
	}
	if e.Line < 1 {
		return fmt.Errorf("hashline: line must be >= 1")
	}
	if e.Hash == "" {
		return fmt.Errorf("hashline: hash is required")
	}
	if e.Op != "delete" && e.Text == "" {
		return fmt.Errorf("hashline: %s requires text", e.Op)
	}
	return nil
}

// Anchor produces the hash for one line's content.
func Anchor(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimRight(content, "\r\n")))
	return hex.EncodeToString(sum[:])[:HashLen]
}

// ReadAnchored loads a file and returns every line with its anchor.
func ReadAnchored(path string) ([]AnchoredLine, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- caller-supplied path validated upstream
	if err != nil {
		return nil, fmt.Errorf("hashline: read: %w", err)
	}
	lines := splitKeepEnds(string(data))
	out := make([]AnchoredLine, len(lines))
	for i, l := range lines {
		out[i] = AnchoredLine{Line: i + 1, Hash: Anchor(l), Content: l}
	}
	return out, nil
}

// RenderAnchored formats lines for model consumption.
func RenderAnchored(lines []AnchoredLine) string {
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "L%d:%s|%s\n", l.Line, l.Hash, l.Content)
	}
	return b.String()
}

// ApplyEdits validates every edit against current file content (with bounded
// shifted-anchor recovery) and applies them atomically: either all succeed or
// nothing is written. Edits are applied bottom-up so earlier line numbers are
// not shifted by later insertions/deletions above them.
func ApplyEdits(path string, edits []EditOp) error {
	if len(edits) == 0 {
		return fmt.Errorf("hashline: no edits")
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- caller-supplied path validated upstream
	if err != nil {
		return fmt.Errorf("hashline: read: %w", err)
	}
	lines := splitKeepEnds(string(raw))

	// Phase 1: resolve every edit to a concrete line index, validating hashes.
	resolved := make([]resolvedEdit, len(edits))
	for i, e := range edits {
		if err := e.Validate(); err != nil {
			return err
		}
		idx, rerr := resolveAnchor(lines, e)
		if rerr != nil {
			return rerr
		}
		resolved[i] = resolvedEdit{edit: e, idx: idx}
	}

	// Phase 2: apply bottom-up (sort descending by index).
	sortEditsDescending(resolved)
	for _, r := range resolved {
		switch r.edit.Op {
		case "replace":
			lines[r.idx] = r.edit.Text
		case "insert_after":
			lines = insertAfter(lines, r.idx, r.edit.Text)
		case "delete":
			lines = append(lines[:r.idx], lines[r.idx+1:]...)
		}
	}

	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(string(raw), "\n") && strings.HasSuffix(out, "\n") {
		out = strings.TrimSuffix(out, "\n") // preserve original trailing-newline state
	}
	info, _ := os.Stat(path)
	mode := os.FileMode(0o644)
	if info != nil {
		mode = info.Mode()
	}
	return os.WriteFile(path, []byte(out), mode) // #nosec G304 -- caller-supplied path
}

type resolvedEdit struct {
	edit EditOp
	idx  int // 0-based
}

// resolveAnchor finds the concrete index for an edit. Exact match first;
// then bounded shifted-anchor recovery (± AnchorWindow).
func resolveAnchor(lines []string, e EditOp) (int, error) {
	// Exact position match.
	if e.Line-1 < len(lines) && Anchor(lines[e.Line-1]) == e.Hash {
		return e.Line - 1, nil
	}
	// Shifted-anchor recovery within the window.
	lo := e.Line - 1 - AnchorWindow
	if lo < 0 {
		lo = 0
	}
	for d := 1; d <= AnchorWindow; d++ {
		for _, idx := range []int{e.Line - 1 - d, e.Line - 1 + d} {
			if idx < 0 || idx >= len(lines) || idx == e.Line-1 {
				continue
			}
			if Anchor(lines[idx]) == e.Hash {
				return idx, nil // drifted to here: recover
			}
		}
	}
	return -1, fmt.Errorf(
		"hashline: anchor L%d:%s not found (line content drifted beyond ±%d-line window); re-read the file",
		e.Line, e.Hash, AnchorWindow,
	)
}

func sortEditsDescending(edits []resolvedEdit) {
	// Stable insertion sort (small N): highest index first.
	for i := 1; i < len(edits); i++ {
		for j := i; j > 0 && edits[j].idx > edits[j-1].idx; j-- {
			edits[j], edits[j-1] = edits[j-1], edits[j]
		}
	}
}

func insertAfter(lines []string, idx int, text string) []string {
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:idx+1]...)
	out = append(out, text)
	out = append(out, lines[idx+1:]...)
	return out
}

func splitKeepEnds(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
