package cmd

import (
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
)

func TestLayoutFooterRow_NarrowRightBlockStillShowsBullet(t *testing.T) {
	right := lipgloss.NewStyle().Foreground(statusTokenColor).Inline(true).Render("[db] 13k · $0.00")
	for _, remain := range []int{1, 5, 10} {
		rb := lipgloss.NewStyle().Width(remain).Align(lipgloss.Right).Inline(true).Render(right)
		if !strings.Contains(rb, "[db]") {
			t.Fatalf("remain=%d dropped tokens: %q", remain, rb)
		}
	}
}
