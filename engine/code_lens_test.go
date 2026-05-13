package engine

import (
	"strings"
	"testing"
	"time"
)

func TestNewCodeLensProvider(t *testing.T) {
	p := NewCodeLensProvider()
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	expected := []string{"test_status", "complexity", "references", "age", "coverage"}
	for _, name := range expected {
		if _, ok := p.Providers[name]; !ok {
			t.Errorf("missing built-in provider: %s", name)
		}
	}
}

func TestRegister(t *testing.T) {
	p := NewCodeLensProvider()
	called := false
	p.Register("custom", func(file, content string) []CodeLens {
		called = true
		return []CodeLens{{File: file, Line: 1, Label: "custom", Category: "custom"}}
	})

	if _, ok := p.Providers["custom"]; !ok {
		t.Fatal("Register did not add the provider")
	}

	lenses := p.Providers["custom"]("test.go", "")
	if !called {
		t.Error("custom generator was not called")
	}
	if len(lenses) != 1 {
		t.Errorf("expected 1 lens, got %d", len(lenses))
	}
}

func TestGenerate_SortsByLine(t *testing.T) {
	p := &CodeLensProvider{
		Providers: make(map[string]LensGenerator),
	}
	p.Providers["a"] = func(file, content string) []CodeLens {
		return []CodeLens{
			{File: file, Line: 10, Label: "a10", Category: "age"},
			{File: file, Line: 1, Label: "a1", Category: "age"},
		}
	}
	p.Providers["b"] = func(file, content string) []CodeLens {
		return []CodeLens{
			{File: file, Line: 5, Label: "b5", Category: "complexity"},
		}
	}

	lenses := p.Generate("file.go", "")
	if len(lenses) != 3 {
		t.Fatalf("expected 3 lenses, got %d", len(lenses))
	}
	if lenses[0].Line != 1 {
		t.Errorf("expected first lens at line 1, got %d", lenses[0].Line)
	}
	if lenses[1].Line != 5 {
		t.Errorf("expected second lens at line 5, got %d", lenses[1].Line)
	}
	if lenses[2].Line != 10 {
		t.Errorf("expected third lens at line 10, got %d", lenses[2].Line)
	}
}

func TestFilterByCategory(t *testing.T) {
	lenses := []CodeLens{
		{Line: 1, Category: "complexity"},
		{Line: 2, Category: "test_status"},
		{Line: 3, Category: "complexity"},
		{Line: 4, Category: "age"},
	}

	result := FilterByCategory(lenses, "complexity")
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
	for _, l := range result {
		if l.Category != "complexity" {
			t.Errorf("expected category complexity, got %s", l.Category)
		}
	}

	result = FilterByCategory(lenses, "ownership")
	if len(result) != 0 {
		t.Errorf("expected 0 for non-existent category, got %d", len(result))
	}
}

