package repomap

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// TreeSitterParser provides language-aware structural parsing that replaces
// simple regex-based symbol extraction. For Go files, it uses the stdlib
// go/parser + go/ast for true AST-level parsing. For other languages, it uses
// enhanced regex patterns with scope-tracking to produce AST-like results.
//
// This is a "tree-sitter-inspired" approach: it produces the same kinds of
// structured, scope-aware symbol extraction that tree-sitter would, without
// requiring any CGO dependencies.
type TreeSitterParser struct {
	// IncludeUnexported controls whether unexported symbols are included.
	// By default only exported/public symbols are returned for Go.
	IncludeUnexported bool
}

// NewTreeSitterParser creates a new parser with default settings.
func NewTreeSitterParser() *TreeSitterParser {
	return &TreeSitterParser{}
}

// ParseFile dispatches to the appropriate language parser based on file extension.
func (p *TreeSitterParser) ParseFile(path string) ([]Symbol, error) {
	ext := filepath.Ext(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("treesitter: read %s: %w", path, err)
	}
	src := string(data)
	return p.ParseSource(src, ext, path)
}

// ParseSource parses source code given its extension and optional path.
func (p *TreeSitterParser) ParseSource(src, ext, path string) ([]Symbol, error) {
	switch ext {
	case ".go":
		return parseGoAST(path, src, p.IncludeUnexported)
	case ".py":
		return parsePythonEnhanced(src), nil
	case ".ts", ".tsx", ".js", ".jsx":
		return parseTypeScriptEnhanced(src), nil
	case ".rs":
		return parseRustEnhanced(src), nil
	default:
		// Fall back to existing regex parsers for other languages
		return parseFallback(src, ext), nil
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Go: Full AST parsing via go/parser + go/ast (stdlib, zero CGO)
// ─────────────────────────────────────────────────────────────────────────────

// parseGoAST extracts symbols from Go source using the standard library AST parser.
// It extracts: functions (with receiver for methods), types (struct, interface,
// type alias), exported constants/vars, interface method signatures, and exported
// struct fields.
func parseGoAST(path, src string, includeUnexported bool) ([]Symbol, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		// If AST parsing fails, fall back to regex
		return parseGo(src), nil
	}

	var symbols []Symbol

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			name := d.Name.Name
			kind := "func"

			// Method with receiver
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recv := d.Recv.List[0]
				recvName := astTypeExprString(recv.Type)
				if recvName != "" {
					name = "(" + recvName + ")." + d.Name.Name
					kind = "method"
				}
			}

			if !includeUnexported && !d.Name.IsExported() && kind == "func" {
				continue
			}
			// Always include methods (they're part of a type's interface)
			if kind == "method" && !includeUnexported && !d.Name.IsExported() {
				continue
			}

			symbols = append(symbols, Symbol{
				Name: name,
				Kind: kind,
				Line: fset.Position(d.Pos()).Line,
			})

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if !includeUnexported && !s.Name.IsExported() {
						continue
					}

					kind := goASTTypeKind(s.Type)
					symbols = append(symbols, Symbol{
						Name: s.Name.Name,
						Kind: kind,
						Line: fset.Position(s.Pos()).Line,
					})

					// Extract interface method signatures
					if iface, ok := s.Type.(*ast.InterfaceType); ok && iface.Methods != nil {
						for _, m := range iface.Methods.List {
							for _, mName := range m.Names {
								if !includeUnexported && !mName.IsExported() {
									continue
								}
								sig := formatInterfaceMethodSig(mName.Name, m.Type)
								symbols = append(symbols, Symbol{
									Name: s.Name.Name + "." + sig,
									Kind: "interface_method",
									Line: fset.Position(m.Pos()).Line,
								})
							}
						}
					}

					// Extract exported struct fields
					if st, ok := s.Type.(*ast.StructType); ok && st.Fields != nil {
						for _, f := range st.Fields.List {
							for _, fName := range f.Names {
								if !fName.IsExported() {
									continue
								}
								fieldType := astTypeExprString(f.Type)
								symbols = append(symbols, Symbol{
									Name: s.Name.Name + "." + fName.Name + " " + fieldType,
									Kind: "field",
									Line: fset.Position(f.Pos()).Line,
								})
							}
						}
					}

				case *ast.ValueSpec:
					// Constants and variables (exported only by default)
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, name := range s.Names {
						if !includeUnexported && !name.IsExported() {
							continue
						}
						symbols = append(symbols, Symbol{
							Name: name.Name,
							Kind: kind,
							Line: fset.Position(name.Pos()).Line,
						})
					}
				}
			}
		}
	}

	return symbols, nil
}

