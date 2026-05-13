package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// createTestGoFile creates a temporary Go source file for testing.
func createTestGoFile(t *testing.T, dir, filename, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create test file %s: %v", filename, err)
	}
}

func TestParseGoPackage_ExtractsFunctionsAndTypes(t *testing.T) {
	dir := t.TempDir()

	src := `// Package auth provides authentication utilities.
package auth

import "time"

// Claims represents JWT claims.
type Claims struct {
	// UserID is the unique user identifier.
	UserID string
	// ExpiresAt is the token expiry time.
	ExpiresAt time.Time
}

// Validator checks tokens.
type Validator interface {
	// Validate checks if a token is valid.
	Validate(token string) error
}

// ValidateToken validates a JWT token and returns the claims.
func ValidateToken(token string) (*Claims, error) {
	return nil, nil
}

// NewClaims creates new claims for a user.
func NewClaims(userID string, ttl time.Duration) *Claims {
	return nil
}

// helper is an unexported function.
func helper() {}
`
	createTestGoFile(t, dir, "auth.go", src)

	dg := NewDocGenerator(dir)
	pkg, err := dg.parseGoPackage(dir)
	if err != nil {
		t.Fatalf("parseGoPackage failed: %v", err)
	}

	if pkg == nil {
		t.Fatal("expected non-nil package doc")
	}

	if pkg.Name != "auth" {
		t.Errorf("expected package name 'auth', got '%s'", pkg.Name)
	}

	if pkg.Description != "Package auth provides authentication utilities." {
		t.Errorf("unexpected package description: %s", pkg.Description)
	}

	// Should have 2 exported functions (helper is unexported)
	if len(pkg.Functions) != 2 {
		t.Errorf("expected 2 exported functions, got %d", len(pkg.Functions))
		for _, f := range pkg.Functions {
			t.Logf("  function: %s", f.Name)
		}
	}

	// Check ValidateToken
	var validateFn *FunctionDoc
	for i := range pkg.Functions {
		if pkg.Functions[i].Name == "ValidateToken" {
			validateFn = &pkg.Functions[i]
			break
		}
	}
	if validateFn == nil {
		t.Fatal("expected to find ValidateToken function")
	}
	if !validateFn.Exported {
		t.Error("ValidateToken should be exported")
	}
	if validateFn.Description != "ValidateToken validates a JWT token and returns the claims." {
		t.Errorf("unexpected description: %s", validateFn.Description)
	}
	if len(validateFn.Parameters) != 1 {
		t.Errorf("expected 1 parameter, got %d", len(validateFn.Parameters))
	} else {
		if validateFn.Parameters[0].Name != "token" {
			t.Errorf("expected param name 'token', got '%s'", validateFn.Parameters[0].Name)
		}
	}
	if !strings.Contains(validateFn.Returns, "*Claims") {
		t.Errorf("expected returns to contain '*Claims', got '%s'", validateFn.Returns)
	}

	// Should have 2 types: Claims (struct) and Validator (interface)
	if len(pkg.Types) != 2 {
		t.Errorf("expected 2 types, got %d", len(pkg.Types))
		for _, typ := range pkg.Types {
			t.Logf("  type: %s (%s)", typ.Name, typ.Kind)
		}
	}

	var claimsType *TypeDoc
	var validatorType *TypeDoc
	for i := range pkg.Types {
		switch pkg.Types[i].Name {
		case "Claims":
			claimsType = &pkg.Types[i]
		case "Validator":
			validatorType = &pkg.Types[i]
		}
	}

	if claimsType == nil {
		t.Fatal("expected to find Claims type")
	}
	if claimsType.Kind != "struct" {
		t.Errorf("expected Claims kind 'struct', got '%s'", claimsType.Kind)
	}
	if len(claimsType.Fields) != 2 {
		t.Errorf("expected 2 fields in Claims, got %d", len(claimsType.Fields))
	}

	if validatorType == nil {
		t.Fatal("expected to find Validator type")
	}
	if validatorType.Kind != "interface" {
		t.Errorf("expected Validator kind 'interface', got '%s'", validatorType.Kind)
	}
}

