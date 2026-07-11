package cmd

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

// ---------------------------------------------------------------------------
// Legacy markdown rendering using lipgloss (used by chat_view.go)
//
// The struct-based MarkdownRenderer (glamour/glow-inspired) lives in
// markdown_renderer.go. Shared helpers defined here (visibleWidth, stripAnsi,
// reAnsi, isHorizontalRule, parseHeader) are used by both renderers.
// ---------------------------------------------------------------------------

// Markdown rendering styles using the project's purpose-named palette.
var (
	mdHeaderStyle     = lipgloss.NewStyle().Foreground(successTeal).Bold(true)
	mdBoldStyle       = lipgloss.NewStyle().Foreground(hawkColor).Bold(true)
	mdItalicStyle     = lipgloss.NewStyle().Italic(true)
	mdInlineCodeStyle = lipgloss.NewStyle().Background(bgCode).Foreground(textPrimary)
	mdCodeBlockStyle  = lipgloss.NewStyle().Background(bgCode)
	mdCodeLabelStyle  = lipgloss.NewStyle().Foreground(textDisabled).Background(bgCode)
	mdLinkTextStyle   = lipgloss.NewStyle().Foreground(successTeal)
	mdLinkURLStyle    = lipgloss.NewStyle().Foreground(textDisabled)
	mdBlockquoteBar   = lipgloss.NewStyle().Foreground(textDisabled)
	mdBlockquoteText  = lipgloss.NewStyle().Foreground(textDisabled)
	mdHRStyle         = lipgloss.NewStyle().Foreground(textDisabled)
	mdBulletStyle     = lipgloss.NewStyle().Foreground(successTeal)
)

// Inline regex patterns, compiled once.
var (
	reInlineCode  = regexp.MustCompile("`([^`]+)`")
	reMDBold      = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reMDItalic    = regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)
	reMDLink      = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reTableSepCol = regexp.MustCompile(`^:?-+:?$`)
)

// renderMarkdown converts a markdown string into styled ANSI terminal output
// that fits within the given width. It handles code blocks, headers, lists,
// blockquotes, horizontal rules, bold, italic, inline code, and links.
func renderMarkdown(content string, width int) string {
	if width < 20 {
		width = 80
	}

	lines := strings.Split(content, "\n")
	var result strings.Builder
	i := 0

	for i < len(lines) {
		line := lines[i]

		// Fenced code block
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			block, end := extractCodeBlock(lines, i)
			result.WriteString(renderCodeBlock(block.lang, block.code, width))
			result.WriteByte('\n')
			i = end + 1
			continue
		}

		// Horizontal rule: ---, ***, ___
		trimmed := strings.TrimSpace(line)
		if isHorizontalRule(trimmed) {
			result.WriteString(mdHRStyle.Render(strings.Repeat("─", width)))
			result.WriteByte('\n')
			i++
			continue
		}

		// Headers
		if level, text := parseHeader(line); level > 0 {
			rendered := renderInlineFormatting(text, width)
			result.WriteString(mdHeaderStyle.Render(rendered))
			result.WriteByte('\n')
			i++
			continue
		}

		// Blockquote
		if strings.HasPrefix(trimmed, "> ") || trimmed == ">" {
			text := ""
			if len(trimmed) > 2 {
				text = trimmed[2:]
			}
			bar := mdBlockquoteBar.Render("│ ")
			wrapped := mdWordWrap(text, width-3)
			for j, wl := range strings.Split(wrapped, "\n") {
				if j > 0 {
					result.WriteString(bar)
				} else {
					result.WriteString(bar)
				}
				result.WriteString(mdBlockquoteText.Render(wl))
				result.WriteByte('\n')
			}
			i++
			continue
		}

		// GFM table: a "| a | b |" row immediately followed by a "|---|---|" separator.
		if isTableRow(line) && i+1 < len(lines) && isTableSeparatorRow(lines[i+1]) {
			header := splitTableRow(line)
			aligns := parseTableAligns(lines[i+1])
			j := i + 2
			var rows [][]string
			for j < len(lines) && isTableRow(lines[j]) {
				rows = append(rows, splitTableRow(lines[j]))
				j++
			}
			result.WriteString(renderMarkdownTable(header, aligns, rows, width))
			result.WriteByte('\n')
			i = j
			continue
		}

		// Unordered list
		if bullet, text := parseUnorderedList(line); bullet != "" {
			rendered := renderInlineFormatting(text, width)
			prefix := "  " + mdBulletStyle.Render(bullet) + " "
			wrapped := mdWordWrap(rendered, width-5)
			wrapLines := strings.Split(wrapped, "\n")
			result.WriteString(prefix + wrapLines[0])
			result.WriteByte('\n')
			for _, wl := range wrapLines[1:] {
				result.WriteString("    " + wl)
				result.WriteByte('\n')
			}
			i++
			continue
		}

		// Ordered list
		if num, text := parseOrderedList(line); num != "" {
			rendered := renderInlineFormatting(text, width)
			prefix := "  " + num + " "
			prefixW := 2 + runewidth.StringWidth(num) + 1
			wrapped := mdWordWrap(rendered, width-prefixW)
			wrapLines := strings.Split(wrapped, "\n")
			result.WriteString(prefix + wrapLines[0])
			result.WriteByte('\n')
			indent := strings.Repeat(" ", prefixW)
			for _, wl := range wrapLines[1:] {
				result.WriteString(indent + wl)
				result.WriteByte('\n')
			}
			i++
			continue
		}

		// Regular paragraph line
		if trimmed == "" {
			result.WriteByte('\n')
		} else {
			rendered := renderInlineFormatting(line, width)
			wrapped := mdWordWrap(rendered, width)
			result.WriteString(wrapped)
			result.WriteByte('\n')
		}
		i++
	}

	return strings.TrimRight(result.String(), "\n")
}

