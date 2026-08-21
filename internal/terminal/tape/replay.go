package tape

import (
	"strings"
)

// Grid is a focused virtual terminal for fxtape replay: a scrollback of text
// lines plus a cursor, interpreting the common ANSI sequences that tool
// output emits (carriage return, newline, backspace, tab, SGR colour codes,
// clear line/screen, and cursor movement/positioning). It is intentionally a
// subset of a full VT100 emulator (fx's terminal/engine.zig) — sufficient to
// reconstruct field-displayable output from a real capture.
type Grid struct {
	// lines is the scrollback: each entry is one row of runes up to Cols.
	lines [][]rune
	row   int // current cursor row within lines
	col   int // current cursor column (0-based)
	Cols  int
	Rows  int
}

// NewGrid builds an empty grid with the given terminal size.
func NewGrid(cols, rows int) *Grid {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	g := &Grid{Cols: cols, Rows: rows, lines: [][]rune{{}}}
	return g
}

func (g *Grid) ensureRow() {
	for g.row >= len(g.lines) {
		g.lines = append(g.lines, make([]rune, 0, g.Cols))
	}
}

func (g *Grid) put(ch rune) {
	g.ensureRow()
	line := g.lines[g.row]
	for len(line) <= g.col {
		line = append(line, ' ')
	}
	line[g.col] = ch
	g.lines[g.row] = line
	g.col++
	if g.col >= g.Cols {
		g.col = 0
		g.newline()
	}
}

func (g *Grid) newline() {
	g.row++
	g.col = 0
	g.ensureRow()
}

// Feed interprets a chunk of raw terminal output bytes.
func (g *Grid) Feed(b []byte) {
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch c {
		case '\r':
			g.col = 0
		case '\n':
			g.newline()
		case '\b':
			if g.col > 0 {
				g.col--
			}
		case '\t':
			g.col += 8 - g.col%8
			if g.col >= g.Cols {
				g.col = g.Cols - 1
			}
		case 0x07: // BEL
			// bell — no visual effect
		case 0x1b: // ESC
			i = g.parseEscape(b, i)
		default:
			if c >= 0x20 {
				g.put(rune(c))
			}
		}
	}
}

// parseEscape consumes an ESC sequence starting at b[i]=='0x1b' and returns
// the index of the last consumed byte. Unknown sequences are skipped.
func (g *Grid) parseEscape(b []byte, i int) int {
	if i+1 >= len(b) {
		return len(b) - 1
	}
	esc := b[i+1]
	switch esc {
	case '[': // CSI
		return g.parseCSI(b, i+2)
	case ']': // OSC — skip until BEL or ST
		j := i + 2
		for j < len(b) {
			if b[j] == 0x07 {
				return j
			}
			if b[j] == 0x1b && j+1 < len(b) && b[j+1] == '\\' {
				return j + 1
			}
			j++
		}
		return len(b) - 1
	case '\\', 'c', '7', '8', '=', '>', '(', ')':
		return i + 1
	default:
		return i + 1
	}
}

// parseCSI consumes a Control Sequence Introducer body after `b[j]` (the byte
// after `\x1b[`) and returns the index of the final byte.
func (g *Grid) parseCSI(b []byte, j int) int {
	params := make([]int, 0, 8)
	var cur int
	have := false
	private := false
	k := j
	for ; k < len(b); k++ {
		c := b[k]
		switch {
		case c >= '0' && c <= '9':
			have = true
			cur = cur*10 + int(c-'0')
		case c == ';':
			params = append(params, cur)
			cur = 0
			have = false
		case c == '?':
			private = true
		case c < 0x20:
			// intervening control byte — ignore
		default:
			// final byte
			if have {
				params = append(params, cur)
			}
			if len(params) == 0 {
				params = []int{0}
			}
			if !private {
				g.applyCSI(params, c)
			}
			return k
		}
	}
	return len(b) - 1
}

