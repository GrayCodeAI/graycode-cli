package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestProject creates a temporary Go project for testing the NavIndex.
func setupTestProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Create package structure
	pkgAuth := filepath.Join(dir, "pkg", "auth")
	pkgHandler := filepath.Join(dir, "pkg", "handler")
	os.MkdirAll(pkgAuth, 0o755)
	os.MkdirAll(pkgHandler, 0o755)

	// pkg/auth/token.go - definitions
	navWriteFile(t, filepath.Join(pkgAuth, "token.go"), `package auth

// Claims represents JWT claims.
type Claims struct {
	UserID string
	Expiry int64
}

// Validator is an interface for token validation.
type Validator interface {
	Validate(token string) (*Claims, error)
	Refresh(token string) (string, error)
}

// ValidateToken checks if a token is valid and returns claims.
func ValidateToken(token string) (*Claims, error) {
	if token == "" {
		return nil, fmt.Errorf("empty token")
	}
	return parseClaims(token)
}

func parseClaims(token string) (*Claims, error) {
	return &Claims{UserID: "user1"}, nil
}

// MaxTokenAge is the maximum age of a token.
const MaxTokenAge = 3600

var defaultIssuer = "graycode"
`)

	// pkg/auth/jwt.go - implements Validator
	navWriteFile(t, filepath.Join(pkgAuth, "jwt.go"), `package auth

// JWTValidator validates JWT tokens.
type JWTValidator struct {
	Secret string
}

// Validate checks a JWT token.
func (v *JWTValidator) Validate(token string) (*Claims, error) {
	return ValidateToken(token)
}

// Refresh generates a new token.
func (v *JWTValidator) Refresh(token string) (string, error) {
	return token + "_refreshed", nil
}
`)

	// pkg/auth/hmac.go - another implementation of Validator
	navWriteFile(t, filepath.Join(pkgAuth, "hmac.go"), `package auth

// HMACValidator validates HMAC tokens.
type HMACValidator struct {
	Key []byte
}

// Validate checks an HMAC token.
func (v *HMACValidator) Validate(token string) (*Claims, error) {
	return ValidateToken(token)
}

// Refresh generates a new HMAC token.
func (v *HMACValidator) Refresh(token string) (string, error) {
	return token + "_hmac_refreshed", nil
}
`)

	// pkg/handler/api.go - references
	navWriteFile(t, filepath.Join(pkgHandler, "api.go"), `package handler

import "example/pkg/auth"

// HandleAPI processes API requests.
func HandleAPI(token string) error {
	claims, err := auth.ValidateToken(token)
	if err != nil {
		return err
	}
	_ = claims
	return nil
}

// HandleAuth verifies authentication.
func HandleAuth(t string) error {
	if _, err := auth.ValidateToken(t); err != nil {
		return err
	}
	return nil
}
`)

	return dir
}

func navWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test file %s: %v", path, err)
	}
}

func TestNewNavIndex(t *testing.T) {
	idx := NewNavIndex()
	if idx == nil {
		t.Fatal("NewNavIndex returned nil")
	}
	if idx.Definitions == nil {
		t.Error("Definitions map is nil")
	}
	if idx.References == nil {
		t.Error("References map is nil")
	}
	if idx.Implementations == nil {
		t.Error("Implementations map is nil")
	}
}

func TestBuildIndex(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()

	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	// Should find function definitions
	if _, ok := idx.Definitions["ValidateToken"]; !ok {
		t.Error("expected to find ValidateToken definition")
	}

	// Should find type definitions
	if _, ok := idx.Definitions["Claims"]; !ok {
		t.Error("expected to find Claims definition")
	}

	// Should find interface definitions
	if def, ok := idx.Definitions["Validator"]; !ok {
		t.Error("expected to find Validator definition")
	} else if def.Kind != "interface" {
		t.Errorf("expected Validator kind=interface, got %s", def.Kind)
	}

	// Should find const definitions
	if def, ok := idx.Definitions["MaxTokenAge"]; !ok {
		t.Error("expected to find MaxTokenAge definition")
	} else if def.Kind != "const" {
		t.Errorf("expected MaxTokenAge kind=const, got %s", def.Kind)
	}

	// Should find var definitions
	if def, ok := idx.Definitions["defaultIssuer"]; !ok {
		t.Error("expected to find defaultIssuer definition")
	} else {
		if def.Kind != "var" {
			t.Errorf("expected defaultIssuer kind=var, got %s", def.Kind)
		}
		if def.Exported {
			t.Error("expected defaultIssuer to be unexported")
		}
	}
}

