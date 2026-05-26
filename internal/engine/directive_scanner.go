package engine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Directive is a parsed hawk: comment from source code.
type Directive struct {
	File    string
	Line    int
	Command string
	Context string
}

var hawkDirectivePattern = regexp.MustCompile(`(?i)(?://|#|--|/\*)\s*hawk:\s*(.+?)(?:\s*\*/)?$`)

// ScanDirectives finds all `// hawk: <command>` comments in source files.
func ScanDirectives(dir string) []Directive {
	var directives []Directive
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".hawk" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go", ".py", ".js", ".ts", ".rs", ".rb", ".java", ".c", ".cpp", ".h", ".jsx", ".tsx":
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			matches := hawkDirectivePattern.FindStringSubmatch(line)
			if len(matches) > 1 {
				start := i - 3
				if start < 0 {
					start = 0
				}
				end := i + 4
				if end > len(lines) {
					end = len(lines)
				}
				directives = append(directives, Directive{
					File:    path,
					Line:    i + 1,
					Command: strings.TrimSpace(matches[1]),
					Context: strings.Join(lines[start:end], "\n"),
				})
			}
		}
		return nil
	})
	return directives
}

// DirectivePrompt formats a directive as a prompt for the LLM.
func DirectivePrompt(d Directive) string {
	return "File: " + d.File + " (line " + fmt.Sprintf("%d", d.Line) + ")\n" +
		"Directive: " + d.Command + "\n" +
		"Context:\n```\n" + d.Context + "\n```\n\n" +
		"Implement what the hawk: comment asks for. Remove the hawk: comment after implementing."
}
