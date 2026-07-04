package cmd

import (
	"strings"
	"testing"
)

func TestNewVisualDiff(t *testing.T) {
	vd := NewVisualDiff(120)
	if vd.Width != 120 {
		t.Errorf("expected width 120, got %d", vd.Width)
	}
	if !vd.ShowLineNumbers {
		t.Error("expected ShowLineNumbers to be true by default")
	}
	if vd.ContextLines != 3 {
		t.Errorf("expected ContextLines 3, got %d", vd.ContextLines)
	}
	if !vd.WordLevel {
		t.Error("expected WordLevel to be true by default")
	}
}

func TestNewVisualDiffMinWidth(t *testing.T) {
	vd := NewVisualDiff(10)
	if vd.Width != 80 {
		t.Errorf("expected minimum width 80, got %d", vd.Width)
	}
}

func TestFindWordChanges(t *testing.T) {
	tests := []struct {
		name     string
		old      string
		new      string
		wantLen  int
		wantType []string
	}{
		{
			name:     "single word change",
			old:      "hello world",
			new:      "hello earth",
			wantLen:  4,
			wantType: []string{"equal", "equal", "delete", "insert"},
		},
		{
			name:     "identical lines",
			old:      "no changes here",
			new:      "no changes here",
			wantLen:  5, // "no", " ", "changes", " ", "here"
			wantType: []string{"equal", "equal", "equal", "equal", "equal"},
		},
		{
			name:     "all different",
			old:      "foo",
			new:      "bar",
			wantLen:  2,
			wantType: []string{"delete", "insert"},
		},
		{
			name:     "insertion",
			old:      "a b",
			new:      "a x b",
			wantLen:  5, // "a", " ", insert "x", insert " ", "b"
			wantType: []string{"equal", "equal", "insert", "insert", "equal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := FindWordChanges(tt.old, tt.new)
			if len(changes) != tt.wantLen {
				t.Errorf("got %d changes, want %d; changes: %+v", len(changes), tt.wantLen, changes)
				return
			}
			for i, wt := range tt.wantType {
				if i < len(changes) && changes[i].Type != wt {
					t.Errorf("change[%d].Type = %q, want %q", i, changes[i].Type, wt)
				}
			}
		})
	}
}

func TestRenderInline(t *testing.T) {
	vd := NewVisualDiff(80)
	diff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,3 @@
 package main
-var x = 1
+var x = 2
 func main() {}`

	result := vd.RenderInline(diff)
	if result == "" {
		t.Fatal("RenderInline returned empty string")
	}

	// Should contain ANSI codes
	if !strings.Contains(result, "\033[") {
		t.Error("expected ANSI color codes in output")
	}

	// Should contain line content
	stripped := stripAnsi(result)
	if !strings.Contains(stripped, "package main") {
		t.Error("expected context line 'package main' in output")
	}
	if !strings.Contains(stripped, "var x") {
		t.Error("expected changed line content in output")
	}
}

func TestRenderInlineEmpty(t *testing.T) {
	vd := NewVisualDiff(80)
	result := vd.RenderInline("")
	if result != "" {
		t.Errorf("expected empty string for empty input, got %q", result)
	}
}

func TestRenderSideBySide(t *testing.T) {
	vd := NewVisualDiff(100)
	diff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,3 @@
 package main
-var x = 1
+var x = 2
 func main() {}`

	result := vd.RenderSideBySide(diff)
	if result == "" {
		t.Fatal("RenderSideBySide returned empty string")
	}

	// Should contain the column separator
	if !strings.Contains(result, "│") {
		t.Error("expected column separator '│' in side-by-side output")
	}

	// Should contain the top separator line
	if !strings.Contains(result, "─") {
		t.Error("expected line separator in output")
	}
}

func TestRenderSideBySideEmpty(t *testing.T) {
	vd := NewVisualDiff(80)
	result := vd.RenderSideBySide("")
	if result != "" {
		t.Errorf("expected empty string for empty input, got %q", result)
	}
}

func TestRenderWordDiff(t *testing.T) {
	vd := NewVisualDiff(80)
	oldLine := "func calculate(x int) int {"
	newLine := "func calculate(x float64) float64 {"

	oldRendered, newRendered := vd.RenderWordDiff(oldLine, newLine)

	// Both should contain ANSI codes for changed words
	if !strings.Contains(oldRendered, vd.Theme.WordDel) {
		t.Error("expected WordDel color in old line rendering")
	}
	if !strings.Contains(newRendered, vd.Theme.WordAdd) {
		t.Error("expected WordAdd color in new line rendering")
	}

	// Common parts should remain uncolored
	oldStripped := stripAnsi(oldRendered)
	if !strings.Contains(oldStripped, "func") {
		t.Error("expected common word 'func' in old rendered line")
	}
}

