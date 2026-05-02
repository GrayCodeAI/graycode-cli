// Package rules provides import/export of AI coding rules between different
// tool formats (hawk, Cursor, Claude Code, Copilot, Gemini).
package rules

import (
	"os"
	"path/filepath"
)

// Format identifies an AI tool's rule file format.
type Format string

const (
	FormatHawk       Format = "hawk"       // .hawk/rules/*.md
	FormatCursor     Format = "cursor"     // .cursorrules or .cursor/rules/*.mdc
	FormatClaudeCode Format = "claudecode" // CLAUDE.md
	FormatCopilot    Format = "copilot"    // .github/copilot-instructions.md
	FormatGemini     Format = "gemini"     // .gemini/style-guide.md
)

// Rule is a single coding rule with its name, content, and originating format.
type Rule struct {
	Name    string
	Content string
	Source  Format
}

// allFormats lists every supported format in detection order.
var allFormats = []Format{
	FormatHawk,
	FormatCursor,
	FormatClaudeCode,
	FormatCopilot,
	FormatGemini,
}

// Detect finds which AI tool rule files exist in dir.
// It returns a map from Format to the path of the first matching file found.
func Detect(dir string) map[Format]string {
	found := make(map[Format]string)

	for _, f := range allFormats {
		for _, candidate := range formatCandidates(dir, f) {
			if info, err := os.Stat(candidate); err == nil {
				if info.IsDir() {
					// For directory candidates, check that it contains at least one matching file.
					entries, err := os.ReadDir(candidate)
					if err != nil {
						continue
					}
					for _, e := range entries {
						if !e.IsDir() && hasFormatExt(e.Name(), f) {
							found[f] = candidate
							break
						}
					}
				} else {
					found[f] = candidate
				}
				if _, ok := found[f]; ok {
					break
				}
			}
		}
	}
	return found
}

// Import reads rules from the given format in dir and returns them.
func Import(dir string, from Format) ([]Rule, error) {
	switch from {
	case FormatHawk:
		return readHawk(dir)
	case FormatCursor:
		return readCursor(dir)
	case FormatClaudeCode:
		return readClaudeCode(dir)
	case FormatCopilot:
		return readCopilot(dir)
	case FormatGemini:
		return readGemini(dir)
	default:
		return nil, &UnsupportedFormatError{Format: from}
	}
}

// Export writes rules to the target format inside dir.
func Export(dir string, to Format, rules []Rule) error {
	switch to {
	case FormatHawk:
		return writeHawk(dir, rules)
	case FormatCursor:
		return writeCursor(dir, rules)
	case FormatClaudeCode:
		return writeClaudeCode(dir, rules)
	case FormatCopilot:
		return writeCopilot(dir, rules)
	case FormatGemini:
		return writeGemini(dir, rules)
	default:
		return &UnsupportedFormatError{Format: to}
	}
}

// UnsupportedFormatError is returned when an unknown Format is given.
type UnsupportedFormatError struct {
	Format Format
}

func (e *UnsupportedFormatError) Error() string {
	return "unsupported rules format: " + string(e.Format)
}

// formatCandidates returns file/directory paths to probe for a given format.
func formatCandidates(dir string, f Format) []string {
	switch f {
	case FormatHawk:
		return []string{filepath.Join(dir, ".hawk", "rules")}
	case FormatCursor:
		return []string{
			filepath.Join(dir, ".cursor", "rules"),
			filepath.Join(dir, ".cursorrules"),
		}
	case FormatClaudeCode:
		return []string{filepath.Join(dir, "CLAUDE.md")}
	case FormatCopilot:
		return []string{filepath.Join(dir, ".github", "copilot-instructions.md")}
	case FormatGemini:
		return []string{filepath.Join(dir, ".gemini", "style-guide.md")}
	}
	return nil
}

// hasFormatExt checks if a filename has the right extension for the given format.
func hasFormatExt(name string, f Format) bool {
	switch f {
	case FormatHawk:
		return filepath.Ext(name) == ".md"
	case FormatCursor:
		return filepath.Ext(name) == ".mdc"
	default:
		return false
	}
}
