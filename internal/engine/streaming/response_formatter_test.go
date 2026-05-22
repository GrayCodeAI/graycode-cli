package streaming

import (
	"regexp"
	"strings"
	"testing"
)

func TestNewResponseFormatter(t *testing.T) {
	rf := NewResponseFormatter()
	if rf == nil {
		t.Fatal("NewResponseFormatter returned nil")
	}
	if len(rf.Rules) == 0 {
		t.Fatal("NewResponseFormatter should have built-in rules")
	}
	// Check that all rules are enabled by default
	for _, rule := range rf.Rules {
		if !rule.Enabled {
			t.Errorf("Rule %q should be enabled by default", rule.Name)
		}
	}
}

func TestFixCodeFences_UnclosedFence(t *testing.T) {
	input := "Here is code:\n```go\nfmt.Println(\"hello\")\n"
	result := FixCodeFences(input)
	if !strings.HasSuffix(result, "```") {
		t.Errorf("Expected closing code fence, got: %q", result)
	}
}

func TestFixCodeFences_AlreadyClosed(t *testing.T) {
	input := "```go\nfmt.Println(\"hello\")\n```"
	result := FixCodeFences(input)
	// Should not add an extra closing fence
	count := strings.Count(result, "```")
	if count != 2 {
		t.Errorf("Expected 2 fence markers, got %d", count)
	}
}

func TestFixCodeFences_MultipleFences(t *testing.T) {
	input := "```go\ncode1\n```\n\n```python\ncode2\n"
	result := FixCodeFences(input)
	// The second fence is unclosed, should be closed
	if !strings.HasSuffix(result, "```") {
		t.Errorf("Expected trailing closing fence")
	}
	count := strings.Count(result, "```")
	if count != 4 {
		t.Errorf("Expected 4 fence markers (2 pairs), got %d", count)
	}
}

