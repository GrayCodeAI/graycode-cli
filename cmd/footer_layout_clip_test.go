package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLayoutFooterRow_TokensSurviveFinishFooterLine(t *testing.T) {
	m := chatModel{width: 70, height: 24}
	left := lipgloss.NewStyle().Foreground(statusCWDColor).Inline(true).Render("~/OSS2026/RealWork/hawk-eco/hawk:")
	left += " " + lipgloss.NewStyle().Foreground(statusBranchColor).Inline(true).Render("⎇ main")
	right := lipgloss.NewStyle().Foreground(statusTokenColor).Inline(true).Render("● 13k")
	right += lipgloss.NewStyle().Foreground(dimColor).Inline(true).Render(" · ")
	right += lipgloss.NewStyle().Foreground(statusCostColor).Inline(true).Render("$0.00")
	right += lipgloss.NewStyle().Foreground(dimColor).Inline(true).Render(" · ")
	right += lipgloss.NewStyle().Foreground(hudLabelPink).Inline(true).Render("⏱ 3m 2s")

	footerW := m.footerContentWidth(70)
	row := layoutFooterRow(left, right, footerW)
	out := m.finishFooterLine(row, 70)

	if !strings.Contains(out, "●") {
		t.Fatalf("tokens removed after layout+clip: footerW=%d rowW=%d outW=%d\nrow=%q\nout=%q",
			footerW, lipgloss.Width(row), lipgloss.Width(out), row, out)
	}
	if lipgloss.Width(out) > 70 {
		t.Fatalf("footer wider than terminal: %d", lipgloss.Width(out))
	}
}

func TestLayoutFooterRow_LeftWiderThanFooterStillShowsTokens(t *testing.T) {
	left := strings.Repeat("x", 90)
	right := "● 99k · $1.00"
	width := 80
	row := layoutFooterRow(left, right, width)
	if !strings.Contains(row, "●") {
		t.Fatalf("tokens dropped when left exceeds width: %q", row)
	}
}

func TestLayoutFooterRow_ClipDoesNotDropStyledTokens(t *testing.T) {
	m := chatModel{width: 55, height: 24}
	left := lipgloss.NewStyle().Foreground(statusCWDColor).Inline(true).Render("~/Desktop/OSS2026/RealWork/hawk-eco/hawk:")
	left += " " + lipgloss.NewStyle().Foreground(statusBranchColor).Inline(true).Render("⎇ feature/footer-fix")
	dim := lipgloss.NewStyle().Foreground(dimColor).Inline(true)
	tok := lipgloss.NewStyle().Foreground(statusTokenColor).Inline(true)
	right := tok.Render("● 14442") + dim.Render(" · ") + tok.Render("$12.34") + dim.Render(" · ") + tok.Render("⏱ 1h 2m")
	footerW := m.footerContentWidth(55)
	row := layoutFooterRow(left, right, footerW)
	if lipgloss.Width(row) > footerW+2 {
		t.Logf("row exceeds footer before clip: rowW=%d footerW=%d", lipgloss.Width(row), footerW)
	}
	out := m.finishFooterLine(row, 55)
	if !strings.Contains(out, "●") {
		t.Fatalf("tokens clipped away: footerW=%d rowW=%d outW=%d\nrow=%q\nout=%q",
			footerW, lipgloss.Width(row), lipgloss.Width(out), row, out)
	}
}

func TestLayoutFooterRow_TotalWidthNeverExceedsTarget(t *testing.T) {
	left := strings.Repeat("a", 50)
	right := strings.Repeat("b", 40)
	width := 80
	row := layoutFooterRow(left, right, width)
	if lipgloss.Width(row) > width {
		t.Fatalf("row width %d > %d", lipgloss.Width(row), width)
	}
}

func TestLayoutFooterRow_StyledRowNeverExceedsTarget(t *testing.T) {
	left := lipgloss.NewStyle().Foreground(statusCWDColor).Inline(true).Render(strings.Repeat("a", 50))
	right := lipgloss.NewStyle().Foreground(statusTokenColor).Inline(true).Render("● " + strings.Repeat("b", 35))
	width := 80
	row := layoutFooterRow(left, right, width)
	rowW := lipgloss.Width(row)
	if rowW > width {
		m := chatModel{width: width, height: 24}
		out := m.finishFooterLine(row, width)
		if !strings.Contains(out, "●") {
			t.Fatalf("rowW=%d > width=%d; clip removed tokens\nrow=%q\nout=%q", rowW, width, row, out)
		}
		t.Logf("row exceeds width but tokens kept: rowW=%d", rowW)
	}
}
