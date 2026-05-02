package rules

import (
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Hawk: .hawk/rules/*.md
// ---------------------------------------------------------------------------

func readHawk(dir string) ([]Rule, error) {
	rulesDir := filepath.Join(dir, ".hawk", "rules")
	return readMDDir(rulesDir, ".md", FormatHawk)
}

func writeHawk(dir string, rules []Rule) error {
	rulesDir := filepath.Join(dir, ".hawk", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		return err
	}
	for _, r := range rules {
		name := sanitizeFilename(r.Name) + ".md"
		if err := os.WriteFile(filepath.Join(rulesDir, name), []byte(r.Content+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Cursor: .cursorrules (single file) or .cursor/rules/*.mdc (multi-file)
// ---------------------------------------------------------------------------

func readCursor(dir string) ([]Rule, error) {
	// Try multi-file first.
	mdcDir := filepath.Join(dir, ".cursor", "rules")
	if rules, err := readMDDir(mdcDir, ".mdc", FormatCursor); err == nil && len(rules) > 0 {
		return rules, nil
	}

	// Fall back to single file.
	data, err := os.ReadFile(filepath.Join(dir, ".cursorrules"))
	if err != nil {
		return nil, err
	}
	return splitSections(string(data), FormatCursor), nil
}

func writeCursor(dir string, rules []Rule) error {
	// Write as multi-file .cursor/rules/*.mdc with frontmatter.
	rulesDir := filepath.Join(dir, ".cursor", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		return err
	}
	for _, r := range rules {
		name := sanitizeFilename(r.Name) + ".mdc"
		// Wrap in minimal frontmatter that Cursor expects.
		content := "---\ndescription: " + r.Name + "\n---\n" + r.Content + "\n"
		if err := os.WriteFile(filepath.Join(rulesDir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Claude Code: CLAUDE.md
// ---------------------------------------------------------------------------

func readClaudeCode(dir string) ([]Rule, error) {
	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		return nil, err
	}
	return splitSections(string(data), FormatClaudeCode), nil
}

func writeClaudeCode(dir string, rules []Rule) error {
	var b strings.Builder
	for i, r := range rules {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## " + r.Name + "\n\n")
		b.WriteString(r.Content + "\n")
	}
	return os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(b.String()), 0o644)
}

// ---------------------------------------------------------------------------
// Copilot: .github/copilot-instructions.md
// ---------------------------------------------------------------------------

func readCopilot(dir string) ([]Rule, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".github", "copilot-instructions.md"))
	if err != nil {
		return nil, err
	}
	return splitSections(string(data), FormatCopilot), nil
}

func writeCopilot(dir string, rules []Rule) error {
	ghDir := filepath.Join(dir, ".github")
	if err := os.MkdirAll(ghDir, 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for i, r := range rules {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## " + r.Name + "\n\n")
		b.WriteString(r.Content + "\n")
	}
	return os.WriteFile(filepath.Join(ghDir, "copilot-instructions.md"), []byte(b.String()), 0o644)
}

// ---------------------------------------------------------------------------
// Gemini: .gemini/style-guide.md
// ---------------------------------------------------------------------------

func readGemini(dir string) ([]Rule, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".gemini", "style-guide.md"))
	if err != nil {
		return nil, err
	}
	return splitSections(string(data), FormatGemini), nil
}

func writeGemini(dir string, rules []Rule) error {
	gemDir := filepath.Join(dir, ".gemini")
	if err := os.MkdirAll(gemDir, 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for i, r := range rules {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## " + r.Name + "\n\n")
		b.WriteString(r.Content + "\n")
	}
	return os.WriteFile(filepath.Join(gemDir, "style-guide.md"), []byte(b.String()), 0o644)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// readMDDir reads all files with the given extension from a directory.
// Files may have optional YAML frontmatter delimited by "---".
func readMDDir(dir, ext string, source Format) ([]Rule, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var rules []Rule
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ext {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ext)
		content := stripFrontmatter(string(data))
		rules = append(rules, Rule{
			Name:    name,
			Content: content,
			Source:  source,
		})
	}
	return rules, nil
}

// stripFrontmatter removes leading YAML frontmatter (---\n...\n---\n) from text.
func stripFrontmatter(s string) string {
	if !strings.HasPrefix(s, "---\n") {
		return strings.TrimSpace(s)
	}
	rest := s[4:]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		// Malformed frontmatter; return everything.
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(rest[idx+4:])
}

// splitSections splits a markdown document into rules by ## or # headers.
// Each section becomes a Rule whose Name is the header text.
// Content before the first header (if any) becomes a rule named "preamble".
func splitSections(text string, source Format) []Rule {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var rules []Rule
	lines := strings.Split(text, "\n")
	var currentName string
	var currentLines []string

	flush := func() {
		body := strings.TrimSpace(strings.Join(currentLines, "\n"))
		if body != "" || currentName != "" {
			if currentName == "" {
				currentName = "preamble"
			}
			rules = append(rules, Rule{
				Name:    currentName,
				Content: body,
				Source:  source,
			})
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if name, ok := parseHeader(trimmed); ok {
			flush()
			currentName = name
			currentLines = nil
		} else {
			currentLines = append(currentLines, line)
		}
	}
	flush()
	return rules
}

// parseHeader checks if a line is a markdown header (# or ##) and returns
// the header text. It returns ("", false) for non-header lines.
func parseHeader(line string) (string, bool) {
	if strings.HasPrefix(line, "## ") {
		return strings.TrimSpace(line[3:]), true
	}
	if strings.HasPrefix(line, "# ") {
		return strings.TrimSpace(line[2:]), true
	}
	return "", false
}

// sanitizeFilename converts a rule name into a safe filename component.
// It lowercases, replaces spaces with hyphens, and strips non-alphanumeric chars.
func sanitizeFilename(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if result == "" {
		return "rule"
	}
	return result
}
