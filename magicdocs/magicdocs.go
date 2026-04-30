package magicdocs

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// DocEntry represents a generated documentation entry.
type DocEntry struct {
	Package string `json:"package"`
	Name    string `json:"name"`
	Type    string `json:"type"` // function, type, method, variable
	Doc     string `json:"doc"`
	File    string `json:"file"`
	Line    int    `json:"line"`
}

// Extract extracts documentation from Go source files.
func Extract(dir string) ([]DocEntry, error) {
	var entries []DocEntry
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil // skip unparseable files
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				name := d.Name.Name
				doc := ""
				if d.Doc != nil {
					doc = d.Doc.Text()
				}
				entries = append(entries, DocEntry{
					Package: f.Name.Name,
					Name:    name,
					Type:    "function",
					Doc:     doc,
					File:    path,
					Line:    fset.Position(d.Pos()).Line,
				})
			case *ast.GenDecl:
				if d.Tok == token.TYPE {
					for _, spec := range d.Specs {
						ts := spec.(*ast.TypeSpec)
						doc := ""
						if d.Doc != nil {
							doc = d.Doc.Text()
						}
						entries = append(entries, DocEntry{
							Package: f.Name.Name,
							Name:    ts.Name.Name,
							Type:    "type",
							Doc:     doc,
							File:    path,
							Line:    fset.Position(ts.Pos()).Line,
						})
					}
				}
			}
		}
		return nil
	})
	return entries, err
}

// GenerateMarkdown generates markdown documentation.
func GenerateMarkdown(entries []DocEntry) string {
	var b strings.Builder
	b.WriteString("# API Documentation\n\n")
	packages := make(map[string][]DocEntry)
	for _, e := range entries {
		packages[e.Package] = append(packages[e.Package], e)
	}
	for pkg, pkgEntries := range packages {
		fmt.Fprintf(&b, "## Package %s\n\n", pkg)
		for _, e := range pkgEntries {
			fmt.Fprintf(&b, "### %s\n\n", e.Name)
			fmt.Fprintf(&b, "- **Type**: %s\n", e.Type)
			fmt.Fprintf(&b, "- **File**: %s:%d\n\n", e.File, e.Line)
			if e.Doc != "" {
				b.WriteString(e.Doc)
				b.WriteString("\n\n")
			}
		}
	}
	return b.String()
}
