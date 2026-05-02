// Package localize implements hierarchical fault localization inspired by
// OpenAutoCoder/Agentless. Given a bug description or task, it narrows down
// the location of relevant code in three stages:
//
//  1. File-level   -- identify the most relevant files by keyword matching
//     against file paths and directory structure.
//  2. Symbol-level -- within those files, extract functions/types/methods
//     via regex-based parsing and rank them against the query.
//  3. Edit-level   -- extract the code blocks for the top symbols with
//     surrounding context so the caller knows exactly where to edit.
//
// The package has zero external dependencies (only stdlib) and works across
// Go, Python, TypeScript, JavaScript, Rust, and Java.
package localize

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Localization holds the results of a three-stage localization pass.
type Localization struct {
	Files   []FileMatch   // Stage 1: ranked files
	Symbols []SymbolMatch // Stage 2: ranked symbols within those files
	Context []CodeBlock   // Stage 3: code blocks for top symbols
}

// FileMatch represents a file identified as relevant in Stage 1.
type FileMatch struct {
	Path   string  // relative to the repo root
	Score  float64 // higher = more relevant
	Reason string  // human-readable explanation of the score
}

// SymbolMatch represents a symbol identified as relevant in Stage 2.
type SymbolMatch struct {
	File      string  // relative path to the file
	Name      string  // symbol name (e.g. "Localize", "Session.Send")
	Kind      string  // "function", "method", "type", "const", "var", "class"
	StartLine int     // first line of the symbol definition
	EndLine   int     // estimated last line
	Score     float64 // higher = more relevant
}

// CodeBlock is a snippet of source code extracted in Stage 3.
type CodeBlock struct {
	File      string // relative path
	StartLine int    // first line (1-based, inclusive)
	EndLine   int    // last line (1-based, inclusive)
	Content   string // the source text (including context lines)
}

// Localize runs hierarchical fault localization on rootDir for the given query.
// It returns results from all three stages.
func Localize(rootDir string, query string, opts ...Option) (*Localization, error) {
	cfg := defaults()
	for _, o := range opts {
		o(cfg)
	}

	// Resolve rootDir to absolute path
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("localize: invalid root dir: %w", err)
	}

	// Stage 1: File-level localization
	files, err := findFiles(absRoot, query, cfg.maxFiles)
	if err != nil {
		return nil, fmt.Errorf("localize: file search failed: %w", err)
	}

	result := &Localization{
		Files: files,
	}

	if len(files) == 0 {
		return result, nil
	}

	// Stage 2: Symbol-level localization
	symbols, err := findSymbols(absRoot, files, query, cfg.maxSymbols, cfg.language)
	if err != nil {
		return nil, fmt.Errorf("localize: symbol search failed: %w", err)
	}
	result.Symbols = symbols

	if len(symbols) == 0 {
		return result, nil
	}

	// Stage 3: Edit-level localization — extract code blocks for top symbols
	result.Context = extractCodeBlocks(absRoot, symbols, cfg.contextLines)

	return result, nil
}

// extractCodeBlocks reads the relevant lines for each symbol match, plus
// contextLines of surrounding context.
func extractCodeBlocks(rootDir string, symbols []SymbolMatch, contextLines int) []CodeBlock {
	// Cache file contents to avoid re-reading the same file.
	fileLines := map[string][]string{}

	var blocks []CodeBlock
	for _, sym := range symbols {
		absPath := filepath.Join(rootDir, sym.File)

		lines, ok := fileLines[sym.File]
		if !ok {
			var err error
			lines, err = readFileLines(absPath)
			if err != nil {
				continue
			}
			fileLines[sym.File] = lines
		}

		totalLines := len(lines)
		start := sym.StartLine - contextLines
		if start < 1 {
			start = 1
		}
		end := sym.EndLine + contextLines
		if end > totalLines {
			end = totalLines
		}

		// Build content with line numbers
		var b strings.Builder
		for i := start; i <= end; i++ {
			fmt.Fprintf(&b, "%4d | %s\n", i, lines[i-1])
		}

		blocks = append(blocks, CodeBlock{
			File:      sym.File,
			StartLine: start,
			EndLine:   end,
			Content:   b.String(),
		})
	}

	return blocks
}

// readFileLines reads a file and returns its lines.
func readFileLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// FormatSummary returns a human-readable summary of the localization result.
func (loc *Localization) FormatSummary() string {
	if loc == nil {
		return "no localization results"
	}

	var b strings.Builder

	b.WriteString("=== File-level localization ===\n")
	if len(loc.Files) == 0 {
		b.WriteString("  (no files matched)\n")
	}
	for i, f := range loc.Files {
		fmt.Fprintf(&b, "  %d. %s (score: %.1f) — %s\n", i+1, f.Path, f.Score, f.Reason)
	}

	b.WriteString("\n=== Symbol-level localization ===\n")
	if len(loc.Symbols) == 0 {
		b.WriteString("  (no symbols matched)\n")
	}
	for i, s := range loc.Symbols {
		fmt.Fprintf(&b, "  %d. %s:%s %s (lines %d-%d, score: %.1f)\n",
			i+1, s.File, s.Kind, s.Name, s.StartLine, s.EndLine, s.Score)
	}

	b.WriteString("\n=== Edit-level localization ===\n")
	if len(loc.Context) == 0 {
		b.WriteString("  (no code blocks)\n")
	}
	for _, cb := range loc.Context {
		fmt.Fprintf(&b, "--- %s (lines %d-%d) ---\n", cb.File, cb.StartLine, cb.EndLine)
		b.WriteString(cb.Content)
		b.WriteString("\n")
	}

	return b.String()
}
