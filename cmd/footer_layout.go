package cmd

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// footerContentWidth is the text width for footer rows (full terminal width).
func (m chatModel) footerContentWidth(totalW int) int {
	if totalW < 20 {
		return 80
	}
	return totalW
}

// finishFooterLine clips a footer row to the content width.
func (m chatModel) finishFooterLine(line string, totalW int) string {
	return clipFooterLine(line, m.footerContentWidth(totalW))
}

// minFooterRightCols is reserved for ● tokens · $cost · duration on the stats row.
const minFooterRightCols = 28 // ● Nk tokens · $cost · duration

// layoutFooterRow places left and right footer segments on one line without wrapping.
// Right text is aligned with lipgloss (not a long run of spaces) so terminals do not
// soft-wrap the gap and push stats to the left margin. Left truncates before right so
// token/cost stats are not dropped by finishFooterLine clipping.
func layoutFooterRow(left, right string, width int) string {
	if width < 1 {
		width = 80
	}
	if right == "" {
		return clipFooterLine(left, width)
	}
	if left == "" {
		return clipFooterLine(right, width)
	}

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	reserve := rightW
	if leftW+rightW > width {
		if reserve < minFooterRightCols {
			reserve = minFooterRightCols
		}
	}
	if reserve > width {
		reserve = width
	}

	maxLeft := width - reserve
	if maxLeft < 1 {
		maxLeft = 1
	}
	if lipgloss.Width(left) > maxLeft {
		left = ansi.Truncate(left, maxLeft, "…")
	}

	leftW = lipgloss.Width(left)
	remain := width - leftW
	if remain < 1 {
		remain = 1
	}
	if lipgloss.Width(right) > remain {
		right = ansi.Truncate(right, remain, "…")
	}

	rightBlock := lipgloss.NewStyle().
		Width(remain).
		Align(lipgloss.Right).
		Inline(true).
		Render(right)
	row := lipgloss.JoinHorizontal(lipgloss.Top, left, rightBlock)
	if lipgloss.Width(row) > width {
		row = ansi.Truncate(row, width, "…")
	}
	return row
}

func clipFooterLine(line string, width int) string {
	if width < 1 {
		return line
	}
	if lipgloss.Width(line) <= width {
		return line
	}
	return ansi.Truncate(line, width, "…")
}

func shortenFooterContainerStatus(status string) string {
	// Docker container IDs are 12+ hex chars — keep the footer row readable.
	if len(status) > 14 {
		return status[:12] + "…"
	}
	return status
}

func formatTokenCountCompact(tokens int) string {
	if tokens >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%dk", (tokens+500)/1000)
	}
	return fmt.Sprintf("%d", tokens)
}