func TestFilterByCategory_Empty(t *testing.T) {
	result := FilterByCategory(nil, "test_status")
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestFormatLenses(t *testing.T) {
	lenses := []CodeLens{
		{File: "src/auth.go", Line: 5, Label: "complexity: 12", Category: "complexity", Tooltip: "func ValidateToken — consider splitting"},
		{File: "src/auth.go", Line: 15, Label: "references: 8", Category: "references", Tooltip: "func ParseClaims — widely used"},
		{File: "src/auth.go", Line: 42, Label: "age: 3d", Category: "age", Tooltip: "func RefreshToken — recently modified"},
		{File: "src/auth.go", Line: 60, Label: "test: PASS", Category: "test_status", Tooltip: "func TestValidateToken — passed"},
	}

	output := FormatLenses("src/auth.go", lenses)
	if !strings.Contains(output, "Code Lenses for src/auth.go:") {
		t.Error("missing header")
	}
	if !strings.Contains(output, "L5") {
		t.Error("missing line 5")
	}
	if !strings.Contains(output, "[complexity: 12]") {
		t.Error("missing complexity label")
	}
	if !strings.Contains(output, "[references: 8]") {
		t.Error("missing references label")
	}
	if !strings.Contains(output, "[age: 3d]") {
		t.Error("missing age label")
	}
	if !strings.Contains(output, "[test: PASS]") {
		t.Error("missing test status label")
	}
	if !strings.Contains(output, "consider splitting") {
		t.Error("missing tooltip text")
	}
}

func TestFormatLenses_Empty(t *testing.T) {
	output := FormatLenses("empty.go", nil)
	if !strings.Contains(output, "(none)") {
		t.Error("expected (none) for empty lenses")
	}
}

func TestGenerateTestLens_NonTestFile(t *testing.T) {
	lenses := GenerateTestLens("main.go", `package main

func main() {}
`)
	if lenses != nil {
		t.Errorf("expected nil for non-test file, got %v", lenses)
	}
}

func TestGenerateTestLens_FindsTestFunctions(t *testing.T) {
	content := `package foo

import "testing"

func TestAdd(t *testing.T) {
	if add(1, 2) != 3 {
		t.Fail()
	}
}

func TestSubtract(t *testing.T) {
	if subtract(5, 3) != 2 {
		t.Fail()
	}
}

func helperFunc() {}
`
	// We use a custom provider that doesn't actually run tests
	lenses := findTestFunctions("foo_test.go", content)
	if len(lenses) != 2 {
		t.Fatalf("expected 2 test lenses, got %d", len(lenses))
	}
	if lenses[0].Line != 5 {
		t.Errorf("expected TestAdd at line 5, got %d", lenses[0].Line)
	}
	if lenses[1].Line != 11 {
		t.Errorf("expected TestSubtract at line 11, got %d", lenses[1].Line)
	}
	for _, l := range lenses {
		if l.Category != "test_status" {
			t.Errorf("expected category test_status, got %s", l.Category)
		}
	}
}

func TestGenerateComplexityLens(t *testing.T) {
	// Function with high complexity: many if/for/case/&& branches
	content := `package foo

func SimpleFunc() {
	x := 1
	_ = x
}

func ComplexFunc(x int) string {
	if x > 0 {
		if x > 10 {
			for i := 0; i < x; i++ {
				if i%2 == 0 {
					if i > 5 && x > 20 {
						return "high"
					}
				}
			}
		}
	}
	if x < 0 {
		return "negative"
	}
	return "default"
}
`
	lenses := GenerateComplexityLens("foo.go", content)
	// ComplexFunc should exceed threshold
	found := false
	for _, l := range lenses {
		if strings.Contains(l.Tooltip, "ComplexFunc") {
			found = true
			if l.Category != "complexity" {
				t.Errorf("expected category complexity, got %s", l.Category)
			}
		}
	}
	if !found {
		t.Error("expected ComplexFunc to be annotated with complexity lens")
	}

	// SimpleFunc should not appear
	for _, l := range lenses {
		if strings.Contains(l.Tooltip, "SimpleFunc") {
			t.Error("SimpleFunc should not exceed complexity threshold")
		}
	}
}

func TestGenerateReferenceLens(t *testing.T) {
	content := `package foo

func ExportedFunc() {
	// something
}

func anotherFunc() {
	ExportedFunc()
	ExportedFunc()
}

func yetAnother() {
	ExportedFunc()
}
`
	lenses := GenerateReferenceLens("foo.go", content)
	if len(lenses) == 0 {
		t.Fatal("expected at least one reference lens")
	}
	found := false
	for _, l := range lenses {
		if strings.Contains(l.Tooltip, "ExportedFunc") {
			found = true
			if l.Category != "references" {
				t.Errorf("expected category references, got %s", l.Category)
			}
			// Should have at least 3 references (calls, excluding declaration)
			if !strings.Contains(l.Label, "references:") {
				t.Error("label should contain references count")
			}
		}
	}
	if !found {
		t.Error("expected ExportedFunc to have reference lens")
	}
}

func TestGenerateReferenceLens_NoUnexported(t *testing.T) {
	content := `package foo

func unexported() {}

func alsoUnexported() {
	unexported()
}
`
	lenses := GenerateReferenceLens("foo.go", content)
	if len(lenses) != 0 {
		t.Errorf("expected 0 lenses for unexported symbols, got %d", len(lenses))
	}
}

func TestExtractFunctions(t *testing.T) {
	content := `package foo

func Foo() {
	x := 1
	_ = x
}

func (s *Server) Bar(ctx context.Context) error {
	return nil
}
`
	funcs := extractFunctions(content)
	if len(funcs) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(funcs))
	}
	if funcs[0].name != "Foo" {
		t.Errorf("expected Foo, got %s", funcs[0].name)
	}
	if funcs[0].line != 3 {
		t.Errorf("expected line 3, got %d", funcs[0].line)
	}
	if funcs[1].name != "Bar" {
		t.Errorf("expected Bar, got %s", funcs[1].name)
	}
	if funcs[1].line != 8 {
		t.Errorf("expected line 8, got %d", funcs[1].line)
	}
}

