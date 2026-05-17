package rules

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Detection tests
// ---------------------------------------------------------------------------

func TestDetect_HawkRules(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".hawk", "rules")
	must(t, os.MkdirAll(rulesDir, 0o755))
	must(t, os.WriteFile(filepath.Join(rulesDir, "style.md"), []byte("Use gofmt."), 0o644))

	found := Detect(dir)
	if _, ok := found[FormatHawk]; !ok {
		t.Fatal("expected hawk rules to be detected")
	}
}

func TestDetect_CursorSingleFile(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, ".cursorrules"), []byte("# Style\nBe concise."), 0o644))

	found := Detect(dir)
	if _, ok := found[FormatCursor]; !ok {
		t.Fatal("expected cursor rules to be detected")
	}
}

func TestDetect_CursorMultiFile(t *testing.T) {
	dir := t.TempDir()
	cursorDir := filepath.Join(dir, ".cursor", "rules")
	must(t, os.MkdirAll(cursorDir, 0o755))
	must(t, os.WriteFile(filepath.Join(cursorDir, "style.mdc"), []byte("---\ndescription: style\n---\nBe concise."), 0o644))

	found := Detect(dir)
	if _, ok := found[FormatCursor]; !ok {
		t.Fatal("expected cursor rules to be detected")
	}
}

func TestDetect_ClaudeCode(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("## Naming\nUse descriptive names."), 0o644))

	found := Detect(dir)
	if _, ok := found[FormatClaudeCode]; !ok {
		t.Fatal("expected claude code rules to be detected")
	}
}

func TestDetect_Copilot(t *testing.T) {
	dir := t.TempDir()
	ghDir := filepath.Join(dir, ".github")
	must(t, os.MkdirAll(ghDir, 0o755))
	must(t, os.WriteFile(filepath.Join(ghDir, "copilot-instructions.md"), []byte("## Tests\nWrite tests."), 0o644))

	found := Detect(dir)
	if _, ok := found[FormatCopilot]; !ok {
		t.Fatal("expected copilot rules to be detected")
	}
}

func TestDetect_Gemini(t *testing.T) {
	dir := t.TempDir()
	gemDir := filepath.Join(dir, ".gemini")
	must(t, os.MkdirAll(gemDir, 0o755))
	must(t, os.WriteFile(filepath.Join(gemDir, "style-guide.md"), []byte("## Style\nBe brief."), 0o644))

	found := Detect(dir)
	if _, ok := found[FormatGemini]; !ok {
		t.Fatal("expected gemini rules to be detected")
	}
}

func TestDetect_Empty(t *testing.T) {
	dir := t.TempDir()
	found := Detect(dir)
	if len(found) != 0 {
		t.Fatalf("expected nothing detected in empty dir, got %v", found)
	}
}

func TestDetect_Multiple(t *testing.T) {
	dir := t.TempDir()

	// Set up hawk
	hawkDir := filepath.Join(dir, ".hawk", "rules")
	must(t, os.MkdirAll(hawkDir, 0o755))
	must(t, os.WriteFile(filepath.Join(hawkDir, "a.md"), []byte("rule a"), 0o644))

	// Set up CLAUDE.md
	must(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("## B\nrule b"), 0o644))

	found := Detect(dir)
	if len(found) != 2 {
		t.Fatalf("expected 2 formats detected, got %d: %v", len(found), found)
	}
}

// ---------------------------------------------------------------------------
// Import tests
// ---------------------------------------------------------------------------

