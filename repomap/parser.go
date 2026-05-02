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
	pyDefRe      = regexp.MustCompile(`^(?:def|class)\s+(\w+)`)
	pyAsyncDefRe = regexp.MustCompile(`^async\s+def\s+(\w+)`)
)

func parsePython(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := pyAsyncDefRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "async func", Line: i + 1})
		} else if m := pyDefRe.FindStringSubmatch(line); m != nil {
			kind := "func"
			if strings.HasPrefix(line, "class") {
				kind = "class"
			}
			symbols = append(symbols, Symbol{Name: m[1], Kind: kind, Line: i + 1})
		}
	}
	return symbols
}

// ── TypeScript / JavaScript ──

var tsRe = regexp.MustCompile(`^(?:export\s+)?(?:function|class|interface|type|const)\s+(\w+)`)

func parseTypeScript(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := tsRe.FindStringSubmatch(line); m != nil {
			kind := detectTSKind(line)
			symbols = append(symbols, Symbol{Name: m[1], Kind: kind, Line: i + 1})
		}
	}
	return symbols
}

func detectTSKind(line string) string {
	stripped := strings.TrimPrefix(line, "export ")
	switch {
	case strings.HasPrefix(stripped, "function"):
		return "func"
	case strings.HasPrefix(stripped, "class"):
		return "class"
	case strings.HasPrefix(stripped, "interface"):
		return "interface"
	case strings.HasPrefix(stripped, "type"):
		return "type"
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

var javaRe = regexp.MustCompile(`^(?:public|private|protected)?\s*(?:static\s+)?(?:class|interface|enum)\s+(\w+)`)

func parseJava(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := javaRe.FindStringSubmatch(line); m != nil {
			kind := "class"
			if strings.Contains(line, "interface") {
				kind = "interface"
			} else if strings.Contains(line, "enum") {
				kind = "enum"
			}
			symbols = append(symbols, Symbol{Name: m[1], Kind: kind, Line: i + 1})
		}
	}
	return symbols
}
