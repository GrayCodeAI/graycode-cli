package cmd

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Markdown rendering styles using the project's existing color palette.
var (
	mdHeaderStyle     = lipgloss.NewStyle().Foreground(tealColor).Bold(true)
	mdBoldStyle       = lipgloss.NewStyle().Bold(true)
	mdItalicStyle     = lipgloss.NewStyle().Italic(true)
	mdInlineCodeStyle = lipgloss.NewStyle().Background(lipgloss.Color("#2A2A3A")).Foreground(lipgloss.Color("#E6E6E6"))
	mdCodeBlockStyle  = lipgloss.NewStyle().Background(lipgloss.Color("#2A2A3A"))
	mdCodeLabelStyle  = lipgloss.NewStyle().Foreground(dimColor).Background(lipgloss.Color("#2A2A3A"))
	mdLinkTextStyle   = lipgloss.NewStyle().Foreground(tealColor)
	mdLinkURLStyle    = lipgloss.NewStyle().Foreground(dimColor)
	mdBlockquoteBar   = lipgloss.NewStyle().Foreground(dimColor)
	mdBlockquoteText  = lipgloss.NewStyle().Foreground(dimColor)
	mdHRStyle         = lipgloss.NewStyle().Foreground(dimColor)
	mdBulletStyle     = lipgloss.NewStyle().Foreground(tealColor)
)

// Inline regex patterns, compiled once.
var (
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reMDBold     = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reMDItalic   = regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)
	reMDLink     = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
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

// renderInlineFormatting applies inline markdown (bold, italic, inline code, links)
// to a line of text.
func renderInlineFormatting(text string, width int) string {
	// Process links first (they contain brackets that could interfere)
	text = reMDLink.ReplaceAllStringFunc(text, func(m string) string {
		parts := reMDLink.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		return mdLinkTextStyle.Render(parts[1]) + " " + mdLinkURLStyle.Render("("+parts[2]+")")
	})

	// Inline code (before bold/italic so backtick content is not further parsed)
	text = reInlineCode.ReplaceAllStringFunc(text, func(m string) string {
		parts := reInlineCode.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		return mdInlineCodeStyle.Render(parts[1])
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

	return text
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

// visibleWidth returns the display width of a string, stripping ANSI escape codes.
func visibleWidth(s string) int {
	return runewidth.StringWidth(stripAnsi(s))
}

// reAnsi matches ANSI escape sequences for stripping in width calculations.
var reAnsi = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripAnsi removes ANSI escape sequences from a string.
func stripAnsi(s string) string {
	return reAnsi.ReplaceAllString(s, "")
}
