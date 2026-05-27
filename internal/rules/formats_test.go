package rules

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// splitSections
// ---------------------------------------------------------------------------

func TestSplitSections_MultipleHeaders(t *testing.T) {
	text := "## Error Handling\n\nCheck all errors.\n\n## Logging\n\nUse structured logging.\n"
	rules := splitSections(text, FormatClaudeCode)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Name != "Error Handling" {
		t.Errorf("name = %q, want 'Error Handling'", rules[0].Name)
	}
	if rules[1].Name != "Logging" {
		t.Errorf("name = %q, want 'Logging'", rules[1].Name)
	}
}

func TestSplitSections_H1Headers(t *testing.T) {
	text := "# Style\n\nBe concise.\n\n# Docs\n\nKeep docs updated.\n"
	rules := splitSections(text, FormatCursor)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Name != "Style" {
		t.Errorf("name = %q, want 'Style'", rules[0].Name)
	}
	if rules[1].Name != "Docs" {
		t.Errorf("name = %q, want 'Docs'", rules[1].Name)
	}
}

func TestSplitSections_Preamble(t *testing.T) {
	text := "This is a preamble.\n\n## Section\n\nContent here.\n"
	rules := splitSections(text, FormatClaudeCode)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Name != "preamble" {
		t.Errorf("expected preamble, got %q", rules[0].Name)
	}
	if rules[1].Name != "Section" {
		t.Errorf("expected 'Section', got %q", rules[1].Name)
	}
}

func TestSplitSections_Empty(t *testing.T) {
	rules := splitSections("", FormatClaudeCode)
	if len(rules) != 0 {
		t.Errorf("expected 0 rules for empty text, got %d", len(rules))
	}
}

func TestSplitSections_WhitespaceOnly(t *testing.T) {
	rules := splitSections("   \n\n  \n  ", FormatClaudeCode)
	if len(rules) != 0 {
		t.Errorf("expected 0 rules for whitespace-only, got %d", len(rules))
	}
}

func TestSplitSections_SingleSection(t *testing.T) {
	text := "## Only Section\n\nContent here.\n"
	rules := splitSections(text, FormatClaudeCode)
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Name != "Only Section" {
		t.Errorf("name = %q, want 'Only Section'", rules[0].Name)
	}
}

func TestSplitSections_SourcePreserved(t *testing.T) {
	text := "## Test\n\nContent.\n"
	rules := splitSections(text, FormatHawk)
	if rules[0].Source != FormatHawk {
		t.Errorf("source = %q, want %q", rules[0].Source, FormatHawk)
	}
}

// ---------------------------------------------------------------------------
// parseHeader
// ---------------------------------------------------------------------------

func TestParseHeader_H2(t *testing.T) {
	name, ok := parseHeader("## My Header")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if name != "My Header" {
		t.Errorf("name = %q, want 'My Header'", name)
	}
}

func TestParseHeader_H1(t *testing.T) {
	name, ok := parseHeader("# My Header")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if name != "My Header" {
		t.Errorf("name = %q, want 'My Header'", name)
	}
}

func TestParseHeader_NotHeader(t *testing.T) {
	_, ok := parseHeader("This is not a header")
	if ok {
		t.Error("expected ok=false for non-header")
	}
}

func TestParseHeader_H3(t *testing.T) {
	// H3 is not a recognized header format
	_, ok := parseHeader("### Deep Header")
	if ok {
		t.Error("expected ok=false for H3 header")
	}
}

func TestParseHeader_Empty(t *testing.T) {
	_, ok := parseHeader("")
	if ok {
		t.Error("expected ok=false for empty string")
	}
}

func TestParseHeader_TrimmedContent(t *testing.T) {
	name, ok := parseHeader("##   Spaced Out   ")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if name != "Spaced Out" {
		t.Errorf("name = %q, want 'Spaced Out'", name)
	}
}

// ---------------------------------------------------------------------------
// stripFrontmatter
// ---------------------------------------------------------------------------

func TestStripFrontmatter_WithFrontmatter(t *testing.T) {
	input := "---\ndescription: test\npaths: [\"src/**\"]\n---\nActual content.\n"
	got := stripFrontmatter(input)
	if got != "Actual content." {
		t.Errorf("stripFrontmatter = %q, want 'Actual content.'", got)
	}
}

func TestStripFrontmatter_NoFrontmatter(t *testing.T) {
	input := "Just regular content.\n"
	got := stripFrontmatter(input)
	if got != "Just regular content." {
		t.Errorf("stripFrontmatter = %q, want 'Just regular content.'", got)
	}
}

