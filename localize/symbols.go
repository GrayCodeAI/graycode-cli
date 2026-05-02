package localize

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ── Language detection ──

// langFromExt returns a language identifier for the given file extension.
func langFromExt(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	}
	return ""
}

// ── Regex patterns per language ──

// Each pattern set captures: the symbol name (group 1) and we derive the kind
// from which regex matched.

type symbolPattern struct {
	re   *regexp.Regexp
	kind string
}

var goPatterns = []symbolPattern{
	{regexp.MustCompile(`^func\s+\(\s*\w+\s+\*?(\w+)\)\s+(\w+)\s*\(`), "method"},
	{regexp.MustCompile(`^func\s+(\w+)\s*\(`), "function"},
	{regexp.MustCompile(`^type\s+(\w+)\s+struct\b`), "type"},
	{regexp.MustCompile(`^type\s+(\w+)\s+interface\b`), "type"},
	{regexp.MustCompile(`^type\s+(\w+)\s+`), "type"},
	{regexp.MustCompile(`^var\s+(\w+)\s+`), "var"},
	{regexp.MustCompile(`^const\s+(\w+)\s+`), "const"},
}

var pythonPatterns = []symbolPattern{
	{regexp.MustCompile(`^class\s+(\w+)`), "class"},
	{regexp.MustCompile(`^async\s+def\s+(\w+)`), "function"},
	{regexp.MustCompile(`^\s{4}async\s+def\s+(\w+)`), "method"},
	{regexp.MustCompile(`^def\s+(\w+)`), "function"},
	{regexp.MustCompile(`^\s{4}def\s+(\w+)`), "method"},
}

var tsPatterns = []symbolPattern{
	{regexp.MustCompile(`^(?:export\s+)?class\s+(\w+)`), "class"},
	{regexp.MustCompile(`^(?:export\s+)?interface\s+(\w+)`), "type"},
	{regexp.MustCompile(`^(?:export\s+)?type\s+(\w+)`), "type"},
	{regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+(\w+)`), "function"},
	{regexp.MustCompile(`^(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?\(`), "function"},
	{regexp.MustCompile(`^(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?\w+\s*=>`), "function"},
	{regexp.MustCompile(`^(?:export\s+)?const\s+(\w+)`), "const"},
	// Method inside a class body
	{regexp.MustCompile(`^\s+(?:async\s+)?(\w+)\s*\([^)]*\)\s*(?::\s*\w+)?\s*\{`), "method"},
}

var jsPatterns = []symbolPattern{
	{regexp.MustCompile(`^(?:export\s+)?class\s+(\w+)`), "class"},
	{regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+(\w+)`), "function"},
	{regexp.MustCompile(`^(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?\(`), "function"},
	{regexp.MustCompile(`^(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?\w+\s*=>`), "function"},
	{regexp.MustCompile(`^(?:export\s+)?const\s+(\w+)`), "const"},
	{regexp.MustCompile(`^\s+(?:async\s+)?(\w+)\s*\([^)]*\)\s*\{`), "method"},
}

var rustPatterns = []symbolPattern{
	{regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?fn\s+(\w+)`), "function"},
	{regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?struct\s+(\w+)`), "type"},
	{regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?enum\s+(\w+)`), "type"},
	{regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?trait\s+(\w+)`), "type"},
	{regexp.MustCompile(`^impl(?:<[^>]*>)?\s+(\w+)`), "type"},
	{regexp.MustCompile(`^\s+(?:pub(?:\([^)]*\))?\s+)?fn\s+(\w+)`), "method"},
}

var javaPatterns = []symbolPattern{
	{regexp.MustCompile(`^(?:public|private|protected)?\s*(?:static\s+)?(?:final\s+)?class\s+(\w+)`), "class"},
	{regexp.MustCompile(`^(?:public|private|protected)?\s*(?:static\s+)?interface\s+(\w+)`), "type"},
	{regexp.MustCompile(`^(?:public|private|protected)?\s*(?:static\s+)?enum\s+(\w+)`), "type"},
	{regexp.MustCompile(`^\s+(?:public|private|protected)?\s*(?:static\s+)?(?:final\s+)?(?:synchronized\s+)?(?:\w+(?:<[^>]*>)?)\s+(\w+)\s*\(`), "method"},
}

// patternsForLang returns the symbol patterns for a given language.
func patternsForLang(lang string) []symbolPattern {
	switch lang {
	case "go":
		return goPatterns
	case "python":
		return pythonPatterns
	case "typescript":
		return tsPatterns
	case "javascript":
		return jsPatterns
	case "rust":
		return rustPatterns
	case "java":
		return javaPatterns
	}
	return nil
}

// ── Symbol extraction ──

