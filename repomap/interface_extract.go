package repomap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// InterfaceExtraction shows only exported signatures (no bodies).
// Uses ~100 tokens per file vs ~500+ for full content.
type InterfaceExtraction struct {
	Functions  []string // "func Name(args) returns"
	Types      []string // "type Name struct/interface"
	Constants  []string // "const Name = ..."
	Package    string
}

// ExtractInterface parses a Go file and returns only its exported API surface.
func ExtractInterface(filePath string) (*InterfaceExtraction, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return extractInterfaceFallback(filePath)
	}

	ie := &InterfaceExtraction{
		Package: file.Name.Name,
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.IsExported() {
				sig := formatFuncSig(fset, d)
				ie.Functions = append(ie.Functions, sig)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name.IsExported() {
						ie.Types = append(ie.Types, formatTypeSpec(s))
					}
				case *ast.ValueSpec:
					for _, name := range s.Names {
						if name.IsExported() {
							ie.Constants = append(ie.Constants, name.Name)
						}
					}
				}
			}
		}
	}
	return ie, nil
}

// Format returns the interface as a compact string.
func (ie *InterfaceExtraction) Format() string {
	var b strings.Builder
	b.WriteString("package " + ie.Package + "\n")
	for _, t := range ie.Types {
		b.WriteString(t + "\n")
	}
	for _, f := range ie.Functions {
		b.WriteString(f + "\n")
	}
	return b.String()
}

// TokenEstimate returns approximate token count for this interface.
func (ie *InterfaceExtraction) TokenEstimate() int {
	total := len(ie.Functions) + len(ie.Types) + len(ie.Constants)
	return total * 15 // ~15 tokens per signature
}

func formatFuncSig(fset *token.FileSet, fn *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString("func ")
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := fn.Recv.List[0]
		b.WriteString("(")
		if len(recv.Names) > 0 {
			b.WriteString(recv.Names[0].Name + " ")
		}
		b.WriteString(exprString(recv.Type))
		b.WriteString(") ")
	}
	b.WriteString(fn.Name.Name)
	b.WriteString("(")
	if fn.Type.Params != nil {
		b.WriteString(fieldListString(fn.Type.Params))
	}
	b.WriteString(")")
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		b.WriteString(" ")
		if len(fn.Type.Results.List) == 1 && len(fn.Type.Results.List[0].Names) == 0 {
			b.WriteString(exprString(fn.Type.Results.List[0].Type))
		} else {
			b.WriteString("(" + fieldListString(fn.Type.Results) + ")")
		}
	}
	return b.String()
}

func formatTypeSpec(ts *ast.TypeSpec) string {
	kind := "struct"
	switch ts.Type.(type) {
	case *ast.InterfaceType:
		kind = "interface"
	case *ast.StructType:
		kind = "struct"
	default:
		return "type " + ts.Name.Name + " " + exprString(ts.Type)
	}
	return "type " + ts.Name.Name + " " + kind
}

func fieldListString(fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}
	var parts []string
	for _, f := range fl.List {
		typStr := exprString(f.Type)
		if len(f.Names) == 0 {
			parts = append(parts, typStr)
		} else {
			for _, name := range f.Names {
				parts = append(parts, name.Name+" "+typStr)
			}
		}
	}
	return strings.Join(parts, ", ")
}

func exprString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + exprString(e.X)
	case *ast.SelectorExpr:
		return exprString(e.X) + "." + e.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprString(e.Elt)
	case *ast.MapType:
		return "map[" + exprString(e.Key) + "]" + exprString(e.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.Ellipsis:
		return "..." + exprString(e.Elt)
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + exprString(e.Value)
	}
	return "?"
}

func extractInterfaceFallback(filePath string) (*InterfaceExtraction, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	ie := &InterfaceExtraction{}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") {
			ie.Package = strings.TrimPrefix(trimmed, "package ")
		}
		if strings.HasPrefix(trimmed, "func ") && len(trimmed) > 5 {
			c := trimmed[5]
			if c >= 'A' && c <= 'Z' {
				if brace := strings.Index(trimmed, "{"); brace > 0 {
					ie.Functions = append(ie.Functions, strings.TrimSpace(trimmed[:brace]))
				}
			}
		}
		if strings.HasPrefix(trimmed, "type ") && len(trimmed) > 5 {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 && len(parts[1]) > 0 && parts[1][0] >= 'A' && parts[1][0] <= 'Z' {
				ie.Types = append(ie.Types, trimmed)
			}
		}
	}
	return ie, nil
}
