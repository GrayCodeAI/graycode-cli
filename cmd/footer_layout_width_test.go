package cmd

import (
	"strings"
	"testing"

		lipgloss "charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

func TestLayoutFooterRow_StyledStringsAlignRight(t *testing.T) {
	cwdStyle := lipgloss.NewStyle().Foreground(statusCWDColor).Inline(true)
	tokenStyle := lipgloss.NewStyle().Foreground(statusTokenColor).Inline(true)
	dim := lipgloss.NewStyle().Foreground(dimColor).Inline(true)

	left := cwdStyle.Render("~/hawk:") + " " + cwdStyle.Render("⎇ main")
	right := tokenStyle.Render("[db] 13k") + dim.Render(" · ") + tokenStyle.Render("$0.00")
	width := 80

	row := layoutFooterRow(left, right, width)
	if lipgloss.Width(row) > width {
		t.Fatalf("row width %d > %d: %q", lipgloss.Width(row), width, row)
	}
	if !strings.HasPrefix(row, left) {
		t.Fatalf("row should start with left segment")
	}
	if !strings.HasSuffix(row, right) {
		t.Fatalf("row should end with right segment, got %q", row)
	}
	// Right segment should start near the right edge (within a few cols).
	rightStart := lipgloss.Width(row) - lipgloss.Width(right)
	leftEnd := lipgloss.Width(left)
	if rightStart < width-lipgloss.Width(right)-2 {
		t.Fatalf("right not flush: rightStart=%d width=%d rightW=%d rowW=%d",
			rightStart, width, lipgloss.Width(right), lipgloss.Width(row))
	}
	if leftEnd > rightStart {
		t.Fatalf("left overlaps right: leftEnd=%d rightStart=%d", leftEnd, rightStart)
	}
}

func TestFinishFooterLine_PreservesRightAlignedStats(t *testing.T) {
	m := chatModel{width: 100, height: 24}
	tokenStyle := lipgloss.NewStyle().Foreground(statusTokenColor).Inline(true)
	left := lipgloss.NewStyle().Foreground(statusCWDColor).Inline(true).Render("~/proj:")
	right := tokenStyle.Render("[db] 1k") + lipgloss.NewStyle().Foreground(dimColor).Inline(true).Render(" · ") + tokenStyle.Render("$1.00")
	row := layoutFooterRow(left, right, m.footerContentWidth(100))
	out := m.finishFooterLine(row, 100)
	if lipgloss.Width(out) > 100 {
		t.Fatalf("footer overflow: %d", lipgloss.Width(out))
	}
	if !strings.HasSuffix(out, right) {
		t.Fatalf("right segment lost after finishFooterLine: %q", out)
	}
}

func TestLayoutFooterRow_LipglossMatchesRunewidthOnPlain(t *testing.T) {
	left, right := "left", "right"
	if runewidth.StringWidth(left) != lipgloss.Width(left) {
		t.Fatalf("plain width mismatch")
	}
	row := layoutFooterRow(left, right, 30)
	if lipgloss.Width(row) != runewidth.StringWidth(row) {
		t.Fatalf("layout width mismatch: lipgloss=%d runewidth=%d",
			lipgloss.Width(row), runewidth.StringWidth(row))
	}
}