// astTypeExprString renders an ast.Expr as a readable type string.
func astTypeExprString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + astTypeExprString(t.X)
	case *ast.SelectorExpr:
		return astTypeExprString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + astTypeExprString(t.Elt)
	case *ast.MapType:
		return "map[" + astTypeExprString(t.Key) + "]" + astTypeExprString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func"
	case *ast.ChanType:
		return "chan " + astTypeExprString(t.Value)
	case *ast.Ellipsis:
		return "..." + astTypeExprString(t.Elt)
	}
	return ""
}

// goASTTypeKind determines the kind of a type spec from its underlying type expr.
func goASTTypeKind(expr ast.Expr) string {
	switch expr.(type) {
	case *ast.InterfaceType:
		return "interface"
	case *ast.StructType:
		return "struct"
	}
	return "type"
}

// formatInterfaceMethodSig formats an interface method name with its type info.
func formatInterfaceMethodSig(name string, typ ast.Expr) string {
	if _, ok := typ.(*ast.FuncType); ok {
		return name + "()"
	}
	return name
}

// ─────────────────────────────────────────────────────────────────────────────
// Python: Enhanced regex with scope tracking
// ─────────────────────────────────────────────────────────────────────────────

var (
	pyClassDefRe    = regexp.MustCompile(`^class\s+(\w+)\s*(?:\(([^)]*)\))?`)
	pyFuncDefRe     = regexp.MustCompile(`^(async\s+)?def\s+(\w+)\s*\(([^)]*)\)`)
	pyDecoratorEnhRe = regexp.MustCompile(`^@([\w.]+)`)
)

// parsePythonEnhanced extracts Python symbols with class/method scope awareness,
// inheritance detection, decorator capture, and async function detection.
func parsePythonEnhanced(src string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(src, "\n")

	var scopeStack []scopeEntry
	var pendingDecorators []string

	for i, line := range lines {
		rawLine := strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(rawLine)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Calculate indentation level
		indent := 0
		for _, ch := range rawLine {
			if ch == ' ' {
				indent++
			} else if ch == '\t' {
				indent += 4
			} else {
				break
			}
		}

		// Pop scope stack for dedented lines
		for len(scopeStack) > 0 && indent <= scopeStack[len(scopeStack)-1].indent {
			scopeStack = scopeStack[:len(scopeStack)-1]
		}

		// Decorator
		if m := pyDecoratorEnhRe.FindStringSubmatch(trimmed); m != nil {
			pendingDecorators = append(pendingDecorators, m[1])
			continue
		}

		// Class definition
		if m := pyClassDefRe.FindStringSubmatch(trimmed); m != nil {
			className := m[1]
			inheritance := strings.TrimSpace(m[2])

			kind := "class"
			if inheritance != "" {
				kind = "class(" + inheritance + ")"
			}

			// Apply decorators
			if len(pendingDecorators) > 0 {
				kind = "@" + strings.Join(pendingDecorators, ",") + " " + kind
				pendingDecorators = nil
			}

			// Build qualified name from scope
			qualName := buildQualName(scopeStack, className)

			symbols = append(symbols, Symbol{
				Name: qualName,
				Kind: kind,
				Line: i + 1,
			})

			scopeStack = append(scopeStack, scopeEntry{name: className, indent: indent})
			continue
		}

		// Function/method definition
		if m := pyFuncDefRe.FindStringSubmatch(trimmed); m != nil {
			isAsync := strings.TrimSpace(m[1]) != ""
			funcName := m[2]

			kind := "func"
			if isAsync {
				kind = "async func"
			}

			// Determine if it's a method (inside a class scope)
			if len(scopeStack) > 0 {
				kind = "method"
				if isAsync {
					kind = "async method"
				}
			}

			// Apply decorators
			if len(pendingDecorators) > 0 {
				kind = "@" + strings.Join(pendingDecorators, ",") + " " + kind
				pendingDecorators = nil
			}

			// Build qualified name from scope
			qualName := buildQualName(scopeStack, funcName)

			symbols = append(symbols, Symbol{
				Name: qualName,
				Kind: kind,
				Line: i + 1,
			})

			scopeStack = append(scopeStack, scopeEntry{name: funcName, indent: indent})
			continue
		}

		// Clear pending decorators if we hit a non-decorator, non-def line
		if len(pendingDecorators) > 0 {
			pendingDecorators = nil
		}
	}

	return symbols
}