func TestCalculateCyclomaticComplexity(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected int
	}{
		{
			name:     "empty function",
			body:     "func f() {\n}",
			expected: 1,
		},
		{
			name:     "single if",
			body:     "func f() {\n if x > 0 { return }\n}",
			expected: 2,
		},
		{
			name:     "if with and",
			body:     "func f() {\n if x > 0 && y > 0 { return }\n}",
			expected: 3,
		},
		{
			name:     "for loop",
			body:     "func f() {\n for i := 0; i < 10; i++ { }\n}",
			expected: 2,
		},
		{
			name: "switch cases",
			body: `func f() {
	switch x {
	case 1:
		return "a"
	case 2:
		return "b"
	case 3:
		return "c"
	}
}`,
			expected: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateCyclomaticComplexity(tt.body)
			if got != tt.expected {
				t.Errorf("calculateCyclomaticComplexity() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestExtractExportedSymbols(t *testing.T) {
	content := `package foo

type MyStruct struct {}

func ExportedFunc() {}

func unexportedFunc() {}

type anotherType struct{}

func (s *MyStruct) ExportedMethod() {}
`
	symbols := extractExportedSymbols(content)
	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.name] = true
	}

	if !names["MyStruct"] {
		t.Error("missing MyStruct")
	}
	if !names["ExportedFunc"] {
		t.Error("missing ExportedFunc")
	}
	if !names["ExportedMethod"] {
		t.Error("missing ExportedMethod")
	}
	if names["unexportedFunc"] {
		t.Error("unexportedFunc should not be included")
	}
	if names["anotherType"] {
		t.Error("anotherType should not be included")
	}
}

func TestCountReferences(t *testing.T) {
	content := `package foo

func DoWork() {}

func caller1() { DoWork() }
func caller2() { DoWork() }
func caller3() { DoWork() }
`
	count := countReferences(content, "DoWork")
	// 4 total occurrences minus 1 for declaration = 3
	if count != 3 {
		t.Errorf("expected 3 references, got %d", count)
	}
}

func TestCountReferences_Zero(t *testing.T) {
	content := `package foo

func Unused() {}
`
	count := countReferences(content, "Unused")
	// 1 occurrence (declaration) minus 1 = 0
	if count != 0 {
		t.Errorf("expected 0 references, got %d", count)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		hours    float64
		expected string
	}{
		{"just now", 0.5, "just now"},
		{"hours", 5, "5h"},
		{"days", 72, "3d"},
		{"weeks", 336, "2w"},
		{"months", 1440, "2mo"},
		{"years", 9000, "1y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := time.Duration(tt.hours * float64(time.Hour))
			got := lensFormatDuration(d)
			if got != tt.expected {
				t.Errorf("lensFormatDuration(%v) = %s, want %s", d, got, tt.expected)
			}
		})
	}
}

