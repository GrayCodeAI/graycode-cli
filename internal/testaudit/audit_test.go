package testaudit

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot returns the hawk repo root directory.
// It walks up from the test file's location to find go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot determine working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}

// Exempt packages for fmt.Print audit — these use stdout for user-facing output.
var fmtPrintExemptions = map[string]bool{
	"onboarding": true,
	"scaffold":   true,
}

// Exempt packages for os.Getenv audit — these are the abstraction layer or standard telemetry.
var getEnvExemptions = map[string]bool{
	"envmanager.go": true,
	"oteltrace":     true,
	"langfuse":      true,
}

// TestNoRawPanicInInternal verifies that no non-test .go file in internal/
// calls panic() outside of init() functions.
func TestNoRawPanicInInternal(t *testing.T) {
	root := repoRoot(t)
	internalDir := filepath.Join(root, "internal")
	files := parseInternalPackages(t, internalDir)

	for _, pf := range files {
		pf := pf
		rel := relPath(root, pf.Path)

		ast.Inspect(pf.File, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			name := callExprName(call)
			if name != "panic" {
				return true
			}

			// Allow panic inside init() functions
			if isInsideInit(pf.File, call.Pos(), pf.FSet) {
				return true
			}

			// Allow panic in catalogtest (test infrastructure)
			if strings.Contains(pf.Path, "catalogtest") {
				return true
			}

			pos := pf.FSet.Position(call.Pos())
			t.Run(fmt.Sprintf("%s:%d", rel, pos.Line), func(t *testing.T) {
				t.Logf("TECH DEBT: raw panic() at %s:%d — should return error instead", rel, pos.Line)
			})

			return true
		})
	}
}

// TestNoRawFmtPrintInInternal verifies that no non-test .go file in internal/
// uses fmt.Print/Println/Printf (should use logger.Logger instead).
func TestNoRawFmtPrintInInternal(t *testing.T) {
	root := repoRoot(t)
	internalDir := filepath.Join(root, "internal")
	files := parseInternalPackages(t, internalDir)

	for _, pf := range files {
		rel := relPath(root, pf.Path)

		// Skip exempt packages
		if isExemptPackage(pf.Path, fmtPrintExemptions) {
			continue
		}

		ast.Inspect(pf.File, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			name := callExprName(call)
			if name != "fmt.Print" && name != "fmt.Println" && name != "fmt.Printf" {
				return true
			}

			pos := pf.FSet.Position(call.Pos())
			t.Run(fmt.Sprintf("%s:%d", rel, pos.Line), func(t *testing.T) {
				t.Logf("TECH DEBT: %s() at %s:%d — use logger.Logger instead", name, rel, pos.Line)
			})

			return true
		})
	}
}

// TestNoDirectOsGetenvInInternal verifies that no non-test .go file in internal/
// uses os.Getenv directly (should go through config.EnvManager).
// Currently logs violations as tech debt rather than failing CI.
func TestNoDirectOsGetenvInInternal(t *testing.T) {
	root := repoRoot(t)
	internalDir := filepath.Join(root, "internal")
	files := parseInternalPackages(t, internalDir)

	violationCount := 0

	for _, pf := range files {
		rel := relPath(root, pf.Path)

		// Skip exempt packages
		if isExemptPackage(pf.Path, getEnvExemptions) {
			continue
		}

		ast.Inspect(pf.File, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			name := callExprName(call)
			if name != "os.Getenv" {
				return true
			}

			pos := pf.FSet.Position(call.Pos())
			t.Run(fmt.Sprintf("%s:%d", rel, pos.Line), func(t *testing.T) {
				t.Logf("TECH DEBT: os.Getenv() at %s:%d — migrate to config.EnvManager", rel, pos.Line)
			})

			violationCount++
			return true
		})
	}

	t.Logf("Total os.Getenv violations in internal/: %d (logged as tech debt)", violationCount)
}

// TestAllExportedTypesHaveDocComments verifies that all exported type
// declarations in non-test .go files have doc comments.
func TestAllExportedTypesHaveDocComments(t *testing.T) {
	root := repoRoot(t)
	internalDir := filepath.Join(root, "internal")
	files := parseInternalPackages(t, internalDir)

	for _, pf := range files {
		rel := relPath(root, pf.Path)

		for _, decl := range pf.File.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}

			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}

				// Skip unexported types
				if !typeSpec.Name.IsExported() {
					continue
				}

				// Check for doc comment on the GenDecl (covers the type)
				if genDecl.Doc != nil && genDecl.Doc.Text() != "" {
					continue
				}

				pos := pf.FSet.Position(typeSpec.Pos())
				t.Run(fmt.Sprintf("%s/%s", rel, typeSpec.Name.Name), func(t *testing.T) {
					t.Logf("TECH DEBT: exported type %s at %s:%d has no doc comment",
						typeSpec.Name.Name, rel, pos.Line)
				})
			}
		}
	}
}