func TestRenderMarkdown_ProducesValidOutput(t *testing.T) {
	doc := &ProjectDoc{
		Name:        "myproject",
		Description: "A test project for documentation.",
		Architecture: "Simple package layout.",
		QuickStart:  "import \"myproject\"",
		GeneratedAt: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Packages: []PackageDoc{
			{
				Name:        "auth",
				Path:        "pkg/auth",
				Description: "Authentication package.",
				Functions: []FunctionDoc{
					{
						Name:        "ValidateToken",
						Signature:   "func ValidateToken(token string) (*Claims, error)",
						Description: "Validates a JWT token.",
						Exported:    true,
					},
				},
				Types: []TypeDoc{
					{
						Name:        "Claims",
						Kind:        "struct",
						Description: "JWT claims.",
						Fields: []FieldDoc{
							{Name: "UserID", Type: "string", Desc: "User identifier"},
						},
						Methods: []FunctionDoc{
							{
								Name:      "IsExpired",
								Signature: "func (c *Claims) IsExpired() bool",
								Exported:  true,
							},
						},
					},
				},
			},
		},
	}

	md := RenderMarkdown(doc)

	// Check title
	if !strings.Contains(md, "# myproject") {
		t.Error("markdown should contain project title")
	}

	// Check description
	if !strings.Contains(md, "A test project for documentation.") {
		t.Error("markdown should contain description")
	}

	// Check architecture section
	if !strings.Contains(md, "## Architecture") {
		t.Error("markdown should contain Architecture section")
	}

	// Check quick start section
	if !strings.Contains(md, "## Quick Start") {
		t.Error("markdown should contain Quick Start section")
	}

	// Check package section
	if !strings.Contains(md, "### package auth") {
		t.Error("markdown should contain package header")
	}

	// Check function signature
	if !strings.Contains(md, "func ValidateToken(token string) (*Claims, error)") {
		t.Error("markdown should contain function signature")
	}

	// Check type
	if !strings.Contains(md, "`type Claims struct`") {
		t.Error("markdown should contain type header")
	}

	// Check field table
	if !strings.Contains(md, "| UserID | string | User identifier |") {
		t.Error("markdown should contain field table row")
	}

	// Check methods
	if !strings.Contains(md, "func (c *Claims) IsExpired() bool") {
		t.Error("markdown should contain method signature")
	}

	// Check timestamp
	if !strings.Contains(md, "2025-01-15") {
		t.Error("markdown should contain generation timestamp")
	}

	// Check valid markdown structure (no double blank lines at start)
	if strings.HasPrefix(md, "\n") {
		t.Error("markdown should not start with blank line")
	}
}

func TestRenderHTML_ContainsExpectedElements(t *testing.T) {
	doc := &ProjectDoc{
		Name:        "testproject",
		Description: "HTML test project.",
		GeneratedAt: time.Now(),
		Packages: []PackageDoc{
			{
				Name:        "core",
				Description: "Core functionality.",
				Functions: []FunctionDoc{
					{
						Name:        "Init",
						Signature:   "func Init() error",
						Description: "Initializes the system.",
						Exported:    true,
					},
				},
				Types: []TypeDoc{
					{
						Name:        "Config",
						Kind:        "struct",
						Description: "Configuration.",
						Fields: []FieldDoc{
							{Name: "Port", Type: "int", Desc: "Server port"},
						},
					},
				},
			},
		},
	}

	html := RenderHTML(doc)

	// Check HTML structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("HTML should contain DOCTYPE")
	}
	if !strings.Contains(html, "<title>testproject - Documentation</title>") {
		t.Error("HTML should contain title element")
	}

	// Check navigation
	if !strings.Contains(html, "<nav>") {
		t.Error("HTML should contain nav element")
	}
	if !strings.Contains(html, "pkg-core") {
		t.Error("HTML should contain package anchor")
	}

	// Check content
	if !strings.Contains(html, "HTML test project.") {
		t.Error("HTML should contain project description")
	}
	if !strings.Contains(html, "Package core") {
		t.Error("HTML should contain package heading")
	}
	if !strings.Contains(html, "func Init() error") {
		t.Error("HTML should contain function signature")
	}
	if !strings.Contains(html, "Config") {
		t.Error("HTML should contain type name")
	}
	if !strings.Contains(html, "Server port") {
		t.Error("HTML should contain field description")
	}

	// Check styling
	if !strings.Contains(html, "<style>") {
		t.Error("HTML should contain style element")
	}
}

