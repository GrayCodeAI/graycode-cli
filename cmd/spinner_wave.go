package cmd

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// spinnerWaveColors — 20 distinct hues; the wave flows glyph → verb → ▪▫▫.
var spinnerWaveColors = [20][3]int{
	{255, 94, 14},
	{255, 60, 60},
	{255, 100, 80},
	{255, 150, 60},
	{255, 200, 60},
	{255, 240, 80},
	{200, 255, 80},
	{120, 255, 100},
	{60, 255, 140},
	{40, 230, 200},
	{60, 200, 255},
	{100, 160, 255},
	{140, 120, 255},
	{180, 100, 255},
	{220, 90, 230},
	{255, 80, 200},
	{255, 70, 150},
	{255, 80, 110},
	{255, 160, 90},
	{255, 125, 40},
}

const spinnerWaveLen = len(spinnerWaveColors)

func ansiSpinnerWaveColor(index int) string {
	i := index % spinnerWaveLen
	if i < 0 {
		i += spinnerWaveLen
	}
	c := spinnerWaveColors[i]
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", c[0], c[1], c[2])
}

// renderSpinnerWaveLine paints glyph + verb + ▪▫▫ as one flowing strip (20 colors).
func renderSpinnerWaveLine(glyph, verb string, wavePhase, dotPhase int) string {
	verbRunes := utf8.RuneCountInString(verb)
	total := 1 + verbRunes + 1 + hawkTypingDots
	if verb == "" {
		total = 1
	}
	head := wavePhase % total

	var b strings.Builder
	b.WriteString(renderSpinnerWaveSlotGlyph(glyph, wavePhase, head == 0, true))
	if verb == "" {
		return b.String()
	}

	b.WriteString("  ")
	pos := 1
	for _, r := range verb {
		b.WriteString(renderSpinnerWaveSlot(r, wavePhase+pos, head == pos, false))
		pos++
	}
	b.WriteString(renderSpinnerWaveSlot(' ', wavePhase+pos, head == pos, false))
	pos++
	for i := 0; i < hawkTypingDots; i++ {
		g := iconDotEmpty
		bold := false
		if i == dotPhase%hawkTypingDots {
			g = iconDotFilled
			bold = true
		}
		b.WriteString(renderSpinnerWaveSlotGlyph(g, wavePhase+pos, head == pos, bold))
		pos++
	}
	return b.String()
}

func renderSpinnerWaveSlot(r rune, colorIdx int, isHead, bold bool) string {
	return renderSpinnerWaveSlotGlyph(string(r), colorIdx, isHead, bold)
}

func renderSpinnerWaveSlotGlyph(glyph string, colorIdx int, isHead, bold bool) string {
	color := ansiSpinnerWaveColor(colorIdx)
	if isHead || bold {
		color += ansiBold
	}
	var b strings.Builder
	b.WriteString(color)
	b.WriteString(ansiItalic)
	b.WriteString(glyph)
	b.WriteString(ansiReset)
	return b.String()
}

func frameContainsSpinnerWave(s string) bool {
	c := spinnerWaveColors[0]
	needle := fmt.Sprintf("38;2;%d;%d;%d", c[0], c[1], c[2])
	return strings.Contains(s, needle)
}