package cmd

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// ---------------------------------------------------------------------------
// Struct-based MarkdownRenderer (glamour/glow-inspired, stdlib-only ANSI)
//
// The legacy lipgloss-based renderer and the shared helpers it defines
// (visibleWidth, stripAnsi, reAnsi, isHorizontalRule, parseHeader) live in
// markdown.go.
// ---------------------------------------------------------------------------

// MarkdownTheme defines ANSI escape codes for styling markdown elements.
type MarkdownTheme struct {
	Heading        string
	Bold           string
	Italic         string
	Code           string
	CodeBlock      string
	Link           string
	ListBullet     string
	BlockQuote     string
	HorizontalRule string
	Reset          string
}

// DefaultTheme returns a visually appealing terminal color theme.
func DefaultTheme() *MarkdownTheme {
	return &MarkdownTheme{
		Heading:        "\x1b[1;36m",        // bold cyan
		Bold:           "\x1b[1m",           // bold
		Italic:         "\x1b[3m",           // italic
		Code:           "\x1b[48;5;236;37m", // dark bg + cyan fg
		CodeBlock:      "\x1b[48;5;236m",    // dark background
		Link:           "\x1b[4;36m",        // underline cyan
		ListBullet:     "\x1b[36m",          // cyan
		BlockQuote:     "\x1b[3;90m",        // italic dim
		HorizontalRule: "\x1b[90m",          // dim
		Reset:          "\x1b[0m",           // reset all
	}
}

// MarkdownRenderer renders markdown text to styled ANSI terminal output.
type MarkdownRenderer struct {
	Width           int
	Theme           *MarkdownTheme
	SyntaxHighlight bool
}

// NewMarkdownRenderer creates a new renderer with the given terminal width.
func NewMarkdownRenderer(width int) *MarkdownRenderer {
	if width <= 0 {
		width = 80
	}
	return &MarkdownRenderer{
		Width:           width,
		Theme:           DefaultTheme(),
		SyntaxHighlight: true,
	}
}

// Compiled regex patterns for the struct-based renderer.
var (
	reRendererBold      = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reRendererItalic    = regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)
	reRendererCode      = regexp.MustCompile("`([^`]+)`")
	reRendererLink      = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reRendererOrderedLi = regexp.MustCompile(`^(\s*)(\d+)\.\s+(.*)$`)
	reRendererTableRow  = regexp.MustCompile(`^\|(.+)\|$`)
	reRendererTableSep  = regexp.MustCompile(`^\|[\s:]*[-]+[\s:]*`)
	reHighlightKeyword  = regexp.MustCompile(`\b(func|var|const|type|struct|interface|map|chan|go|defer|return|if|else|for|range|switch|case|default|break|continue|select|package|import|nil|true|false|def|class|self|from|import|as|with|yield|lambda|try|except|finally|raise|assert|pass|del|global|nonlocal|async|await|function|let|const|var|new|this|typeof|instanceof|export|import|from|async|await|fn|pub|mod|use|impl|trait|enum|match|loop|move|mut|ref|where|unsafe|extern|crate|macro|then|fi|do|done|elif|esac)\b`)
	reHighlightString   = regexp.MustCompile(`("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|` + "`" + `[^` + "`" + `]*` + "`" + `)`)
	reHighlightComment  = regexp.MustCompile(`(//.*$|#.*$|/\*.*?\*/)`)
	reHighlightNumber   = regexp.MustCompile(`\b(\d+\.?\d*)\b`)
)