func TestGenerateREADME_Format(t *testing.T) {
	doc := &ProjectDoc{
		Name:        "awesomelib",
		Description: "A library for awesome things.",
		QuickStart:  "lib.Init()",
		GeneratedAt: time.Now(),
		Packages: []PackageDoc{
			{
				Name:        "core",
				Description: "Core package.",
				Functions: []FunctionDoc{
					{Name: "Init", Signature: "func Init()", Description: "Initialize.", Exported: true},
					{Name: "Run", Signature: "func Run()", Description: "Run the app.", Exported: true},
				},
				Types: []TypeDoc{
					{Name: "App", Kind: "struct", Description: "Main application."},
				},
			},
		},
	}

	readme := GenerateREADME(doc)

	// Check title
	if !strings.HasPrefix(readme, "# awesomelib\n") {
		t.Error("README should start with title")
	}

	// Check description
	if !strings.Contains(readme, "A library for awesome things.") {
		t.Error("README should contain description")
	}

	// Check installation section
	if !strings.Contains(readme, "## Installation") {
		t.Error("README should contain Installation section")
	}
	if !strings.Contains(readme, "go get awesomelib") {
		t.Error("README should contain go get command")
	}

	// Check quick start
	if !strings.Contains(readme, "## Quick Start") {
		t.Error("README should contain Quick Start section")
	}
	if !strings.Contains(readme, "lib.Init()") {
		t.Error("README should contain quick start code")
	}

	// Check API overview
	if !strings.Contains(readme, "## API Overview") {
		t.Error("README should contain API Overview section")
	}
	if !strings.Contains(readme, "`Init`") {
		t.Error("README should list functions")
	}
	if !strings.Contains(readme, "`App` (struct)") {
		t.Error("README should list types")
	}

	// Check license
	if !strings.Contains(readme, "## License") {
		t.Error("README should contain License section")
	}
}

func TestInferDescription_FromREADME(t *testing.T) {
	dir := t.TempDir()

	readme := `# MyProject

This is a fantastic project that does amazing things.

## Installation

go get myproject
`
	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0644)
	if err != nil {
		t.Fatal(err)
	}

	dg := NewDocGenerator(dir)
	desc := dg.InferDescription(dir)

	if desc != "This is a fantastic project that does amazing things." {
		t.Errorf("unexpected description: %q", desc)
	}
}

func TestInferDescription_Fallback_PackageDoc(t *testing.T) {
	dir := t.TempDir()

	// No README, but has a Go file with package doc
	src := `// Package mylib provides useful utilities for data processing.
package mylib
`
	createTestGoFile(t, dir, "mylib.go", src)

	dg := NewDocGenerator(dir)
	desc := dg.InferDescription(dir)

	if desc != "Package mylib provides useful utilities for data processing." {
		t.Errorf("unexpected description: %q", desc)
	}
}

func TestInferDescription_Fallback_GoMod(t *testing.T) {
	dir := t.TempDir()

	// No README, no Go files with docs, but has go.mod
	gomod := `module github.com/user/myproject

go 1.21
`
	err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644)
	if err != nil {
		t.Fatal(err)
	}

	dg := NewDocGenerator(dir)
	desc := dg.InferDescription(dir)

	if desc != "Go module: github.com/user/myproject" {
		t.Errorf("unexpected description: %q", desc)
	}
}

