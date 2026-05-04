package repomap

import (
	"regexp"
	"strings"
)

// ── Go ──

var (
	goFuncRe = regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?(\w+)`)
	goTypeRe = regexp.MustCompile(`^type\s+(\w+)\s+(struct|interface)`)
)

func parseGo(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := goFuncRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "func", Line: i + 1})
		} else if m := goTypeRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: m[2], Line: i + 1})
		}
	}
	return symbols
}

// ── Python ──

var (
	pyDefRe       = regexp.MustCompile(`^(?:def|class)\s+(\w+)`)
	pyAsyncDefRe  = regexp.MustCompile(`^async\s+def\s+(\w+)`)
	pyDecoratorRe = regexp.MustCompile(`^@(\w[\w.]*)`)
)

func parsePython(src string) []Symbol {
	var symbols []Symbol
	pendingDecorator := ""
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if m := pyDecoratorRe.FindStringSubmatch(trimmed); m != nil {
			pendingDecorator = m[1]
			continue
		}
		if m := pyAsyncDefRe.FindStringSubmatch(trimmed); m != nil {
			kind := "async func"
			if pendingDecorator != "" {
				kind = "@" + pendingDecorator + " async func"
				pendingDecorator = ""
			}
			symbols = append(symbols, Symbol{Name: m[1], Kind: kind, Line: i + 1})
		} else if m := pyDefRe.FindStringSubmatch(trimmed); m != nil {
			kind := "func"
			if strings.HasPrefix(trimmed, "class") {
				kind = "class"
			}
			if pendingDecorator != "" {
				kind = "@" + pendingDecorator + " " + kind
				pendingDecorator = ""
			}
			symbols = append(symbols, Symbol{Name: m[1], Kind: kind, Line: i + 1})
		} else if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			pendingDecorator = ""
		}
	}
	return symbols
}

// ── TypeScript / JavaScript ──

var (
	tsRe      = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:function|class|interface|type|const|let|var|abstract\s+class|enum)\s+(\w+)`)
	tsArrowRe = regexp.MustCompile(`^(?:export\s+)?(?:const|let|var)\s+(\w+)\s*(?::\s*\w[^=]*)?\s*=\s*(?:async\s+)?\(`)
)

func parseTypeScript(src string) []Symbol {
	var symbols []Symbol
	seen := map[string]bool{}
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		// Check arrow functions first — they're const/let with = (
		if m := tsArrowRe.FindStringSubmatch(trimmed); m != nil {
			if !seen[m[1]] {
				symbols = append(symbols, Symbol{Name: m[1], Kind: "func", Line: i + 1})
				seen[m[1]] = true
			}
		} else if m := tsRe.FindStringSubmatch(trimmed); m != nil {
			kind := detectTSKind(trimmed)
			if !seen[m[1]] {
				symbols = append(symbols, Symbol{Name: m[1], Kind: kind, Line: i + 1})
				seen[m[1]] = true
			}
		}
	}
	return symbols
}

func detectTSKind(line string) string {
	stripped := strings.TrimPrefix(strings.TrimPrefix(line, "export "), "default ")
	switch {
	case strings.HasPrefix(stripped, "function"):
		return "func"
	case strings.HasPrefix(stripped, "abstract class"):
		return "abstract class"
	case strings.HasPrefix(stripped, "class"):
		return "class"
	case strings.HasPrefix(stripped, "interface"):
		return "interface"
	case strings.HasPrefix(stripped, "type"):
		return "type"
	case strings.HasPrefix(stripped, "enum"):
		return "enum"
	case strings.HasPrefix(stripped, "const"):
		return "const"
	}
	return "symbol"
}

// ── Rust ──

var rustRe = regexp.MustCompile(`^(?:pub\s+)?(?:fn|struct|trait|impl|enum)\s+(\w+)`)

func parseRust(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := rustRe.FindStringSubmatch(line); m != nil {
			kind := detectRustKind(line)
			symbols = append(symbols, Symbol{Name: m[1], Kind: kind, Line: i + 1})
		}
	}
	return symbols
}

func detectRustKind(line string) string {
	stripped := strings.TrimPrefix(line, "pub ")
	switch {
	case strings.HasPrefix(stripped, "fn"):
		return "func"
	case strings.HasPrefix(stripped, "struct"):
		return "struct"
	case strings.HasPrefix(stripped, "trait"):
		return "trait"
	case strings.HasPrefix(stripped, "impl"):
		return "impl"
	case strings.HasPrefix(stripped, "enum"):
		return "enum"
	}
	return "symbol"
}

// ── Java ──

var (
	javaClassRe  = regexp.MustCompile(`^(?:public|private|protected)?\s*(?:static\s+)?(?:abstract\s+)?(?:final\s+)?(?:class|interface|enum|record)\s+(\w+)`)
	javaMethodRe = regexp.MustCompile(`^\s+(?:public|private|protected)\s+(?:static\s+)?(?:final\s+)?(?:synchronized\s+)?(?:abstract\s+)?(?:<[^>]+>\s+)?\w[\w<>\[\],?\s]*\s+(\w+)\s*\(`)
	javaFieldRe  = regexp.MustCompile(`^\s+(?:public|protected)\s+(?:static\s+)?(?:final\s+)?\w[\w<>\[\],?\s]*\s+(\w+)\s*[;=]`)
)

func parseJava(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := javaClassRe.FindStringSubmatch(line); m != nil {
			kind := "class"
			if strings.Contains(line, "interface") {
				kind = "interface"
			} else if strings.Contains(line, "enum") {
				kind = "enum"
			} else if strings.Contains(line, "record") {
				kind = "record"
			}
			symbols = append(symbols, Symbol{Name: m[1], Kind: kind, Line: i + 1})
		} else if m := javaMethodRe.FindStringSubmatch(line); m != nil {
			name := m[1]
			if name != "if" && name != "for" && name != "while" && name != "return" && name != "new" && name != "throw" {
				symbols = append(symbols, Symbol{Name: name, Kind: "method", Line: i + 1})
			}
		} else if m := javaFieldRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "field", Line: i + 1})
		}
	}
	return symbols
}