func TestRenderFileDiff(t *testing.T) {
	vd := NewVisualDiff(80)
	oldContent := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10"
	newContent := "line 1\nline 2\nline 3\nline CHANGED\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10"

	result := vd.RenderFileDiff("test.go", oldContent, newContent)

	if !strings.Contains(result, "test.go") {
		t.Error("expected filename in header")
	}

	stripped := stripAnsi(result)
	// Should collapse far-away unchanged lines
	if !strings.Contains(stripped, "lines") {
		t.Error("expected collapsed region indicator")
	}
}

func TestRenderFileDiffNoChanges(t *testing.T) {
	vd := NewVisualDiff(80)
	content := "line 1\nline 2\nline 3"
	result := vd.RenderFileDiff("same.go", content, content)

	// Should have header but all lines collapsed
	if !strings.Contains(result, "same.go") {
		t.Error("expected filename in header")
	}
}

func TestRenderDiffSummary(t *testing.T) {
	vd := NewVisualDiff(80)
	files := []FileDiffStat{
		{Path: "src/auth/token.go", Additions: 15, Deletions: 3, Status: "M"},
		{Path: "src/handler/api.go", Additions: 42, Deletions: 12, Status: "M"},
		{Path: "src/middleware/jwt.go", Additions: 65, Deletions: 0, Status: "A"},
		{Path: "src/auth/old.go", Additions: 0, Deletions: 30, Status: "D"},
		{Path: "go.mod", Additions: 2, Deletions: 1, Status: "M"},
	}

	result := vd.RenderDiffSummary(files)

	if !strings.Contains(result, "Files changed: 5") {
		t.Error("expected 'Files changed: 5' in summary")
	}
	if !strings.Contains(result, "Total: +124 -46") {
		t.Error("expected 'Total: +124 -46' in summary")
	}

	stripped := stripAnsi(result)
	if !strings.Contains(stripped, "token.go") {
		t.Error("expected file path in summary")
	}
	if !strings.Contains(stripped, "+15 -3") {
		t.Error("expected addition/deletion counts")
	}
	if !strings.Contains(stripped, "█") {
		t.Error("expected bar chart characters")
	}
}

func TestRenderDiffSummaryEmpty(t *testing.T) {
	vd := NewVisualDiff(80)
	result := vd.RenderDiffSummary(nil)
	if result != "" {
		t.Errorf("expected empty string for no files, got %q", result)
	}
}

func TestColorizeByLanguage(t *testing.T) {
	vd := NewVisualDiff(80)

	tests := []struct {
		name     string
		line     string
		lang     string
		contains string
	}{
		{
			name:     "go keyword",
			line:     "func main() {",
			lang:     "go",
			contains: "\033[34;1m", // blue+bold for keywords
		},
		{
			name:     "python keyword",
			line:     "def hello():",
			lang:     "python",
			contains: "\033[34;1m",
		},
		{
			name:     "go comment",
			line:     "x := 1 // comment",
			lang:     "go",
			contains: "\033[2;32m", // dim green for comments
		},
		{
			name:     "unknown language passthrough",
			line:     "some text",
			lang:     "unknown",
			contains: "some text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := vd.ColorizeByLanguage(tt.line, tt.lang)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected %q to contain %q", result, tt.contains)
			}
		})
	}
}

func TestVisualDiffStripAnsi(t *testing.T) {
	input := "\033[31mhello\033[0m \033[32mworld\033[0m"
	expected := "hello world"
	result := stripAnsi(input)
	if result != expected {
		t.Errorf("stripAnsi(%q) = %q, want %q", input, result, expected)
	}
}

