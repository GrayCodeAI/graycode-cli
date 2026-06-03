package cmd

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// hawkBlockGlyphs — fixed 8-column ██ font (H/A/W/K match hawkLogoArtLines).
var hawkBlockGlyphs = map[rune][5]string{
	'H': {"██   ██ ", "██   ██ ", "███████ ", "██   ██ ", "██   ██ "},
	'A': {" █████  ", "██   ██ ", "███████ ", "██   ██ ", "██   ██ "},
	'W': {"██   ██ ", "██   ██ ", "██ █ ██ ", "███ ███ ", "██   ██ "},
	'K': {"██   ██ ", "██  ██  ", "█████   ", "██  ██  ", "██   ██ "},
	'E': {"███████ ", "██      ", "███████ ", "██      ", "███████ "},
	'L': {"██      ", "██      ", "██      ", "██      ", "███████ "},
	'C': {" ██████ ", "██    ██", "██      ", "██    ██", " ██████ "},
	'O': {" ██████ ", "██    ██", "██    ██", "██    ██", " ██████ "},
	'M': {"██    ██", "██ ██ ██", "██ ██ ██", "██    ██", "██    ██"},
	'T': {"████████", "   ██   ", "   ██   ", "   ██   ", "   ██   "},
}

// hawkLogoArtLines is the canonical HAWK wordmark.
var hawkLogoArtLines = []string{
	"██   ██  █████   ██     ██ ██   ██",
	"██   ██ ██   ██  ██     ██ ██  ██ ",
	"███████ ███████  ██  █  ██ █████  ",
	"██   ██ ██   ██  ██ ███ ██ ██  ██ ",
	"██   ██ ██   ██   ███ ███  ██   ██",
}

const (
	hawkBlockCellW     = 8
	hawkBlockLetterGap = 1
	hawkBlockWordGap   = 2
)

// welcomeWordLines — "WELCOME" block (row-aligned, fixed grid).
var welcomeWordLines = composeHawkBlockLines("WELCOME")

// welcomeToWordLines — "TO" block, centered under WELCOME on the gate.
var welcomeToWordLines = composeHawkBlockLines("TO")

// welcomeToPhraseLines — "WELCOME TO" block for wide welcome gates.
var welcomeToPhraseLines = composeHawkBlockLines("WELCOME TO")

// welcomeToBannerMinWidth is the visible width for the WELCOME block.
const welcomeToBannerMinWidth = 61

// welcomeToPhraseMinWidth is the visible width for the combined "WELCOME TO" block.
var welcomeToPhraseMinWidth = blockLinesWidth(welcomeToPhraseLines)

// welcomeGateBannerWidth returns the widest row across WELCOME + TO blocks.
func welcomeGateBannerWidth() int {
	w := welcomeToPhraseMinWidth
	for _, line := range welcomeToWordLines {
		if n := runewidth.StringWidth(line); n > w {
			w = n
		}
	}
	return w
}

func composeHawkBlockLines(text string) []string {
	rows := make([]string, 5)
	words := strings.Fields(text)
	for wi, word := range words {
		for ci, ch := range word {
			glyph, ok := hawkBlockGlyphs[ch]
			if !ok {
				continue
			}
			for i := range rows {
				if rows[i] != "" {
					if ci == 0 && wi > 0 {
						rows[i] += strings.Repeat(" ", hawkBlockWordGap)
					} else {
						rows[i] += strings.Repeat(" ", hawkBlockLetterGap)
					}
				}
				cell := padBlockCell(glyph[i], hawkBlockCellW)
				rows[i] += cell
			}
		}
	}
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	return rows
}

func blockLinesWidth(lines []string) int {
	w := 0
	for _, line := range lines {
		if n := runewidth.StringWidth(line); n > w {
			w = n
		}
	}
	return w
}

func padBlockCell(s string, w int) string {
	if runewidth.StringWidth(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-runewidth.StringWidth(s))
}