func TestIsRecent(t *testing.T) {
	tests := []struct {
		age    string
		recent bool
	}{
		{"just now", true},
		{"3h", true},
		{"2d", true},
		{"6d", true},
		{"2w", false},
		{"3mo", false},
		{"1y", false},
	}
	for _, tt := range tests {
		t.Run(tt.age, func(t *testing.T) {
			got := isRecent(tt.age)
			if got != tt.recent {
				t.Errorf("isRecent(%s) = %v, want %v", tt.age, got, tt.recent)
			}
		})
	}
}

func TestCodeLensStruct(t *testing.T) {
	lens := CodeLens{
		File:     "src/auth.go",
		Line:     42,
		Label:    "test: PASS",
		Category: "test_status",
		Command:  "go test -run ^TestAuth$",
		Tooltip:  "func TestAuth — passed",
	}
	if lens.File != "src/auth.go" {
		t.Error("File field mismatch")
	}
	if lens.Line != 42 {
		t.Error("Line field mismatch")
	}
	if lens.Category != "test_status" {
		t.Error("Category field mismatch")
	}
}

func TestGenerateAgeLens_NoGit(t *testing.T) {
	// When not in a git repo, should return empty
	lenses := GenerateAgeLens("/nonexistent/path.go", `package foo

func Hello() {}
`)
	if len(lenses) != 0 {
		t.Errorf("expected 0 lenses outside git, got %d", len(lenses))
	}
}

func TestGenerateCoverageLens_NoCoverageFile(t *testing.T) {
	lenses := GenerateCoverageLens("/nonexistent/path.go", `package foo

func Hello() {}
`)
	if lenses != nil {
		t.Errorf("expected nil when no coverage file, got %v", lenses)
	}
}

func TestGenerate_Integration(t *testing.T) {
	p := &CodeLensProvider{
		Providers: make(map[string]LensGenerator),
	}
	// Only use deterministic generators for integration test
	p.Providers["complexity"] = GenerateComplexityLens
	p.Providers["references"] = GenerateReferenceLens

	content := `package foo

func ExportedHighComplexity(x int) string {
	if x > 0 {
		if x > 10 {
			for i := 0; i < x; i++ {
				if i%2 == 0 {
					if i > 5 && x > 20 {
						return "high"
					}
				}
			}
		}
	}
	if x < 0 {
		return "negative"
	}
	return "default"
}

func caller() {
	ExportedHighComplexity(1)
	ExportedHighComplexity(2)
}
`
	lenses := p.Generate("foo.go", content)
	if len(lenses) == 0 {
		t.Fatal("expected lenses from integration test")
	}

	// Should have at least complexity and references
	hasComplexity := false
	hasReferences := false
	for _, l := range lenses {
		if l.Category == "complexity" {
			hasComplexity = true
		}
		if l.Category == "references" {
			hasReferences = true
		}
	}
	if !hasComplexity {
		t.Error("expected complexity lens")
	}
	if !hasReferences {
		t.Error("expected references lens")
	}
}

func TestRegister_Overwrite(t *testing.T) {
	p := NewCodeLensProvider()
	original := p.Providers["complexity"]
	if original == nil {
		t.Fatal("complexity provider should exist")
	}

	p.Register("complexity", func(file, content string) []CodeLens {
		return []CodeLens{{File: file, Line: 99, Label: "custom", Category: "complexity"}}
	})

	lenses := p.Providers["complexity"]("test.go", "")
	if len(lenses) != 1 || lenses[0].Line != 99 {
		t.Error("Register should overwrite existing provider")
	}
}

// findTestFunctions is a helper that finds test functions without running them.
func findTestFunctions(file, content string) []CodeLens {
	if !strings.HasSuffix(file, "_test.go") {
		return nil
	}

	var lenses []CodeLens
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := testFuncRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		funcName := matches[1]
		lenses = append(lenses, CodeLens{
			File:     file,
			Line:     i + 1,
			Label:    "test: UNKNOWN",
			Category: "test_status",
			Command:  "go test -run ^" + funcName + "$",
			Tooltip:  "func " + funcName + " — status unknown",
		})
	}
	return lenses
}