func TestRemoveFluff_Sure(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Sure, I'll help with that.", "Help with that."},
		{"Sure! here is the code.", "Here is the code."},
		{"Certainly, I can do that.", "Do that."},
		{"Certainly! the answer is 42.", "The answer is 42."},
		{"Here's the solution:", "Solution:"},
		{"Of course! I'll explain.", "Explain."},
	}

	for _, tt := range tests {
		result := RemoveFluff(tt.input)
		if result != tt.expected {
			t.Errorf("RemoveFluff(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestRemoveFluff_NoMatch(t *testing.T) {
	input := "The function works by iterating over the slice."
	result := RemoveFluff(input)
	if result != input {
		t.Errorf("RemoveFluff should not modify non-filler text, got %q", result)
	}
}

func TestNormalizeLists_MixedBullets(t *testing.T) {
	input := "* Item one\n• Item two\n- Item three\n* Item four"
	result := NormalizeLists(input)
	expected := "- Item one\n- Item two\n- Item three\n- Item four"
	if result != expected {
		t.Errorf("NormalizeLists:\ngot:  %q\nwant: %q", result, expected)
	}
}

func TestNormalizeLists_IndentedBullets(t *testing.T) {
	input := "  * Sub item\n    • Deep item"
	result := NormalizeLists(input)
	expected := "  - Sub item\n    - Deep item"
	if result != expected {
		t.Errorf("NormalizeLists:\ngot:  %q\nwant: %q", result, expected)
	}
}

func TestFixMarkdown_BrokenLinks(t *testing.T) {
	input := "Check [this link] (https://example.com) for info."
	result := FixMarkdown(input)
	expected := "Check [this link](https://example.com) for info."
	if result != expected {
		t.Errorf("FixMarkdown:\ngot:  %q\nwant: %q", result, expected)
	}
}

func TestFixMarkdown_UnclosedBold(t *testing.T) {
	input := "This is **bold text without closing"
	result := FixMarkdown(input)
	if !strings.Contains(result, "**bold text without closing**") {
		t.Errorf("FixMarkdown should close bold, got: %q", result)
	}
}

func TestFixMarkdown_UnclosedItalic(t *testing.T) {
	input := "This is *italic text without closing"
	result := FixMarkdown(input)
	if !strings.HasSuffix(result, "*") {
		t.Errorf("FixMarkdown should close italic, got: %q", result)
	}
}

func TestFormat_Integration(t *testing.T) {
	rf := NewResponseFormatter()
	input := "Sure, I'll help! Here is the code:\n\n```Go\nfmt.Println(\"hello\")\n\n\nLet me know if you need anything else!"

	result := rf.Format(input)

	if result.Original != input {
		t.Error("Original should be preserved")
	}
	if result.Formatted == input {
		t.Error("Formatted should differ from original")
	}
	if len(result.Changes) == 0 {
		t.Error("Expected changes to be recorded")
	}
}

func TestFormat_NoChanges(t *testing.T) {
	rf := NewResponseFormatter()
	input := "The function returns an integer.\n"

	result := rf.Format(input)

	if len(result.Changes) != 0 {
		t.Errorf("Expected no changes, got: %v", result.Changes)
	}
	if result.Formatted != input {
		t.Errorf("Formatted should equal original when no changes needed")
	}
}

func TestFormat_RemoveTrailingOffers(t *testing.T) {
	rf := NewResponseFormatter()
	input := "The result is 42.\n\nLet me know if you have any questions!"

	result := rf.Format(input)

	if strings.Contains(result.Formatted, "Let me know") {
		t.Error("Trailing offer should be removed")
	}
}

func TestFormat_DoubleBlankLines(t *testing.T) {
	rf := NewResponseFormatter()
	input := "Line one.\n\n\n\nLine two.\n"

	result := rf.Format(input)

	if strings.Contains(result.Formatted, "\n\n\n") {
		t.Error("Double blank lines should be reduced")
	}
}

func TestFormat_TrailingWhitespace(t *testing.T) {
	rf := NewResponseFormatter()
	input := "Line one.   \nLine two.\t\n"

	result := rf.Format(input)

	lines := strings.Split(result.Formatted, "\n")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if line != trimmed {
			t.Errorf("Line %d still has trailing whitespace: %q", i, line)
		}
	}
}

func TestFormat_SelfReferential(t *testing.T) {
	rf := NewResponseFormatter()
	input := "As an AI language model, I cannot browse the internet.\nBut here is what I know.\n"

	result := rf.Format(input)

	if strings.Contains(result.Formatted, "As an AI") {
		t.Error("Self-referential phrase should be removed")
	}
	if !strings.Contains(result.Formatted, "But here is what I know.") {
		t.Error("Non-self-referential content should be preserved")
	}
}

func TestFormat_NormalizeHeadings(t *testing.T) {
	rf := NewResponseFormatter()
	input := "### First heading\n\nSome text.\n\n##### Sub heading\n"

	result := rf.Format(input)

	if strings.Contains(result.Formatted, "###") && !strings.Contains(result.Formatted, "# ") {
		// The heading levels should be compressed
	}
	// h3 and h5 should become h1 and h2
	if !strings.HasPrefix(result.Formatted, "# First heading") {
		t.Errorf("Expected heading normalization, got: %q", result.Formatted)
	}
}

func TestFormat_CodeFenceLanguageLabel(t *testing.T) {
	rf := NewResponseFormatter()
	input := "```Go\nfmt.Println(\"hi\")\n```\n"

	result := rf.Format(input)

	if strings.Contains(result.Formatted, "```Go") {
		t.Error("Language label should be lowercased")
	}
	if !strings.Contains(result.Formatted, "```go") {
		t.Errorf("Expected ```go, got: %q", result.Formatted)
	}
}

func TestEstimateTokenSavings(t *testing.T) {
	original := "Sure, I'll help! Here is a detailed explanation of how the code works."
	formatted := "Here is how the code works."

	savings := EstimateTokenSavings(original, formatted)
	if savings <= 0 {
		t.Errorf("Expected positive savings, got %d", savings)
	}
}

func TestEstimateTokenSavings_NoChange(t *testing.T) {
	text := "No change needed."
	savings := EstimateTokenSavings(text, text)
	if savings != 0 {
		t.Errorf("Expected 0 savings for identical text, got %d", savings)
	}
}

func TestResponseFormatterAddRule(t *testing.T) {
	rf := NewResponseFormatter()
	initialCount := len(rf.Rules)

	rf.AddRule(FormatRule{
		Name:    "custom_rule",
		Pattern: regexp.MustCompile(`(?i)TODO`),
		Fix: func(text string) string {
			return strings.ReplaceAll(text, "TODO", "DONE")
		},
		Enabled: true,
	})

	if len(rf.Rules) != initialCount+1 {
		t.Error("AddRule should increase rule count")
	}

	result := rf.Format("TODO: implement this")
	if !strings.Contains(result.Formatted, "DONE") {
		t.Error("Custom rule should be applied")
	}
}

func TestEnableRule(t *testing.T) {
	rf := NewResponseFormatter()
	rf.DisableRule("fix_unclosed_code_fences")

	// Verify it's disabled
	for _, rule := range rf.Rules {
		if rule.Name == "fix_unclosed_code_fences" && rule.Enabled {
			t.Fatal("Rule should be disabled")
		}
	}

	rf.EnableRule("fix_unclosed_code_fences")

	// Verify it's re-enabled
	found := false
	for _, rule := range rf.Rules {
		if rule.Name == "fix_unclosed_code_fences" {
			found = true
			if !rule.Enabled {
				t.Error("Rule should be enabled after EnableRule")
			}
		}
	}
	if !found {
		t.Error("Rule not found")
	}
}

func TestDisableRule(t *testing.T) {
	rf := NewResponseFormatter()
	rf.DisableRule("remove_filler_prefixes")

	input := "Sure, I'll help with that."
	result := rf.Format(input)

	// With the rule disabled, the prefix should remain
	if !strings.HasPrefix(result.Formatted, "Sure") {
		t.Error("Disabled rule should not be applied")
	}
}

func TestFormatChanges_WithChanges(t *testing.T) {
	result := &FormattedResponse{
		Changes: []string{
			"Removed filler prefix",
			"Fixed unclosed code fence",
			"Normalized 3 bullet points",
		},
		TokensDiff: 45,
	}

	output := FormatChanges(result)

	if !strings.Contains(output, "3 changes") {
		t.Errorf("Should mention change count, got: %q", output)
	}
	if !strings.Contains(output, "Removed filler prefix") {
		t.Error("Should list changes")
	}
	if !strings.Contains(output, "Token savings: ~45") {
		t.Errorf("Should show token savings, got: %q", output)
	}
}

func TestFormatChanges_NoChanges(t *testing.T) {
	result := &FormattedResponse{
		Changes:    []string{},
		TokensDiff: 0,
	}

	output := FormatChanges(result)

	if !strings.Contains(output, "No changes") {
		t.Errorf("Should indicate no changes, got: %q", output)
	}
}

func TestFormat_ConcurrentAccess(t *testing.T) {
	rf := NewResponseFormatter()
	input := "Sure! Here's the answer.\n\n\n\nThe code is:\n```Go\nfmt.Println()\n"

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			result := rf.Format(input)
			if result == nil {
				t.Error("Format returned nil")
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestFixCodeFences_NoFences(t *testing.T) {
	input := "Just regular text without any code."
	result := FixCodeFences(input)
	if result != input {
		t.Error("Should not modify text without code fences")
	}
}

func TestNormalizeLists_NoBullets(t *testing.T) {
	input := "Regular paragraph.\nAnother line.\n"
	result := NormalizeLists(input)
	if result != input {
		t.Error("Should not modify text without bullets")
	}
}

func TestNormalizeLists_DashesUnchanged(t *testing.T) {
	input := "- Item one\n- Item two\n- Item three"
	result := NormalizeLists(input)
	if result != input {
		t.Errorf("Dashes should not be modified, got: %q", result)
	}
}

func TestFixMarkdown_ValidLinks(t *testing.T) {
	input := "See [docs](https://example.com) for more."
	result := FixMarkdown(input)
	if result != input {
		t.Error("Valid links should not be modified")
	}
}

func TestFormat_ComplexResponse(t *testing.T) {
	rf := NewResponseFormatter()
	input := `Sure, I'll help! Here's a comprehensive guide:

### Installation

* Run the following command:

` + "```Go" + `
go install github.com/example/tool@latest
` + "```" + `

##### Configuration

• Set the config file
• Add your API key

As an AI, I want to note that this is simplified.

Let me know if you need more help!`

	result := rf.Format(input)

	// Filler prefix removed
	if strings.HasPrefix(result.Formatted, "Sure") {
		t.Error("Filler prefix should be removed")
	}
	// Language label lowercased
	if strings.Contains(result.Formatted, "```Go") {
		t.Error("Language label should be lowercase")
	}
	// Bullets normalized
	if strings.Contains(result.Formatted, "•") {
		t.Error("Bullets should be normalized to dashes")
	}
	// Self-referential removed
	if strings.Contains(result.Formatted, "As an AI") {
		t.Error("Self-referential phrase should be removed")
	}
	// Trailing offer removed
	if strings.Contains(result.Formatted, "Let me know") {
		t.Error("Trailing offer should be removed")
	}
	// Trailing whitespace removed
	if strings.Contains(result.Formatted, "   \n") {
		t.Error("Trailing whitespace should be removed")
	}
	// Multiple changes recorded
	if len(result.Changes) < 3 {
		t.Errorf("Expected multiple changes, got %d: %v", len(result.Changes), result.Changes)
	}
	// Token savings should be positive
	if result.TokensDiff <= 0 {
		t.Errorf("Expected positive token savings, got %d", result.TokensDiff)
	}
}