func TestGoToDefinition(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	tests := []struct {
		symbol   string
		wantKind string
		wantNil  bool
	}{
		{"ValidateToken", "func", false},
		{"Claims", "type", false},
		{"Validator", "interface", false},
		{"JWTValidator", "type", false},
		{"NonExistent", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.symbol, func(t *testing.T) {
			def := idx.GoToDefinition(tc.symbol)
			if tc.wantNil {
				if def != nil {
					t.Errorf("expected nil for %s, got %+v", tc.symbol, def)
				}
				return
			}
			if def == nil {
				t.Fatalf("expected definition for %s, got nil", tc.symbol)
			}
			if def.Kind != tc.wantKind {
				t.Errorf("expected kind %s for %s, got %s", tc.wantKind, tc.symbol, def.Kind)
			}
		})
	}
}

func TestGoToDefinitionDetails(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	def := idx.GoToDefinition("ValidateToken")
	if def == nil {
		t.Fatal("expected definition for ValidateToken")
	}

	if def.Package != "auth" {
		t.Errorf("expected package=auth, got %s", def.Package)
	}
	if !def.Exported {
		t.Error("expected ValidateToken to be exported")
	}
	if !strings.Contains(def.Signature, "ValidateToken") {
		t.Errorf("expected signature to contain ValidateToken, got %s", def.Signature)
	}
	if !strings.Contains(def.Signature, "token string") {
		t.Errorf("expected signature to contain param, got %s", def.Signature)
	}
	if def.DocComment == "" {
		t.Error("expected non-empty doc comment for ValidateToken")
	}
}

func TestFindReferences(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	refs := idx.FindReferences("ValidateToken")
	if len(refs) == 0 {
		t.Fatal("expected references for ValidateToken")
	}

	// Should have call references from handler and jwt/hmac validators
	callCount := 0
	for _, ref := range refs {
		if ref.Kind == "call" {
			callCount++
		}
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 call references for ValidateToken, got %d", callCount)
	}

	// Check that context is populated
	for _, ref := range refs {
		if ref.Context == "" {
			t.Errorf("expected non-empty context for reference at %s:%d", ref.File, ref.Line)
		}
	}

	// References should be sorted
	for i := 1; i < len(refs); i++ {
		if refs[i].File < refs[i-1].File {
			t.Error("references not sorted by file")
			break
		}
		if refs[i].File == refs[i-1].File && refs[i].Line < refs[i-1].Line {
			t.Error("references not sorted by line within file")
			break
		}
	}
}

func TestFindReferencesNonExistent(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	refs := idx.FindReferences("CompletelyNonExistent")
	if refs != nil {
		t.Errorf("expected nil for non-existent symbol, got %d refs", len(refs))
	}
}

func TestFindImplementations(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	impls := idx.FindImplementations("Validator")
	if len(impls) < 2 {
		t.Fatalf("expected at least 2 implementations of Validator, got %d: %v", len(impls), impls)
	}

	found := make(map[string]bool)
	for _, impl := range impls {
		found[impl] = true
	}
	if !found["JWTValidator"] {
		t.Error("expected JWTValidator to implement Validator")
	}
	if !found["HMACValidator"] {
		t.Error("expected HMACValidator to implement Validator")
	}

	// Should be sorted
	for i := 1; i < len(impls); i++ {
		if impls[i] < impls[i-1] {
			t.Error("implementations not sorted")
			break
		}
	}
}

