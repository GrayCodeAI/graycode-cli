// Package testaudit provides meta-audit tests that enforce architectural
// invariants across the hawk codebase using go/ast analysis.
//
// These tests parse the source tree and fail when forbidden patterns are
// detected (e.g., raw panic calls, direct os.Getenv usage, missing doc
// comments on exported types).
package testaudit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parsedFile holds the AST and metadata for a single Go source file.
type parsedFile struct {
	Path string
	File *ast.File
	FSet *token.FileSet
}

// parseInternalPackages walks the internal/ directory tree rooted at root,
// parses all non-test .go files, and returns them keyed by package import path.
func parseInternalPackages(t *testing.T, root string) []parsedFile {
	t.Helper()
	return parseGoFiles(t, root)
}

// parseGoFiles walks a directory tree, parses all non-test .go files, and returns them.
func parseGoFiles(t *testing.T, root string) []parsedFile {
	t.Helper()

	var files []parsedFile

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Logf("warning: could not parse %s: %v", path, err)
			return nil
		}

		files = append(files, parsedFile{Path: path, File: f, FSet: fset})
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk internal/: %v", err)
	}

	return files
}

// isExemptPackage reports whether the file path belongs to an exempt package.
func isExemptPackage(filePath string, exemptions map[string]bool) bool {
	for exempt := range exemptions {
		if strings.Contains(filePath, exempt) {
			return true
		}
	}
	return false
}

// relPath returns the path relative to the hawk repo root for cleaner test output.
func relPath(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return abs
	}
	return rel
}

// callExprName extracts the fully qualified function name from a call expression.
// Returns "" if the call is not a simple selector or ident.
func callExprName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		if x, ok := fn.X.(*ast.Ident); ok {
			return x.Name + "." + fn.Sel.Name
		}
	}
	return ""
}

// isInsideInit reports whether pos is inside an init() function declaration
// in the given file.
func isInsideInit(file *ast.File, pos token.Pos, fset *token.FileSet) bool {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name.Name != "init" {
			continue
		}
		if pos > fn.Pos() && pos < fn.End() {
			return true
		}
	}
	return false
}

// isInsideMustFunction reports whether pos is inside a Must* function.
// The Go convention (MustCompile, MustPut, etc.) allows panics in functions
// prefixed with "Must" — these are intended for package-level initializers.
func isInsideMustFunction(file *ast.File, pos token.Pos) bool {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if !strings.HasPrefix(fn.Name.Name, "Must") {
			continue
		}
		if pos > fn.Pos() && pos < fn.End() {
			return true
		}
	}
	return false
}
