package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDeadCodeDetector(t *testing.T) {
	d := NewDeadCodeDetector()
	if d == nil {
		t.Fatal("NewDeadCodeDetector returned nil")
	}
	if d.Declarations == nil {
		t.Error("Declarations map is nil")
	}
	if d.References == nil {
		t.Error("References map is nil")
	}
}

func TestScanFile_Functions(t *testing.T) {
	d := NewDeadCodeDetector()
	content := `package example

func usedFunc() {}
func unusedFunc() {}
func main() { usedFunc() }
`
	d.ScanFile("test.go", content)

	d.mu.RLock()
	defer d.mu.RUnlock()

	// Should have declarations for usedFunc, unusedFunc, main
	if len(d.Declarations) < 3 {
		t.Errorf("expected at least 3 declarations, got %d", len(d.Declarations))
	}

	// usedFunc should have references (called in main)
	if d.References["usedFunc"] < 2 {
		t.Errorf("expected usedFunc to have >= 2 references (decl + use), got %d", d.References["usedFunc"])
	}
}

func TestScanFile_Types(t *testing.T) {
	d := NewDeadCodeDetector()
	content := `package example

type UsedType struct {
	Field string
}

type UnusedType struct {
	Field int
}

func createUsed() UsedType {
	return UsedType{}
}
`
	d.ScanFile("types.go", content)

	d.mu.RLock()
	defer d.mu.RUnlock()

	if _, ok := d.Declarations["types.go:UsedType"]; !ok {
		t.Error("UsedType declaration not found")
	}
	if _, ok := d.Declarations["types.go:UnusedType"]; !ok {
		t.Error("UnusedType declaration not found")
	}

	// UsedType should have more references than UnusedType
	if d.References["UsedType"] <= d.References["UnusedType"] {
		t.Errorf("UsedType refs (%d) should be > UnusedType refs (%d)",
			d.References["UsedType"], d.References["UnusedType"])
	}
}

func TestScanFile_VarsAndConsts(t *testing.T) {
	d := NewDeadCodeDetector()
	content := `package example

var usedVar = 42
var unusedVar = "hello"
const usedConst = 100
const unusedConst = "world"

func doStuff() int {
	return usedVar + usedConst
}
`
	d.ScanFile("vars.go", content)

	d.mu.RLock()
	defer d.mu.RUnlock()

	if _, ok := d.Declarations["vars.go:usedVar"]; !ok {
		t.Error("usedVar declaration not found")
	}
	if _, ok := d.Declarations["vars.go:unusedVar"]; !ok {
		t.Error("unusedVar declaration not found")
	}
	if _, ok := d.Declarations["vars.go:usedConst"]; !ok {
		t.Error("usedConst declaration not found")
	}
	if _, ok := d.Declarations["vars.go:unusedConst"]; !ok {
		t.Error("unusedConst declaration not found")
	}

	if d.Declarations["vars.go:usedVar"].Kind != "var" {
		t.Errorf("expected kind 'var', got %q", d.Declarations["vars.go:usedVar"].Kind)
	}
	if d.Declarations["vars.go:usedConst"].Kind != "const" {
		t.Errorf("expected kind 'const', got %q", d.Declarations["vars.go:usedConst"].Kind)
	}
}

func TestScanFile_Methods(t *testing.T) {
	d := NewDeadCodeDetector()
	content := `package example

type Server struct{}

func (s *Server) Start() {}
func (s *Server) unusedMethod() {}

func callStart() {
	s := &Server{}
	s.Start()
}
`
	d.ScanFile("methods.go", content)

	d.mu.RLock()
	defer d.mu.RUnlock()

	if _, ok := d.Declarations["methods.go:Server.Start"]; !ok {
		t.Error("Server.Start method not found")
	}
	if _, ok := d.Declarations["methods.go:Server.unusedMethod"]; !ok {
		t.Error("Server.unusedMethod method not found")
	}
	if d.Declarations["methods.go:Server.Start"].Kind != "method" {
		t.Error("Server.Start should be kind 'method'")
	}
}

func TestFindUnused_SkipsMain(t *testing.T) {
	d := NewDeadCodeDetector()
	content := `package main

func main() {}
`
	d.ScanFile("main.go", content)
	results := d.FindUnused()

	for _, r := range results {
		if r.Name == "main" {
			t.Error("main() should not appear in unused results")
		}
	}
}

func TestFindUnused_SkipsInit(t *testing.T) {
	d := NewDeadCodeDetector()
	content := `package example

func init() {}
`
	d.ScanFile("init.go", content)
	results := d.FindUnused()

	for _, r := range results {
		if r.Name == "init" {
			t.Error("init() should not appear in unused results")
		}
	}
}

func TestFindUnused_SkipsTestFunctions(t *testing.T) {
	d := NewDeadCodeDetector()
	content := `package example

func TestSomething(t *testing.T) {}
func BenchmarkSomething(b *testing.B) {}
func ExampleSomething() {}
func FuzzSomething(f *testing.F) {}
`
	d.ScanFile("example_test.go", content)
	results := d.FindUnused()

	for _, r := range results {
		if IsTestFunction(r.Name) {
			t.Errorf("test function %q should not appear in unused results", r.Name)
		}
	}
}