func TestFindImplementationsNonInterface(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	impls := idx.FindImplementations("Claims")
	if impls != nil {
		t.Errorf("expected nil for non-interface, got %v", impls)
	}
}

func TestFindCallers(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	callers := idx.FindCallers("ValidateToken")
	if len(callers) == 0 {
		t.Fatal("expected callers for ValidateToken")
	}

	for _, c := range callers {
		if c.Kind != "call" {
			t.Errorf("expected kind=call, got %s", c.Kind)
		}
	}

	// Should be sorted
	for i := 1; i < len(callers); i++ {
		if callers[i].File < callers[i-1].File {
			t.Error("callers not sorted by file")
			break
		}
	}
}

func TestFindCallees(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	callees := idx.FindCallees("ValidateToken")
	if callees == nil {
		t.Fatal("expected callees for ValidateToken")
	}

	found := false
	for _, c := range callees {
		if c == "parseClaims" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected parseClaims in callees of ValidateToken, got %v", callees)
	}
}

func TestFindCalleesMethod(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	// JWTValidator.Validate should call ValidateToken
	callees := idx.FindCallees("JWTValidator.Validate")
	if callees == nil {
		t.Fatal("expected callees for JWTValidator.Validate")
	}

	found := false
	for _, c := range callees {
		if c == "ValidateToken" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ValidateToken in callees of JWTValidator.Validate, got %v", callees)
	}
}

func TestSearchSymbols(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	tests := []struct {
		query    string
		kind     string
		wantMin  int
		wantName string
	}{
		{"Validate", "", 2, "ValidateToken"},       // fuzzy matches ValidateToken, Validate methods
		{"Validator", "interface", 1, "Validator"}, // filtered by kind
		{"JWT", "", 1, "JWTValidator"},
		{"claims", "", 1, "Claims"}, // case insensitive
		{"zzzzz", "", 0, ""},        // no match
	}

	for _, tc := range tests {
		t.Run(tc.query+"_"+tc.kind, func(t *testing.T) {
			results := idx.SearchSymbols(tc.query, tc.kind)
			if len(results) < tc.wantMin {
				t.Errorf("SearchSymbols(%q, %q): got %d results, want at least %d",
					tc.query, tc.kind, len(results), tc.wantMin)
			}
			if tc.wantName != "" && len(results) > 0 {
				found := false
				for _, r := range results {
					if r.Name == tc.wantName {
						found = true
						break
					}
				}
				if !found {
					names := make([]string, len(results))
					for i, r := range results {
						names[i] = r.Name
					}
					t.Errorf("expected %s in results, got %v", tc.wantName, names)
				}
			}
		})
	}
}

func TestSearchSymbolsEmpty(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	// Empty query should match everything
	results := idx.SearchSymbols("", "")
	if len(results) == 0 {
		t.Error("empty query should return all symbols")
	}
}

func TestFormatDefinition(t *testing.T) {
	def := &Definition{
		Name:      "ValidateToken",
		Kind:      "func",
		File:      "pkg/auth/token.go",
		Line:      42,
		Signature: "func ValidateToken(token string) (*Claims, error)",
	}

	result := FormatDefinition(def)
	if !strings.Contains(result, "func ValidateToken") {
		t.Errorf("expected formatted output to contain kind+name, got: %s", result)
	}
	if !strings.Contains(result, "pkg/auth/token.go:42") {
		t.Errorf("expected formatted output to contain file:line, got: %s", result)
	}
	if !strings.Contains(result, "func ValidateToken(token string) (*Claims, error)") {
		t.Errorf("expected formatted output to contain signature, got: %s", result)
	}
}

func TestFormatDefinitionNil(t *testing.T) {
	result := FormatDefinition(nil)
	if result != "" {
		t.Errorf("expected empty string for nil definition, got: %s", result)
	}
}