func TestExportedOnlyFiltering(t *testing.T) {
	dir := t.TempDir()

	src := `package mylib

// PublicFunc is exported.
func PublicFunc() {}

// privateFunc is not exported.
func privateFunc() {}

// PublicType is exported.
type PublicType struct {
	// PublicField is exported.
	PublicField string
	// privateField is not exported.
	privateField int
}

// privateType is not exported.
type privateType struct{}
`
	createTestGoFile(t, dir, "mylib.go", src)

	// Test with IncludePrivate = false (default)
	dg := NewDocGenerator(dir)
	pkg, err := dg.parseGoPackage(dir)
	if err != nil {
		t.Fatalf("parseGoPackage failed: %v", err)
	}

	// Should only have PublicFunc
	if len(pkg.Functions) != 1 {
		t.Errorf("expected 1 exported function, got %d", len(pkg.Functions))
	}
	if len(pkg.Functions) > 0 && pkg.Functions[0].Name != "PublicFunc" {
		t.Errorf("expected function 'PublicFunc', got '%s'", pkg.Functions[0].Name)
	}

	// Should only have PublicType
	if len(pkg.Types) != 1 {
		t.Errorf("expected 1 exported type, got %d", len(pkg.Types))
	}
	if len(pkg.Types) > 0 {
		if pkg.Types[0].Name != "PublicType" {
			t.Errorf("expected type 'PublicType', got '%s'", pkg.Types[0].Name)
		}
		// Should only have PublicField
		if len(pkg.Types[0].Fields) != 1 {
			t.Errorf("expected 1 exported field, got %d", len(pkg.Types[0].Fields))
		}
		if len(pkg.Types[0].Fields) > 0 && pkg.Types[0].Fields[0].Name != "PublicField" {
			t.Errorf("expected field 'PublicField', got '%s'", pkg.Types[0].Fields[0].Name)
		}
	}

	// Test with IncludePrivate = true
	dg2 := NewDocGenerator(dir)
	dg2.IncludePrivate = true
	pkg2, err := dg2.parseGoPackage(dir)
	if err != nil {
		t.Fatalf("parseGoPackage with IncludePrivate failed: %v", err)
	}

	// Should have both functions
	if len(pkg2.Functions) != 2 {
		t.Errorf("expected 2 functions with IncludePrivate, got %d", len(pkg2.Functions))
	}

	// Should have both types
	if len(pkg2.Types) != 2 {
		t.Errorf("expected 2 types with IncludePrivate, got %d", len(pkg2.Types))
	}

	// PublicType should have both fields
	for _, typ := range pkg2.Types {
		if typ.Name == "PublicType" {
			if len(typ.Fields) != 2 {
				t.Errorf("expected 2 fields with IncludePrivate, got %d", len(typ.Fields))
			}
		}
	}
}

func TestEmptyPackageHandling(t *testing.T) {
	dir := t.TempDir()

	// A package with no exported symbols
	src := `package empty
`
	createTestGoFile(t, dir, "empty.go", src)

	dg := NewDocGenerator(dir)
	pkg, err := dg.parseGoPackage(dir)
	if err != nil {
		t.Fatalf("parseGoPackage failed for empty package: %v", err)
	}

	if pkg == nil {
		t.Fatal("expected non-nil package doc for empty package")
	}

	if pkg.Name != "empty" {
		t.Errorf("expected package name 'empty', got '%s'", pkg.Name)
	}

	if len(pkg.Functions) != 0 {
		t.Errorf("expected 0 functions in empty package, got %d", len(pkg.Functions))
	}

	if len(pkg.Types) != 0 {
		t.Errorf("expected 0 types in empty package, got %d", len(pkg.Types))
	}

	// Test RenderMarkdown with empty package
	doc := &ProjectDoc{
		Name:        "emptyproject",
		GeneratedAt: time.Now(),
		Packages:    []PackageDoc{*pkg},
	}
	md := RenderMarkdown(doc)
	if !strings.Contains(md, "# emptyproject") {
		t.Error("markdown for empty project should still have title")
	}
	if !strings.Contains(md, "### package empty") {
		t.Error("markdown should list the empty package")
	}
}

func TestParseGoPackage_SkipsTestFiles(t *testing.T) {
	dir := t.TempDir()

	src := `package mylib

// Exported is a regular function.
func Exported() {}
`
	testSrc := `package mylib

// TestHelper is a test helper that should not appear in docs.
func TestHelper() {}
`
	createTestGoFile(t, dir, "mylib.go", src)
	createTestGoFile(t, dir, "mylib_test.go", testSrc)

	dg := NewDocGenerator(dir)
	pkg, err := dg.parseGoPackage(dir)
	if err != nil {
		t.Fatalf("parseGoPackage failed: %v", err)
	}

	// Should only have Exported, not TestHelper
	if len(pkg.Functions) != 1 {
		t.Errorf("expected 1 function (test file should be skipped), got %d", len(pkg.Functions))
	}
	if len(pkg.Functions) > 0 && pkg.Functions[0].Name != "Exported" {
		t.Errorf("expected function 'Exported', got '%s'", pkg.Functions[0].Name)
	}
}