// codeBlock holds a parsed fenced code block.
type codeBlock struct {
	lang string
	code string
}

// extractCodeBlock reads a fenced code block starting at index i.
// Returns the block and the index of the closing ``` line.
func extractCodeBlock(lines []string, start int) (codeBlock, int) {
	opener := strings.TrimSpace(lines[start])
	lang := strings.TrimPrefix(opener, "```")
	lang = strings.TrimSpace(lang)

	var code strings.Builder
	end := start + 1
	for end < len(lines) {
		if strings.TrimSpace(lines[end]) == "```" {
			break
		}
		if code.Len() > 0 {
			code.WriteByte('\n')
		}
		code.WriteString(lines[end])
		end++
	}
	// If we reached end of input without closing ```, end stays at len(lines)-1
	if end >= len(lines) {
		end = len(lines) - 1
	}
	return codeBlock{lang: lang, code: code.String()}, end
}

// renderCodeBlock renders a code block with a dim background, optional language label,
// and indentation.
func renderCodeBlock(lang, code string, width int) string {
	var b strings.Builder
	indent := "  "
	innerWidth := width - 4
	if innerWidth < 10 {
		innerWidth = width
	}

	if lang != "" {
		label := mdCodeLabelStyle.Render(" " + lang + " ")
		b.WriteString(indent + label)
		b.WriteByte('\n')
	}

	for _, line := range strings.Split(code, "\n") {
		// Pad line to inner width for consistent background
		visW := runewidth.StringWidth(line)
		pad := ""
		if visW < innerWidth {
			pad = strings.Repeat(" ", innerWidth-visW)
		}
		styled := mdCodeBlockStyle.Render(" " + line + pad + " ")
		b.WriteString(indent + styled)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// isHorizontalRule detects ---, ***, ___ (3 or more of same char, optional spaces).
func isHorizontalRule(trimmed string) bool {
	if len(trimmed) < 3 {
		return false
	}
	cleaned := strings.ReplaceAll(trimmed, " ", "")
	if len(cleaned) < 3 {
		return false
	}
	ch := cleaned[0]
	if ch != '-' && ch != '*' && ch != '_' {
		return false
	}
	for _, c := range cleaned {
		if byte(c) != ch {
			return false
		}
	}
	return true
}

// parseHeader returns the header level (1-6) and text, or 0 if not a header.
func parseHeader(line string) (int, string) {
	trimmed := strings.TrimSpace(line)
	level := 0
	for _, c := range trimmed {
		if c == '#' {
			level++
		} else {
			break
		}
	}
	if level == 0 || level > 6 {
		return 0, ""
	}
	if len(trimmed) <= level {
		return level, ""
	}
	if trimmed[level] != ' ' {
		return 0, ""
	}
	return level, strings.TrimSpace(trimmed[level+1:])
}

// parseUnorderedList detects lines like "- item", "* item", "+ item"
// with optional leading whitespace.
func parseUnorderedList(line string) (string, string) {
	trimmed := strings.TrimLeft(line, " \t")
	for _, prefix := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(trimmed, prefix) {
			return string(prefix[0]), strings.TrimSpace(trimmed[2:])
		}
	}
	return "", ""
}

// parseOrderedList detects lines like "1. item", "12. item".
func parseOrderedList(line string) (string, string) {
	trimmed := strings.TrimLeft(line, " \t")
	dotIdx := strings.Index(trimmed, ". ")
	if dotIdx <= 0 || dotIdx > 4 {
		return "", ""
	}
	numPart := trimmed[:dotIdx]
	for _, c := range numPart {
		if c < '0' || c > '9' {
			return "", ""
		}
	}
	return numPart + ".", strings.TrimSpace(trimmed[dotIdx+2:])
}

// isTableRow reports whether line looks like a GFM table row (contains a pipe).
func isTableRow(line string) bool {
	return strings.Contains(strings.TrimSpace(line), "|")
}

// isTableSeparatorRow reports whether line is a GFM table header separator,
// e.g. "|---|---|", "| :-- | --: | :-: |".
func isTableSeparatorRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.Contains(trimmed, "-") {
		return false
	}
	cells := splitTableRow(trimmed)
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		if !reTableSepCol.MatchString(strings.TrimSpace(c)) {
			return false
		}
	}
	return true
}

