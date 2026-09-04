package cmd

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// graycodeBlockGlyphs — fixed 8-column ██ font used by the welcome gate banners.
var graycodeBlockGlyphs = map[rune][5]string{
	'H': {"██   ██ ", "██   ██ ", "███████ ", "██   ██ ", "██   ██ "},
	'A': {"  ███   ", " █████  ", "███████ ", "██   ██ ", "██   ██ "},
	'W': {"██   ██ ", "██   ██ ", "██ █ ██ ", "███ ███ ", "██   ██ "},
	'K': {"██   ██ ", "██  ██  ", "█████   ", "██  ██  ", "██   ██ "},
	'E': {"███████ ", "██      ", "███████ ", "██      ", "███████ "},
	'L': {"██      ", "██      ", "██      ", "██      ", "███████ "},
	'C': {" ██████ ", "██    ██", "██      ", "██    ██", " ██████ "},
	'O': {" ██████ ", "██    ██", "██    ██", "██    ██", " ██████ "},
	'M': {"██    ██", "██ ██ ██", "██ ██ ██", "██    ██", "██    ██"},
	'T': {"████████", "   ██   ", "   ██   ", "   ██   ", "   ██   "},
}

// graycodeLogoArtLines is the canonical GRAYCODE wordmark, with the graycode forming the W.
var graycodeLogoArtLines = []string{
	"                                     .  .",
	"                                  .  .  .  .",
	"                                  .  |  |  .",
	"                               .  |        |  .",
	"                               .              .",
	" ___     ___    _________    . |  (\\.|\\/|./)  | .   ___   ____",
	"|   |   |   |  /    _    \\   .   (\\ |||||| /)   .  |   | /   /",
	"|   |___|   | |    /_\\    |  |  (\\  |/  \\|  /)  |  |   |/   /",
	"|           | |           |    (\\            /)    |       /",
	"|    ___    | |    ___    |   (\\              /)   |       \\",
	"|   |   |   | |   |   |   |    \\      \\/      /    |   |\\   \\",
	"|___|   |___| |___|   |___|     \\____/\\/\\____/     |___| \\___\\",
	"                                    |0\\/0|",
	"                                     \\/\\/",
	"                                      \\/",
}

const (
	graycodeBlockCellW     = 8
	graycodeBlockLetterGap = 1
	graycodeBlockWordGap   = 4
)

// welcomeWordLines — "WELCOME" block (row-aligned, fixed grid).
var welcomeWordLines = composeGraycodeBlockLines("WELCOME")

// welcomeToWordLines — "TO" block, centered under WELCOME on the gate.
var welcomeToWordLines = composeGraycodeBlockLines("TO")

// welcomeToPhraseLines — "WELCOME TO" block for wide welcome gates.
var welcomeToPhraseLines = composeGraycodeBlockLines("WELCOME TO")

// welcomeToBannerMinWidth is the visible width for the WELCOME block.
const welcomeToBannerMinWidth = 61

// welcomeToPhraseMinWidth is the visible width for the combined "WELCOME TO" block.
var welcomeToPhraseMinWidth = blockLinesWidth(welcomeToPhraseLines)

func composeGraycodeBlockLines(text string) []string {
	rows := make([]string, 5)
	words := strings.Fields(text)
	for wi, word := range words {
		for ci, ch := range word {
			glyph, ok := graycodeBlockGlyphs[ch]
			if !ok {
				continue
			}
			for i := range rows {
				if rows[i] != "" {
					if ci == 0 && wi > 0 {
						rows[i] += strings.Repeat(" ", graycodeBlockWordGap)
					} else {
						rows[i] += strings.Repeat(" ", graycodeBlockLetterGap)
					}
				}
				cell := padBlockCell(glyph[i], graycodeBlockCellW)
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
