package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestClipRenderedBlock_TrimsWideBorder(t *testing.T) {
	border := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, true, false).
		BorderForeground(borderDim).
		Width(40).
		Render("hello")
	got := clipRenderedBlock(border, 40)
	for _, line := range strings.Split(got, "\n") {
		if line == "" {
			continue
		}
		if lipgloss.Width(line) > 40 {
			t.Fatalf("line wider than 40: width=%d line=%q", lipgloss.Width(line), line)
		}
	}
}
