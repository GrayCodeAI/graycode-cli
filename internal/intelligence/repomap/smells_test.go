package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSmellDetector(t *testing.T) {
	sd := NewSmellDetector()
	if sd == nil {
		t.Fatal("NewSmellDetector returned nil")
	}
	if sd.Thresholds.MaxParams != 5 {
		t.Errorf("expected MaxParams=5, got %d", sd.Thresholds.MaxParams)
	}
	if sd.Thresholds.MaxMethodsPerType != 15 {
		t.Errorf("expected MaxMethodsPerType=15, got %d", sd.Thresholds.MaxMethodsPerType)
	}
	if sd.Thresholds.MaxFieldsPerStruct != 12 {
		t.Errorf("expected MaxFieldsPerStruct=12, got %d", sd.Thresholds.MaxFieldsPerStruct)
	}
	if sd.Thresholds.MaxImports != 15 {
		t.Errorf("expected MaxImports=15, got %d", sd.Thresholds.MaxImports)
	}
	if sd.Thresholds.MaxFileLines != 500 {
		t.Errorf("expected MaxFileLines=500, got %d", sd.Thresholds.MaxFileLines)
	}
	if sd.Thresholds.MaxFuncLines != 50 {
		t.Errorf("expected MaxFuncLines=50, got %d", sd.Thresholds.MaxFuncLines)
	}
	if sd.Thresholds.MinMethodCohesion != 0.3 {
		t.Errorf("expected MinMethodCohesion=0.3, got %f", sd.Thresholds.MinMethodCohesion)
	}
}

func TestDetectGodObject_Methods(t *testing.T) {
	sd := NewSmellDetector()
	sd.Thresholds.MaxMethodsPerType = 3 // Lower threshold for testing

	// Generate a type with too many methods
	var sb strings.Builder
	sb.WriteString("package main\n\ntype BigService struct{}\n\n")
	for i := 0; i < 5; i++ {
		sb.WriteString("func (b *BigService) Method")
		sb.WriteString(string(rune('A' + i)))
		sb.WriteString("() {}\n\n")
	}

	smells := sd.DetectGodObject(sb.String())
	if len(smells) == 0 {
		t.Fatal("expected God Object smell for type with too many methods")
	}

	found := false
	for _, s := range smells {
		if s.ID == "god-object-methods" && strings.Contains(s.Description, "BigService") {
			found = true
			if s.Category != "design" {
				t.Errorf("expected category 'design', got %q", s.Category)
			}
			if s.Severity != "major" {
				t.Errorf("expected severity 'major', got %q", s.Severity)
			}
		}
	}
	if !found {
		t.Error("expected god-object-methods smell for BigService")
	}
}

func TestDetectGodObject_Fields(t *testing.T) {
	sd := NewSmellDetector()
	sd.Thresholds.MaxFieldsPerStruct = 3 // Lower threshold for testing

	src := `package main

type HugeStruct struct {
	A string
	B string
	C string
	D string
	E string
}
`
	smells := sd.DetectGodObject(src)
	if len(smells) == 0 {
		t.Fatal("expected God Object smell for struct with too many fields")
	}

	found := false
	for _, s := range smells {
		if s.ID == "god-object-fields" && strings.Contains(s.Description, "HugeStruct") {
			found = true
			if !strings.Contains(s.Description, "5 fields") {
				t.Errorf("expected description mentioning 5 fields, got %q", s.Description)
			}
		}
	}
	if !found {
		t.Error("expected god-object-fields smell for HugeStruct")
	}
}

func TestDetectGodObject_NoSmell(t *testing.T) {
	sd := NewSmellDetector()

	src := `package main

type SmallService struct {
	name string
}

func (s *SmallService) Name() string { return s.name }
func (s *SmallService) SetName(n string) { s.name = n }
`
	smells := sd.DetectGodObject(src)
	if len(smells) != 0 {
		t.Errorf("expected no smells for well-designed type, got %d: %v", len(smells), smells)
	}
}

