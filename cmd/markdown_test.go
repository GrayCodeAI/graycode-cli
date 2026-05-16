package cmd

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Legacy renderMarkdown tests (existing)
// ---------------------------------------------------------------------------

func TestRenderMarkdownHeaders(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"# Hello", "Hello"},
		{"## Sub Header", "Sub Header"},
		{"### Third Level", "Third Level"},
	}
	for _, tt := range tests {
		out := renderMarkdown(tt.input, 80)
		plain := stripAnsi(out)
		if !strings.Contains(plain, tt.want) {
			t.Errorf("renderMarkdown(%q): expected %q in output, got %q", tt.input, tt.want, plain)
		}
	}
}

func TestRenderMarkdownBold(t *testing.T) {
	out := renderMarkdown("This is **bold** text", 80)
	plain := stripAnsi(out)
	if !strings.Contains(plain, "bold") {
		t.Errorf("expected bold text in output, got %q", plain)
	}
	// The ** markers should be removed
	if strings.Contains(plain, "**") {
		t.Errorf("bold markers should be removed, got %q", plain)
	}
}

func TestRenderMarkdownItalic(t *testing.T) {
	out := renderMarkdown("This is *italic* text", 80)
	plain := stripAnsi(out)
	if !strings.Contains(plain, "italic") {
		t.Errorf("expected italic text in output, got %q", plain)
	}
}

func TestRenderMarkdownInlineCode(t *testing.T) {
	out := renderMarkdown("Use `go build` here", 80)
	plain := stripAnsi(out)
	if !strings.Contains(plain, "go build") {
		t.Errorf("expected inline code in output, got %q", plain)
	}
	// The backtick markers should be removed
	if strings.Contains(plain, "`") {
		t.Errorf("backtick markers should be removed, got %q", plain)
	}
}

func TestRenderMarkdownCodeBlock(t *testing.T) {
	input := "```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```"
	out := renderMarkdown(input, 80)
	plain := stripAnsi(out)
	if !strings.Contains(plain, "go") {
		t.Errorf("expected language label in code block, got %q", plain)
	}
	if !strings.Contains(plain, "func main()") {
		t.Errorf("expected code content in code block, got %q", plain)
	}
	if !strings.Contains(plain, "fmt.Println") {
		t.Errorf("expected code content preserved in code block, got %q", plain)
	}
}

func TestRenderMarkdownCodeBlockPreservesNewlines(t *testing.T) {
	input := "```\nline1\nline2\nline3\n```"
	out := renderMarkdown(input, 80)
	plain := stripAnsi(out)
	if !strings.Contains(plain, "line1") || !strings.Contains(plain, "line2") || !strings.Contains(plain, "line3") {
		t.Errorf("code block should preserve all lines, got %q", plain)
	}
	// Each line should be on its own line (has newlines between them)
	lines := strings.Split(plain, "\n")
	foundLines := 0
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "line") {
			foundLines++
		}
	}
	if foundLines < 3 {
		t.Errorf("expected 3 code lines on separate lines, found %d", foundLines)
	}
}

func TestRenderMarkdownUnorderedList(t *testing.T) {
	input := "- first item\n- second item\n* third item"
	out := renderMarkdown(input, 80)
	plain := stripAnsi(out)
	if !strings.Contains(plain, "first item") {
		t.Errorf("expected list items in output, got %q", plain)
	}
	if !strings.Contains(plain, "second item") {
		t.Errorf("expected list items in output, got %q", plain)
	}
	if !strings.Contains(plain, "third item") {
		t.Errorf("expected list items in output, got %q", plain)
	}
}

func TestRenderMarkdownOrderedList(t *testing.T) {
	input := "1. first\n2. second\n3. third"
	out := renderMarkdown(input, 80)
	plain := stripAnsi(out)
	if !strings.Contains(plain, "1.") {
		t.Errorf("expected ordered list numbers in output, got %q", plain)
	}
	if !strings.Contains(plain, "first") || !strings.Contains(plain, "second") || !strings.Contains(plain, "third") {
		t.Errorf("expected all ordered list items, got %q", plain)
	}
}

func TestRenderMarkdownLinks(t *testing.T) {
	input := "Visit [Hawk](https://example.com) for info"
	out := renderMarkdown(input, 80)
	plain := stripAnsi(out)
	if !strings.Contains(plain, "Hawk") {
		t.Errorf("expected link text in output, got %q", plain)
	}
	if !strings.Contains(plain, "https://example.com") {
		t.Errorf("expected link URL in output, got %q", plain)
	}
}