func TestImport_CursorSingleFile(t *testing.T) {
	dir := t.TempDir()
	content := "# Naming\n\nUse descriptive names.\n\n## Testing\n\nAlways write tests.\n"
	must(t, os.WriteFile(filepath.Join(dir, ".cursorrules"), []byte(content), 0o644))

	rules, err := Import(dir, FormatCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	assertRule(t, rules[0], "Naming", "Use descriptive names.", FormatCursor)
	assertRule(t, rules[1], "Testing", "Always write tests.", FormatCursor)
}

func TestImport_CursorMultiFile(t *testing.T) {
	dir := t.TempDir()
	cursorDir := filepath.Join(dir, ".cursor", "rules")
	must(t, os.MkdirAll(cursorDir, 0o755))

	must(t, os.WriteFile(filepath.Join(cursorDir, "style.mdc"),
		[]byte("---\ndescription: style\n---\nUse gofmt.\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(cursorDir, "tests.mdc"),
		[]byte("---\ndescription: tests\n---\nWrite table-driven tests.\n"), 0o644))

	rules, err := Import(dir, FormatCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	// Sort for deterministic order.
	sort.Slice(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
	assertRule(t, rules[0], "style", "Use gofmt.", FormatCursor)
	assertRule(t, rules[1], "tests", "Write table-driven tests.", FormatCursor)
}

func TestImport_ClaudeCode(t *testing.T) {
	dir := t.TempDir()
	content := "## Error Handling\n\nAlways check errors.\n\n## Logging\n\nUse structured logging.\n"
	must(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0o644))

	rules, err := Import(dir, FormatClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	assertRule(t, rules[0], "Error Handling", "Always check errors.", FormatClaudeCode)
	assertRule(t, rules[1], "Logging", "Use structured logging.", FormatClaudeCode)
}

func TestImport_ClaudeCode_WithPreamble(t *testing.T) {
	dir := t.TempDir()
	content := "This is a preamble.\n\n## Style\n\nBe concise.\n"
	must(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0o644))

	rules, err := Import(dir, FormatClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Name != "preamble" {
		t.Fatalf("expected first rule to be preamble, got %q", rules[0].Name)
	}
	assertRule(t, rules[1], "Style", "Be concise.", FormatClaudeCode)
}

func TestImport_Copilot(t *testing.T) {
	dir := t.TempDir()
	ghDir := filepath.Join(dir, ".github")
	must(t, os.MkdirAll(ghDir, 0o755))
	content := "## Performance\n\nAvoid premature optimization.\n"
	must(t, os.WriteFile(filepath.Join(ghDir, "copilot-instructions.md"), []byte(content), 0o644))

	rules, err := Import(dir, FormatCopilot)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	assertRule(t, rules[0], "Performance", "Avoid premature optimization.", FormatCopilot)
}

func TestImport_Gemini(t *testing.T) {
	dir := t.TempDir()
	gemDir := filepath.Join(dir, ".gemini")
	must(t, os.MkdirAll(gemDir, 0o755))
	content := "## Clarity\n\nWrite clear code.\n"
	must(t, os.WriteFile(filepath.Join(gemDir, "style-guide.md"), []byte(content), 0o644))

	rules, err := Import(dir, FormatGemini)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	assertRule(t, rules[0], "Clarity", "Write clear code.", FormatGemini)
}

func TestImport_Hawk(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".hawk", "rules")
	must(t, os.MkdirAll(rulesDir, 0o755))
	must(t, os.WriteFile(filepath.Join(rulesDir, "style.md"), []byte("Use gofmt.\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(rulesDir, "docs.md"),
		[]byte("---\npaths: [\"docs/**\"]\n---\nKeep docs updated.\n"), 0o644))

	rules, err := Import(dir, FormatHawk)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
	assertRule(t, rules[0], "docs", "Keep docs updated.", FormatHawk)
	assertRule(t, rules[1], "style", "Use gofmt.", FormatHawk)
}

func TestImport_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	_, err := Import(dir, Format("unknown"))
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

// ---------------------------------------------------------------------------
// Export tests
// ---------------------------------------------------------------------------

func TestExport_Hawk(t *testing.T) {
	dir := t.TempDir()
	rules := []Rule{
		{Name: "style", Content: "Use gofmt."},
		{Name: "naming", Content: "Use descriptive names."},
	}
	if err := Export(dir, FormatHawk, rules); err != nil {
		t.Fatal(err)
	}

	// Verify files were written.
	data, err := os.ReadFile(filepath.Join(dir, ".hawk", "rules", "style.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Use gofmt.") {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestExport_CursorMultiFile(t *testing.T) {
	dir := t.TempDir()
	rules := []Rule{
		{Name: "style", Content: "Use gofmt."},
		{Name: "testing", Content: "Write tests."},
	}
	if err := Export(dir, FormatCursor, rules); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".cursor", "rules", "style.mdc"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "description: style") {
		t.Fatalf("expected frontmatter, got: %q", content)
	}
	if !strings.Contains(content, "Use gofmt.") {
		t.Fatalf("expected content, got: %q", content)
	}
}

func TestExport_ClaudeCode(t *testing.T) {
	dir := t.TempDir()
	rules := []Rule{
		{Name: "Error Handling", Content: "Always check errors."},
		{Name: "Logging", Content: "Use structured logging."},
	}
	if err := Export(dir, FormatClaudeCode, rules); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "## Error Handling") {
		t.Fatalf("expected header, got: %q", content)
	}
	if !strings.Contains(content, "Always check errors.") {
		t.Fatalf("expected content, got: %q", content)
	}
}

func TestExport_Copilot(t *testing.T) {
	dir := t.TempDir()
	rules := []Rule{{Name: "Perf", Content: "Avoid allocations."}}
	if err := Export(dir, FormatCopilot, rules); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".github", "copilot-instructions.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Avoid allocations.") {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestExport_Gemini(t *testing.T) {
	dir := t.TempDir()
	rules := []Rule{{Name: "Clarity", Content: "Write clear code."}}
	if err := Export(dir, FormatGemini, rules); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gemini", "style-guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Write clear code.") {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestExport_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	err := Export(dir, Format("unknown"), nil)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

// ---------------------------------------------------------------------------
// Round-trip tests
// ---------------------------------------------------------------------------

func TestRoundTrip_ClaudeCodeToHawk(t *testing.T) {
	dir := t.TempDir()

	// Write a CLAUDE.md source.
	content := "## Naming\n\nUse descriptive names.\n\n## Testing\n\nAlways write tests.\n"
	must(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0o644))

	// Import from Claude Code.
	imported, err := Import(dir, FormatClaudeCode)
	if err != nil {
		t.Fatal(err)
	}

	// Export to hawk.
	if err := Export(dir, FormatHawk, imported); err != nil {
		t.Fatal(err)
	}

	// Re-import from hawk.
	reimported, err := Import(dir, FormatHawk)
	if err != nil {
		t.Fatal(err)
	}

	if len(reimported) != len(imported) {
		t.Fatalf("round-trip count mismatch: %d vs %d", len(reimported), len(imported))
	}

	// Sort both for comparison.
	sort.Slice(imported, func(i, j int) bool { return imported[i].Name < imported[j].Name })
	sort.Slice(reimported, func(i, j int) bool { return reimported[i].Name < reimported[j].Name })

	for i := range imported {
		if reimported[i].Content != imported[i].Content {
			t.Fatalf("content mismatch for %q:\n  imported:    %q\n  reimported:  %q",
				imported[i].Name, imported[i].Content, reimported[i].Content)
		}
	}
}

func TestRoundTrip_CursorToHawkToClaudeCode(t *testing.T) {
	dir := t.TempDir()

	// Write a .cursorrules source.
	content := "## Style\n\nUse gofmt.\n\n## Docs\n\nKeep docs updated.\n"
	must(t, os.WriteFile(filepath.Join(dir, ".cursorrules"), []byte(content), 0o644))

	// Import from Cursor.
	imported, err := Import(dir, FormatCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported) != 2 {
		t.Fatalf("expected 2 rules from cursor, got %d", len(imported))
	}

	// Export to hawk, then to Claude Code.
	if err := Export(dir, FormatHawk, imported); err != nil {
		t.Fatal(err)
	}
	hawkRules, err := Import(dir, FormatHawk)
	if err != nil {
		t.Fatal(err)
	}
	if err := Export(dir, FormatClaudeCode, hawkRules); err != nil {
		t.Fatal(err)
	}

	// Re-import from Claude Code.
	final, err := Import(dir, FormatClaudeCode)
	if err != nil {
		t.Fatal(err)
	}

	if len(final) != len(imported) {
		t.Fatalf("round-trip count mismatch: %d vs %d", len(final), len(imported))
	}

	sort.Slice(imported, func(i, j int) bool { return imported[i].Name < imported[j].Name })
	sort.Slice(final, func(i, j int) bool { return final[i].Name < final[j].Name })

	for i := range imported {
		if final[i].Content != imported[i].Content {
			t.Fatalf("content mismatch for %q: %q vs %q",
				imported[i].Name, final[i].Content, imported[i].Content)
		}
	}
}

func TestRoundTrip_HawkToCursorToHawk(t *testing.T) {
	dir := t.TempDir()

	original := []Rule{
		{Name: "style", Content: "Use gofmt."},
		{Name: "naming", Content: "Use descriptive names."},
	}

	// Export to hawk, import, export to cursor, import from cursor, export back to hawk.
	if err := Export(dir, FormatHawk, original); err != nil {
		t.Fatal(err)
	}
	fromHawk, err := Import(dir, FormatHawk)
	if err != nil {
		t.Fatal(err)
	}
	if err := Export(dir, FormatCursor, fromHawk); err != nil {
		t.Fatal(err)
	}
	fromCursor, err := Import(dir, FormatCursor)
	if err != nil {
		t.Fatal(err)
	}

	// Re-export back to hawk in a fresh directory.
	dir2 := t.TempDir()
	if err := Export(dir2, FormatHawk, fromCursor); err != nil {
		t.Fatal(err)
	}
	final, err := Import(dir2, FormatHawk)
	if err != nil {
		t.Fatal(err)
	}

	if len(final) != len(original) {
		t.Fatalf("round-trip count mismatch: %d vs %d", len(final), len(original))
	}

	sort.Slice(original, func(i, j int) bool { return original[i].Name < original[j].Name })
	sort.Slice(final, func(i, j int) bool { return final[i].Name < final[j].Name })

	for i := range original {
		if final[i].Content != original[i].Content {
			t.Fatalf("content mismatch for %q: %q vs %q",
				original[i].Name, final[i].Content, original[i].Content)
		}
	}
}

// ---------------------------------------------------------------------------
// Helper: sanitizeFilename
// ---------------------------------------------------------------------------

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Error Handling", "error-handling"},
		{"naming", "naming"},
		{"My Rule #1!", "my-rule-1"},
		{"", "rule"},
		{"---", "---"},
	}
	for _, tt := range tests {
		got := sanitizeFilename(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func assertRule(t *testing.T, r Rule, name, content string, source Format) {
	t.Helper()
	if r.Name != name {
		t.Fatalf("expected name %q, got %q", name, r.Name)
	}
	if strings.TrimSpace(r.Content) != strings.TrimSpace(content) {
		t.Fatalf("expected content %q, got %q", content, r.Content)
	}
	if r.Source != source {
		t.Fatalf("expected source %q, got %q", source, r.Source)
	}
}