// Render converts a markdown string to ANSI-styled terminal output.
func (r *MarkdownRenderer) Render(markdown string) string {
	if markdown == "" {
		return ""
	}

	theme := r.Theme
	if theme == nil {
		theme = DefaultTheme()
	}
	width := r.Width
	if width <= 0 {
		width = 80
	}

	lines := strings.Split(markdown, "\n")
	var result strings.Builder
	i := 0

	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Fenced code block
		if strings.HasPrefix(trimmed, "```") {
			lang := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			var codeLines []string
			i++
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == "```" {
					i++
					break
				}
				codeLines = append(codeLines, lines[i])
				i++
			}
			codeContent := strings.Join(codeLines, "\n")
			result.WriteString(r.renderFencedCodeBlock(codeContent, lang, width))
			result.WriteByte('\n')
			continue
		}

		// Table detection
		if reRendererTableRow.MatchString(trimmed) {
			var tableLines []string
			for i < len(lines) && reRendererTableRow.MatchString(strings.TrimSpace(lines[i])) {
				tableLines = append(tableLines, strings.TrimSpace(lines[i]))
				i++
			}
			result.WriteString(r.renderTableFromLines(tableLines, width))
			result.WriteByte('\n')
			continue
		}

		// Horizontal rule
		if isHorizontalRule(trimmed) {
			result.WriteString(theme.HorizontalRule)
			ruleWidth := width
			if ruleWidth > 80 {
				ruleWidth = 80
			}
			result.WriteString(strings.Repeat("─", ruleWidth))
			result.WriteString(theme.Reset)
			result.WriteByte('\n')
			i++
			continue
		}

		// Headers
		if level, text := parseHeader(line); level > 0 {
			rendered := r.renderInline(text)
			if level == 1 {
				result.WriteString(theme.Heading)
				result.WriteString("\x1b[4m") // underline for h1
				result.WriteString(rendered)
				result.WriteString(theme.Reset)
			} else {
				result.WriteString(theme.Heading)
				result.WriteString(rendered)
				result.WriteString(theme.Reset)
			}
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
			rendered := r.renderInline(text)
			wrapped := WrapText(rendered, width-4)
			for _, wl := range strings.Split(wrapped, "\n") {
				result.WriteString(theme.BlockQuote)
				result.WriteString("│ ")
				result.WriteString(wl)
				result.WriteString(theme.Reset)
				result.WriteByte('\n')
			}
			i++
			continue
		}

		// Unordered list
		if bullet, text := r.parseListItem(line); bullet != "" {
			indent := r.countLeadingSpaces(line) / 2
			indentStr := strings.Repeat("  ", indent)
			rendered := r.renderInline(text)
			wrapped := WrapText(rendered, width-len(indentStr)-4)
			wrapLines := strings.Split(wrapped, "\n")
			result.WriteString(indentStr)
			result.WriteString("  ")
			result.WriteString(theme.ListBullet)
			result.WriteString(bullet)
			result.WriteString(theme.Reset)
			result.WriteString(" ")
			result.WriteString(wrapLines[0])
			result.WriteByte('\n')
			contIndent := indentStr + "    "
			for _, wl := range wrapLines[1:] {
				result.WriteString(contIndent)
				result.WriteString(wl)
				result.WriteByte('\n')
			}
			i++
			continue
		}

		// Ordered list
		if m := reRendererOrderedLi.FindStringSubmatch(line); m != nil {
			indentStr := m[1]
			num := m[2]
			text := m[3]
			rendered := r.renderInline(text)
			prefix := num + "."
			wrapped := WrapText(rendered, width-len(indentStr)-len(prefix)-3)
			wrapLines := strings.Split(wrapped, "\n")
			result.WriteString(indentStr)
			result.WriteString("  ")
			result.WriteString(prefix)
			result.WriteString(" ")
			result.WriteString(wrapLines[0])
			result.WriteByte('\n')
			contIndent := indentStr + strings.Repeat(" ", len(prefix)+3)
			for _, wl := range wrapLines[1:] {
				result.WriteString(contIndent)
				result.WriteString(wl)
				result.WriteByte('\n')
			}
			i++
			continue
		}

		// Empty line
		if trimmed == "" {
			result.WriteByte('\n')
			i++
			continue
		}

		// Regular paragraph
		rendered := r.renderInline(line)
		wrapped := WrapText(rendered, width)
		result.WriteString(wrapped)
		result.WriteByte('\n')
		i++
	}

	return strings.TrimRight(result.String(), "\n")
}

