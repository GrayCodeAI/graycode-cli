package cmd

import (
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const (
	configTableColGap = 4
	configTableIndent = 6
	configTableColPad = 2
)

type configTableLayout struct {
	Widths []int
}

func computeConfigTableLayout(viewWidth int, headers []string, rows [][]string, colPad []int, activeLastCol bool) configTableLayout {
	if viewWidth <= 0 {
		viewWidth = 80
	}
	n := len(headers)
	widths := make([]int, n)
	for i, h := range headers {
		widths[i] = runewidth.StringWidth(h)
	}
	for _, row := range rows {
		for i := 0; i < n && i < len(row); i++ {
			text := row[i]
			if activeLastCol && i == n-1 && strings.Contains(text, "●") {
				text = strings.TrimSpace(text) + " ●"
			}
			widths[i] = maxInt(widths[i], runewidth.StringWidth(text))
		}
	}
	for i := range widths {
		pad := configTableColPad
		if i < len(colPad) && colPad[i] > 0 {
			pad = colPad[i]
		}
		widths[i] += pad
	}
	gaps := configTableColGap * maxInt(0, n-1)
	usable := viewWidth - configTableIndent
	if usable < 32 {
		usable = 32
	}
	other := gaps
	for i := 1; i < n; i++ {
		other += widths[i]
	}
	maxFirst := usable - other
	if maxFirst < 12 {
		maxFirst = 12
	}
	if n > 0 && widths[0] > maxFirst {
		widths[0] = maxFirst
	}
	return configTableLayout{Widths: widths}
}

func renderConfigTableHeader(headers []string, layout configTableLayout, headerStyle, metaStyle lipgloss.Style) string {
	line := renderConfigTableLine(headers, layout, repeatStyle(len(headers), headerStyle, metaStyle))
	ruleLen := configTableLineWidth(layout)
	indent := strings.Repeat(" ", configTableIndent)
	return indent + line + "\n" + indent + metaStyle.Render(strings.Repeat("─", ruleLen))
}

func renderConfigTableRow(values []string, cursor, active, markEnd bool, layout configTableLayout, rowStyle, cursorStyle, activeStyle, metaStyle lipgloss.Style) string {
	style := rowStyle
	meta := metaStyle
	if active && !cursor {
		style = activeStyle
		meta = activeStyle
	}
	if cursor {
		style = cursorStyle
		meta = cursorStyle
	}
	prefix := strings.Repeat(" ", configTableIndent)
	if cursor {
		prefix = strings.Repeat(" ", configTableIndent-2) + cursorStyle.Render(icons.ChevronRight()) + " "
	}
	cols := make([]string, len(values))
	copy(cols, values)
	if markEnd && len(cols) > 0 {
		last := len(cols) - 1
		if active {
			text := strings.TrimSpace(cols[last])
			if text == "" || text == "—" {
				cols[last] = "●"
			} else {
				cols[last] = text + " ●"
			}
		} else {
			cols[last] = strings.TrimSpace(cols[last])
		}
	}
	for i := range cols {
		cols[i] = truncateRunes(cols[i], layout.Widths[i])
	}
	styles := repeatStyle(len(cols), style, meta)
	return prefix + renderConfigTableLine(cols, layout, styles)
}

func renderConfigTableLine(values []string, layout configTableLayout, styles []lipgloss.Style) string {
	parts := make([]string, len(values))
	for i, v := range values {
		w := 0
		if i < len(layout.Widths) {
			w = layout.Widths[i]
		}
		s := lipgloss.NewStyle()
		if i < len(styles) {
			s = styles[i]
		}
		parts[i] = s.Render(padCellLeft(v, w))
	}
	return strings.Join(parts, strings.Repeat(" ", configTableColGap))
}

func configTableLineWidth(layout configTableLayout) int {
	total := 0
	for i, w := range layout.Widths {
		total += w
		if i > 0 {
			total += configTableColGap
		}
	}
	return total
}

func configTableScrollHint(above, below int, muted lipgloss.Style) string {
	prefix := strings.Repeat(" ", configTableIndent)
	switch {
	case above > 0 && below > 0:
		return muted.Render(fmt.Sprintf("%s↑ %d above · ↓ %d below", prefix, above, below))
	case above > 0:
		return muted.Render(fmt.Sprintf("%s↑ %d above", prefix, above))
	case below > 0:
		return muted.Render(fmt.Sprintf("%s↓ %d below", prefix, below))
	default:
		return ""
	}
}

func configTableSelectionFooter(total, scroll, end int, muted lipgloss.Style, suffix string) string {
	prefix := strings.Repeat(" ", configTableIndent)
	if total == 0 {
		return muted.Render(prefix + suffix)
	}
	start := scroll + 1
	if end > total {
		end = total
	}
	if start > end {
		start = end
	}
	return muted.Render(fmt.Sprintf("%s%d–%d of %d · %s", prefix, start, end, total, suffix))
}

func repeatStyle(n int, primary, fallback lipgloss.Style) []lipgloss.Style {
	out := make([]lipgloss.Style, n)
	for i := range out {
		if i == 0 {
			out[i] = primary
		} else {
			out[i] = fallback
		}
	}
	return out
}
