package repomap

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// ── Helper: create a temp Go module ──

// createGoModule sets up a temp directory with a go.mod and several packages
// that import each other:
//
//	mymod/
//	  go.mod              (module mymod)
//	  main.go             (imports mymod/pkg/auth, mymod/pkg/models)
//	  pkg/auth/auth.go    (imports mymod/pkg/models)
//	  pkg/models/user.go  (no local imports)
//	  pkg/api/routes.go   (imports mymod/pkg/auth)
func createGoModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeFile(t, root, "go.mod", "module mymod\n\ngo 1.21\n")

	writeFile(t, root, "main.go", `package main

import (
	"fmt"
	"mymod/pkg/auth"
	"mymod/pkg/models"
)

func main() {
	fmt.Println(auth.Check(), models.User{})
}
`)

	writeFile(t, root, "pkg/auth/auth.go", `package auth

import "mymod/pkg/models"

func Check() bool {
	_ = models.User{}
	return true
}
`)

	writeFile(t, root, "pkg/models/user.go", `package models

type User struct {
	Name string
}
`)

	writeFile(t, root, "pkg/api/routes.go", `package api

import "mymod/pkg/auth"

func SetupRoutes() {
	_ = auth.Check()
}
`)

	return root
}

func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	abs := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", relPath, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

// ── Go import graph tests ──

func TestBuildImportGraph_GoModule(t *testing.T) {
	root := createGoModule(t)

	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatalf("BuildImportGraph failed: %v", err)
	}

	// main.go should import auth and models files
	mainDeps := g.edges["main.go"]
	if len(mainDeps) == 0 {
		t.Fatal("expected main.go to have dependencies")
	}
	assertContains(t, mainDeps, "pkg/auth/auth.go", "main.go should depend on auth")
	assertContains(t, mainDeps, "pkg/models/user.go", "main.go should depend on models")

	// auth.go should import models
	authDeps := g.edges["pkg/auth/auth.go"]
	assertContains(t, authDeps, "pkg/models/user.go", "auth should depend on models")

	// models/user.go should have no local dependencies
	modelDeps := g.edges["pkg/models/user.go"]
	if len(modelDeps) != 0 {
		t.Errorf("expected models/user.go to have no local deps, got %v", modelDeps)
	}

	// routes.go should import auth
	routeDeps := g.edges["pkg/api/routes.go"]
	assertContains(t, routeDeps, "pkg/auth/auth.go", "routes should depend on auth")
}

func TestImportGraph_DependenciesOf_Depth1(t *testing.T) {
	root := createGoModule(t)
	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	// main.go imports auth and models at depth 1
	deps := g.DependenciesOf("main.go", 1)
	assertContains(t, deps, "pkg/auth/auth.go", "depth-1 deps of main.go")
	assertContains(t, deps, "pkg/models/user.go", "depth-1 deps of main.go")
}

func TestImportGraph_DependenciesOf_Depth2(t *testing.T) {
	root := createGoModule(t)
	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	// routes.go -> auth -> models at depth 2
	deps := g.DependenciesOf("pkg/api/routes.go", 2)
	assertContains(t, deps, "pkg/auth/auth.go", "depth-2 should include auth")
	assertContains(t, deps, "pkg/models/user.go", "depth-2 should include models via auth")
}

func TestImportGraph_DependentsOf(t *testing.T) {
	root := createGoModule(t)
	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	// Who depends on models/user.go?
	dependents := g.DependentsOf("pkg/models/user.go", 1)
	assertContains(t, dependents, "main.go", "main.go imports models")
	assertContains(t, dependents, "pkg/auth/auth.go", "auth imports models")

	// Who depends on auth/auth.go?
	authDependents := g.DependentsOf("pkg/auth/auth.go", 1)
	assertContains(t, authDependents, "main.go", "main.go imports auth")
	assertContains(t, authDependents, "pkg/api/routes.go", "routes imports auth")
}

func TestImportGraph_DependentsOf_Depth2(t *testing.T) {
	root := createGoModule(t)
	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	// At depth 2, dependents of models should include routes (via auth)
	dependents := g.DependentsOf("pkg/models/user.go", 2)
	assertContains(t, dependents, "pkg/api/routes.go", "depth-2 dependents should reach routes via auth")
}

func TestImportGraph_ImpactSet(t *testing.T) {
	root := createGoModule(t)
	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	// Impact set for auth/auth.go should include models (dependency) and main, routes (dependents)
	impact := g.ImpactSet([]string{"pkg/auth/auth.go"}, 1)
	assertContains(t, impact, "pkg/models/user.go", "auth's dependency")
	assertContains(t, impact, "main.go", "auth's dependent")
	assertContains(t, impact, "pkg/api/routes.go", "auth's dependent")

	// auth.go itself should NOT be in the impact set
	assertNotContains(t, impact, "pkg/auth/auth.go", "input file should not be in impact set")
}

func TestImportGraph_ImpactSet_MultipleFiles(t *testing.T) {
	root := createGoModule(t)
	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	// Impact of both auth.go and routes.go
	impact := g.ImpactSet([]string{"pkg/auth/auth.go", "pkg/api/routes.go"}, 1)

	// Should not include the input files themselves
	assertNotContains(t, impact, "pkg/auth/auth.go", "input files excluded")
	assertNotContains(t, impact, "pkg/api/routes.go", "input files excluded")

	// Should include models (auth's dep) and main (auth's dependent)
	assertContains(t, impact, "pkg/models/user.go", "models is auth's dependency")
	assertContains(t, impact, "main.go", "main depends on auth")
}