// scopeEntry represents a named scope with its indentation level.
type scopeEntry struct {
	name   string
	indent int
}

// buildQualName constructs a qualified name from the scope stack.
func buildQualName(stack []scopeEntry, name string) string {
	if len(stack) == 0 {
		return name
	}
	parts := make([]string, 0, len(stack)+1)
	for _, s := range stack {
		parts = append(parts, s.name)
	}
	parts = append(parts, name)
	return strings.Join(parts, ".")
}

// ─────────────────────────────────────────────────────────────────────────────
// TypeScript/JavaScript: Enhanced regex with export/scope awareness
// ─────────────────────────────────────────────────────────────────────────────

var (
	tsExportFuncRe      = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+(\w+)`)
	tsExportClassRe     = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+(\w+)`)
	tsExportInterfaceRe = regexp.MustCompile(`^(?:export\s+)?interface\s+(\w+)`)
	tsExportTypeRe      = regexp.MustCompile(`^(?:export\s+)?type\s+(\w+)`)
	tsExportEnumRe      = regexp.MustCompile(`^(?:export\s+)?(?:const\s+)?enum\s+(\w+)`)
	tsArrowExportRe     = regexp.MustCompile(`^(?:export\s+)?(?:const|let|var)\s+(\w+)\s*(?::\s*[^=]*)?\s*=\s*(?:async\s+)?(?:\([^)]*\)|[^=])`)
	tsClassMethodRe     = regexp.MustCompile(`^\s+(?:(?:public|private|protected|static|readonly|async|abstract|override)\s+)*(\w+)\s*(?:<[^>]*>)?\s*\(`)
	tsClassPropertyRe   = regexp.MustCompile(`^\s+(?:(?:public|private|protected|static|readonly)\s+)+(\w+)\s*[;:=]`)
	tsReactComponentRe  = regexp.MustCompile(`^(?:export\s+)?(?:const|function)\s+([A-Z]\w+)`)
)

