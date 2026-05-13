package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestNewConventionSet(t *testing.T) {
	cs := NewConventionSet("/some/project")
	if cs == nil {
		t.Fatal("NewConventionSet returned nil")
	}
	if cs.ProjectDir != "/some/project" {
		t.Errorf("ProjectDir = %q, want %q", cs.ProjectDir, "/some/project")
	}
	if len(cs.Conventions) != 0 {
		t.Errorf("Conventions length = %d, want 0", len(cs.Conventions))
	}
}

func TestLearnConventions_NonExistentDir(t *testing.T) {
	cs := NewConventionSet("/nonexistent")
	err := cs.LearnConventions("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestLearnConventions_GoProject(t *testing.T) {
	// Create a temporary project with Go files.
	dir := t.TempDir()

	// Write source files with camelCase vars and error wrapping.
	src := `package myproject

import (
	"fmt"
	"os"

	"github.com/example/pkg"
)

var ErrNotFound = fmt.Errorf("not found")
var ErrTimeout = fmt.Errorf("timeout")
var ErrInvalid = fmt.Errorf("invalid")

func GetUser(id string) (*User, error) {
	userName := lookupName(id)
	userAge := lookupAge(id)
	if userName == "" {
		return nil, fmt.Errorf("getting user %s: %w", id, ErrNotFound)
	}
	f, err := os.Open("data.json")
	if err != nil {
		return nil, fmt.Errorf("opening data: %w", err)
	}
	defer f.Close()
	_ = userAge
	_ = pkg.Version
	return &User{Name: userName}, nil
}

func ProcessItems(items []string) error {
	for _, item := range items {
		itemValue := transform(item)
		if err := validate(itemValue); err != nil {
			return fmt.Errorf("processing item %s: %w", item, err)
		}
	}
	return nil
}
`
	if err := os.WriteFile(filepath.Join(dir, "user.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a test file with table-driven tests.
	testSrc := `package myproject

import "testing"

func TestGetUser(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{name: "valid", id: "123", wantErr: false},
		{name: "missing", id: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := GetUser(tc.id)
			if (err != nil) != tc.wantErr {
				t.Errorf("GetUser(%q) error = %v, wantErr %v", tc.id, err, tc.wantErr)
			}
		})
	}
}

func TestProcessItems(t *testing.T) {
	tests := []struct {
		name    string
		items   []string
		wantErr bool
	}{
		{name: "empty", items: nil, wantErr: false},
		{name: "valid", items: []string{"a", "b"}, wantErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ProcessItems(tc.items)
			if (err != nil) != tc.wantErr {
				t.Errorf("ProcessItems error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "user_test.go"), []byte(testSrc), 0644); err != nil {
		t.Fatal(err)
	}

	cs := NewConventionSet(dir)
	if err := cs.LearnConventions(dir); err != nil {
		t.Fatalf("LearnConventions failed: %v", err)
	}

	if len(cs.Conventions) == 0 {
		t.Fatal("expected conventions to be learned")
	}

	// Should have learned error wrapping convention.
	found := false
	for _, c := range cs.Conventions {
		if c.Name == "error wrapping with %w" {
			found = true
			if c.Category != "error_handling" {
				t.Errorf("error wrapping category = %q, want %q", c.Category, "error_handling")
			}
			if c.Confidence <= 0 {
				t.Error("expected positive confidence")
			}
			break
		}
	}
	if !found {
		t.Error("expected to learn 'error wrapping with %w' convention")
	}

	// Should have learned grouped imports.
	found = false
	for _, c := range cs.Conventions {
		if c.Name == "grouped imports" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to learn 'grouped imports' convention")
	}

	// Should have learned table-driven tests.
	found = false
	for _, c := range cs.Conventions {
		if c.Name == "table-driven tests" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to learn 'table-driven tests' convention")
	}
}

func TestCheckNaming(t *testing.T) {
	cs := NewConventionSet("/test")
	cs.AddConvention(Convention{
		Name:        "camelCase variables",
		Description: "Local variables use camelCase",
		Pattern:     regexp.MustCompile(`[a-z][a-zA-Z0-9]*\s*:=`),
		AntiPattern: regexp.MustCompile(`[a-z]+_[a-z]+\s*:=`),
		Language:    "go",
		Category:    "naming",
		Example:     "userName := getValue()",
		Confidence:  0.9,
	})

	code := `package main

func main() {
	user_name := "test"
	user_age := 25
	goodVar := true
	_ = user_name
	_ = user_age
	_ = goodVar
}
`
	violations := cs.CheckNaming(code)
	if len(violations) < 2 {
		t.Errorf("expected at least 2 violations, got %d", len(violations))
	}

	// Verify that goodVar does not trigger a violation.
	for _, v := range violations {
		if strings.Contains(v.Code, "goodVar") {
			t.Error("goodVar should not be flagged")
		}
	}
}

func TestCheckErrorHandling(t *testing.T) {
	cs := NewConventionSet("/test")
	cs.AddConvention(Convention{
		Name:        "error wrapping with %w",
		Description: "Errors are wrapped with fmt.Errorf and %w",
		Pattern:     regexp.MustCompile(`fmt\.Errorf\(.+%w`),
		AntiPattern: regexp.MustCompile(`return\s+err\s*$`),
		Language:    "go",
		Category:    "error_handling",
		Example:     `return fmt.Errorf("context: %w", err)`,
		Confidence:  0.9,
	})

	code := `package main

import "fmt"

func doSomething() error {
	if err := step1(); err != nil {
		return err
	}
	if err := step2(); err != nil {
		return fmt.Errorf("step2: %w", err)
	}
	return nil
}
`
	violations := cs.CheckErrorHandling(code)
	if len(violations) != 1 {
		t.Errorf("expected 1 violation, got %d", len(violations))
		for _, v := range violations {
			t.Logf("  violation: %s at line %d", v.Convention, v.Line)
		}
		return
	}

	v := violations[0]
	if v.Line != 7 {
		t.Errorf("violation line = %d, want 7", v.Line)
	}
	if v.Convention != "error wrapping with %w" {
		t.Errorf("violation convention = %q", v.Convention)
	}
}

func TestCheckTestStyle(t *testing.T) {
	cs := NewConventionSet("/test")
	cs.AddConvention(Convention{
		Name:        "table-driven tests",
		Description: "Tests use table-driven pattern with subtests",
		Pattern:     regexp.MustCompile(`(?:tests|cases)\s*:=\s*\[\]`),
		AntiPattern: nil,
		Language:    "go",
		Category:    "testing",
		Example:     "tests := []struct{ name string; input string; want string }{...}",
		Confidence:  0.8,
	})
	cs.AddConvention(Convention{
		Name:        "stdlib testing only",
		Description: "Tests use only stdlib testing package",
		Pattern:     regexp.MustCompile(`"testing"`),
		AntiPattern: regexp.MustCompile(`"github\.com/stretchr/testify`),
		Language:    "go",
		Category:    "testing",
		Example:     `if got != want { t.Errorf(...) }`,
		Confidence:  0.9,
	})

	// Code without table-driven tests.
	code := `package main

import "testing"

func TestFoo(t *testing.T) {
	got := Foo()
	if got != "bar" {
		t.Errorf("got %q, want %q", got, "bar")
	}
}
`
	violations := cs.CheckTestStyle(code)
	if len(violations) == 0 {
		t.Error("expected violations for non-table-driven test")
	}

	// Table-driven tests should pass.
	tableCode := `package main

import "testing"

func TestFoo(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"basic", "bar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Foo()
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
`
	violations = cs.CheckTestStyle(tableCode)
	// Should have no table-driven violation.
	for _, v := range violations {
		if v.Convention == "table-driven tests" {
			t.Errorf("table-driven code should not trigger violation, got: %+v", v)
		}
	}

	// Code with testify should be flagged.
	testifyCode := `package main

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestBar(t *testing.T) {
	assert.Equal(t, "bar", Bar())
}
`
	violations = cs.CheckTestStyle(testifyCode)
	found := false
	for _, v := range violations {
		if v.Convention == "stdlib testing only" {
			found = true
		}
	}
	if !found {
		t.Error("expected stdlib testing violation for testify usage")
	}
}

func TestCheck(t *testing.T) {
	cs := NewConventionSet("/test")
	cs.AddConvention(Convention{
		Name:        "camelCase variables",
		Description: "Local variables use camelCase",
		Pattern:     regexp.MustCompile(`[a-z][a-zA-Z0-9]*\s*:=`),
		AntiPattern: regexp.MustCompile(`[a-z]+_[a-z]+\s*:=`),
		Language:    "go",
		Category:    "naming",
		Example:     "userName := getValue()",
		Confidence:  0.9,
	})
	cs.AddConvention(Convention{
		Name:        "error wrapping with %w",
		Description: "Errors are wrapped",
		Pattern:     regexp.MustCompile(`fmt\.Errorf\(.+%w`),
		AntiPattern: regexp.MustCompile(`return\s+err\s*$`),
		Language:    "go",
		Category:    "error_handling",
		Example:     `return fmt.Errorf("context: %w", err)`,
		Confidence:  0.9,
	})

	code := `package main

func handler() error {
	bad_var := getData()
	if err := process(bad_var); err != nil {
		return err
	}
	return nil
}
`
	violations := cs.Check(code, "handler.go")
	if len(violations) < 2 {
		t.Errorf("expected at least 2 violations, got %d", len(violations))
	}

	// All violations should have file set.
	for _, v := range violations {
		if v.File != "handler.go" {
			t.Errorf("violation file = %q, want %q", v.File, "handler.go")
		}
	}
}

func TestEnforce(t *testing.T) {
	cs := NewConventionSet("/test")
	cs.AddConvention(Convention{
		Name:        "error wrapping with %w",
		Description: "Errors are wrapped",
		Pattern:     regexp.MustCompile(`fmt\.Errorf\(.+%w`),
		AntiPattern: regexp.MustCompile(`return\s+err\s*$`),
		Language:    "go",
		Category:    "error_handling",
		Example:     `return fmt.Errorf("context: %w", err)`,
		Confidence:  0.9,
	})
	cs.AddConvention(Convention{
		Name:        "camelCase variables",
		Description: "Local variables use camelCase",
		Pattern:     regexp.MustCompile(`[a-z][a-zA-Z0-9]*\s*:=`),
		AntiPattern: regexp.MustCompile(`[a-z]+_[a-z]+\s*:=`),
		Language:    "go",
		Category:    "naming",
		Example:     "userName := getValue()",
		Confidence:  0.9,
	})

	code := `package main

func GetUser(id string) (*User, error) {
	user_name := lookupName(id)
	if err := validate(user_name); err != nil {
		return err
	}
	return &User{Name: user_name}, nil
}
`
	fixed, remaining := cs.Enforce(code)

	// The bare "return err" should be fixed.
	if strings.Contains(fixed, "return err\n") {
		t.Error("expected bare 'return err' to be fixed")
	}
	if !strings.Contains(fixed, "%w") {
		t.Error("expected error wrapping in fixed code")
	}

	// The snake_case var should be fixed.
	if strings.Contains(fixed, "user_name :=") {
		t.Error("expected snake_case variable to be fixed to camelCase")
	}
	if !strings.Contains(fixed, "userName :=") {
		t.Error("expected 'userName' in fixed code")
	}

	// There may still be remaining violations if the fix isn't perfect.
	_ = remaining
}

func TestAddConvention(t *testing.T) {
	cs := NewConventionSet("/test")
	if len(cs.Conventions) != 0 {
		t.Fatalf("expected 0 conventions, got %d", len(cs.Conventions))
	}

	cs.AddConvention(Convention{
		Name:       "custom rule",
		Category:   "style",
		Confidence: 1.0,
	})

	if len(cs.Conventions) != 1 {
		t.Fatalf("expected 1 convention, got %d", len(cs.Conventions))
	}
	if cs.Conventions[0].Name != "custom rule" {
		t.Errorf("convention name = %q, want %q", cs.Conventions[0].Name, "custom rule")
	}
}

func TestFormatViolations(t *testing.T) {
	violations := []Violation{
		{Convention: "naming", File: "main.go", Line: 5, Code: "user_name := x", Expected: "camelCase", Got: "user_name := x"},
		{Convention: "error_handling", File: "main.go", Line: 10, Code: "return err", Expected: "wrap with %w", Got: "return err"},
		{Convention: "testing", File: "main_test.go", Line: 1, Code: "no table-driven", Expected: "table-driven pattern", Got: "individual assertions"},
	}

	output := FormatViolations(violations)
	if !strings.Contains(output, "Convention Violations (3)") {
		t.Errorf("missing header, got:\n%s", output)
	}
	if !strings.Contains(output, "─") {
		t.Error("missing separator line")
	}
	if !strings.Contains(output, "⚠") {
		t.Error("missing warning symbol")
	}
	if !strings.Contains(output, "naming") {
		t.Error("missing naming violation")
	}
	if !strings.Contains(output, "error_handling") {
		t.Error("missing error_handling violation")
	}
}

func TestFormatViolations_Empty(t *testing.T) {
	output := FormatViolations(nil)
	if !strings.Contains(output, "No convention violations found") {
		t.Errorf("unexpected output for empty violations: %s", output)
	}
}

func TestFormatConventions(t *testing.T) {
	cs := NewConventionSet("/test")
	cs.AddConvention(Convention{
		Name:        "camelCase variables",
		Description: "Local variables use camelCase",
		Category:    "naming",
		Example:     "userName := getValue()",
		Confidence:  0.9,
	})
	cs.AddConvention(Convention{
		Name:        "error wrapping with %w",
		Description: "Errors are wrapped with %w",
		Category:    "error_handling",
		Example:     `return fmt.Errorf("ctx: %w", err)`,
		Confidence:  0.85,
	})
	cs.AddConvention(Convention{
		Name:        "table-driven tests",
		Description: "Tests use table-driven pattern",
		Category:    "testing",
		Example:     "tests := []struct{...}{...}",
		Confidence:  0.75,
	})

	output := cs.FormatConventions()
	if !strings.Contains(output, "Project Conventions (3)") {
		t.Errorf("missing header, got:\n%s", output)
	}
	if !strings.Contains(output, "[Naming]") {
		t.Error("missing Naming category")
	}
	if !strings.Contains(output, "[Error Handling]") {
		t.Error("missing Error Handling category")
	}
	if !strings.Contains(output, "[Testing]") {
		t.Error("missing Testing category")
	}
	if !strings.Contains(output, "90%") {
		t.Error("missing confidence percentage")
	}
	if !strings.Contains(output, "Example:") {
		t.Error("missing example")
	}
}

func TestFormatConventions_Empty(t *testing.T) {
	cs := NewConventionSet("/test")
	output := cs.FormatConventions()
	if !strings.Contains(output, "No conventions learned yet") {
		t.Errorf("unexpected output for empty conventions: %s", output)
	}
}

func TestSnakeToCamel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user_name", "userName"},
		{"get_user_by_id", "getUserById"},
		{"simple", "simple"},
		{"a_b_c", "aBC"},
	}
	for _, tc := range tests {
		got := snakeToCamel(tc.input)
		if got != tc.want {
			t.Errorf("snakeToCamel(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCamelToWords(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"GetUser", "get user"},
		{"processItems", "process items"},
		{"", ""},
		{"simple", "simple"},
	}
	for _, tc := range tests {
		got := camelToWords(tc.input)
		if got != tc.want {
			t.Errorf("camelToWords(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestConventionConfidence(t *testing.T) {
	tests := []struct {
		matching    int
		nonMatching int
		wantMin     float64
		wantMax     float64
	}{
		{100, 0, 0.9, 0.96},
		{0, 0, 0.0, 0.01},
		{50, 50, 0.49, 0.51},
		{80, 20, 0.79, 0.81},
	}
	for _, tc := range tests {
		got := conventionConfidence(tc.matching, tc.nonMatching)
		if got < tc.wantMin || got > tc.wantMax {
			t.Errorf("conventionConfidence(%d, %d) = %f, want in [%f, %f]",
				tc.matching, tc.nonMatching, got, tc.wantMin, tc.wantMax)
		}
	}
}

func TestLearnConventions_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	cs := NewConventionSet(dir)
	err := cs.LearnConventions(dir)
	if err != nil {
		t.Fatalf("unexpected error for empty directory: %v", err)
	}
	if len(cs.Conventions) != 0 {
		t.Errorf("expected 0 conventions for empty dir, got %d", len(cs.Conventions))
	}
}

func TestConventionEnforcerConcurrency(t *testing.T) {
	cs := NewConventionSet("/test")
	cs.AddConvention(Convention{
		Name:        "test rule",
		Description: "test",
		AntiPattern: regexp.MustCompile(`bad_pattern`),
		Category:    "naming",
		Confidence:  0.9,
	})

	done := make(chan struct{})
	// Concurrent reads.
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			cs.Check("some bad_pattern code", "test.go")
		}()
	}
	// Concurrent write.
	go func() {
		defer func() { done <- struct{}{} }()
		cs.AddConvention(Convention{
			Name:     "another",
			Category: "style",
		})
	}()

	for i := 0; i < 11; i++ {
		<-done
	}
}