func TestDetectLongParamList(t *testing.T) {
	sd := NewSmellDetector()
	sd.Thresholds.MaxParams = 3

	src := `package main

func TooManyParams(a string, b int, c bool, d float64, e string) {}
func OkParams(a string, b int) {}
`
	smells := sd.DetectLongParamList(src)
	if len(smells) != 1 {
		t.Fatalf("expected 1 smell, got %d", len(smells))
	}

	s := smells[0]
	if s.ID != "long-param-list" {
		t.Errorf("expected ID 'long-param-list', got %q", s.ID)
	}
	if !strings.Contains(s.Description, "TooManyParams") {
		t.Errorf("expected description to mention TooManyParams, got %q", s.Description)
	}
	if s.Category != "design" {
		t.Errorf("expected category 'design', got %q", s.Category)
	}
}

func TestDetectFeatureEnvy(t *testing.T) {
	sd := NewSmellDetector()

	src := `package main

type User struct {
	Name string
	Email string
	Age int
}

type Formatter struct {
	prefix string
}

func (f *Formatter) FormatUser(u User) string {
	return u.Name + " " + u.Email + " " + u.Name + " " + u.Email
}
`
	smells := sd.DetectFeatureEnvy(src)
	if len(smells) == 0 {
		t.Fatal("expected Feature Envy smell")
	}

	found := false
	for _, s := range smells {
		if s.ID == "feature-envy" {
			found = true
			if s.Category != "coupling" {
				t.Errorf("expected category 'coupling', got %q", s.Category)
			}
			if s.Severity != "minor" {
				t.Errorf("expected severity 'minor', got %q", s.Severity)
			}
		}
	}
	if !found {
		t.Error("expected feature-envy smell")
	}
}

func TestDetectFeatureEnvy_NoSmell(t *testing.T) {
	sd := NewSmellDetector()

	src := `package main

type Service struct {
	name string
	count int
}

func (s *Service) Process() {
	_ = s.name
	_ = s.count
	_ = s.name
}
`
	smells := sd.DetectFeatureEnvy(src)
	if len(smells) != 0 {
		t.Errorf("expected no smells, got %d", len(smells))
	}
}

func TestDetectDataClump(t *testing.T) {
	sd := NewSmellDetector()

	src := `package main

func CreateUser(name string, email string, age int, addr string) {}
func UpdateUser(name string, email string, age int, phone string) {}
`
	smells := sd.DetectDataClump(src)
	if len(smells) == 0 {
		t.Fatal("expected Data Clump smell")
	}

	found := false
	for _, s := range smells {
		if s.ID == "data-clump" {
			found = true
			if s.Category != "design" {
				t.Errorf("expected category 'design', got %q", s.Category)
			}
			if !strings.Contains(s.Description, "CreateUser") || !strings.Contains(s.Description, "UpdateUser") {
				t.Errorf("expected description to mention both functions, got %q", s.Description)
			}
		}
	}
	if !found {
		t.Error("expected data-clump smell")
	}
}

func TestDetectDataClump_NoSmell(t *testing.T) {
	sd := NewSmellDetector()

	src := `package main

func Foo(a string) {}
func Bar(b int) {}
`
	smells := sd.DetectDataClump(src)
	if len(smells) != 0 {
		t.Errorf("expected no smells, got %d", len(smells))
	}
}

func TestDetectPrimitiveObsession(t *testing.T) {
	sd := NewSmellDetector()

	src := `package main

func ProcessOrder(id string, name string, email string, phone string, amount float64) {}
`
	smells := sd.DetectPrimitiveObsession(src)
	if len(smells) == 0 {
		t.Fatal("expected Primitive Obsession smell")
	}

	s := smells[0]
	if s.ID != "primitive-obsession" {
		t.Errorf("expected ID 'primitive-obsession', got %q", s.ID)
	}
	if s.Category != "design" {
		t.Errorf("expected category 'design', got %q", s.Category)
	}
	if !strings.Contains(s.Description, "ProcessOrder") {
		t.Errorf("expected description to mention ProcessOrder, got %q", s.Description)
	}
}