// parseTypeScriptEnhanced extracts TypeScript/JavaScript symbols with awareness
// of exports, classes, interfaces, types, arrow functions, and React components.
func parseTypeScriptEnhanced(src string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(src, "\n")
	seen := make(map[string]bool)

	var currentClass string
	var classIndent int
	inClass := false

	for i, line := range lines {
		rawLine := strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(rawLine)

		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		// Calculate indentation
		indent := 0
		for _, ch := range rawLine {
			if ch == ' ' {
				indent++
			} else if ch == '\t' {
				indent += 4
			} else {
				break
			}
		}

		// Detect if we've left the class scope
		if inClass && indent <= classIndent && trimmed != "}" && trimmed != "" {
			// Check if this is a closing brace or truly out of class
			if !strings.HasPrefix(trimmed, "}") {
				inClass = false
				currentClass = ""
			}
		}

		// Export interface
		if m := tsExportInterfaceRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			if !seen[name] {
				kind := "interface"
				if strings.HasPrefix(trimmed, "export") {
					kind = "export interface"
				}
				symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
				seen[name] = true
			}
			continue
		}

		// Export type
		if m := tsExportTypeRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			if !seen[name] {
				kind := "type"
				if strings.HasPrefix(trimmed, "export") {
					kind = "export type"
				}
				symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
				seen[name] = true
			}
			continue
		}

		// Export enum
		if m := tsExportEnumRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			if !seen[name] {
				kind := "enum"
				if strings.HasPrefix(trimmed, "export") {
					kind = "export enum"
				}
				symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
				seen[name] = true
			}
			continue
		}

		// Export class
		if m := tsExportClassRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			if !seen[name] {
				kind := "class"
				if strings.Contains(trimmed, "abstract") {
					kind = "abstract class"
				}
				if strings.HasPrefix(trimmed, "export") {
					kind = "export " + kind
				}
				symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
				seen[name] = true
			}
			currentClass = m[1]
			classIndent = indent
			inClass = true
			continue
		}

		// Export function (or React component if name starts with uppercase)
		if m := tsExportFuncRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			if !seen[name] {
				kind := "func"
				if len(name) > 0 && unicode.IsUpper(rune(name[0])) {
					kind = "component"
				}
				if strings.Contains(trimmed, "async") && kind == "func" {
					kind = "async func"
				}
				if strings.HasPrefix(trimmed, "export") {
					kind = "export " + kind
				}
				symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
				seen[name] = true
			}
			continue
		}

		// Arrow function export (export const name = (...) => or export const name = function)
		if m := tsArrowExportRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			if !seen[name] {
				// Check if it's a React component (starts with uppercase)
				kind := "func"
				if len(name) > 0 && unicode.IsUpper(rune(name[0])) {
					kind = "component"
				}
				if strings.HasPrefix(trimmed, "export") {
					kind = "export " + kind
				}
				symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
				seen[name] = true
			}
			continue
		}

		// React component detection (function starting with uppercase)
		if m := tsReactComponentRe.FindStringSubmatch(trimmed); m != nil && !seen[m[1]] {
			name := m[1]
			// Only count as component if it hasn't been captured yet
			if !seen[name] {
				kind := "component"
				if strings.HasPrefix(trimmed, "export") {
					kind = "export component"
				}
				symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
				seen[name] = true
			}
			continue
		}

		// Class methods and properties (inside a class body)
		if inClass && indent > classIndent {
			if m := tsClassMethodRe.FindStringSubmatch(rawLine); m != nil {
				name := m[1]
				if name != "if" && name != "for" && name != "while" && name != "return" && name != "switch" && name != "constructor" {
					qualName := currentClass + "." + name
					if !seen[qualName] {
						kind := "method"
						if strings.Contains(trimmed, "static") {
							kind = "static method"
						}
						if strings.Contains(trimmed, "async") {
							kind = "async " + kind
						}
						symbols = append(symbols, Symbol{Name: qualName, Kind: kind, Line: i + 1})
						seen[qualName] = true
					}
				} else if name == "constructor" {
					qualName := currentClass + ".constructor"
					if !seen[qualName] {
						symbols = append(symbols, Symbol{Name: qualName, Kind: "constructor", Line: i + 1})
						seen[qualName] = true
					}
				}
			} else if m := tsClassPropertyRe.FindStringSubmatch(rawLine); m != nil {
				name := m[1]
				qualName := currentClass + "." + name
				if !seen[qualName] {
					kind := "property"
					if strings.Contains(trimmed, "static") {
						kind = "static property"
					}
					symbols = append(symbols, Symbol{Name: qualName, Kind: kind, Line: i + 1})
					seen[qualName] = true
				}
			}
		}
	}

	return symbols
}

// ─────────────────────────────────────────────────────────────────────────────
// Rust: Enhanced regex with pub visibility, impl blocks, traits, derive attrs
// ─────────────────────────────────────────────────────────────────────────────

var (
	rustPubFnRe     = regexp.MustCompile(`^(\s*)(?:(pub(?:\([^)]*\))?)\s+)?(?:(async|const|unsafe)\s+)?fn\s+(\w+)`)
	rustStructRe    = regexp.MustCompile(`^(?:(pub(?:\([^)]*\))?)\s+)?struct\s+(\w+)`)
	rustEnumRe      = regexp.MustCompile(`^(?:(pub(?:\([^)]*\))?)\s+)?enum\s+(\w+)`)
	rustTraitRe     = regexp.MustCompile(`^(?:(pub(?:\([^)]*\))?)\s+)?trait\s+(\w+)`)
	rustImplRe      = regexp.MustCompile(`^impl(?:<[^>]*>)?\s+(?:(\w+)\s+for\s+)?(\w+)`)
	rustTypeAliasRe = regexp.MustCompile(`^(?:(pub(?:\([^)]*\))?)\s+)?type\s+(\w+)`)
	rustDeriveRe    = regexp.MustCompile(`^#\[derive\(([^)]+)\)\]`)
	rustModRe       = regexp.MustCompile(`^(?:(pub(?:\([^)]*\))?)\s+)?mod\s+(\w+)`)
)