func TestFormatReferences(t *testing.T) {
	refs := []*Reference{
		{File: "pkg/handler/api.go", Line: 15, Context: "\tclaims, err := auth.ValidateToken(token)", Kind: "call"},
		{File: "pkg/handler/auth.go", Line: 28, Context: "\tif _, err := auth.ValidateToken(t); err != nil {", Kind: "call"},
	}

	result := FormatReferences("ValidateToken", refs)
	if !strings.Contains(result, "References to ValidateToken (2 found)") {
		t.Errorf("expected header in formatted output, got: %s", result)
	}
	if !strings.Contains(result, "pkg/handler/api.go:15") {
		t.Errorf("expected file:line in formatted output, got: %s", result)
	}
}

func TestFormatReferencesEmpty(t *testing.T) {
	result := FormatReferences("Foo", nil)
	if !strings.Contains(result, "No references found") {
		t.Errorf("expected 'No references found' message, got: %s", result)
	}
}

func TestTypeHierarchy(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	result := idx.TypeHierarchy("Validator")
	if !strings.Contains(result, "interface Validator") {
		t.Errorf("expected 'interface Validator' in hierarchy, got: %s", result)
	}
	if !strings.Contains(result, "JWTValidator") {
		t.Errorf("expected JWTValidator in hierarchy, got: %s", result)
	}
	if !strings.Contains(result, "HMACValidator") {
		t.Errorf("expected HMACValidator in hierarchy, got: %s", result)
	}
}

func TestTypeHierarchyImplementor(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	result := idx.TypeHierarchy("JWTValidator")
	if !strings.Contains(result, "type JWTValidator") {
		t.Errorf("expected 'type JWTValidator' in hierarchy, got: %s", result)
	}
	if !strings.Contains(result, "Validator") {
		t.Errorf("expected Validator in implements list, got: %s", result)
	}
}

func TestMethodDefinition(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	def := idx.GoToDefinition("JWTValidator.Validate")
	if def == nil {
		t.Fatal("expected definition for JWTValidator.Validate")
	}
	if def.Kind != "method" {
		t.Errorf("expected kind=method, got %s", def.Kind)
	}
	if def.Name != "Validate" {
		t.Errorf("expected Name=Validate, got %s", def.Name)
	}
}

func TestBuildIndexSkipsVendor(t *testing.T) {
	dir := t.TempDir()

	// Create a vendor directory with a Go file
	vendorDir := filepath.Join(dir, "vendor", "pkg")
	os.MkdirAll(vendorDir, 0o755)
	navWriteFile(t, filepath.Join(vendorDir, "lib.go"), `package pkg
func VendorFunc() {}
`)

	// Create a regular Go file
	navWriteFile(t, filepath.Join(dir, "main.go"), `package main
func MainFunc() {}
`)

	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	if _, ok := idx.Definitions["VendorFunc"]; ok {
		t.Error("should not index vendor directory")
	}
	if _, ok := idx.Definitions["MainFunc"]; !ok {
		t.Error("should index non-vendor files")
	}
}

func TestBuildIndexEmptyDir(t *testing.T) {
	dir := t.TempDir()
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex on empty dir should not fail: %v", err)
	}
	if len(idx.Definitions) != 0 {
		t.Errorf("expected 0 definitions for empty dir, got %d", len(idx.Definitions))
	}
}

func TestNavConcurrentAccess(t *testing.T) {
	dir := setupTestProject(t)
	idx := NewNavIndex()
	if err := idx.BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	// Concurrent reads should not panic
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			idx.GoToDefinition("ValidateToken")
			idx.FindReferences("ValidateToken")
			idx.FindImplementations("Validator")
			idx.FindCallers("ValidateToken")
			idx.FindCallees("ValidateToken")
			idx.SearchSymbols("Token", "")
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		query  string
		target string
		want   bool
	}{
		{"", "anything", true},
		{"abc", "abc", true},
		{"abc", "aXbXc", true},
		{"vt", "validatetoken", true},
		{"xyz", "abc", false},
		{"ab", "ba", false},
	}

	for _, tc := range tests {
		t.Run(tc.query+"_"+tc.target, func(t *testing.T) {
			got := fuzzyMatch(tc.query, tc.target)
			if got != tc.want {
				t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tc.query, tc.target, got, tc.want)
			}
		})
	}
}
