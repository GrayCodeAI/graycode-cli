package repomap

import (
	"regexp"
	"strings"
)

// ── C ──

var (
	cFuncRe   = regexp.MustCompile(`^(?:static\s+)?(?:inline\s+)?(?:const\s+)?(?:unsigned\s+)?(?:struct\s+)?\w[\w\s*]+\s+(\w+)\s*\(`)
	cStructRe = regexp.MustCompile(`^(?:typedef\s+)?struct\s+(\w+)`)
	cEnumRe   = regexp.MustCompile(`^(?:typedef\s+)?enum\s+(\w+)`)
	cDefineRe = regexp.MustCompile(`^#define\s+(\w+)`)
)

func parseC(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}
		if m := cStructRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "struct", Line: i + 1})
		} else if m := cEnumRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "enum", Line: i + 1})
		} else if m := cDefineRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "define", Line: i + 1})
		} else if m := cFuncRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			// Skip control flow keywords
			if name == "if" || name == "for" || name == "while" || name == "switch" || name == "return" {
				continue
			}
			symbols = append(symbols, Symbol{Name: name, Kind: "func", Line: i + 1})
		}
	}
	return symbols
}

// ── C++ ──

var (
	cppClassRe     = regexp.MustCompile(`^(?:template\s*<[^>]*>\s*)?class\s+(\w+)`)
	cppNamespaceRe = regexp.MustCompile(`^namespace\s+(\w+)`)
)

func parseCpp(src string) []Symbol {
	symbols := parseC(src) // C++ is a superset of C for top-level symbols
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if m := cppClassRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "class", Line: i + 1})
		} else if m := cppNamespaceRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "namespace", Line: i + 1})
		}
	}
	return symbols
}

// ── C# ──

var (
	csClassRe     = regexp.MustCompile(`(?:public|private|protected|internal)?\s*(?:static\s+)?(?:abstract\s+)?(?:sealed\s+)?(?:partial\s+)?(?:class|struct|interface|record|enum)\s+(\w+)`)
	csMethodRe    = regexp.MustCompile(`(?:public|private|protected|internal)\s+(?:static\s+)?(?:async\s+)?(?:override\s+)?(?:virtual\s+)?\w[\w<>\[\],\s?]*\s+(\w+)\s*\(`)
	csNamespaceRe = regexp.MustCompile(`^namespace\s+([\w.]+)`)
)

func parseCSharp(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if m := csNamespaceRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "namespace", Line: i + 1})
		} else if m := csClassRe.FindStringSubmatch(trimmed); m != nil {
			kind := "class"
			for _, k := range []string{"interface", "struct", "record", "enum"} {
				if strings.Contains(trimmed, k+" ") {
					kind = k
					break
				}
			}
			symbols = append(symbols, Symbol{Name: m[1], Kind: kind, Line: i + 1})
		} else if m := csMethodRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			if name != "if" && name != "for" && name != "while" && name != "switch" && name != "return" && name != "new" {
				symbols = append(symbols, Symbol{Name: name, Kind: "method", Line: i + 1})
			}
		}
	}
	return symbols
}

// ── PHP ──

var (
	phpClassRe = regexp.MustCompile(`^(?:abstract\s+)?(?:final\s+)?class\s+(\w+)`)
	phpFuncRe  = regexp.MustCompile(`^(?:public|private|protected)?\s*(?:static\s+)?function\s+(\w+)`)
	phpIfaceRe = regexp.MustCompile(`^interface\s+(\w+)`)
	phpTraitRe = regexp.MustCompile(`^trait\s+(\w+)`)
)

func parsePHP(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if m := phpClassRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "class", Line: i + 1})
		} else if m := phpIfaceRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "interface", Line: i + 1})
		} else if m := phpTraitRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "trait", Line: i + 1})
		} else if m := phpFuncRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "func", Line: i + 1})
		}
	}
	return symbols
}

// ── Ruby ──

var (
	rbClassRe  = regexp.MustCompile(`^class\s+(\w+)`)
	rbModuleRe = regexp.MustCompile(`^module\s+(\w+)`)
	rbDefRe    = regexp.MustCompile(`^\s*def\s+(self\.)?(\w+[?!=]?)`)
)

func parseRuby(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if m := rbClassRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "class", Line: i + 1})
		} else if m := rbModuleRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "module", Line: i + 1})
		} else if m := rbDefRe.FindStringSubmatch(trimmed); m != nil {
			name := m[2]
			if m[1] != "" {
				name = "self." + name
			}
			symbols = append(symbols, Symbol{Name: name, Kind: "func", Line: i + 1})
		}
	}
	return symbols
}

// ── Kotlin ──

var (
	ktClassRe = regexp.MustCompile(`(?:data\s+|sealed\s+|abstract\s+|open\s+|enum\s+)?class\s+(\w+)`)
	ktFunRe   = regexp.MustCompile(`(?:private\s+|internal\s+|protected\s+)?(?:suspend\s+)?fun\s+(?:<[^>]+>\s+)?(\w+)`)
	ktObjRe   = regexp.MustCompile(`(?:companion\s+)?object\s+(\w+)`)
	ktIfaceRe = regexp.MustCompile(`interface\s+(\w+)`)
)

