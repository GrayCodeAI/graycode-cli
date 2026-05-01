package cmd

import (
	"strings"
	"testing"
)

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

	checks := []string{"Title", "bold", "inline code", "list item one",
		"A blockquote", "│", "─", "1.", "ordered one", "python", "print"}
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