func (g *Grid) applyCSI(p []int, final byte) {
	n := p[0]
	switch final {
	case 'A': // cursor up
		g.row -= n
		if g.row < 0 {
			g.row = 0
		}
	case 'B': // cursor down
		g.row += n
		g.ensureRow()
	case 'C': // cursor forward
		g.col += n
	case 'D': // cursor back
		g.col -= n
		if g.col < 0 {
			g.col = 0
		}
	case 'H', 'f': // cursor position (row;col), 1-based
		row, col := p[0], 1
		if len(p) >= 2 {
			col = p[1]
		}
		if row < 1 {
			row = 1
		}
		if col < 1 {
			col = 1
		}
		g.row = row - 1
		g.col = col - 1
		g.ensureRow()
	case 'G': // cursor column (1-based)
		if n < 1 {
			n = 1
		}
		g.col = n - 1
	case 'K': // erase in line
		g.eraseLine(p[0])
	case 'J': // erase in display
		g.eraseDisplay(p[0])
	case 'm': // SGR — colour/attribute, no layout effect
	case 's', 'u': // save/restore cursor (approx: ignore)
	case 'g': // tab clear — ignore
	default:
		// unknown CSI — ignore
	}
}

func (g *Grid) eraseLine(mode int) {
	g.ensureRow()
	line := g.lines[g.row]
	switch mode {
	case 0: // erase from cursor to end of line
		for len(line) <= g.col {
			line = append(line, ' ')
		}
		for i := g.col; i < len(line); i++ {
			line[i] = ' '
		}
	case 1: // erase from start to cursor
		for i := 0; i <= g.col && i < len(line); i++ {
			line[i] = ' '
		}
	case 2: // erase entire line
		for i := range line {
			line[i] = ' '
		}
	}
	g.lines[g.row] = line
}

func (g *Grid) eraseDisplay(mode int) {
	switch mode {
	case 0: // below cursor
		g.eraseLine(0)
		for r := g.row + 1; r < len(g.lines); r++ {
			g.lines[r] = []rune{}
		}
	case 1: // above cursor
		for r := 0; r < g.row; r++ {
			g.lines[r] = []rune{}
		}
		g.eraseLine(1)
	case 2, 3: // clear whole screen (+ scrollback)
		g.lines = [][]rune{{}}
		g.row = 0
		g.col = 0
	}
}

// Resize changes the grid columns; rows affect the snapshotted window.
func (g *Grid) Resize(cols, rows int) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	g.Cols = cols
	g.Rows = rows
	if g.col >= cols {
		g.col = cols - 1
	}
}

// Snapshot renders the visible terminal contents (the last Rows lines, each
// trimmed of trailing space and padded to Cols).
func (g *Grid) Snapshot() string {
	start := len(g.lines) - g.Rows
	if start < 0 {
		start = 0
	}
	var rows []string
	for r := start; r < len(g.lines); r++ {
		line := strings.TrimRight(string(g.lines[r]), " ")
		rows = append(rows, line)
	}
	// Drop fully-empty trailing rows so golden files are stable.
	for len(rows) > 0 && strings.TrimRight(rows[len(rows)-1], " ") == "" {
		rows = rows[:len(rows)-1]
	}
	return strings.Join(rows, "\n")
}

// Replay applies a parsed tape's stdout frames into a grid, honoring resizes
// and markers. It returns the final snapshot and per-frame stats.
type Replay struct {
	Grid       *Grid
	Stdout     int // total stdout bytes fed
	Frames     int // frames processed
	Markers    []string
	RenderedMS int64 // sum of deltas
}

// ReplayTape feeds every frame of a parsed tape into a grid sized from its
// header, returning the finished Replay and final visible snapshot.
func ReplayTape(t *Tape) (*Replay, string) {
	r := &Replay{Grid: NewGrid(int(t.Header.Cols), int(t.Header.Rows))}
	for _, f := range t.Frames {
		r.Frames++
		r.RenderedMS += int64(f.DeltaMS)
		switch f.Kind {
		case KindStdout:
			r.Grid.Feed(f.Payload)
			r.Stdout += len(f.Payload)
		case KindResize:
			if len(f.Payload) >= 4 {
				cols := int(f.Payload[0]) | int(f.Payload[1])<<8
				rows := int(f.Payload[2]) | int(f.Payload[3])<<8
				r.Grid.Resize(cols, rows)
			}
		case KindMarker:
			r.Markers = append(r.Markers, string(f.Payload))
		}
	}
	return r, r.Grid.Snapshot()
}
