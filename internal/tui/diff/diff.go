// Package diff implements the core of a differential terminal renderer: it
// compares consecutive rendered frames and emits only the changed lines,
// wrapped in synchronized-output sequences to avoid flicker/tearing.
//
// This mirrors the algorithm in earendil-works/pi-tui: render the full frame,
// diff against the previous frame, then re-emit only the changed range with the
// cursor positioned at the first changed row and each changed line cleared
// before rewrite. It is a self-contained, testable unit that a terminal render
// loop can adopt without coupling to any specific agent runtime.
package diff

import (
	"fmt"
	"io"
	"strings"
)

// Range describes the contiguous changed line region between two frames.
type Range struct {
	First    int  // first changed line (0-based)
	Last     int  // last changed line (0-based, inclusive)
	Changed  bool // true when any line changed
	Appended bool // true when lines were appended beyond the previous frame
}

// Changed compares prev and next frames line by line and returns the minimal
// contiguous range covering every difference. Appended lines extend the range
// to the end of next. A nil/empty next returns Changed=false.
func Changed(prev, next []string) Range {
	if len(next) == 0 {
		return Range{}
	}
	first := -1
	last := -1
	limit := len(prev)
	if len(next) < limit {
		limit = len(next)
	}
	for i := 0; i < limit; i++ {
		if prev[i] != next[i] {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	// Appended lines extend the range to the end of next.
	if len(next) > len(prev) {
		if first < 0 {
			first = len(prev)
		}
		last = len(next) - 1
	}
	if first < 0 {
		return Range{}
	}
	return Range{First: first, Last: last, Changed: true, Appended: len(next) > len(prev)}
}

// SynchronizedRender emits only the changed lines from prev to next, wrapped in
// synchronized-output sequences. It writes the sync-open, moves the cursor to
// the first changed row, clears and rewrites each changed line, and closes the
// sync block. It returns the number of lines emitted. When nothing changed it
// writes nothing.
func SynchronizedRender(w io.Writer, prev, next []string) (int, error) {
	changed := Changed(prev, next)
	if !changed.Changed {
		return 0, nil
	}
	var b strings.Builder
	// Synchronized output: open the marker block before any emission.
	b.WriteString("\x1b[?2026h")
	b.WriteString(fmt.Sprintf("\x1b[%d;1H", changed.First+1)) // move to first changed row
	for i := changed.First; i <= changed.Last; i++ {
		b.WriteString("\x1b[2K") // clear the line
		b.WriteString(next[i])
		b.WriteString("\r\n")
	}
	b.WriteString("\x1b[?2026l")
	_, err := io.WriteString(w, b.String())
	if err != nil {
		return 0, err
	}
	return changed.Last - changed.First + 1, nil
}
