package tool

import (
	"strings"
	"testing"
)

func TestFuzzyFind_ExactMatch(t *testing.T) {
	// When the exact string is present, the main Edit flow handles it
	// without calling fuzzyFind. This test verifies that the fuzzy
	// mechanism doesn't break when content and oldStr are very similar
	// (only whitespace differences).
	content := "func main() {\n    fmt.Println(\"hello\")\n    return\n}\n"
	oldStr := "func main() {\n\tfmt.Println(\"hello\")\n\treturn\n}"

	matched, actual, sim := fuzzyFind(content, oldStr)
	if !matched {
		t.Fatal("expected fuzzy match to find nearly-identical content")
	}
	// The actual should be from the content (with 4-space indent)
	if !strings.Contains(content, actual) {
		t.Errorf("actual %q not found in content", actual)
	}
	if sim < 0.85 {
		t.Errorf("expected high similarity, got %f", sim)
	}
}

func TestFuzzyFind_WhitespaceNormalized_ExtraSpaces(t *testing.T) {
	content := "if  x  ==  y {\n\treturn true\n}\n"
	oldStr := "if x == y {\n\treturn true\n}\n"

	matched, actual, sim := fuzzyFind(content, oldStr)
	if !matched {
		t.Fatal("expected whitespace-normalized match to succeed")
	}
	if actual != "if  x  ==  y {\n\treturn true\n}\n" {
		t.Errorf("unexpected actual: %q", actual)
	}
	if sim < 1.0 {
		t.Errorf("expected similarity 1.0 for whitespace match, got %f", sim)
	}
}

func TestFuzzyFind_WhitespaceNormalized_TabsVsSpaces(t *testing.T) {
	content := "func foo() {\n\tvar x\t= 1\n}\n"
	oldStr := "func foo() {\n\tvar x = 1\n}\n"

	matched, actual, sim := fuzzyFind(content, oldStr)
	if !matched {
		t.Fatal("expected whitespace-normalized match (tabs vs spaces) to succeed")
	}
	if actual != "func foo() {\n\tvar x\t= 1\n}\n" {
		t.Errorf("unexpected actual: %q", actual)
	}
	if sim < 1.0 {
		t.Errorf("expected similarity 1.0 for whitespace match, got %f", sim)
	}
}

func TestFuzzyFind_LeadingWhitespace(t *testing.T) {
	content := "    func hello() {\n        fmt.Println(\"hi\")\n    }\n"
	oldStr := "func hello() {\n    fmt.Println(\"hi\")\n}"

	matched, actual, _ := fuzzyFind(content, oldStr)
	if !matched {
		t.Fatal("expected leading-whitespace match to succeed")
	}
	expected := "    func hello() {\n        fmt.Println(\"hi\")\n    }"
	if actual != expected {
		t.Errorf("unexpected actual:\ngot:  %q\nwant: %q", actual, expected)
	}
}

func TestFuzzyFind_LevenshteinSmallTypo(t *testing.T) {
	content := "func calculate() {\n\tresult := x + y\n\treturn result\n}\n"
	// Small typo: "reuslt" instead of "result", "retrn" instead of "return"
	oldStr := "func calculate() {\n\treuslt := x + y\n\tretrn reuslt\n}"

	matched, actual, sim := fuzzyFind(content, oldStr)
	if !matched {
		t.Fatal("expected Levenshtein-based match to succeed for small typos")
	}
	// The matched actual should be a substring of content
	if !strings.Contains(content, actual) {
		t.Errorf("actual %q is not a substring of content", actual)
	}
	// Should contain the correct lines
	if !strings.Contains(actual, "result := x + y") {
		t.Errorf("actual should contain the corrected code, got %q", actual)
	}
	if sim < 0.85 {
		t.Errorf("similarity %f should be >= 0.85", sim)
	}
}

func TestFuzzyFind_ThresholdRejection(t *testing.T) {
	content := "func doSomething() {\n\tx := computeValue()\n\treturn x * 2\n}\n"
	// Completely different content — should not match
	oldStr := "class Widget:\n    def __init__(self):\n        self.value = 0\n"

	matched, _, sim := fuzzyFind(content, oldStr)
	if matched {
		t.Fatalf("expected fuzzy match to be rejected (below threshold), but got match with similarity %f", sim)
	}
}

func TestLevenshteinDistance_FuzzyEdit(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
		{"hello", "helo", 1},
	}
	for _, tt := range tests {
		got := levenshteinDistance(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello  world", "hello world"},
		{"a\t\tb", "a b"},
		{"  leading", " leading"},
		{"trailing  ", "trailing "},
		{"no change", "no change"},
		{"\t \t mixed \t\t", " mixed "},
	}
	for _, tt := range tests {
		got := normalizeWhitespace(tt.input)
		if got != tt.want {
			t.Errorf("normalizeWhitespace(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFuzzyFindUnicodeNormalized(t *testing.T) {
	// Model emitted em-dash and smart quotes; file has ASCII.
	content := "option = \"fast\"  # use the built-in mode\n"
	old := "option = \u201cfast\u201d  # use the built\u2013in mode\n"
	matched, actual, sim := fuzzyFind(content, old)
	if !matched {
		t.Fatal("unicode-normalized match failed")
	}
	if actual != content {
		t.Fatalf("actual = %q", actual)
	}
	if sim != 1.0 {
		t.Fatalf("sim = %v", sim)
	}
}

func TestUnicodeNormalizedFindRequiresTypographicNeedle(t *testing.T) {
	matched, _ := unicodeNormalizedFind("abc", "abc")
	if matched {
		t.Fatal("plain needle should be skipped (covered by earlier strategies)")
	}
}

func TestFoldTypographic(t *testing.T) {
	in := "\u201ca\u201d \u2014 b\u2026 c\u00a0d"
	want := "\"a\" - b. cd" // nbsp folds to regular space; trailing join is rune-level
	got := normalizeTypographic(in)
	// nbsp becomes ' ', so expected includes the space
	want = "\"a\" - b. c d"
	if got != want {
		t.Fatalf("normalizeTypographic = %q, want %q", got, want)
	}
}
