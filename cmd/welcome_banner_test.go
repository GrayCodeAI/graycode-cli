package cmd

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestWelcomeToBanner_TOAligned(t *testing.T) {
	to := composeGraycodeBlockLines("TO")
	if len(to) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(to))
	}
	if strings.Contains(to[1], "███████") {
		t.Fatalf("T stem row should not show a full top bar: %q", to[1])
	}
	if !strings.HasPrefix(strings.TrimLeft(to[1], " "), "██") {
		t.Fatalf("O side walls should align on row 1: %q", to[1])
	}
}

func TestWelcomeWordLinesMinWidth(t *testing.T) {
	for i, line := range welcomeWordLines {
		w := runewidth.StringWidth(line)
		if w > welcomeToBannerMinWidth {
			t.Fatalf("line %d wider than min width %d: %d", i, welcomeToBannerMinWidth, w)
		}
	}
}

func TestWelcomeToWordLinesCompact(t *testing.T) {
	w := runewidth.StringWidth(welcomeToWordLines[0])
	if w > 20 {
		t.Fatalf("TO block should stay compact, got width %d: %q", w, welcomeToWordLines[0])
	}
}

func TestWelcomeToPhraseLinesContainSingleRowPhrase(t *testing.T) {
	if len(welcomeToPhraseLines) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(welcomeToPhraseLines))
	}
	if welcomeToPhraseMinWidth <= welcomeToBannerMinWidth {
		t.Fatalf("combined phrase should be wider than stacked banner: phrase=%d stacked=%d", welcomeToPhraseMinWidth, welcomeToBannerMinWidth)
	}
	if !strings.HasPrefix(welcomeToPhraseLines[0], welcomeWordLines[0]) {
		t.Fatalf("combined phrase should start with WELCOME row:\nphrase:  %q\nwelcome: %q", welcomeToPhraseLines[0], welcomeWordLines[0])
	}
}

func TestWelcomeWordLines_SameWGlyphAsGRAYCODE(t *testing.T) {
	w := strings.TrimRight(graycodeBlockGlyphs['W'][0], " ")
	if !strings.HasPrefix(welcomeWordLines[0], w) {
		t.Fatalf("W row should start with graycode logo glyph:\nwelcome: %q\ngraycode W:  %q",
			welcomeWordLines[0], w)
	}
}