// renderInline applies inline formatting (bold, italic, code, links).
func (r *MarkdownRenderer) renderInline(text string) string {
	theme := r.Theme
	protected, restore := protectRendererInlineCode(text, func(code string) string {
		return theme.Code + code + theme.Reset
	})
	text = protected

	// Links
	text = reRendererLink.ReplaceAllStringFunc(text, func(m string) string {
		parts := reRendererLink.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		return theme.Link + parts[1] + theme.Reset + " (" + parts[2] + ")"
	})

	// Bold
	text = reRendererBold.ReplaceAllStringFunc(text, func(m string) string {
		parts := reRendererBold.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		return theme.Bold + parts[1] + theme.Reset
	})

	// Italic
	text = reRendererItalic.ReplaceAllStringFunc(text, func(m string) string {
		parts := reRendererItalic.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		prefix := ""
		suffix := ""
		if len(m) > 0 && m[0] != '*' {
			prefix = string(m[0])
		}
		if len(m) > 0 && m[len(m)-1] != '*' {
			suffix = string(m[len(m)-1])
		}
		return prefix + theme.Italic + parts[1] + theme.Reset + suffix
	})

	return restore(text)
}

func protectRendererInlineCode(text string, render func(string) string) (string, func(string) string) {
	var replacements []string
	protected := reRendererCode.ReplaceAllStringFunc(text, func(m string) string {
		parts := reRendererCode.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		placeholder := fmt.Sprintf("\x00HAWK_R_INLINE_CODE_%d\x00", len(replacements))
		replacements = append(replacements, render(parts[1]))
		return placeholder
	})
	restore := func(s string) string {
		for i, repl := range replacements {
			s = strings.ReplaceAll(s, fmt.Sprintf("\x00HAWK_R_INLINE_CODE_%d\x00", i), repl)
		}
		return s
	}
	return protected, restore
}

// parseListItem detects unordered list items with various bullet markers.
func (r *MarkdownRenderer) parseListItem(line string) (string, string) {
	trimmed := strings.TrimLeft(line, " \t")
	for _, prefix := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(trimmed, prefix) {
			return "•", strings.TrimSpace(trimmed[2:])
		}
	}
	return "", ""
}

// countLeadingSpaces returns the number of leading space characters.
func (r *MarkdownRenderer) countLeadingSpaces(line string) int {
	count := 0
	for _, ch := range line {
		if ch == ' ' {
			count++
		} else if ch == '\t' {
			count += 2
		} else {
			break
		}
	}
	return count
}