// parseRustEnhanced extracts Rust symbols with pub visibility, impl blocks,
// trait definitions, derive attributes, and lifetime stripping for display.
func parseRustEnhanced(src string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(src, "\n")

	var currentImpl string
	var pendingDerives []string
	implIndent := -1

	for i, line := range lines {
		rawLine := strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(rawLine)

		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Calculate indentation
		indent := 0
		for _, ch := range rawLine {
			if ch == ' ' {
				indent++
			} else if ch == '\t' {
				indent += 4
			} else {
				break
			}
		}

		// Check if we've left the impl block
		if implIndent >= 0 && indent <= implIndent && trimmed != "}" {
			if !strings.HasPrefix(trimmed, "}") {
				currentImpl = ""
				implIndent = -1
			}
		}

		// Handle closing brace at impl level
		if trimmed == "}" && indent == implIndent {
			currentImpl = ""
			implIndent = -1
			continue
		}

		// Derive attribute
		if m := rustDeriveRe.FindStringSubmatch(trimmed); m != nil {
			pendingDerives = append(pendingDerives, strings.Split(m[1], ",")...)
			continue
		}

		// Module
		if m := rustModRe.FindStringSubmatch(trimmed); m != nil {
			name := m[2]
			kind := "mod"
			if m[1] != "" {
				kind = "pub mod"
			}
			symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
			continue
		}

		// Impl block
		if m := rustImplRe.FindStringSubmatch(trimmed); m != nil {
			traitName := m[1]
			typeName := m[2]
			if traitName != "" {
				currentImpl = typeName + " (impl " + traitName + ")"
			} else {
				currentImpl = typeName
			}
			implIndent = indent

			kind := "impl"
			name := typeName
			if traitName != "" {
				name = traitName + " for " + typeName
			}
			symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
			continue
		}

		// Struct
		if m := rustStructRe.FindStringSubmatch(trimmed); m != nil {
			name := m[2]
			kind := "struct"
			if m[1] != "" {
				kind = "pub struct"
			}
			if len(pendingDerives) > 0 {
				kind += " #[derive(" + strings.Join(cleanDerives(pendingDerives), ",") + ")]"
				pendingDerives = nil
			}
			symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
			continue
		}

		// Enum
		if m := rustEnumRe.FindStringSubmatch(trimmed); m != nil {
			name := m[2]
			kind := "enum"
			if m[1] != "" {
				kind = "pub enum"
			}
			if len(pendingDerives) > 0 {
				kind += " #[derive(" + strings.Join(cleanDerives(pendingDerives), ",") + ")]"
				pendingDerives = nil
			}
			symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
			continue
		}

		// Trait
		if m := rustTraitRe.FindStringSubmatch(trimmed); m != nil {
			name := m[2]
			kind := "trait"
			if m[1] != "" {
				kind = "pub trait"
			}
			symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
			continue
		}

		// Type alias
		if m := rustTypeAliasRe.FindStringSubmatch(trimmed); m != nil {
			name := m[2]
			kind := "type"
			if m[1] != "" {
				kind = "pub type"
			}
			symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
			continue
		}

		// Function (standalone or inside impl)
		if m := rustPubFnRe.FindStringSubmatch(rawLine); m != nil {
			name := m[4]
			kind := "fn"
			if m[2] != "" {
				kind = "pub fn"
			}
			qualifier := strings.TrimSpace(m[3])
			if qualifier != "" {
				kind = qualifier + " " + kind
			}

			// If inside an impl block, qualify the name
			if currentImpl != "" {
				name = currentImpl + "::" + name
			}

			symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: i + 1})
			continue
		}

		// Clear pending derives if we hit something that's not a derive or blank
		if len(pendingDerives) > 0 && !strings.HasPrefix(trimmed, "#[") {
			pendingDerives = nil
		}
	}

	return symbols
}