func TestRenderMarkdownBlockquote(t *testing.T) {
	input := "> This is a quote"
	out := renderMarkdown(input, 80)
	plain := stripAnsi(out)
	if !strings.Contains(plain, "This is a quote") {
		t.Errorf("expected blockquote text in output, got %q", plain)
	}
	// Should have bar character
	if !strings.Contains(plain, "│") {
		t.Errorf("expected blockquote bar in output, got %q", plain)
	}
}

func TestRenderMarkdownHorizontalRule(t *testing.T) {
	tests := []string{"---", "***", "___", "- - -"}
	for _, input := range tests {
		out := renderMarkdown(input, 40)
		plain := stripAnsi(out)
		if !strings.Contains(plain, "─") {
			t.Errorf("renderMarkdown(%q): expected horizontal rule, got %q", input, plain)
		}
	}
}

func TestRenderMarkdownWordWrap(t *testing.T) {
	long := "This is a very long line that should be wrapped at the specified width boundary so it does not overflow the terminal"
	out := renderMarkdown(long, 40)
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		w := visibleWidth(line)
		if w > 42 { // small tolerance for ANSI reset sequences
			t.Errorf("line exceeds width 40: width=%d line=%q", w, line)
		}
	}
}

func TestRenderMarkdownNestedFormatting(t *testing.T) {
	input := "- **bold in list**"
	out := renderMarkdown(input, 80)
	plain := stripAnsi(out)
	if !strings.Contains(plain, "bold in list") {
		t.Errorf("expected nested bold in list, got %q", plain)
	}
	// The ** markers should be removed (bold was processed)
	if strings.Contains(plain, "**") {
		t.Errorf("bold markers should be removed inside list, got %q", plain)
	}
}

func TestRenderMarkdownMixed(t *testing.T) {
	input := `# Title

Some **bold** text with ` + "`" + `inline code` + "`" + `.

- list item one
- list item two

> A blockquote

---

1. ordered one
2. ordered two

` + "```python\nprint('hello')\n```"

	out := renderMarkdown(input, 80)
	plain := stripAnsi(out)

	checks := []string{
		"Title", "bold", "inline code", "list item one",
		"A blockquote", "│", "─", "1.", "ordered one", "python", "print",
	}
	for _, want := range checks {
		if !strings.Contains(plain, want) {
			t.Errorf("mixed markdown: expected %q in output", want)
		}
	}
}

func TestParseHeader(t *testing.T) {
	tests := []struct {
		input     string
		wantLevel int
		wantText  string
	}{
		{"# Hello", 1, "Hello"},
		{"## Sub", 2, "Sub"},
		{"### Third", 3, "Third"},
		{"Not a header", 0, ""},
		{"#NoSpace", 0, ""},
		{"", 0, ""},
	}
	for _, tt := range tests {
		level, text := parseHeader(tt.input)
		if level != tt.wantLevel || text != tt.wantText {
			t.Errorf("parseHeader(%q) = (%d, %q), want (%d, %q)", tt.input, level, text, tt.wantLevel, tt.wantText)
		}
	}
}

func TestParseUnorderedList(t *testing.T) {
	tests := []struct {
		input      string
		wantBullet string
		wantText   string
	}{
		{"- item", "-", "item"},
		{"* item", "*", "item"},
		{"+ item", "+", "item"},
		{"  - indented", "-", "indented"},
		{"not a list", "", ""},
	}
	for _, tt := range tests {
		bullet, text := parseUnorderedList(tt.input)
		if bullet != tt.wantBullet || text != tt.wantText {
			t.Errorf("parseUnorderedList(%q) = (%q, %q), want (%q, %q)", tt.input, bullet, text, tt.wantBullet, tt.wantText)
		}
	}
}

func TestParseOrderedList(t *testing.T) {
	tests := []struct {
		input   string
		wantNum string
		wantTxt string
	}{
		{"1. first", "1.", "first"},
		{"12. twelfth", "12.", "twelfth"},
		{"not a list", "", ""},
		{". no number", "", ""},
	}
	for _, tt := range tests {
		num, text := parseOrderedList(tt.input)
		if num != tt.wantNum || text != tt.wantTxt {
			t.Errorf("parseOrderedList(%q) = (%q, %q), want (%q, %q)", tt.input, num, text, tt.wantNum, tt.wantTxt)
		}
	}
}