// renderFencedCodeBlock renders a code block with optional syntax highlighting.
func (r *MarkdownRenderer) renderFencedCodeBlock(code, lang string, width int) string {
	theme := r.Theme
	var b strings.Builder
	innerWidth := width - 6
	if innerWidth < 20 {
		innerWidth = width - 2
	}

	// Language label
	if lang != "" {
		b.WriteString("  \x1b[90m")
		b.WriteString(" " + lang + " ")
		b.WriteString(theme.Reset)
		b.WriteByte('\n')
	}

	// Optionally syntax highlight
	highlighted := code
	if r.SyntaxHighlight && lang != "" {
		highlighted = HighlightCode(code, lang)
	}

	for _, line := range strings.Split(highlighted, "\n") {
		b.WriteString("  ")
		b.WriteString(theme.CodeBlock)
		b.WriteString(" ")
		// Pad to innerWidth for consistent background
		plain := StripANSI(line)
		visW := 0
		for _, r := range plain {
			visW += runeWidth(r)
		}
		b.WriteString(line)
		if visW < innerWidth {
			b.WriteString(strings.Repeat(" ", innerWidth-visW))
		}
		b.WriteString(" ")
		b.WriteString(theme.Reset)
		b.WriteByte('\n')
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderTableFromLines parses markdown table lines and renders with box-drawing.
func (r *MarkdownRenderer) renderTableFromLines(tableLines []string, width int) string {
	if len(tableLines) == 0 {
		return ""
	}

	// Parse rows
	var rows [][]string
	for _, line := range tableLines {
		// Skip separator lines (e.g., |---|---|)
		if reRendererTableSep.MatchString(line) {
			continue
		}
		cells := parseTableRow(line)
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	}

	if len(rows) == 0 {
		return ""
	}

	return RenderTable(rows)
}

// parseTableRow splits a table row like "|a|b|c|" into cells.
func parseTableRow(line string) []string {
	// Remove leading/trailing |
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

// HighlightCode performs regex-based syntax highlighting for common languages.
// Supports Go, Python, JavaScript/TypeScript, Rust, and Bash.
func HighlightCode(code string, language string) string {
	lang := strings.ToLower(language)

	// Only highlight supported languages
	switch lang {
	case "go", "golang", "python", "py", "javascript", "js", "typescript", "ts", "rust", "rs", "bash", "sh", "shell", "zsh":
		// proceed
	default:
		return code
	}

	// ANSI color codes for syntax elements
	const (
		keywordColor = "\x1b[38;5;198m" // magenta/pink for keywords
		stringColor  = "\x1b[38;5;113m" // green for strings
		commentColor = "\x1b[38;5;242m" // gray for comments
		numberColor  = "\x1b[38;5;141m" // purple for numbers
		resetColor   = "\x1b[0m"
	)

	// Process line by line to handle comments correctly
	lines := strings.Split(code, "\n")
	var result []string
	for _, line := range lines {
		highlighted := line

		// Comments first (they override everything else on the line)
		if loc := reHighlightComment.FindStringIndex(highlighted); loc != nil {
			before := highlighted[:loc[0]]
			comment := highlighted[loc[0]:loc[1]]
			after := highlighted[loc[1]:]
			before = highlightNonComment(before, keywordColor, stringColor, numberColor, resetColor)
			highlighted = before + commentColor + comment + resetColor + after
		} else {
			highlighted = highlightNonComment(highlighted, keywordColor, stringColor, numberColor, resetColor)
		}

		result = append(result, highlighted)
	}

	return strings.Join(result, "\n")
}

// highlightNonComment highlights keywords, strings, and numbers in non-comment text.
func highlightNonComment(text, keywordColor, stringColor, numberColor, resetColor string) string {
	// Strings first (so keywords inside strings are not highlighted)
	text = reHighlightString.ReplaceAllStringFunc(text, func(m string) string {
		return stringColor + m + resetColor
	})

	// Keywords (only highlight if not inside a string - simplified approach)
	text = reHighlightKeyword.ReplaceAllStringFunc(text, func(m string) string {
		return keywordColor + m + resetColor
	})

	// Numbers
	text = reHighlightNumber.ReplaceAllStringFunc(text, func(m string) string {
		// Don't highlight numbers that are part of ANSI escape sequences
		return numberColor + m + resetColor
	})

	return text
}

// WrapText performs word-wrapping at the specified width boundary.
// It respects ANSI escape codes by measuring only visible characters.
func WrapText(text string, width int) string {
	if width <= 0 {
		width = 80
	}
	if text == "" {
		return ""
	}

	// Quick check: if text already fits, return as-is
	plainLen := len(StripANSI(text))
	if plainLen <= width {
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

// RenderTable renders a table with box-drawing characters.
// The first row is treated as the header. Column widths are auto-calculated.
func RenderTable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}

	// Determine number of columns
	numCols := 0
	for _, row := range rows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}
	if numCols == 0 {
		return ""
	}

	// Calculate column widths
	colWidths := make([]int, numCols)
	for _, row := range rows {
		for i, cell := range row {
			if i < numCols {
				w := len(StripANSI(cell))
				if w > colWidths[i] {
					colWidths[i] = w
				}
			}
		}
	}

	// Ensure minimum width of 3
	for i := range colWidths {
		if colWidths[i] < 3 {
			colWidths[i] = 3
		}
	}

	var b strings.Builder

	// Top border: ┌───┬───┐
	b.WriteString("┌")
	for i, w := range colWidths {
		b.WriteString(strings.Repeat("─", w+2))
		if i < numCols-1 {
			b.WriteString("┬")
		}
	}
	b.WriteString("┐\n")

	for rowIdx, row := range rows {
		// Row content: │ cell │ cell │
		b.WriteString("│")
		for i := 0; i < numCols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			plainCell := StripANSI(cell)
			pad := colWidths[i] - len(plainCell)
			if pad < 0 {
				pad = 0
			}
			b.WriteString(" ")
			b.WriteString(cell)
			b.WriteString(strings.Repeat(" ", pad))
			b.WriteString(" │")
		}
		b.WriteString("\n")

		// After header row: ├───┼───┤
		if rowIdx == 0 && len(rows) > 1 {
			b.WriteString("├")
			for i, w := range colWidths {
				b.WriteString(strings.Repeat("─", w+2))
				if i < numCols-1 {
					b.WriteString("┼")
				}
			}
			b.WriteString("┤\n")
		}
	}

	// Bottom border: └───┴───┘
	b.WriteString("└")
	for i, w := range colWidths {
		b.WriteString(strings.Repeat("─", w+2))
		if i < numCols-1 {
			b.WriteString("┴")
		}
	}
	b.WriteString("┘")

	return b.String()
}

// StripANSI removes all ANSI escape codes from a string (for plain output).
func StripANSI(text string) string {
	return reAnsi.ReplaceAllString(text, "")
}

// RenderStreaming takes a channel of raw markdown chunks and returns a channel
// of rendered chunks. It buffers partial markdown elements until they can be
// completely rendered.
func RenderStreaming(ch <-chan string) <-chan string {
	out := make(chan string, 16)

	go func() {
		defer close(out)

		renderer := NewMarkdownRenderer(80)
		var buffer strings.Builder
		var lastRendered string

		for chunk := range ch {
			buffer.WriteString(chunk)
			current := buffer.String()

			// Check if we have incomplete elements that need buffering
			if hasIncompleteElement(current) {
				// Try to render what we can
				safe := findSafeRenderPoint(current)
				if safe == "" {
					continue // buffer more
				}
				rendered := renderer.Render(safe)
				if rendered != lastRendered {
					// Send only the new part
					diff := computeStreamDiff(lastRendered, rendered)
					if diff != "" {
						out <- diff
					}
					lastRendered = rendered
				}
			} else {
				rendered := renderer.Render(current)
				if rendered != lastRendered {
					diff := computeStreamDiff(lastRendered, rendered)
					if diff != "" {
						out <- diff
					}
					lastRendered = rendered
				}
			}
		}

		// Final flush
		final := renderer.Render(buffer.String())
		if final != lastRendered {
			diff := computeStreamDiff(lastRendered, final)
			if diff != "" {
				out <- diff
			}
		}
	}()

	return out
}

// hasIncompleteElement checks for partial markdown elements that should be buffered.
func hasIncompleteElement(s string) bool {
	// Unclosed bold
	count := strings.Count(s, "**")
	if count%2 != 0 {
		return true
	}

	// Unclosed inline code
	inCode := false
	for _, ch := range s {
		if ch == '`' {
			inCode = !inCode
		}
	}
	if inCode {
		return true
	}

	// Unclosed fenced code block
	fenceCount := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenceCount++
		}
	}
	return fenceCount%2 != 0
}