func TestDetectPrimitiveObsession_NoSmell(t *testing.T) {
	sd := NewSmellDetector()

	src := `package main

type Config struct{}

func Process(cfg *Config, name string) {}
`
	smells := sd.DetectPrimitiveObsession(src)
	if len(smells) != 0 {
		t.Errorf("expected no smells, got %d", len(smells))
	}
}

func TestDetectLongMethod(t *testing.T) {
	sd := NewSmellDetector()
	sd.Thresholds.MaxFuncLines = 5

	// Generate a function that exceeds the threshold
	var sb strings.Builder
	sb.WriteString("package main\n\nfunc LongFunc() {\n")
	for i := 0; i < 10; i++ {
		sb.WriteString("\t_ = 1\n")
	}
	sb.WriteString("}\n")

	smells := sd.detectLongMethod(sb.String())
	if len(smells) == 0 {
		t.Fatal("expected Long Method smell")
	}

	s := smells[0]
	if s.ID != "long-method" {
		t.Errorf("expected ID 'long-method', got %q", s.ID)
	}
	if s.Category != "complexity" {
		t.Errorf("expected category 'complexity', got %q", s.Category)
	}
}

func TestDetectExcessiveImports(t *testing.T) {
	sd := NewSmellDetector()
	sd.Thresholds.MaxImports = 3

	src := `package main

import (
	"fmt"
	"os"
	"strings"
	"path/filepath"
	"io"
)
`
	smells := sd.detectExcessiveImports(src)
	if len(smells) == 0 {
		t.Fatal("expected Excessive Imports smell")
	}

	s := smells[0]
	if s.ID != "excessive-imports" {
		t.Errorf("expected ID 'excessive-imports', got %q", s.ID)
	}
	if s.Category != "coupling" {
		t.Errorf("expected category 'coupling', got %q", s.Category)
	}
}

func TestDetectInFile_GoFile(t *testing.T) {
	sd := NewSmellDetector()
	sd.Thresholds.MaxParams = 3
	sd.Thresholds.MaxFuncLines = 5

	var sb strings.Builder
	sb.WriteString("package main\n\n")
	sb.WriteString("func TooMany(a, b, c, d, e string) {\n")
	for i := 0; i < 10; i++ {
		sb.WriteString("\t_ = 1\n")
	}
	sb.WriteString("}\n")

	smells := sd.DetectInFile("handler.go", sb.String())
	if len(smells) == 0 {
		t.Fatal("expected smells in Go file")
	}

	// Should have both long-param-list and long-method
	foundParam := false
	foundMethod := false
	for _, s := range smells {
		if s.ID == "long-param-list" {
			foundParam = true
		}
		if s.ID == "long-method" {
			foundMethod = true
		}
		if s.File != "handler.go" {
			t.Errorf("expected File='handler.go', got %q", s.File)
		}
	}
	if !foundParam {
		t.Error("expected long-param-list smell")
	}
	if !foundMethod {
		t.Error("expected long-method smell")
	}
}

func TestDetectInFile_NonGoFile(t *testing.T) {
	sd := NewSmellDetector()
	sd.Thresholds.MaxFileLines = 5

	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\n"
	smells := sd.DetectInFile("script.py", content)
	if len(smells) == 0 {
		t.Fatal("expected large-file smell for non-Go file")
	}
	if smells[0].ID != "large-file" {
		t.Errorf("expected large-file, got %q", smells[0].ID)
	}
}

