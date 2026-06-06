package repomap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

// extractGo parses a Go source file with go/parser and returns its top-level
// symbols and import paths. The bool result is false if the source could not be
// parsed at all (the caller may then fall back to regex extraction).
func extractGo(path string, data []byte) ([]Symbol, []string, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, parser.SkipObjectResolution)
	if err != nil && file == nil {
		return nil, nil, false
	}

	var imports []string
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		if p, uerr := strconv.Unquote(imp.Path.Value); uerr == nil {
			imports = append(imports, p)
		}
	}

	var symbols []Symbol
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			kind := "func"
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = "method"
			}
			name := d.Name.Name
			symbols = append(symbols, Symbol{
				Name:     name,
				Kind:     kind,
				Exported: ast.IsExported(name),
			})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					symbols = append(symbols, Symbol{
						Name:     s.Name.Name,
						Kind:     "type",
						Exported: ast.IsExported(s.Name.Name),
					})
				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, n := range s.Names {
						if n.Name == "_" {
							continue
						}
						symbols = append(symbols, Symbol{
							Name:     n.Name,
							Kind:     kind,
							Exported: ast.IsExported(n.Name),
						})
					}
				}
			}
		}
	}
	return symbols, imports, true
}