// rawSymbol is an intermediate representation before scoring.
type rawSymbol struct {
	name      string
	kind      string
	startLine int
	endLine   int // estimated end line
}

// extractSymbols reads a file and extracts symbols using regex patterns.
// If forceLang is non-empty, it overrides language detection.
func extractSymbols(filePath string, forceLang string) ([]rawSymbol, error) {
	ext := filepath.Ext(filePath)
	lang := forceLang
	if lang == "" {
		lang = langFromExt(ext)
	}
	if lang == "" {
		return nil, nil
	}

	patterns := patternsForLang(lang)
	if len(patterns) == 0 {
		return nil, nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	// Increase buffer for files with long lines
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var symbols []rawSymbol
	for i, line := range lines {
		lineNum := i + 1
		for _, pat := range patterns {
			m := pat.re.FindStringSubmatch(line)
			if m == nil {
				continue
			}

			name := m[1]
			// For Go methods, the receiver type is group 1, method name is group 2
			if pat.kind == "method" && lang == "go" && len(m) >= 3 {
				name = m[1] + "." + m[2]
			}

			symbols = append(symbols, rawSymbol{
				name:      name,
				kind:      pat.kind,
				startLine: lineNum,
			})
			break // first pattern match wins for this line
		}
	}

	// Estimate end lines: each symbol ends one line before the next symbol starts,
	// or at end-of-file for the last symbol.
	for i := range symbols {
		if i+1 < len(symbols) {
			symbols[i].endLine = symbols[i+1].startLine - 1
		} else {
			symbols[i].endLine = len(lines)
		}
	}

	return symbols, nil
}

// findSymbols locates symbols in the given files that match the query keywords.
// It returns at most maxSymbols results sorted by score descending.
func findSymbols(rootDir string, files []FileMatch, query string, maxSymbols int, forceLang string) ([]SymbolMatch, error) {
	keywords := extractKeywords(query)
	if len(keywords) == 0 {
		return nil, nil
	}

	var results []SymbolMatch

	for _, fm := range files {
		absPath := filepath.Join(rootDir, fm.Path)
		symbols, err := extractSymbols(absPath, forceLang)
		if err != nil {
			continue // skip files we cannot read
		}

		for _, sym := range symbols {
			score := scoreSymbol(sym.name, sym.kind, keywords)
			if score <= 0 {
				continue
			}
			// Boost symbols in higher-scoring files
			score += fm.Score * 0.1

			results = append(results, SymbolMatch{
				File:      fm.Path,
				Name:      sym.name,
				Kind:      sym.kind,
				StartLine: sym.startLine,
				EndLine:   sym.endLine,
				Score:     score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].File != results[j].File {
			return results[i].File < results[j].File
		}
		return results[i].StartLine < results[j].StartLine
	})

	if len(results) > maxSymbols {
		results = results[:maxSymbols]
	}

	return results, nil
}

// scoreSymbol scores a symbol name against the query keywords.
func scoreSymbol(name string, kind string, keywords []string) float64 {
	nameLower := strings.ToLower(name)
	// Split camelCase and snake_case into parts
	nameParts := splitIdentifier(nameLower)

	var score float64
	for _, kw := range keywords {
		kwLower := strings.ToLower(kw)

		// Exact name match
		if nameLower == kwLower {
			score += 10.0
			continue
		}

		// Name contains keyword
		if strings.Contains(nameLower, kwLower) {
			score += 5.0
			continue
		}

		// Keyword matches a name part (from camelCase/snake_case split)
		partMatch := false
		for _, part := range nameParts {
			if part == kwLower {
				score += 4.0
				partMatch = true
				break
			}
		}
		if partMatch {
			continue
		}

		// Partial substring match in parts
		for _, part := range nameParts {
			if strings.Contains(part, kwLower) || strings.Contains(kwLower, part) {
				score += 2.0
				break
			}
		}
	}

	return score
}

// splitIdentifier splits a camelCase or snake_case identifier into lowercase parts.
func splitIdentifier(name string) []string {
	// First split on underscores, dots and hyphens
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '.' || r == '-'
	})

	var result []string
	for _, part := range parts {
		// Split camelCase
		result = append(result, splitCamelCase(part)...)
	}
	return result
}

// splitCamelCase splits a camelCase string into lowercase tokens.
func splitCamelCase(s string) []string {
	if s == "" {
		return nil
	}

	var parts []string
	start := 0
	runes := []rune(s)
	for i := 1; i < len(runes); i++ {
		if runes[i] >= 'A' && runes[i] <= 'Z' {
			parts = append(parts, strings.ToLower(string(runes[start:i])))
			start = i
		}
	}
	parts = append(parts, strings.ToLower(string(runes[start:])))
	return parts
}
