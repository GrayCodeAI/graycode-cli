package repomap

import (
	"regexp"
	"strings"
	"unicode"
)

// langPatterns holds the regex extractors for non-Go languages. Each language
// has a set of symbol patterns (with capture group 1 = identifier and a kind)
// and a set of import patterns (capture group 1 = target).
type symPattern struct {
	re   *regexp.Regexp
	kind string
}

type langSpec struct {
	symbols []symPattern
	imports []*regexp.Regexp
}

var langSpecs = map[string]langSpec{
	"python": {
		symbols: []symPattern{
			{regexp.MustCompile(`(?m)^\s*def\s+([A-Za-z_]\w*)\s*\(`), "func"},
			{regexp.MustCompile(`(?m)^\s*class\s+([A-Za-z_]\w*)`), "type"},
		},
		imports: []*regexp.Regexp{
			regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z_][\w.]*)`),
			regexp.MustCompile(`(?m)^\s*from\s+([A-Za-z_][\w.]*)\s+import`),
		},
	},
	"javascript": jsSpec(),
	"typescript": jsSpec(),
	"rust": {
		symbols: []symPattern{
			{regexp.MustCompile(`(?m)^\s*(?:pub\s+)?fn\s+([A-Za-z_]\w*)`), "func"},
			{regexp.MustCompile(`(?m)^\s*(?:pub\s+)?struct\s+([A-Za-z_]\w*)`), "type"},
			{regexp.MustCompile(`(?m)^\s*(?:pub\s+)?enum\s+([A-Za-z_]\w*)`), "type"},
			{regexp.MustCompile(`(?m)^\s*(?:pub\s+)?trait\s+([A-Za-z_]\w*)`), "type"},
		},
		imports: []*regexp.Regexp{
			regexp.MustCompile(`(?m)^\s*use\s+([A-Za-z_][\w:]*)`),
		},
	},
	"java": {
		symbols: []symPattern{
			{regexp.MustCompile(`(?m)^\s*(?:public|private|protected)?\s*(?:abstract\s+|final\s+)?class\s+([A-Za-z_]\w*)`), "type"},
			{regexp.MustCompile(`(?m)^\s*(?:public|private|protected)?\s*interface\s+([A-Za-z_]\w*)`), "type"},
		},
		imports: []*regexp.Regexp{
			regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z_][\w.]*)`),
		},
	},
	"ruby": {
		symbols: []symPattern{
			{regexp.MustCompile(`(?m)^\s*def\s+([A-Za-z_]\w*[!?]?)`), "func"},
			{regexp.MustCompile(`(?m)^\s*class\s+([A-Za-z_]\w*)`), "type"},
			{regexp.MustCompile(`(?m)^\s*module\s+([A-Za-z_]\w*)`), "type"},
		},
		imports: []*regexp.Regexp{
			regexp.MustCompile(`(?m)^\s*require(?:_relative)?\s+['"]([^'"]+)['"]`),
		},
	},
	"c": cSpec(),
	"cpp": cSpec(),
}

func jsSpec() langSpec {
	return langSpec{
		symbols: []symPattern{
			{regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)`), "func"},
			{regexp.MustCompile(`(?m)^\s*(?:export\s+)?class\s+([A-Za-z_$][\w$]*)`), "type"},
			{regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s+)?\(`), "func"},
		},
		imports: []*regexp.Regexp{
			regexp.MustCompile(`(?m)^\s*import\s+.*?from\s+['"]([^'"]+)['"]`),
			regexp.MustCompile(`require\(\s*['"]([^'"]+)['"]\s*\)`),
		},
	}
}

func cSpec() langSpec {
	return langSpec{
		symbols: []symPattern{
			{regexp.MustCompile(`(?m)^[A-Za-z_][\w\s\*]*?\b([A-Za-z_]\w*)\s*\([^;]*\)\s*\{`), "func"},
			{regexp.MustCompile(`(?m)^\s*(?:typedef\s+)?struct\s+([A-Za-z_]\w*)`), "type"},
		},
		imports: []*regexp.Regexp{
			regexp.MustCompile(`(?m)^\s*#\s*include\s+[<"]([^>"]+)[>"]`),
		},
	}
}

// extractRegex applies the regex extractor for the given language. Unknown
// languages yield no symbols or imports.
func extractRegex(lang string, data []byte) ([]Symbol, []string) {
	spec, ok := langSpecs[lang]
	if !ok {
		return nil, nil
	}
	src := string(data)

	var symbols []Symbol
	seen := make(map[string]struct{})
	for _, sp := range spec.symbols {
		for _, m := range sp.re.FindAllStringSubmatch(src, -1) {
			if len(m) < 2 {
				continue
			}
			name := m[1]
			key := sp.kind + ":" + name
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			symbols = append(symbols, Symbol{
				Name:     name,
				Kind:     sp.kind,
				Exported: isExportedName(lang, name, src),
			})
		}
	}

	var imports []string
	for _, re := range spec.imports {
		for _, m := range re.FindAllStringSubmatch(src, -1) {
			if len(m) >= 2 && m[1] != "" {
				imports = append(imports, m[1])
			}
		}
	}
	return symbols, imports
}

// isExportedName applies a best-effort "public surface" heuristic per language.
func isExportedName(lang, name, src string) bool {
	switch lang {
	case "python":
		return !strings.HasPrefix(name, "_")
	case "ruby":
		// Ruby has no syntactic export marker at the name level; treat all as public.
		return true
	case "javascript", "typescript":
		// Approximate: a symbol is "exported" if its name appears after `export`.
		return strings.Contains(src, "export") && regexp.MustCompile(`(?m)export\b[^\n]*\b`+regexp.QuoteMeta(name)+`\b`).MatchString(src)
	default:
		// C/C++/Rust/Java: treat capitalized identifiers as the notable surface.
		for _, r := range name {
			return unicode.IsUpper(r) || lang == "c" || lang == "cpp"
		}
		return false
	}
}