// cleanDerives strips whitespace from derive trait names.
func cleanDerives(derives []string) []string {
	out := make([]string, 0, len(derives))
	for _, d := range derives {
		d = strings.TrimSpace(d)
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// Tree Context Renderer (inspired by Aider's TreeContext)
// ─────────────────────────────────────────────────────────────────────────────

// TreeContextRenderer renders symbols with parent scope context, showing
// definition signatures indented under their parent scope like a tree outline.
type TreeContextRenderer struct{}

// RenderTreeContext produces a tree-context rendering of symbols for a file.
// It groups symbols by their parent scope and renders them with proper indentation,
// similar to Aider's TreeContext. If maxLines <= 0, no truncation is applied.
func RenderTreeContext(file string, symbols []Symbol, maxLines int) string {
	if len(symbols) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(file + "\n")

	// Separate top-level symbols from scoped ones
	var topLevel []Symbol
	groups := make(map[string][]Symbol)
	var groupOrder []string

	for _, sym := range symbols {
		parts := strings.SplitN(sym.Name, ".", 2)
		if len(parts) == 2 {
			parent := parts[0]
			if _, exists := groups[parent]; !exists {
				groupOrder = append(groupOrder, parent)
			}
			// Create a child symbol with just the method/field name
			child := Symbol{
				Name: parts[1],
				Kind: sym.Kind,
				Line: sym.Line,
			}
			groups[parent] = append(groups[parent], child)
		} else {
			topLevel = append(topLevel, sym)
		}
	}

	lineCount := 1 // header line

	// Render top-level symbols that are parents (have children)
	rendered := make(map[string]bool)

	for _, parent := range groupOrder {
		children := groups[parent]

		// Find the parent symbol to get its kind
		parentKind := ""
		for _, sym := range topLevel {
			if sym.Name == parent {
				parentKind = sym.Kind
				break
			}
		}

		if parentKind == "" {
			parentKind = "scope"
		}

		// Render parent
		parentLine := fmt.Sprintf("  %s %s:", parentKind, parent)
		if maxLines > 0 && lineCount >= maxLines {
			b.WriteString("  ...\n")
			break
		}
		b.WriteString(parentLine + "\n")
		lineCount++
		rendered[parent] = true

		// Render children
		for _, child := range children {
			if maxLines > 0 && lineCount >= maxLines {
				b.WriteString("    ...\n")
				lineCount++
				break
			}
			childLine := fmt.Sprintf("    %s %s", child.Kind, child.Name)
			b.WriteString(childLine + "\n")
			lineCount++
		}
	}

	// Render remaining top-level symbols (not parents of groups)
	for _, sym := range topLevel {
		if rendered[sym.Name] {
			continue
		}
		if maxLines > 0 && lineCount >= maxLines {
			b.WriteString("  ...\n")
			break
		}
		line := fmt.Sprintf("  %s %s", sym.Kind, sym.Name)
		b.WriteString(line + "\n")
		lineCount++
	}

	return b.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// ParseFileEnhanced: Main entry point replacing regex parsers
// ─────────────────────────────────────────────────────────────────────────────

// ParseFileEnhanced is the new default parser entry point. It dispatches to
// parseGoAST for .go files and enhanced regex parsers for Python, TypeScript,
// JavaScript, and Rust. For other languages, it falls back to the existing
// regex parsers.
func ParseFileEnhanced(path string) ([]Symbol, error) {
	p := NewTreeSitterParser()
	return p.ParseFile(path)
}

// ParseSourceEnhanced parses source code directly without reading from disk.
// Useful for testing.
func ParseSourceEnhanced(src, ext string) ([]Symbol, error) {
	p := NewTreeSitterParser()
	return p.ParseSource(src, ext, "source"+ext)
}

// ─────────────────────────────────────────────────────────────────────────────
// Fallback: delegates to existing regex parsers for unsupported languages
// ─────────────────────────────────────────────────────────────────────────────

func parseFallback(src, ext string) []Symbol {
	switch ext {
	case ".java":
		return parseJava(src)
	case ".c", ".h":
		return parseC(src)
	case ".cpp", ".cc", ".cxx", ".hpp", ".hh":
		return parseCpp(src)
	case ".cs":
		return parseCSharp(src)
	case ".php":
		return parsePHP(src)
	case ".rb":
		return parseRuby(src)
	case ".kt", ".kts":
		return parseKotlin(src)
	case ".swift":
		return parseSwift(src)
	case ".scala", ".sc":
		return parseScala(src)
	case ".lua":
		return parseLua(src)
	case ".dart":
		return parseDart(src)
	case ".ex", ".exs":
		return parseElixir(src)
	case ".hs":
		return parseHaskell(src)
	}
	return nil
}