// splitTableRow splits a "| a | b |" row into trimmed cells, tolerating rows
// without leading/trailing pipes.
func splitTableRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

// tableAlign values for a column's text alignment.
const (
	tableAlignLeft = iota
	tableAlignRight
	tableAlignCenter
)

// parseTableAligns reads column alignment from a separator row's colon placement.
func parseTableAligns(sepLine string) []int {
	cells := splitTableRow(sepLine)
	aligns := make([]int, len(cells))
	for i, c := range cells {
		left := strings.HasPrefix(c, ":")
		right := strings.HasSuffix(c, ":")
		switch {
		case left && right:
			aligns[i] = tableAlignCenter
		case right:
			aligns[i] = tableAlignRight
		default:
			aligns[i] = tableAlignLeft
		}
	}
	return aligns
}

// renderMarkdownTable renders a GFM table as a box-drawn table, wrapping and
// shrinking columns as needed to fit width.
func renderMarkdownTable(header []string, aligns []int, rows [][]string, width int) string {
	cols := len(header)
	if cols == 0 {
		return ""
	}
	for len(aligns) < cols {
		aligns = append(aligns, tableAlignLeft)
	}
	normalize := func(cells []string) []string {
		out := make([]string, cols)
		copy(out, cells)
		return out
	}
	for i, r := range rows {
		rows[i] = normalize(r)
	}

	styledHeader := make([]string, cols)
	styledRows := make([][]string, len(rows))
	naturalW := make([]int, cols)
	for i, h := range header {
		styledHeader[i] = mdBoldStyle.Render(renderInlineFormatting(h, width))
		if w := visibleWidth(styledHeader[i]); w > naturalW[i] {
			naturalW[i] = w
		}
	}
	for ri, row := range rows {
		styledRows[ri] = make([]string, cols)
		for ci, cell := range row {
			styled := renderInlineFormatting(cell, width)
			styledRows[ri][ci] = styled
			if w := visibleWidth(styled); w > naturalW[ci] {
				naturalW[ci] = w
			}
		}
	}

	const maxColW = 40
	colW := make([]int, cols)
	total := 0
	for i, w := range naturalW {
		if w > maxColW {
			w = maxColW
		}
		if w < 3 {
			w = 3
		}
		colW[i] = w
		total += w
	}

	overhead := (cols + 1) + cols*2 // vertical bars + one space of padding either side
	if avail := width - overhead; avail > 0 && total > avail {
		scale := float64(avail) / float64(total)
		for i := range colW {
			w := int(float64(colW[i]) * scale)
			if w < 4 {
				w = 4
			}
			colW[i] = w
		}
	}

	hBar := func(left, mid, right string) string {
		var b strings.Builder
		b.WriteString(left)
		for i, w := range colW {
			b.WriteString(strings.Repeat("─", w+2))
			if i < cols-1 {
				b.WriteString(mid)
			}
		}
		b.WriteString(right)
		return mdHRStyle.Render(b.String())
	}
	bar := mdHRStyle.Render("│")

	renderRow := func(cells []string) string {
		wrappedCols := make([][]string, cols)
		maxLines := 1
		for i, c := range cells {
			lines := strings.Split(mdWordWrap(c, colW[i]), "\n")
			wrappedCols[i] = lines
			if len(lines) > maxLines {
				maxLines = len(lines)
			}
		}
		var rb strings.Builder
		for ln := 0; ln < maxLines; ln++ {
			rb.WriteString(bar)
			for i := 0; i < cols; i++ {
				var cellLine string
				if ln < len(wrappedCols[i]) {
					cellLine = wrappedCols[i][ln]
				}
				pad := colW[i] - visibleWidth(cellLine)
				if pad < 0 {
					pad = 0
				}
				var aligned string
				switch aligns[i] {
				case tableAlignRight:
					aligned = strings.Repeat(" ", pad) + cellLine
				case tableAlignCenter:
					lp := pad / 2
					aligned = strings.Repeat(" ", lp) + cellLine + strings.Repeat(" ", pad-lp)
				default:
					aligned = cellLine + strings.Repeat(" ", pad)
				}
				rb.WriteString(" " + aligned + " " + bar)
			}
			rb.WriteByte('\n')
		}
		return strings.TrimRight(rb.String(), "\n")
	}

	var out strings.Builder
	out.WriteString(hBar("┌", "┬", "┐"))
	out.WriteByte('\n')
	out.WriteString(renderRow(styledHeader))
	out.WriteByte('\n')
	out.WriteString(hBar("├", "┼", "┤"))
	out.WriteByte('\n')
	for _, row := range styledRows {
		out.WriteString(renderRow(row))
		out.WriteByte('\n')
	}
	out.WriteString(hBar("└", "┴", "┘"))
	return out.String()
}