func TestFormatSmells(t *testing.T) {
	smells := []CodeSmell{
		{
			ID:                    "god-object-methods",
			Name:                  "God Object",
			File:                  "src/handler.go",
			Line:                  1,
			Severity:              "critical",
			Description:           "HandleService has 18 methods — extract into smaller services",
			RefactoringSuggestion: "Split into smaller services",
			Category:              "design",
		},
		{
			ID:                    "long-param-list",
			Name:                  "Long Parameter List",
			File:                  "src/handler.go",
			Line:                  45,
			Severity:              "major",
			Description:           "func Process(a, b, c, d, e, f string) — group into config struct",
			RefactoringSuggestion: "Group into config struct",
			Category:              "design",
		},
		{
			ID:                    "feature-envy",
			Name:                  "Feature Envy",
			File:                  "src/handler.go",
			Line:                  78,
			Severity:              "minor",
			Description:           "method formatUser uses User fields 8 times but own fields 1 time",
			RefactoringSuggestion: "Move method to User",
			Category:              "coupling",
		},
	}

	output := FormatSmells(smells)

	if !strings.Contains(output, "Code Smells in src/handler.go:") {
		t.Error("expected file header in output")
	}
	if !strings.Contains(output, "─────────────────────────────────") {
		t.Error("expected separator in output")
	}
	if !strings.Contains(output, "[critical]") {
		t.Error("expected [critical] in output")
	}
	if !strings.Contains(output, "[major]") {
		t.Error("expected [major] in output")
	}
	if !strings.Contains(output, "[minor]") {
		t.Error("expected [minor] in output")
	}
	if !strings.Contains(output, "God Object") {
		t.Error("expected 'God Object' in output")
	}
	if !strings.Contains(output, "Feature Envy") {
		t.Error("expected 'Feature Envy' in output")
	}

	// Critical should come before major, major before minor
	critIdx := strings.Index(output, "[critical]")
	majIdx := strings.Index(output, "[major]")
	minIdx := strings.Index(output, "[minor]")
	if critIdx > majIdx || majIdx > minIdx {
		t.Error("expected smells sorted by severity: critical > major > minor")
	}
}

func TestFormatSmells_Empty(t *testing.T) {
	output := FormatSmells(nil)
	if !strings.Contains(output, "No code smells detected") {
		t.Errorf("expected 'No code smells detected', got %q", output)
	}
}