func TestFindUnused_DetectsUnusedFunction(t *testing.T) {
	d := NewDeadCodeDetector()
	content := `package example

func neverCalled() int {
	return 42
}

func alsoNeverCalled() string {
	return "hello"
}
`
	d.ScanFile("unused.go", content)
	results := d.FindUnused()

	if len(results) < 2 {
		t.Errorf("expected at least 2 unused items, got %d", len(results))
	}

	found := map[string]bool{}
	for _, r := range results {
		found[r.Name] = true
	}
	if !found["neverCalled"] {
		t.Error("neverCalled should be detected as unused")
	}
	if !found["alsoNeverCalled"] {
		t.Error("alsoNeverCalled should be detected as unused")
	}
}

func TestFindUnused_ExportedLowerConfidence(t *testing.T) {
	d := NewDeadCodeDetector()
	content := `package example

func ExportedButUnused() {}
func unexportedUnused() {}
`
	d.ScanFile("exported.go", content)
	results := d.FindUnused()

	var exportedConf, unexportedConf float64
	for _, r := range results {
		if r.Name == "ExportedButUnused" {
			exportedConf = r.Confidence
		}
		if r.Name == "unexportedUnused" {
			unexportedConf = r.Confidence
		}
	}

	if exportedConf >= unexportedConf {
		t.Errorf("exported confidence (%.2f) should be lower than unexported (%.2f)",
			exportedConf, unexportedConf)
	}
}

func TestIsTestFunction(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"TestFoo", true},
		{"TestA", true},
		{"Test", false},    // "Test" alone is not a test function (no uppercase after)
		{"Testfoo", false}, // lowercase after Test
		{"BenchmarkFoo", true},
		{"Benchmark", false},
		{"ExampleFoo", true},
		{"Example", true}, // Example alone is valid
		{"FuzzFoo", true},
		{"Fuzz", false},
		{"Helper", false},
		{"TestHelper", true}, // 'H' is uppercase, valid test function pattern
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTestFunction(tt.name)
			if got != tt.expected {
				t.Errorf("IsTestFunction(%q) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestIsInterfaceImpl(t *testing.T) {
	content := `package example

type Writer interface {
	Write(p []byte) (n int, err error)
}

type Reader interface {
	Read(p []byte) (n int, err error)
}
`

	if !IsInterfaceImpl("Write", content) {
		t.Error("Write should be detected as interface implementation")
	}
	if !IsInterfaceImpl("MyType.Read", content) {
		t.Error("MyType.Read should be detected as interface implementation")
	}
	if IsInterfaceImpl("DoSomething", content) {
		t.Error("DoSomething should not be detected as interface implementation")
	}
}

func TestFormatDeadCode_Empty(t *testing.T) {
	result := FormatDeadCode(nil)
	if !strings.Contains(result, "0 items") {
		t.Error("empty result should mention 0 items")
	}
}

func TestFormatDeadCode_Mixed(t *testing.T) {
	items := []DeadCode{
		{File: "src/util.go", Line: 42, Name: "oldHelper", Kind: "func", Confidence: 0.9, Reason: "0 references"},
		{File: "src/api.go", Line: 23, Name: "LegacyHandler", Kind: "func", Confidence: 0.5, Reason: "0 internal references"},
	}

	result := FormatDeadCode(items)
	if !strings.Contains(result, "2 items") {
		t.Error("should mention 2 items")
	}
	if !strings.Contains(result, "HIGH confidence") {
		t.Error("should have HIGH confidence section")
	}
	if !strings.Contains(result, "MEDIUM confidence") {
		t.Error("should have MEDIUM confidence section")
	}
	if !strings.Contains(result, "oldHelper") {
		t.Error("should contain oldHelper")
	}
	if !strings.Contains(result, "LegacyHandler") {
		t.Error("should contain LegacyHandler")
	}
	if !strings.Contains(result, "Estimated removable") {
		t.Error("should contain estimated removable lines")
	}
}

func TestEstimateRemovableLines(t *testing.T) {
	items := []DeadCode{
		{Kind: "function"},
		{Kind: "method"},
		{Kind: "type"},
		{Kind: "var"},
		{Kind: "const"},
	}

	total := EstimateRemovableLines(items)
	// function=10, method=10, type=5, var=1, const=1 = 27
	if total != 27 {
		t.Errorf("expected 27 estimated lines, got %d", total)
	}
}

func TestEstimateRemovableLines_Empty(t *testing.T) {
	total := EstimateRemovableLines(nil)
	if total != 0 {
		t.Errorf("expected 0 for empty input, got %d", total)
	}
}

func TestGenerateRemovalPlan(t *testing.T) {
	items := []DeadCode{
		{File: "a.go", Line: 10, Name: "unused1", Kind: "function", Confidence: 0.9, Reason: "0 references"},
		{File: "a.go", Line: 20, Name: "unused2", Kind: "var", Confidence: 0.5, Reason: "0 internal references"},
		{File: "b.go", Line: 5, Name: "unused3", Kind: "type", Confidence: 0.8, Reason: "0 references"},
	}

	plan := GenerateRemovalPlan(items)
	if !strings.Contains(plan, "Removal Plan") {
		t.Error("plan should have title")
	}
	if !strings.Contains(plan, "a.go") {
		t.Error("plan should reference a.go")
	}
	if !strings.Contains(plan, "b.go") {
		t.Error("plan should reference b.go")
	}
	if !strings.Contains(plan, "HIGH") {
		t.Error("plan should mention HIGH confidence")
	}
	if !strings.Contains(plan, "MEDIUM") {
		t.Error("plan should mention MEDIUM confidence")
	}
	if !strings.Contains(plan, "Total items: 3") {
		t.Error("plan should have total count")
	}
}

func TestGenerateRemovalPlan_Empty(t *testing.T) {
	plan := GenerateRemovalPlan(nil)
	if !strings.Contains(plan, "No dead code") {
		t.Error("empty plan should say no dead code")
	}
}

func TestScan_Integration(t *testing.T) {
	// Create a temp directory with Go files
	tmpDir := t.TempDir()

	// Write go.mod
	goMod := `module example.com/testproject

go 1.21
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a file with used and unused code
	mainFile := `package main

import "fmt"

func main() {
	fmt.Println(helper())
}

func helper() string {
	return "hello"
}

func deadCode() int {
	return 42
}

type UsedConfig struct {
	Name string
}

type DeadConfig struct {
	Value int
}

var activeVar = UsedConfig{Name: "test"}
var deadVar = 999
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainFile), 0o644); err != nil {
		t.Fatal(err)
	}

	d := NewDeadCodeDetector()
	results, err := d.Scan(tmpDir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should detect some dead code
	if len(results) == 0 {
		t.Error("expected to find some dead code")
	}

	// deadCode function should be found
	foundDeadCode := false
	for _, r := range results {
		if r.Name == "deadCode" {
			foundDeadCode = true
			if r.Kind != "function" {
				t.Errorf("deadCode should be kind 'function', got %q", r.Kind)
			}
			if r.Confidence < 0.7 {
				t.Errorf("unexported dead function should have high confidence, got %.2f", r.Confidence)
			}
		}
	}
	if !foundDeadCode {
		t.Error("deadCode function should be detected as dead")
	}
}