func protectInlineCode(text string, render func(string) string) (string, func(string) string) {
	var replacements []string
	protected := reInlineCode.ReplaceAllStringFunc(text, func(m string) string {
		parts := reInlineCode.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		placeholder := fmt.Sprintf("\x00HAWK_INLINE_CODE_%d\x00", len(replacements))
		replacements = append(replacements, render(parts[1]))
		return placeholder
	})
	restore := func(s string) string {
		for i, repl := range replacements {
			s = strings.ReplaceAll(s, fmt.Sprintf("\x00HAWK_INLINE_CODE_%d\x00", i), repl)
		}
		return s
	}
	return protected, restore
}

// renderInlineFormatting applies inline markdown (bold, italic, inline code, links)
// to a line of text.
func renderInlineFormatting(text string, width int) string {
	protected, restore := protectInlineCode(text, func(code string) string {
		return mdInlineCodeStyle.Render(code)
	})
	text = protected

	// Process links first (they contain brackets that could interfere)
	text = reMDLink.ReplaceAllStringFunc(text, func(m string) string {
		parts := reMDLink.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		return mdLinkTextStyle.Render(parts[1]) + " " + mdLinkURLStyle.Render("("+parts[2]+")")
	})

	// Bold
	text = reMDBold.ReplaceAllStringFunc(text, func(m string) string {
		parts := reMDBold.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		return mdBoldStyle.Render(parts[1])
	})

	// Italic (single *)
	text = reMDItalic.ReplaceAllStringFunc(text, func(m string) string {
		parts := reMDItalic.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		// Preserve surrounding characters that were matched by the boundary assertions
		prefix := ""
		suffix := ""
		if len(m) > 0 && m[0] != '*' {
			prefix = string(m[0])
		}
		if len(m) > 0 && m[len(m)-1] != '*' {
			suffix = string(m[len(m)-1])
		}
		return prefix + mdItalicStyle.Render(parts[1]) + suffix
	})

	return restore(text)
}

// mdWordWrap wraps text to the specified width, respecting word boundaries.
// It handles text that may contain ANSI escape codes by measuring visible width.
func mdWordWrap(text string, width int) string {
	if width < 10 {
		width = 80
	}
	// If the visible width fits, return as-is
	if visibleWidth(text) <= width {
		return text
	}

	var result strings.Builder
	words := strings.Fields(text)
	curWidth := 0

	for _, word := range words {
		wordW := visibleWidth(word)
		if curWidth > 0 && curWidth+1+wordW > width {
			result.WriteByte('\n')
			result.WriteString(word)
			curWidth = wordW
		} else if curWidth > 0 {
			result.WriteByte(' ')
			result.WriteString(word)
			curWidth += 1 + wordW
		} else {
			result.WriteString(word)
			curWidth = wordW
		}
	}
	return result.String()
}

// visibleWidth returns the display width of a string, skipping ANSI escape
// codes. Implemented as a direct scan (no regex, no allocation) because it is
// called per word on the streaming render path.
func visibleWidth(s string) int {
	w := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			// CSI sequence: skip until the final byte (an ASCII letter).
			j := i + 2
			for j < len(s) {
				c := s[j]
				j++
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
					break
				}
			}
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		w += runewidth.RuneWidth(r)
		i += size
	}
	return w
}

// reAnsi matches ANSI escape sequences for stripping in width calculations.
var reAnsi = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripAnsi removes ANSI escape sequences from a string.
func stripAnsi(s string) string {
	return reAnsi.ReplaceAllString(s, "")
}