func TestScanDirectory(t *testing.T) {
	// Create a temporary directory with test files
	dir := t.TempDir()

	// Write a Go file with smells
	var sb strings.Builder
	sb.WriteString("package main\n\n")
	sb.WriteString("type Monster struct{}\n\n")
	for i := 0; i < 5; i++ {
		sb.WriteString("func (m *Monster) Method")
		sb.WriteString(string(rune('A' + i)))
		sb.WriteString("() {}\n\n")
	}

	err := os.WriteFile(filepath.Join(dir, "monster.go"), []byte(sb.String()), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Write a clean Go file
	clean := `package main

func Hello() string { return "hello" }
`
	err = os.WriteFile(filepath.Join(dir, "clean.go"), []byte(clean), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	sd := NewSmellDetector()
	sd.Thresholds.MaxMethodsPerType = 3 // Lower threshold

	smells := sd.ScanDirectory(dir)
	if len(smells) == 0 {
		t.Fatal("expected smells from directory scan")
	}

	// Should find god-object in monster.go
	found := false
	for _, s := range smells {
		if s.ID == "god-object-methods" && strings.Contains(s.File, "monster.go") {
			found = true
		}
	}
	if !found {
		t.Error("expected god-object-methods in monster.go from directory scan")
	}
}

func TestScanDirectory_SkipsVendor(t *testing.T) {
	dir := t.TempDir()

	// Create vendor directory with smelly code
	vendorDir := filepath.Join(dir, "vendor")
	err := os.Mkdir(vendorDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder
	sb.WriteString("package vendored\n\n")
	sb.WriteString("type Huge struct{}\n\n")
	for i := 0; i < 20; i++ {
		sb.WriteString("func (h *Huge) M")
		sb.WriteString(string(rune('A' + i)))
		sb.WriteString("() {}\n\n")
	}
	err = os.WriteFile(filepath.Join(vendorDir, "huge.go"), []byte(sb.String()), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	sd := NewSmellDetector()
	smells := sd.ScanDirectory(dir)

	for _, s := range smells {
		if strings.Contains(s.File, "vendor") {
			t.Error("should not report smells from vendor directory")
		}
	}
}

func TestScanDirectory_SkipsTestFiles(t *testing.T) {
	dir := t.TempDir()

	var sb strings.Builder
	sb.WriteString("package main\n\n")
	sb.WriteString("type TestHelper struct{}\n\n")
	for i := 0; i < 20; i++ {
		sb.WriteString("func (h *TestHelper) Helper")
		sb.WriteString(string(rune('A' + i)))
		sb.WriteString("() {}\n\n")
	}
	err := os.WriteFile(filepath.Join(dir, "helpers_test.go"), []byte(sb.String()), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	sd := NewSmellDetector()
	smells := sd.ScanDirectory(dir)

	for _, s := range smells {
		if strings.Contains(s.File, "_test.go") {
			t.Error("should not report smells from test files")
		}
	}
}

func TestCodeSmellStruct(t *testing.T) {
	smell := CodeSmell{
		ID:                    "test-smell",
		Name:                  "Test Smell",
		File:                  "test.go",
		Line:                  42,
		Severity:              "major",
		Description:           "A test smell",
		RefactoringSuggestion: "Fix it",
		Category:              "design",
	}

	if smell.ID != "test-smell" {
		t.Errorf("unexpected ID: %s", smell.ID)
	}
	if smell.Name != "Test Smell" {
		t.Errorf("unexpected Name: %s", smell.Name)
	}
	if smell.File != "test.go" {
		t.Errorf("unexpected File: %s", smell.File)
	}
	if smell.Line != 42 {
		t.Errorf("unexpected Line: %d", smell.Line)
	}
	if smell.Severity != "major" {
		t.Errorf("unexpected Severity: %s", smell.Severity)
	}
	if smell.Description != "A test smell" {
		t.Errorf("unexpected Description: %s", smell.Description)
	}
	if smell.RefactoringSuggestion != "Fix it" {
		t.Errorf("unexpected RefactoringSuggestion: %s", smell.RefactoringSuggestion)
	}
	if smell.Category != "design" {
		t.Errorf("unexpected Category: %s", smell.Category)
	}
}

func TestSeverityRanking(t *testing.T) {
	// Test that critical severity is detected for very high values
	sd := NewSmellDetector()
	sd.Thresholds.MaxMethodsPerType = 3

	var sb strings.Builder
	sb.WriteString("package main\n\ntype CriticalService struct{}\n\n")
	for i := 0; i < 7; i++ { // More than 2x threshold
		sb.WriteString("func (c *CriticalService) M")
		sb.WriteString(string(rune('A' + i)))
		sb.WriteString("() {}\n\n")
	}

	smells := sd.DetectGodObject(sb.String())
	found := false
	for _, s := range smells {
		if s.ID == "god-object-methods" && s.Severity == "critical" {
			found = true
		}
	}
	if !found {
		t.Error("expected critical severity for > 2x threshold")
	}
}

func TestDetectInFile_InvalidGoSyntax(t *testing.T) {
	sd := NewSmellDetector()

	// Invalid Go code should not panic
	content := "package main\n\nfunc broken syntax here {"
	smells := sd.DetectInFile("broken.go", content)
	// Should not panic; may return empty or partial results
	_ = smells
}

func TestExprToString(t *testing.T) {
	cases := []struct {
		input    string
		contains string
	}{
		{"package main\nfunc F(a string) {}", "string"},
		{"package main\nfunc F(a *int) {}", "*int"},
		{"package main\nfunc F(a []byte) {}", "[]byte"},
	}

	for _, tc := range cases {
		sd := NewSmellDetector()
		sd.Thresholds.MaxParams = 0 // trigger on any params for test
		smells := sd.DetectLongParamList(tc.input)
		if len(smells) == 0 {
			t.Errorf("expected smell for input %q", tc.input)
			continue
		}
		if !strings.Contains(smells[0].Description, tc.contains) {
			t.Errorf("expected description containing %q, got %q", tc.contains, smells[0].Description)
		}
	}
}
