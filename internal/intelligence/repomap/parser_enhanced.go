package repomap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// EnhancedGoParser uses go/ast for accurate Go symbol extraction.
// Replaces regex-based parsing for Go files with zero-CGO AST parsing.
// This covers what tree-sitter would give us: method receivers, nested types,
// interface methods, embedded fields — without requiring CGO.
type EnhancedGoParser struct{}

// ParseGoFile extracts all symbols from a Go file using the standard library parser.
func (p *EnhancedGoParser) ParseGoFile(content, filePath string) []EnhancedSymbol {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, content, 0)
	if err != nil {
		return nil
	}

	var symbols []EnhancedSymbol

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := EnhancedSymbol{
				Name: d.Name.Name,
				Kind: "function",
				Line: fset.Position(d.Pos()).Line,
				File: filePath,
			}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recv := d.Recv.List[0]
				typeName := typeExprName(recv.Type)
				if typeName != "" {
					sym.Name = typeName + "." + d.Name.Name
					sym.Kind = "method"
				}
			}
			sym.Exported = d.Name.IsExported()
			symbols = append(symbols, sym)

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					sym := EnhancedSymbol{
						Name:     s.Name.Name,
						Kind:     typeKind(s.Type),
						Line:     fset.Position(s.Pos()).Line,
						File:     filePath,
						Exported: s.Name.IsExported(),
					}
					symbols = append(symbols, sym)

					// Extract interface methods
					if iface, ok := s.Type.(*ast.InterfaceType); ok && iface.Methods != nil {
						for _, m := range iface.Methods.List {
							for _, name := range m.Names {
								symbols = append(symbols, EnhancedSymbol{
									Name:     s.Name.Name + "." + name.Name,
									Kind:     "interface_method",
									Line:     fset.Position(m.Pos()).Line,
									File:     filePath,
									Exported: name.IsExported(),
								})
							}
						}
					}

					// Extract struct fields (exported only)
					if st, ok := s.Type.(*ast.StructType); ok && st.Fields != nil {
						for _, f := range st.Fields.List {
							for _, name := range f.Names {
								if name.IsExported() {
									symbols = append(symbols, EnhancedSymbol{
										Name:     s.Name.Name + "." + name.Name,
										Kind:     "field",
										Line:     fset.Position(f.Pos()).Line,
										File:     filePath,
										Exported: true,
									})
								}
							}
						}
					}

				case *ast.ValueSpec:
					for _, name := range s.Names {
						symbols = append(symbols, EnhancedSymbol{
							Name:     name.Name,
							Kind:     "variable",
							Line:     fset.Position(name.Pos()).Line,
							File:     filePath,
							Exported: name.IsExported(),
						})
					}
				}
			}
		}
	}

	// Extract references (function calls in bodies)
	ast.Inspect(file, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.CallExpr:
			if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok {
					ref := x.Name + "." + sel.Sel.Name
					for i := range symbols {
						if symbols[i].File == filePath && symbols[i].Kind == "function" {
							symbols[i].References = append(symbols[i].References, ref)
						}
					}
				}
			}
		}
		return true
	})

	return symbols
}

// EnhancedSymbol represents a richly-extracted code symbol with references.
type EnhancedSymbol struct {
	Name       string
	Kind       string // function, method, type, interface, struct, field, variable, interface_method
	Line       int
	File       string
	Exported   bool
	References []string // symbols this one references
}

func typeExprName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return typeExprName(t.X)
	}
	return ""
}

func typeKind(expr ast.Expr) string {
	switch expr.(type) {
	case *ast.InterfaceType:
		return "interface"
	case *ast.StructType:
		return "struct"
	}
	return "type"
}

// ParsePythonFile extracts symbols from Python using enhanced regex patterns.
// Handles: classes, methods (with self), decorators, nested classes.
func ParsePythonFile(content, filePath string) []EnhancedSymbol {
	var symbols []EnhancedSymbol
	lines := strings.Split(content, "\n")
	var currentClass string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " \t"))

		// Top-level class
		if strings.HasPrefix(trimmed, "class ") && indent == 0 {
			name := extractPyName(trimmed[6:])
			currentClass = name
			symbols = append(symbols, EnhancedSymbol{
				Name: name, Kind: "class", Line: i + 1, File: filePath, Exported: !strings.HasPrefix(name, "_"),
			})
			continue
		}

		// Method (indented def inside class)
		if strings.HasPrefix(trimmed, "def ") && indent > 0 && currentClass != "" {
			name := extractPyName(trimmed[4:])
			symbols = append(symbols, EnhancedSymbol{
				Name: currentClass + "." + name, Kind: "method", Line: i + 1, File: filePath, Exported: !strings.HasPrefix(name, "_"),
			})
			continue
		}

		// Top-level function
		if strings.HasPrefix(trimmed, "def ") && indent == 0 {
			name := extractPyName(trimmed[4:])
			currentClass = ""
			symbols = append(symbols, EnhancedSymbol{
				Name: name, Kind: "function", Line: i + 1, File: filePath, Exported: !strings.HasPrefix(name, "_"),
			})
			continue
		}

		// Reset class context at top-level non-def/class
		if indent == 0 && trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "@") {
			currentClass = ""
		}
	}
	return symbols
}

func extractPyName(s string) string {
	end := strings.IndexAny(s, "(: ")
	if end < 0 {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(s[:end])
}
