package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AIDirective represents a found AI comment directive in a source file.
type AIDirective struct {
	Path        string
	Line        int
	Instruction string
	Mode        string // "!" (do) or "?" (ask)
}

// aiCommentPatterns matches AI directives in various comment styles.
// Supported: // AI!, # AI!, /* AI! */, -- AI!, and the ? variants.
var aiCommentRe = regexp.MustCompile(
	`(?://|#|/\*|--)\s*AI([!?])\s*(.+?)(?:\s*\*/)?$`,
)

// aiSupportedExts are file extensions scanned for AI comments.
var aiSupportedExts = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true,
	".rs": true, ".java": true,
}

// scanForAIComments walks dir looking for AI directives in source files,
// skipping directories matching ignore patterns.
func scanForAIComments(dir string, ignore []string) []AIDirective {
	ignoreSet := make(map[string]bool, len(ignore))
	for _, p := range ignore {
		ignoreSet[p] = true
	}

	var directives []AIDirective

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if ignoreSet[filepath.Base(path)] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if !aiSupportedExts[ext] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			m := aiCommentRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			mode := m[1]
			instruction := strings.TrimSpace(m[2])
			relPath, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				relPath = path
			}
			directives = append(directives, AIDirective{
				Path:        relPath,
				Line:        i + 1,
				Instruction: instruction,
				Mode:        mode,
			})
		}
		return nil
	})

	return directives
}

// formatDirectivesAsPrompt formats found directives into a prompt string.
func formatDirectivesAsPrompt(directives []AIDirective) string {
	if len(directives) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("The following AI directives were found in your files:\n")
	for _, d := range directives {
		prefix := "DO"
		if d.Mode == "?" {
			prefix = "ASK"
		}
		b.WriteString(fmt.Sprintf("- %s:%d: [%s] %s\n", d.Path, d.Line, prefix, d.Instruction))
	}
	return b.String()
}

// removeAIComment removes the AI comment at the given line from the file.
// Line numbers are 1-based.
func removeAIComment(path string, line int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return fmt.Errorf("line %d out of range (file has %d lines)", line, len(lines))
	}

	// Check if the entire line is just the AI comment (possibly with whitespace)
	trimmed := strings.TrimSpace(lines[line-1])
	if aiCommentRe.MatchString(trimmed) && !strings.Contains(trimmed, ";") {
		// Remove the entire line
		lines = append(lines[:line-1], lines[line:]...)
	} else {
		// Remove just the AI comment portion from the line
		lines[line-1] = aiCommentRe.ReplaceAllString(lines[line-1], "")
		lines[line-1] = strings.TrimRight(lines[line-1], " \t")
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}