func TestVisibleLength(t *testing.T) {
	plain := "hello"
	colored := "\033[31mhello\033[0m"

	if visibleLength(plain) != 5 {
		t.Errorf("visibleLength(%q) = %d, want 5", plain, visibleLength(plain))
	}
	if visibleLength(colored) != 5 {
		t.Errorf("visibleLength(%q) = %d, want 5", colored, visibleLength(colored))
	}
}

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		header  string
		oldLine int
		newLine int
	}{
		{"@@ -1,3 +1,3 @@", 1, 1},
		{"@@ -10,5 +12,7 @@", 10, 12},
		{"@@ -100,20 +105,25 @@ func main()", 100, 105},
	}

	for _, tt := range tests {
		old, new := parseHunkHeader(tt.header)
		if old != tt.oldLine {
			t.Errorf("parseHunkHeader(%q) old = %d, want %d", tt.header, old, tt.oldLine)
		}
		if new != tt.newLine {
			t.Errorf("parseHunkHeader(%q) new = %d, want %d", tt.header, new, tt.newLine)
		}
	}
}

func TestSplitWords(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"hello world", []string{"hello", " ", "world"}},
		{"  leading", []string{"  ", "leading"}},
		{"no_spaces", []string{"no_spaces"}},
		{"a  b", []string{"a", "  ", "b"}},
	}

	for _, tt := range tests {
		got := splitWords(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitWords(%q) = %v (len %d), want %v (len %d)",
				tt.input, got, len(got), tt.want, len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitWords(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestPadOrTruncate(t *testing.T) {
	tests := []struct {
		input string
		width int
		want  int // visible length of result
	}{
		{"hello", 10, 10},
		{"hello", 3, 3},
		{"hello", 5, 5},
	}

	for _, tt := range tests {
		result := padOrTruncate(tt.input, tt.width)
		got := visibleLength(result)
		if got != tt.want {
			t.Errorf("padOrTruncate(%q, %d) visible length = %d, want %d",
				tt.input, tt.width, got, tt.want)
		}
	}
}

func TestTruncateVisible(t *testing.T) {
	colored := "\033[31mhello world\033[0m"
	result := truncateVisible(colored, 5)
	stripped := stripAnsi(result)
	if stripped != "hello" {
		t.Errorf("truncateVisible visible content = %q, want %q", stripped, "hello")
	}
}

func TestComputeLCS(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"a", "c", "d", "e"}
	lcs := computeLCS(a, b)
	expected := []string{"a", "c", "d"}
	if len(lcs) != len(expected) {
		t.Errorf("computeLCS got %v, want %v", lcs, expected)
		return
	}
	for i := range lcs {
		if lcs[i] != expected[i] {
			t.Errorf("computeLCS[%d] = %q, want %q", i, lcs[i], expected[i])
		}
	}
}

func TestRenderInlineWithoutLineNumbers(t *testing.T) {
	vd := NewVisualDiff(80)
	vd.ShowLineNumbers = false
	diff := `@@ -1,2 +1,2 @@
-old line
+new line`

	result := vd.RenderInline(diff)
	stripped := stripAnsi(result)

	// Should not have line number formatting (4-digit number followed by space)
	lines := strings.Split(stripped, "\n")
	for _, line := range lines {
		if len(line) > 4 {
			// Check first chars aren't all digits (line numbers)
			allDigits := true
			for _, c := range line[:4] {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				t.Errorf("found line number in output when ShowLineNumbers=false: %q", line)
			}
		}
	}
}

func TestDefaultDiffTheme(t *testing.T) {
	theme := DefaultDiffTheme()
	if theme.Reset != "\033[0m" {
		t.Errorf("unexpected Reset value: %q", theme.Reset)
	}
	if theme.Added != ansiDone {
		t.Errorf("unexpected Added value: %q", theme.Added)
	}
	if theme.Removed != ansiCoral {
		t.Errorf("unexpected Removed value: %q", theme.Removed)
	}
}

func TestRenderWordDiffIdentical(t *testing.T) {
	vd := NewVisualDiff(80)
	oldRendered, newRendered := vd.RenderWordDiff("same line", "same line")

	// No highlighting should be present for identical lines
	if strings.Contains(oldRendered, vd.Theme.WordDel) {
		t.Error("expected no WordDel in identical line old rendering")
	}
	if strings.Contains(newRendered, vd.Theme.WordAdd) {
		t.Error("expected no WordAdd in identical line new rendering")
	}
}

func TestFileDiffStatFields(t *testing.T) {
	stat := FileDiffStat{
		Path:      "src/main.go",
		Additions: 10,
		Deletions: 5,
		Status:    "M",
	}
	if stat.Path != "src/main.go" {
		t.Errorf("unexpected Path: %s", stat.Path)
	}
	if stat.Status != "M" {
		t.Errorf("unexpected Status: %s", stat.Status)
	}
}
