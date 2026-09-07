// table.go — ANSI-safe aligned table rendering for CLI list output.
//
// text/tabwriter counts ANSI escape bytes as cell width, so any colored cell
// (header or value) shifts column alignment. This helper computes column
// widths on the ANSI-stripped visible width and pads accordingly, so callers
// can colorize cells freely without breaking the grid.

package theme

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// visibleWidth returns the display width of s with ANSI escape sequences
// stripped, so alignment is computed on what the terminal actually renders.
func visibleWidth(s string) int {
	return utf8.RuneCountInString(ansiRe.ReplaceAllString(s, ""))
}

// PrintTable writes an aligned table to w. The header row is rendered muted;
// data cells are written verbatim (callers may pre-colorize them). Column
// widths are derived from visible width, so colors never break alignment.
//
// A header is required; rows shorter than the header are left-blank in the
// missing columns and extra trailing cells are emitted unaligned. The result
// is deterministic and safe for non-TTY and --quiet output (muted header is
// plain when color is disabled).
func PrintTable(w io.Writer, header []string, rows [][]string) error {
	if len(header) == 0 {
		return nil
	}
	cols := len(header)
	widths := make([]int, cols)
	for i, h := range header {
		widths[i] = visibleWidth(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if i >= cols {
				break
			}
			if vw := visibleWidth(c); vw > widths[i] {
				widths[i] = vw
			}
		}
	}

	writeRow := func(cells []string) error {
		for i := 0; i < cols; i++ {
			if i > 0 {
				if _, err := fmt.Fprint(w, "  "); err != nil {
					return err
				}
			}
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			if _, err := fmt.Fprint(w, cell); err != nil {
				return err
			}
			// Pad all but the last column to the column width. Trailing cells
			// beyond the header width are emitted unaligned.
			if i < cols-1 {
				if pad := widths[i] - visibleWidth(cell); pad > 0 {
					if _, err := fmt.Fprint(w, strings.Repeat(" ", pad)); err != nil {
						return err
					}
				}
			}
		}
		_, err := fmt.Fprintln(w)
		return err
	}

	hc := make([]string, cols)
	for i, h := range header {
		hc[i] = Tint(h, ReportMuted)
	}
	if err := writeRow(hc); err != nil {
		return err
	}
	for _, r := range rows {
		if err := writeRow(r); err != nil {
			return err
		}
	}
	return nil
}