func TestGenerate_FullProject(t *testing.T) {
	dir := t.TempDir()

	// Create a sub-package
	subDir := filepath.Join(dir, "pkg", "util")
	err := os.MkdirAll(subDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Root package
	mainSrc := `// Package main is the entry point.
package main

// Run starts the application.
func Run() error { return nil }
`
	createTestGoFile(t, dir, "main.go", mainSrc)

	// Sub-package
	utilSrc := `// Package util provides utility functions.
package util

// FormatSize formats a byte size as a human-readable string.
func FormatSize(bytes int64) string { return "" }
`
	createTestGoFile(t, subDir, "util.go", utilSrc)

	// README
	readme := `# TestProject

A test project for the doc generator.

## Usage

Just run it.
`
	err = os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0644)
	if err != nil {
		t.Fatal(err)
	}

	dg := NewDocGenerator(dir)
	doc, err := dg.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if doc.Name != filepath.Base(dir) {
		t.Errorf("unexpected project name: %s", doc.Name)
	}

	if doc.Description != "A test project for the doc generator." {
		t.Errorf("unexpected description: %q", doc.Description)
	}

	// Should find at least 2 packages (main and util)
	if len(doc.Packages) < 2 {
		t.Errorf("expected at least 2 packages, got %d", len(doc.Packages))
	}

	if doc.Architecture == "" {
		t.Error("expected non-empty architecture")
	}

	if doc.QuickStart == "" {
		t.Error("expected non-empty quick start")
	}
}

func TestRenderMarkdown_EmptyProject(t *testing.T) {
	doc := &ProjectDoc{
		Name:        "empty",
		GeneratedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	md := RenderMarkdown(doc)

	if !strings.Contains(md, "# empty") {
		t.Error("should contain project title")
	}
	if !strings.Contains(md, "Generated at:") {
		t.Error("should contain generated timestamp")
	}
}

func TestInferDescription_FromREADME_SkipsBadges(t *testing.T) {
	dir := t.TempDir()

	readme := `# BadgeProject

[![Build](https://img.shields.io/badge/build-passing-green)]()
![Coverage](https://img.shields.io/badge/coverage-90-blue)

This is the real description after badges.

## More stuff
`
	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0644)
	if err != nil {
		t.Fatal(err)
	}

	dg := NewDocGenerator(dir)
	desc := dg.InferDescription(dir)

	if desc != "This is the real description after badges." {
		t.Errorf("unexpected description (should skip badges): %q", desc)
	}
}

func TestParseGoPackage_Methods(t *testing.T) {
	dir := t.TempDir()

	src := `package mylib

// Server handles HTTP requests.
type Server struct {
	Port int
}

// Start begins listening on the configured port.
func (s *Server) Start() error { return nil }

// Stop gracefully shuts down the server.
func (s *Server) Stop() error { return nil }

// internal is unexported.
func (s *Server) internal() {}
`
	createTestGoFile(t, dir, "server.go", src)

	dg := NewDocGenerator(dir)
	pkg, err := dg.parseGoPackage(dir)
	if err != nil {
		t.Fatalf("parseGoPackage failed: %v", err)
	}

	if len(pkg.Types) != 1 {
		t.Fatalf("expected 1 type, got %d", len(pkg.Types))
	}

	serverType := pkg.Types[0]
	if serverType.Name != "Server" {
		t.Errorf("expected type 'Server', got '%s'", serverType.Name)
	}

	// Should have 2 exported methods (Start, Stop) but not internal
	if len(serverType.Methods) != 2 {
		t.Errorf("expected 2 exported methods, got %d", len(serverType.Methods))
		for _, m := range serverType.Methods {
			t.Logf("  method: %s", m.Name)
		}
	}
}