func TestStripFrontmatter_MalformedFrontmatter(t *testing.T) {
	input := "---\nno closing delimiter\nSome content.\n"
	got := stripFrontmatter(input)
	// Should return everything trimmed
	if got == "" {
		t.Error("expected non-empty result for malformed frontmatter")
	}
}

func TestStripFrontmatter_Empty(t *testing.T) {
	got := stripFrontmatter("")
	if got != "" {
		t.Errorf("expected empty for empty input, got %q", got)
	}
}

func TestStripFrontmatter_OnlyFrontmatter(t *testing.T) {
	input := "---\nkey: value\n---\n"
	got := stripFrontmatter(input)
	if got != "" {
		t.Errorf("expected empty when only frontmatter, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// sanitizeFilename
// ---------------------------------------------------------------------------

func TestSanitizeFilename_Basic(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Error Handling", "error-handling"},
		{"my rule", "my-rule"},
		{"UPPERCASE", "uppercase"},
		{"with-numbers-123", "with-numbers-123"},
		{"special!@#chars", "specialchars"},
		{"", "rule"},
		{"---", "---"},
		{"hello_world", "hello_world"},
		{"with spaces and symbols!@#", "with-spaces-and-symbols"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeFilename_PreservesHyphens(t *testing.T) {
	got := sanitizeFilename("already-hyphenated")
	if got != "already-hyphenated" {
		t.Errorf("expected 'already-hyphenated', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// readMDDir
// ---------------------------------------------------------------------------

func TestReadMDDir_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	os.MkdirAll(rulesDir, 0o755)

	os.WriteFile(filepath.Join(rulesDir, "style.md"), []byte("Use gofmt."), 0o644)
	os.WriteFile(filepath.Join(rulesDir, "testing.md"), []byte("Write tests."), 0o644)
	os.WriteFile(filepath.Join(rulesDir, "readme.txt"), []byte("Not an md file."), 0o644)

	rules, err := readMDDir(rulesDir, ".md", FormatHawk)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}

	names := map[string]bool{}
	for _, r := range rules {
		names[r.Name] = true
		if r.Source != FormatHawk {
			t.Errorf("source = %q, want %q", r.Source, FormatHawk)
		}
	}
	if !names["style"] {
		t.Error("expected 'style' rule")
	}
	if !names["testing"] {
		t.Error("expected 'testing' rule")
	}
}

func TestReadMDDir_WithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir, 0o755)

	content := "---\ndescription: coding style\n---\nUse gofmt.\n"
	os.WriteFile(filepath.Join(dir, "style.md"), []byte(content), 0o644)

	rules, err := readMDDir(dir, ".md", FormatHawk)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	// Frontmatter should be stripped
	if rules[0].Content != "Use gofmt." {
		t.Errorf("content = %q, want 'Use gofmt.'", rules[0].Content)
	}
}

func TestReadMDDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	rules, err := readMDDir(dir, ".md", FormatHawk)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules from empty dir, got %d", len(rules))
	}
}

func TestReadMDDir_WrongExtension(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0o644)

	rules, err := readMDDir(dir, ".md", FormatHawk)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules for wrong extension, got %d", len(rules))
	}
}

func TestReadMDDir_NonexistentDir(t *testing.T) {
	_, err := readMDDir("/nonexistent/path/12345", ".md", FormatHawk)
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

// ---------------------------------------------------------------------------
// hasFormatExt
// ---------------------------------------------------------------------------

func TestHasFormatExt(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		want   bool
	}{
		{"style.md", FormatHawk, true},
		{"style.mdc", FormatCursor, true},
		{"style.txt", FormatHawk, false},
		{"style.md", FormatCursor, false},
		{"CLAUDE.md", FormatClaudeCode, false}, // hasFormatExt only handles hawk/cursor
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasFormatExt(tt.name, tt.format)
			if got != tt.want {
				t.Errorf("hasFormatExt(%q, %q) = %v, want %v", tt.name, tt.format, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// formatCandidates
// ---------------------------------------------------------------------------

func TestFormatCandidates_Hawk(t *testing.T) {
	candidates := formatCandidates("/project", FormatHawk)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0] != filepath.Join("/project", ".hawk", "rules") {
		t.Errorf("unexpected path: %q", candidates[0])
	}
}

func TestFormatCandidates_Cursor(t *testing.T) {
	candidates := formatCandidates("/project", FormatCursor)
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
}

func TestFormatCandidates_ClaudeCode(t *testing.T) {
	candidates := formatCandidates("/project", FormatClaudeCode)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
}

func TestFormatCandidates_UnknownFormat(t *testing.T) {
	candidates := formatCandidates("/project", Format("unknown"))
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates for unknown format, got %d", len(candidates))
	}
}