// findSafeRenderPoint finds the longest prefix that can be safely rendered.
func findSafeRenderPoint(s string) string {
	// Try to find the last complete line
	lastNewline := strings.LastIndex(s, "\n")
	if lastNewline <= 0 {
		return ""
	}

	candidate := s[:lastNewline]
	// Verify this candidate doesn't have incomplete elements
	if !hasIncompleteElement(candidate) {
		return candidate
	}

	// Try second-to-last newline
	prevNewline := strings.LastIndex(candidate, "\n")
	if prevNewline > 0 {
		candidate = s[:prevNewline]
		if !hasIncompleteElement(candidate) {
			return candidate
		}
	}

	return ""
}

// computeStreamDiff computes what new content to emit given old and new rendered text.
func computeStreamDiff(old, new string) string {
	if old == "" {
		return new
	}
	if strings.HasPrefix(new, old) {
		return new[len(old):]
	}
	// Content changed (re-rendering), send full new content with clear
	return "\r\x1b[J" + new
}

// runeWidth returns the display width of a single rune.
func runeWidth(r rune) int {
	if r == '\t' {
		return 4
	}
	if !unicode.IsPrint(r) {
		return 0
	}
	// Use East Asian width awareness
	if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hangul, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hiragana, r) {
		return 2
	}
	return 1
}