func TestImportGraph_ExternalImportsIgnored(t *testing.T) {
	root := createGoModule(t)
	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	// main.go imports "fmt" which is external -- should not appear
	for _, dep := range g.edges["main.go"] {
		if dep == "fmt" {
			t.Error("external import 'fmt' should not be in edges")
		}
	}
}

func TestImportGraph_EmptyDirectory(t *testing.T) {
	root := t.TempDir()

	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatalf("BuildImportGraph on empty dir should succeed: %v", err)
	}

	if len(g.edges) != 0 {
		t.Errorf("expected no edges for empty dir, got %d", len(g.edges))
	}
}

// ── Python import tests ──

func TestBuildImportGraph_Python(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "app.py", `import models
from utils import helper
`)
	writeFile(t, root, "models.py", `class User:
    pass
`)
	writeFile(t, root, "utils.py", `def helper():
    pass
`)

	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	appDeps := g.edges["app.py"]
	assertContains(t, appDeps, "models.py", "app.py imports models")
	assertContains(t, appDeps, "utils.py", "app.py imports utils")
}

func TestBuildImportGraph_PythonPackage(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "main.py", `from pkg.sub import thing
`)
	writeFile(t, root, "pkg/sub.py", `def thing():
    pass
`)

	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	mainDeps := g.edges["main.py"]
	assertContains(t, mainDeps, filepath.Join("pkg", "sub.py"), "main.py imports pkg.sub")
}

// ── TypeScript import tests ──

func TestBuildImportGraph_TypeScript(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "app.ts", `import { Router } from './router'
import { User } from './models/user'
import express from 'express'
`)
	writeFile(t, root, "router.ts", `import { User } from './models/user'
export function Router() {}
`)
	writeFile(t, root, "models/user.ts", `export interface User {
    name: string
}
`)

	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	appDeps := g.edges["app.ts"]
	assertContains(t, appDeps, "router.ts", "app.ts imports router")
	assertContains(t, appDeps, filepath.Join("models", "user.ts"), "app.ts imports models/user")

	// External import 'express' should not appear
	for _, d := range appDeps {
		if d == "express" {
			t.Error("external import 'express' should not be in edges")
		}
	}

	// router.ts should import models/user
	routerDeps := g.edges["router.ts"]
	assertContains(t, routerDeps, filepath.Join("models", "user.ts"), "router imports models/user")

	// Reverse: models/user.ts should have app.ts and router.ts as dependents
	userDependents := g.reverse[filepath.Join("models", "user.ts")]
	assertContains(t, userDependents, "app.ts", "app.ts depends on user")
	assertContains(t, userDependents, "router.ts", "router.ts depends on user")
}

// ── parseGoImports unit tests ──

func TestParseGoImports_SingleImport(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "single.go")
	content := `package main

import "fmt"

func main() {}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	imports := parseGoImports(path)
	if len(imports) != 1 || imports[0] != "fmt" {
		t.Errorf("expected [fmt], got %v", imports)
	}
}

func TestParseGoImports_BlockImport(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "block.go")
	content := `package main

import (
	"fmt"
	"os"
	myalias "mymod/pkg"
)

func main() {}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	imports := parseGoImports(path)
	expected := []string{"fmt", "os", "mymod/pkg"}
	sort.Strings(imports)
	sort.Strings(expected)

	if len(imports) != len(expected) {
		t.Fatalf("expected %d imports, got %d: %v", len(expected), len(imports), imports)
	}
	for i, imp := range imports {
		if imp != expected[i] {
			t.Errorf("import[%d]: expected %q, got %q", i, expected[i], imp)
		}
	}
}

// ── detectModulePath tests ──

func TestDetectModulePath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module github.com/example/myapp\n\ngo 1.21\n")

	mod := detectModulePath(root)
	if mod != "github.com/example/myapp" {
		t.Errorf("expected github.com/example/myapp, got %q", mod)
	}
}

func TestDetectModulePath_NoGoMod(t *testing.T) {
	root := t.TempDir()
	mod := detectModulePath(root)
	if mod != "" {
		t.Errorf("expected empty module path, got %q", mod)
	}
}

// ── dedup tests ──

func TestDedup(t *testing.T) {
	input := []string{"c", "a", "b", "a", "c"}
	got := dedup(input)
	expected := []string{"a", "b", "c"}
	if len(got) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("dedup[%d]: expected %q, got %q", i, expected[i], got[i])
		}
	}
}

func TestDedup_Empty(t *testing.T) {
	got := dedup(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// ── BFS boundary tests ──

func TestImportGraph_DependenciesOf_ZeroDepth(t *testing.T) {
	root := createGoModule(t)
	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	// maxDepth 0 should behave as maxDepth 1 (clamped)
	deps := g.DependenciesOf("main.go", 0)
	if len(deps) == 0 {
		t.Error("maxDepth 0 should be clamped to 1 and return dependencies")
	}
}

func TestImportGraph_DependentsOf_NonexistentFile(t *testing.T) {
	root := createGoModule(t)
	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	deps := g.DependentsOf("nonexistent.go", 1)
	if len(deps) != 0 {
		t.Errorf("expected no dependents for nonexistent file, got %v", deps)
	}
}

// ── Assertion helpers ──

func assertContains(t *testing.T, slice []string, want, msg string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Errorf("%s: %v does not contain %q", msg, slice, want)
}

func assertNotContains(t *testing.T, slice []string, unwanted, msg string) {
	t.Helper()
	for _, s := range slice {
		if s == unwanted {
			t.Errorf("%s: %v should not contain %q", msg, slice, unwanted)
			return
		}
	}
}