func TestScan_SkipsVendor(t *testing.T) {
	tmpDir := t.TempDir()

	// Write go.mod
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a main file
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create vendor dir with Go files that should be skipped
	vendorDir := filepath.Join(tmpDir, "vendor", "pkg")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	vendorFile := `package pkg
func VendorFunc() {}
`
	if err := os.WriteFile(filepath.Join(vendorDir, "pkg.go"), []byte(vendorFile), 0o644); err != nil {
		t.Fatal(err)
	}

	d := NewDeadCodeDetector()
	results, err := d.Scan(tmpDir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	for _, r := range results {
		if strings.Contains(r.File, "vendor") {
			t.Error("should not report dead code from vendor directory")
		}
	}
}

func TestFindUnusedExports(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/myapp\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	content := `package myapp

func ExportedUsed() string { return "used" }
func ExportedUnused() string { return "unused" }
func unexportedUsed() int { return 1 }
func unexportedUnused() int { return 2 }

func init() {
	_ = ExportedUsed()
	_ = unexportedUsed()
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "lib.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	d := NewDeadCodeDetector()
	results := d.FindUnusedExports(tmpDir)

	foundExportedUnused := false
	for _, r := range results {
		if r.Name == "ExportedUnused" {
			foundExportedUnused = true
		}
		// Verify only exported symbols appear
		baseName := r.Name
		if idx := strings.LastIndex(baseName, "."); idx >= 0 {
			baseName = baseName[idx+1:]
		}
		if len(baseName) > 0 && baseName[0] >= 'a' && baseName[0] <= 'z' {
			t.Errorf("FindUnusedExports should only return exported symbols, got %q", r.Name)
		}
	}
	if !foundExportedUnused {
		t.Error("ExportedUnused should be detected as unused export")
	}
}

func TestDeadCode_ConcurrentAccess(t *testing.T) {
	d := NewDeadCodeDetector()

	content1 := `package a
func FuncA() {}
func FuncB() { FuncA() }
`
	content2 := `package b
func FuncC() {}
func FuncD() { FuncC() }
`

	// Simulate concurrent scanning
	done := make(chan struct{}, 2)
	go func() {
		d.ScanFile("a.go", content1)
		done <- struct{}{}
	}()
	go func() {
		d.ScanFile("b.go", content2)
		done <- struct{}{}
	}()
	<-done
	<-done

	// Should not panic and should have results
	results := d.FindUnused()
	_ = results // just ensure no panic
}