func TestIsHorizontalRule(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"---", true},
		{"***", true},
		{"___", true},
		{"- - -", true},
		{"----", true},
		{"--", false},
		{"abc", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isHorizontalRule(tt.input)
		if got != tt.want {
			t.Errorf("isHorizontalRule(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestStripAnsi(t *testing.T) {
	input := "\x1b[1mbold\x1b[22m normal"
	got := stripAnsi(input)
	want := "bold normal"
	if got != want {
		t.Errorf("stripAnsi(%q) = %q, want %q", input, got, want)
	}
}

func TestVisibleWidth(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"\x1b[1mhello\x1b[22m", 5},
		{"", 0},
	}
	for _, tt := range tests {
		got := visibleWidth(tt.input)
		if got != tt.want {
			t.Errorf("visibleWidth(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestMdWordWrap(t *testing.T) {
	text := "one two three four five six seven eight"
	out := mdWordWrap(text, 15)
	for _, line := range strings.Split(out, "\n") {
		w := visibleWidth(line)
		if w > 15 {
			t.Errorf("mdWordWrap: line width %d exceeds 15: %q", w, line)
		}
	}
}

func TestExtractCodeBlock(t *testing.T) {
	lines := []string{"```go", "line1", "line2", "```", "after"}
	block, end := extractCodeBlock(lines, 0)
	if block.lang != "go" {
		t.Errorf("expected lang 'go', got %q", block.lang)
	}
	if block.code != "line1\nline2" {
		t.Errorf("expected code 'line1\\nline2', got %q", block.code)
	}
	if end != 3 {
		t.Errorf("expected end index 3, got %d", end)
	}
}

func TestExtractCodeBlockUnclosed(t *testing.T) {
	lines := []string{"```", "only line"}
	block, end := extractCodeBlock(lines, 0)
	if block.code != "only line" {
		t.Errorf("unclosed code block: expected 'only line', got %q", block.code)
	}
	if end != 1 {
		t.Errorf("unclosed code block: expected end=1, got %d", end)
	}
}

func TestRenderMarkdownEmptyInput(t *testing.T) {
	out := renderMarkdown("", 80)
	if out != "" {
		t.Errorf("expected empty output for empty input, got %q", out)
	}
}

func TestRenderMarkdownNarrowWidth(t *testing.T) {
	// Should not panic on very narrow widths
	out := renderMarkdown("# Hello\n\n- item\n\n```\ncode\n```", 5)
	if out == "" {
		t.Error("expected non-empty output even at narrow width")
	}
}

// ---------------------------------------------------------------------------
// Struct-based MarkdownRenderer tests
// ---------------------------------------------------------------------------

func TestMarkdownRendererHeadings(t *testing.T) {
	r := NewMarkdownRenderer(80)

	tests := []struct {
		input string
		want  string
	}{
		{"# Heading 1", "Heading 1"},
		{"## Heading 2", "Heading 2"},
		{"### Heading 3", "Heading 3"},
		{"#### Heading 4", "Heading 4"},
	}
	for _, tt := range tests {
		out := r.Render(tt.input)
		plain := StripANSI(out)
		if !strings.Contains(plain, tt.want) {
			t.Errorf("Render(%q): expected %q in plain output, got %q", tt.input, tt.want, plain)
		}
		// Headers should not contain # in output
		if strings.Contains(plain, "#") {
			t.Errorf("Render(%q): should not contain # in output, got %q", tt.input, plain)
		}
	}
}

func TestMarkdownRendererH1Underline(t *testing.T) {
	r := NewMarkdownRenderer(80)
	out := r.Render("# Title")
	// H1 should have underline escape code
	if !strings.Contains(out, "\x1b[4m") {
		t.Error("H1 should be underlined")
	}
}

func TestMarkdownRendererBold(t *testing.T) {
	r := NewMarkdownRenderer(80)
	out := r.Render("This is **bold** text")
	plain := StripANSI(out)

	if !strings.Contains(plain, "bold") {
		t.Errorf("expected 'bold' in output, got %q", plain)
	}
	if strings.Contains(plain, "**") {
		t.Errorf("** markers should be removed, got %q", plain)
	}
	// Should contain ANSI bold
	if !strings.Contains(out, "\x1b[1m") {
		t.Error("expected ANSI bold sequence in output")
	}
}

func TestMarkdownRendererItalic(t *testing.T) {
	r := NewMarkdownRenderer(80)
	out := r.Render("This is *italic* text")
	plain := StripANSI(out)

	if !strings.Contains(plain, "italic") {
		t.Errorf("expected 'italic' in output, got %q", plain)
	}
	// Should contain ANSI italic
	if !strings.Contains(out, "\x1b[3m") {
		t.Error("expected ANSI italic sequence in output")
	}
}

func TestMarkdownRendererInlineCode(t *testing.T) {
	r := NewMarkdownRenderer(80)
	out := r.Render("Use `go build` to compile")
	plain := StripANSI(out)

	if !strings.Contains(plain, "go build") {
		t.Errorf("expected 'go build' in output, got %q", plain)
	}
	if strings.Contains(plain, "`") {
		t.Errorf("backticks should be removed, got %q", plain)
	}
}

func TestMarkdownRendererCodeBlockWithLanguage(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```"
	out := r.Render(input)
	plain := StripANSI(out)

	if !strings.Contains(plain, "go") {
		t.Errorf("expected language label 'go', got %q", plain)
	}
	if !strings.Contains(plain, "func main()") {
		t.Errorf("expected 'func main()' in code block, got %q", plain)
	}
	if !strings.Contains(plain, "fmt.Println") {
		t.Errorf("expected fmt.Println in code block, got %q", plain)
	}
}

func TestMarkdownRendererBulletLists(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "- first\n- second\n- third"
	out := r.Render(input)
	plain := StripANSI(out)

	for _, item := range []string{"first", "second", "third"} {
		if !strings.Contains(plain, item) {
			t.Errorf("expected %q in bullet list output, got %q", item, plain)
		}
	}
	// Should have bullet character
	if !strings.Contains(plain, "•") {
		t.Errorf("expected bullet character in output, got %q", plain)
	}
}

func TestMarkdownRendererNestedBulletLists(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "- parent\n  - child\n    - grandchild"
	out := r.Render(input)
	plain := StripANSI(out)

	for _, item := range []string{"parent", "child", "grandchild"} {
		if !strings.Contains(plain, item) {
			t.Errorf("expected %q in nested list output, got %q", item, plain)
		}
	}
}

func TestMarkdownRendererNumberedLists(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "1. first\n2. second\n3. third"
	out := r.Render(input)
	plain := StripANSI(out)

	if !strings.Contains(plain, "1.") {
		t.Errorf("expected '1.' in output, got %q", plain)
	}
	if !strings.Contains(plain, "2.") {
		t.Errorf("expected '2.' in output, got %q", plain)
	}
	for _, item := range []string{"first", "second", "third"} {
		if !strings.Contains(plain, item) {
			t.Errorf("expected %q in numbered list, got %q", item, plain)
		}
	}
}

func TestMarkdownRendererBlockQuotes(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "> This is a quoted statement"
	out := r.Render(input)
	plain := StripANSI(out)

	if !strings.Contains(plain, "This is a quoted statement") {
		t.Errorf("expected quote text in output, got %q", plain)
	}
	if !strings.Contains(plain, "│") {
		t.Errorf("expected vertical bar in blockquote, got %q", plain)
	}
}

func TestMarkdownRendererHorizontalRules(t *testing.T) {
	r := NewMarkdownRenderer(80)
	tests := []string{"---", "***", "___"}
	for _, input := range tests {
		out := r.Render(input)
		plain := StripANSI(out)
		if !strings.Contains(plain, "─") {
			t.Errorf("Render(%q): expected horizontal rule character, got %q", input, plain)
		}
	}
}

func TestMarkdownRendererLinks(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "Check [the docs](https://docs.example.com) for details"
	out := r.Render(input)
	plain := StripANSI(out)

	if !strings.Contains(plain, "the docs") {
		t.Errorf("expected link text in output, got %q", plain)
	}
	if !strings.Contains(plain, "https://docs.example.com") {
		t.Errorf("expected URL in output, got %q", plain)
	}
}

func TestMarkdownRendererTables(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "| Name | Age |\n|------|-----|\n| Alice | 30 |\n| Bob | 25 |"
	out := r.Render(input)
	plain := StripANSI(out)

	// Should contain table content
	if !strings.Contains(plain, "Name") {
		t.Errorf("expected 'Name' in table, got %q", plain)
	}
	if !strings.Contains(plain, "Alice") {
		t.Errorf("expected 'Alice' in table, got %q", plain)
	}
	if !strings.Contains(plain, "Bob") {
		t.Errorf("expected 'Bob' in table, got %q", plain)
	}
	// Should have box-drawing characters
	if !strings.Contains(plain, "┌") {
		t.Errorf("expected box-drawing character in table, got %q", plain)
	}
	if !strings.Contains(plain, "│") {
		t.Errorf("expected vertical box-drawing in table, got %q", plain)
	}
	if !strings.Contains(plain, "└") {
		t.Errorf("expected bottom box-drawing in table, got %q", plain)
	}
}

func TestWrapTextAtBoundary(t *testing.T) {
	tests := []struct {
		input string
		width int
	}{
		{"one two three four five six seven eight nine ten", 20},
		{"short", 80},
		{"a b c d e f g h i j k l m n o p", 10},
	}
	for _, tt := range tests {
		out := WrapText(tt.input, tt.width)
		for i, line := range strings.Split(out, "\n") {
			w := len(StripANSI(line))
			if w > tt.width {
				t.Errorf("WrapText(%q, %d): line %d width %d exceeds %d: %q",
					tt.input, tt.width, i, w, tt.width, line)
			}
		}
	}
}

func TestWrapTextEmpty(t *testing.T) {
	out := WrapText("", 80)
	if out != "" {
		t.Errorf("WrapText empty: expected empty, got %q", out)
	}
}

func TestWrapTextFitsInWidth(t *testing.T) {
	input := "short text"
	out := WrapText(input, 80)
	if out != input {
		t.Errorf("WrapText: text that fits should be unchanged, got %q", out)
	}
}

func TestStripANSIFunction(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"\x1b[1mbold\x1b[0m", "bold"},
		{"\x1b[36mcyan\x1b[0m text", "cyan text"},
		{"no ansi here", "no ansi here"},
		{"\x1b[38;5;198mkeyword\x1b[0m", "keyword"},
		{"", ""},
		{"\x1b[1m\x1b[4m\x1b[36mstacked\x1b[0m", "stacked"},
	}
	for _, tt := range tests {
		got := StripANSI(tt.input)
		if got != tt.want {
			t.Errorf("StripANSI(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHighlightCodeGoKeywords(t *testing.T) {
	code := "func main() {\n\tvar x = 42\n\treturn x\n}"
	out := HighlightCode(code, "go")

	// Should contain ANSI sequences (it is highlighted)
	if out == code {
		t.Error("HighlightCode should add ANSI codes to Go code")
	}

	// Plain text should still be the same code
	plain := StripANSI(out)
	if !strings.Contains(plain, "func") {
		t.Errorf("expected 'func' in highlighted output, got %q", plain)
	}
	if !strings.Contains(plain, "var") {
		t.Errorf("expected 'var' in highlighted output, got %q", plain)
	}
	if !strings.Contains(plain, "return") {
		t.Errorf("expected 'return' in highlighted output, got %q", plain)
	}
	if !strings.Contains(plain, "42") {
		t.Errorf("expected '42' in highlighted output, got %q", plain)
	}
}

func TestHighlightCodePython(t *testing.T) {
	code := "def hello():\n    return \"world\""
	out := HighlightCode(code, "python")

	if out == code {
		t.Error("HighlightCode should add ANSI codes to Python code")
	}
	plain := StripANSI(out)
	if !strings.Contains(plain, "def") {
		t.Errorf("expected 'def' in output, got %q", plain)
	}
}

func TestHighlightCodeUnsupportedLanguage(t *testing.T) {
	code := "some code here"
	out := HighlightCode(code, "brainfuck")
	if out != code {
		t.Errorf("unsupported language should return code unchanged, got %q", out)
	}
}

func TestHighlightCodeComments(t *testing.T) {
	code := "x := 1 // this is a comment"
	out := HighlightCode(code, "go")
	plain := StripANSI(out)
	if !strings.Contains(plain, "// this is a comment") {
		t.Errorf("expected comment preserved in output, got %q", plain)
	}
}

func TestHighlightCodeStrings(t *testing.T) {
	code := `fmt.Println("hello world")`
	out := HighlightCode(code, "go")
	plain := StripANSI(out)
	if !strings.Contains(plain, `"hello world"`) {
		t.Errorf("expected string preserved in output, got %q", plain)
	}
}

func TestMarkdownRendererMixedContent(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "# Project Setup\n\nInstall with:\n\n```bash\nnpm install\n```\n\n- Run tests\n- Build project"
	out := r.Render(input)
	plain := StripANSI(out)

	checks := []string{"Project Setup", "Install with", "npm install", "bash", "Run tests", "Build project"}
	for _, want := range checks {
		if !strings.Contains(plain, want) {
			t.Errorf("mixed content: expected %q in output, got %q", want, plain)
		}
	}
}

func TestMarkdownRendererEmptyInput(t *testing.T) {
	r := NewMarkdownRenderer(80)
	out := r.Render("")
	if out != "" {
		t.Errorf("expected empty output for empty input, got %q", out)
	}
}

func TestMarkdownRendererPlainText(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "Just some plain text with no markdown formatting at all."
	out := r.Render(input)
	plain := StripANSI(out)
	if !strings.Contains(plain, input) {
		t.Errorf("plain text should pass through unchanged, got %q", plain)
	}
}

func TestRenderTableFunction(t *testing.T) {
	rows := [][]string{
		{"Name", "Language", "Stars"},
		{"hawk", "Go", "1200"},
		{"glow", "Go", "15000"},
		{"bat", "Rust", "47000"},
	}
	out := RenderTable(rows)

	// Should contain all cell content
	for _, row := range rows {
		for _, cell := range row {
			if !strings.Contains(out, cell) {
				t.Errorf("RenderTable: expected %q in output", cell)
			}
		}
	}

	// Should have proper box characters
	if !strings.Contains(out, "┌") {
		t.Error("expected top-left corner")
	}
	if !strings.Contains(out, "┐") {
		t.Error("expected top-right corner")
	}
	if !strings.Contains(out, "└") {
		t.Error("expected bottom-left corner")
	}
	if !strings.Contains(out, "┘") {
		t.Error("expected bottom-right corner")
	}
	if !strings.Contains(out, "├") {
		t.Error("expected left separator")
	}
	if !strings.Contains(out, "┤") {
		t.Error("expected right separator")
	}
	if !strings.Contains(out, "┬") {
		t.Error("expected top separator")
	}
	if !strings.Contains(out, "┴") {
		t.Error("expected bottom separator")
	}
	if !strings.Contains(out, "┼") {
		t.Error("expected cross separator")
	}
}

func TestRenderTableEmpty(t *testing.T) {
	out := RenderTable(nil)
	if out != "" {
		t.Errorf("empty table should return empty string, got %q", out)
	}
}

func TestRenderTableSingleRow(t *testing.T) {
	rows := [][]string{{"only", "row"}}
	out := RenderTable(rows)
	if !strings.Contains(out, "only") || !strings.Contains(out, "row") {
		t.Errorf("single row table should contain cells, got %q", out)
	}
	// No separator row since there's no body
	if strings.Contains(out, "├") {
		t.Error("single row table should not have header separator")
	}
}

func TestRenderTableColumnAlignment(t *testing.T) {
	rows := [][]string{
		{"A", "BB", "CCC"},
		{"DDDD", "E", "FF"},
	}
	out := RenderTable(rows)
	lines := strings.Split(out, "\n")
	// All row lines (containing │) should be the same width
	var rowWidths []int
	for _, line := range lines {
		if strings.Contains(line, "│") || strings.Contains(line, "┌") || strings.Contains(line, "└") {
			rowWidths = append(rowWidths, len([]rune(line)))
		}
	}
	if len(rowWidths) > 1 {
		first := rowWidths[0]
		for i, w := range rowWidths {
			if w != first {
				t.Errorf("table row %d width %d differs from first row width %d", i, w, first)
			}
		}
	}
}

func TestDefaultTheme(t *testing.T) {
	theme := DefaultTheme()
	if theme == nil {
		t.Fatal("DefaultTheme() returned nil")
	}
	if theme.Reset != "\x1b[0m" {
		t.Errorf("expected reset to be ESC[0m, got %q", theme.Reset)
	}
	if theme.Bold == "" {
		t.Error("Bold should not be empty")
	}
	if theme.Italic == "" {
		t.Error("Italic should not be empty")
	}
	if theme.Heading == "" {
		t.Error("Heading should not be empty")
	}
}

func TestNewMarkdownRenderer(t *testing.T) {
	r := NewMarkdownRenderer(120)
	if r.Width != 120 {
		t.Errorf("expected width 120, got %d", r.Width)
	}
	if r.Theme == nil {
		t.Error("theme should not be nil")
	}
	if !r.SyntaxHighlight {
		t.Error("syntax highlight should be true by default")
	}
}

func TestNewMarkdownRendererDefaultWidth(t *testing.T) {
	r := NewMarkdownRenderer(0)
	if r.Width != 80 {
		t.Errorf("zero width should default to 80, got %d", r.Width)
	}
}

func TestRenderStreamingBasic(t *testing.T) {
	in := make(chan string, 10)
	out := RenderStreaming(in)

	// Send complete markdown
	in <- "# Hello\n"
	in <- "\nWorld"
	close(in)

	var collected strings.Builder
	for chunk := range out {
		collected.WriteString(chunk)
	}

	result := collected.String()
	plain := StripANSI(result)
	if !strings.Contains(plain, "Hello") {
		t.Errorf("streaming: expected 'Hello' in output, got %q", plain)
	}
	if !strings.Contains(plain, "World") {
		t.Errorf("streaming: expected 'World' in output, got %q", plain)
	}
}

func TestRenderStreamingPartialBold(t *testing.T) {
	in := make(chan string, 10)
	out := RenderStreaming(in)

	// Send partial bold (incomplete element)
	in <- "Hello **bol"
	in <- "d** world"
	close(in)

	var collected strings.Builder
	for chunk := range out {
		collected.WriteString(chunk)
	}

	result := collected.String()
	plain := StripANSI(result)
	if !strings.Contains(plain, "bold") {
		t.Errorf("streaming partial bold: expected 'bold' in output, got %q", plain)
	}
	if strings.Contains(plain, "**") {
		t.Errorf("streaming: ** markers should be removed, got %q", plain)
	}
}

func TestRenderStreamingCodeBlock(t *testing.T) {
	in := make(chan string, 10)
	out := RenderStreaming(in)

	in <- "```go\nfunc"
	in <- " main() {}\n```"
	close(in)

	var collected strings.Builder
	for chunk := range out {
		collected.WriteString(chunk)
	}

	result := collected.String()
	plain := StripANSI(result)
	if !strings.Contains(plain, "func main()") {
		t.Errorf("streaming code block: expected 'func main()' in output, got %q", plain)
	}
}

func TestHighlightCodeBash(t *testing.T) {
	code := "#!/bin/bash\necho \"hello\" # comment\nif [ -f file ]; then\n  exit 0\nfi"
	out := HighlightCode(code, "bash")
	if out == code {
		t.Error("HighlightCode should add ANSI codes to bash code")
	}
	plain := StripANSI(out)
	if !strings.Contains(plain, "echo") {
		t.Errorf("expected 'echo' preserved, got %q", plain)
	}
	if !strings.Contains(plain, "# comment") {
		t.Errorf("expected comment preserved, got %q", plain)
	}
}

func TestHighlightCodeRust(t *testing.T) {
	code := "fn main() {\n    let x = 42;\n    println!(\"{}\", x);\n}"
	out := HighlightCode(code, "rust")
	if out == code {
		t.Error("HighlightCode should add ANSI codes to Rust code")
	}
	plain := StripANSI(out)
	if !strings.Contains(plain, "fn") {
		t.Errorf("expected 'fn' preserved, got %q", plain)
	}
	if !strings.Contains(plain, "let") {
		t.Errorf("expected 'let' preserved, got %q", plain)
	}
}

func TestHighlightCodeJavaScript(t *testing.T) {
	code := "const x = 'hello';\nfunction greet() {\n  return x;\n}"
	out := HighlightCode(code, "javascript")
	if out == code {
		t.Error("HighlightCode should add ANSI codes to JavaScript code")
	}
	plain := StripANSI(out)
	if !strings.Contains(plain, "const") {
		t.Errorf("expected 'const' preserved, got %q", plain)
	}
	if !strings.Contains(plain, "function") {
		t.Errorf("expected 'function' preserved, got %q", plain)
	}
}
