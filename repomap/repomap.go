// Package repomap generates a lightweight code structure map of a repository
// by scanning files and extracting top-level symbols using regex-based parsers.
// The result is a token-budgeted summary suitable for injection into LLM prompts.
package repomap

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Symbol represents a top-level code symbol (function, type, class, etc.).
type Symbol struct {
	Name string
	Kind string
	Line int
}

// FileMap holds the extracted symbols for a single file.
type FileMap struct {
	Path    string
	Symbols []Symbol
}

// RepoMap is the full repository map result.
type RepoMap struct {
	Files    []FileMap
	TokenEst int
}

// Options configures repo map generation.
type Options struct {
	MaxFiles       int
	MaxTokens      int
	IgnorePatterns []string
}

// defaultIgnorePatterns are directories/files that are always skipped.
var defaultIgnorePatterns = []string{
	".git", "node_modules", "vendor", "__pycache__", ".venv", "venv",
	"dist", "build", ".next", ".nuxt", "target", "bin", "obj",
	".idea", ".vscode", ".DS_Store",
}

// Generate scans dir and produces a RepoMap with symbols from supported files.
func Generate(dir string, opts Options) (*RepoMap, error) {
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = 500
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 2000
	}

	ignoreSet := make(map[string]bool)
	for _, p := range defaultIgnorePatterns {
		ignoreSet[p] = true
	}
	for _, p := range opts.IgnorePatterns {
		ignoreSet[p] = true
	}

	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if ignoreSet[base] {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= opts.MaxFiles {
			return filepath.SkipAll
		}
		ext := filepath.Ext(path)
		if isSupportedExt(ext) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("repomap: walk error: %w", err)
	}

	sort.Strings(files)

	rm := &RepoMap{}
	for _, f := range files {
		symbols := parseFileSymbols(f)
		if len(symbols) == 0 {
			continue
		}
		relPath, err := filepath.Rel(dir, f)
		if err != nil {
			relPath = f
		}
		fm := FileMap{Path: relPath, Symbols: symbols}
		rm.Files = append(rm.Files, fm)
	}

	rm.TokenEst = estimateTokens(rm)
	return rm, nil
}

// Format renders the repo map as a text block, truncated to fit maxTokens.
func (rm *RepoMap) Format(maxTokens int) string {
	if rm == nil || len(rm.Files) == 0 {
		return ""
	}
	if maxTokens <= 0 {
		maxTokens = 2000
	}

	var b strings.Builder
	tokenCount := 0

	for _, fm := range rm.Files {
		// Estimate this file's contribution
		lineEst := 1 + len(fm.Symbols) // file header + one line per symbol
		tokEst := lineEst * 6           // ~6 tokens per line on average

		if tokenCount+tokEst > maxTokens {
			remaining := len(rm.Files) - countFormattedFiles(&b)
			if remaining > 0 {
				b.WriteString(fmt.Sprintf("\n... and %d more files\n", remaining))
			}
			break
		}

		b.WriteString(fm.Path + "\n")
		for _, sym := range fm.Symbols {
			b.WriteString(fmt.Sprintf("  %s %s (line %d)\n", sym.Kind, sym.Name, sym.Line))
		}
		tokenCount += tokEst
	}

	return b.String()
}

func countFormattedFiles(b *strings.Builder) int {
	count := 0
	for _, line := range strings.Split(b.String(), "\n") {
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "...") {
			count++
		}
	}
	return count
}

func estimateTokens(rm *RepoMap) int {
	total := 0
	for _, fm := range rm.Files {
		total += 6 // file path
		total += len(fm.Symbols) * 6
	}
	return total
}

// isSupportedExt returns true for file extensions that have a parser.
func isSupportedExt(ext string) bool {
	switch ext {
	case ".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".rs", ".java",
		".c", ".h", ".cpp", ".cc", ".cxx", ".hpp", ".hh",
		".cs", ".php",
		".rb", ".kt", ".kts", ".swift", ".scala", ".sc",
		".lua", ".dart", ".ex", ".exs", ".hs":
		return true
	}
	return false
}

// parseFileSymbols reads a file and extracts symbols using the appropriate parser.
func parseFileSymbols(path string) []Symbol {
	// Check cache first
	if symbols, ok := cacheGet(path); ok {
		return symbols
	}

	ext := filepath.Ext(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var symbols []Symbol
	src := string(data)
	switch ext {
	case ".go":
		symbols = parseGo(src)
	case ".py":
		symbols = parsePython(src)
	case ".ts", ".tsx", ".js", ".jsx":
		symbols = parseTypeScript(src)
	case ".rs":
		symbols = parseRust(src)
	case ".java":
		symbols = parseJava(src)
	case ".c", ".h":
		symbols = parseC(src)
	case ".cpp", ".cc", ".cxx", ".hpp", ".hh":
		symbols = parseCpp(src)
	case ".cs":
		symbols = parseCSharp(src)
	case ".php":
		symbols = parsePHP(src)
	case ".rb":
		symbols = parseRuby(src)
	case ".kt", ".kts":
		symbols = parseKotlin(src)
	case ".swift":
		symbols = parseSwift(src)
	case ".scala", ".sc":
		symbols = parseScala(src)
	case ".lua":
		symbols = parseLua(src)
	case ".dart":
		symbols = parseDart(src)
	case ".ex", ".exs":
		symbols = parseElixir(src)
	case ".hs":
		symbols = parseHaskell(src)
	}

	// Update cache
	cachePut(path, symbols)

	return symbols
}