func parseKotlin(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if m := ktIfaceRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "interface", Line: i + 1})
		} else if m := ktClassRe.FindStringSubmatch(trimmed); m != nil {
			kind := "class"
			if strings.Contains(trimmed, "enum ") {
				kind = "enum"
			} else if strings.Contains(trimmed, "data ") {
				kind = "data class"
			}
			symbols = append(symbols, Symbol{Name: m[1], Kind: kind, Line: i + 1})
		} else if m := ktObjRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "object", Line: i + 1})
		} else if m := ktFunRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "func", Line: i + 1})
		}
	}
	return symbols
}

// ── Swift ──

var (
	swClassRe    = regexp.MustCompile(`(?:public\s+|private\s+|internal\s+|open\s+|final\s+)?class\s+(\w+)`)
	swStructRe   = regexp.MustCompile(`(?:public\s+|private\s+)?struct\s+(\w+)`)
	swEnumRe     = regexp.MustCompile(`(?:public\s+|private\s+)?enum\s+(\w+)`)
	swProtocolRe = regexp.MustCompile(`(?:public\s+|private\s+)?protocol\s+(\w+)`)
	swFuncRe     = regexp.MustCompile(`(?:public\s+|private\s+|internal\s+|open\s+)?(?:static\s+|class\s+)?(?:override\s+)?func\s+(\w+)`)
)

func parseSwift(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if m := swProtocolRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "protocol", Line: i + 1})
		} else if m := swClassRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "class", Line: i + 1})
		} else if m := swStructRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "struct", Line: i + 1})
		} else if m := swEnumRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "enum", Line: i + 1})
		} else if m := swFuncRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "func", Line: i + 1})
		}
	}
	return symbols
}

// ── Scala ──

var (
	scClassRe  = regexp.MustCompile(`(?:case\s+)?class\s+(\w+)`)
	scObjRe    = regexp.MustCompile(`object\s+(\w+)`)
	scTraitRe  = regexp.MustCompile(`trait\s+(\w+)`)
	scDefRe    = regexp.MustCompile(`\bdef\s+(\w+)`)
)

func parseScala(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if m := scTraitRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "trait", Line: i + 1})
		} else if m := scObjRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "object", Line: i + 1})
		} else if m := scClassRe.FindStringSubmatch(trimmed); m != nil {
			kind := "class"
			if strings.Contains(trimmed, "case ") {
				kind = "case class"
			}
			symbols = append(symbols, Symbol{Name: m[1], Kind: kind, Line: i + 1})
		} else if m := scDefRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "func", Line: i + 1})
		}
	}
	return symbols
}

// ── Lua ──

var (
	luaFuncRe      = regexp.MustCompile(`^(?:local\s+)?function\s+([\w.:]+)\s*\(`)
	luaAssignFuncRe = regexp.MustCompile(`^(?:local\s+)?([\w.]+)\s*=\s*function\s*\(`)
)

func parseLua(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if m := luaFuncRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "func", Line: i + 1})
		} else if m := luaAssignFuncRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "func", Line: i + 1})
		}
	}
	return symbols
}

// ── Dart ──

var (
	dartClassRe = regexp.MustCompile(`(?:abstract\s+)?class\s+(\w+)`)
	dartEnumRe  = regexp.MustCompile(`enum\s+(\w+)`)
	dartFuncRe  = regexp.MustCompile(`^\s*(?:static\s+)?(?:Future|Stream|void|int|double|String|bool|dynamic|var|\w+)(?:<[^>]+>)?\s+(\w+)\s*\(`)
)

func parseDart(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if m := dartClassRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "class", Line: i + 1})
		} else if m := dartEnumRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "enum", Line: i + 1})
		} else if m := dartFuncRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			if name != "if" && name != "for" && name != "while" && name != "return" && name != "switch" {
				symbols = append(symbols, Symbol{Name: name, Kind: "func", Line: i + 1})
			}
		}
	}
	return symbols
}

// ── Elixir ──

var (
	exModuleRe = regexp.MustCompile(`^defmodule\s+([\w.]+)`)
	exDefRe    = regexp.MustCompile(`^\s*(?:def|defp|defmacro)\s+(\w+[?!]?)`)
)

func parseElixir(src string) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if m := exModuleRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "module", Line: i + 1})
		} else if m := exDefRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "func", Line: i + 1})
		}
	}
	return symbols
}

// ── Haskell ──

var (
	hsTypeSigRe = regexp.MustCompile(`^(\w+)\s*::`)
	hsDataRe    = regexp.MustCompile(`^(?:data|newtype|type)\s+(\w+)`)
	hsClassRe   = regexp.MustCompile(`^class\s+(?:\([^)]*\)\s*=>)?\s*(\w+)`)
)

func parseHaskell(src string) []Symbol {
	var symbols []Symbol
	seen := map[string]bool{}
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		if m := hsDataRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "type", Line: i + 1})
			seen[m[1]] = true
		} else if m := hsClassRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "class", Line: i + 1})
			seen[m[1]] = true
		} else if m := hsTypeSigRe.FindStringSubmatch(line); m != nil {
			if !seen[m[1]] {
				symbols = append(symbols, Symbol{Name: m[1], Kind: "func", Line: i + 1})
				seen[m[1]] = true
			}
		}
	}
	return symbols
}
