package validation

import (
	"strings"
	"testing"
)

func TestNewGenValidator(t *testing.T) {
	gv := NewGenValidator()
	if gv == nil {
		t.Fatal("NewGenValidator returned nil")
	}
	if len(gv.Checks) == 0 {
		t.Fatal("NewGenValidator should have built-in checks")
	}

	// Verify expected check names exist
	names := make(map[string]bool)
	for _, c := range gv.Checks {
		names[c.Name] = true
	}
	for _, expected := range []string{"syntax", "imports", "naming", "completeness", "compilation", "types"} {
		if !names[expected] {
			t.Errorf("expected check %q not found", expected)
		}
	}
}

func TestValidate_CleanGoCode(t *testing.T) {
	gv := NewGenValidator()

	code := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	result := gv.Validate(code, "go")
	// Filter out compilation issues (temp env may not have go toolchain configured properly)
	var nonCompileIssues []GenIssue
	for _, issue := range result.Issues {
		if issue.Check != "compilation" {
			nonCompileIssues = append(nonCompileIssues, issue)
		}
	}
	if len(nonCompileIssues) > 0 {
		for _, issue := range nonCompileIssues {
			t.Errorf("unexpected issue: %s — %s (line %d)", issue.Check, issue.Message, issue.Line)
		}
	}
}

func TestValidate_UnbalancedBraces(t *testing.T) {
	gv := NewGenValidator()

	code := `package main

func main() {
	if true {
		println("hello")
}
`
	result := gv.Validate(code, "go")
	if result.Valid {
		t.Error("expected validation to fail for unbalanced braces")
	}

	foundSyntax := false
	for _, issue := range result.Issues {
		if issue.Check == "syntax" && strings.Contains(issue.Message, "unclosed") {
			foundSyntax = true
			break
		}
	}
	if !foundSyntax {
		t.Error("expected syntax issue for unclosed brace")
	}
}

func TestValidate_TodoMarkers(t *testing.T) {
	gv := NewGenValidator()

	code := `package main

func process() {
	// TODO: implement this
}
`
	result := gv.Validate(code, "go")

	foundCompleteness := false
	for _, issue := range result.Issues {
		if issue.Check == "completeness" && strings.Contains(issue.Message, "TODO") {
			foundCompleteness = true
			break
		}
	}
	if !foundCompleteness {
		t.Error("expected completeness issue for TODO marker")
	}
}

func TestValidate_DifferentLanguage(t *testing.T) {
	gv := NewGenValidator()

	code := `def hello():
    print("hello")
`
	result := gv.Validate(code, "python")

	// Go-specific checks should not run
	for _, issue := range result.Issues {
		if issue.Check == "imports" || issue.Check == "naming" || issue.Check == "compilation" || issue.Check == "types" {
			t.Errorf("Go-specific check %q ran on Python code", issue.Check)
		}
	}
}

func TestCheckBalancedDelimiters_Balanced(t *testing.T) {
	code := `func foo() {
	x := []int{1, 2, 3}
	y := bar(x[0])
}`
	issues := CheckBalancedDelimiters(code)
	if len(issues) > 0 {
		for _, issue := range issues {
			t.Errorf("unexpected issue: %s (line %d)", issue.Message, issue.Line)
		}
	}
}

func TestCheckBalancedDelimiters_ExtraClosing(t *testing.T) {
	code := `func foo() {
	x := 1
}}
`
	issues := CheckBalancedDelimiters(code)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "unexpected closing brace") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected issue for extra closing brace")
	}
}

func TestCheckBalancedDelimiters_InStrings(t *testing.T) {
	code := `func foo() {
	x := "this has { unmatched braces"
	y := 1
}`
	issues := CheckBalancedDelimiters(code)
	if len(issues) > 0 {
		t.Errorf("braces in strings should be ignored, got %d issues", len(issues))
	}
}

func TestCheckBalancedDelimiters_InComments(t *testing.T) {
	code := `func foo() {
	// this has { unmatched braces
	/* and { this too */
	x := 1
}`
	issues := CheckBalancedDelimiters(code)
	if len(issues) > 0 {
		t.Errorf("braces in comments should be ignored, got %d issues", len(issues))
	}
}

func TestCheckCompleteness(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantMsg string
	}{
		{
			name:    "TODO marker",
			code:    "// TODO: implement\nfunc foo() {}",
			wantMsg: "TODO",
		},
		{
			name:    "FIXME marker",
			code:    "// FIXME: broken\nfunc foo() {}",
			wantMsg: "FIXME",
		},
		{
			name:    "NotImplementedError",
			code:    "raise NotImplementedError",
			wantMsg: "NotImplementedError",
		},
		{
			name:    "ellipsis placeholder",
			code:    "def foo():\n    ...",
			wantMsg: "ellipsis",
		},
		{
			name:    "pass placeholder",
			code:    "def foo():\n    pass",
			wantMsg: "pass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := CheckCompleteness(tt.code)
			found := false
			for _, issue := range issues {
				if strings.Contains(issue.Message, tt.wantMsg) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected issue containing %q, got %v", tt.wantMsg, issues)
			}
		})
	}
}

func TestCheckCompleteness_CleanCode(t *testing.T) {
	code := `package main

func main() {
	println("hello world")
}
`
	issues := CheckCompleteness(code)
	if len(issues) > 0 {
		t.Errorf("clean code should have no completeness issues, got %d", len(issues))
	}
}

func TestValidateGo_ValidCode(t *testing.T) {
	code := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	issues := ValidateGo(code)
	// Filter syntax-only issues (parser-based)
	var syntaxIssues []GenIssue
	for _, issue := range issues {
		if issue.Check == "go-syntax" {
			syntaxIssues = append(syntaxIssues, issue)
		}
	}
	if len(syntaxIssues) > 0 {
		t.Errorf("valid Go code should parse without syntax errors, got: %v", syntaxIssues)
	}
}

func TestValidateGo_SyntaxError(t *testing.T) {
	code := `package main

func main() {
	x :=
}
`
	issues := ValidateGo(code)
	found := false
	for _, issue := range issues {
		if issue.Check == "go-syntax" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected go-syntax issue for invalid code")
	}
}

func TestValidatePython_MixedIndent(t *testing.T) {
	code := "def foo():\n    x = 1\n\ty = 2\n"
	issues := ValidatePython(code)
	found := false
	for _, issue := range issues {
		if issue.Check == "python-indent" && strings.Contains(issue.Message, "mixed") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected indent issue for mixed tabs/spaces")
	}
}

func TestValidatePython_UnclosedString(t *testing.T) {
	code := "x = \"hello\ny = 1\n"
	issues := ValidatePython(code)
	found := false
	for _, issue := range issues {
		if issue.Check == "python-string" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected string issue for unclosed quote")
	}
}

func TestValidatePython_ConsistentIndent(t *testing.T) {
	code := "def foo():\n    x = 1\n    y = 2\n    z = 3\n"
	issues := ValidatePython(code)
	for _, issue := range issues {
		if issue.Check == "python-indent" {
			t.Errorf("consistent indentation should not produce issues: %s", issue.Message)
		}
	}
}

func TestValidateTypeScript_DuplicateImport(t *testing.T) {
	code := `import { foo } from './utils';
import { bar } from './utils';

export function baz() {
	return foo() + bar();
}
`
	issues := ValidateTypeScript(code)
	found := false
	for _, issue := range issues {
		if issue.Check == "ts-import" && strings.Contains(issue.Message, "duplicate") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected duplicate import issue")
	}
}

func TestValidateTypeScript_DoubleExport(t *testing.T) {
	code := `export default export function foo() {
	return 1;
}
`
	issues := ValidateTypeScript(code)
	found := false
	for _, issue := range issues {
		if issue.Check == "ts-export" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected export issue for double export")
	}
}

func TestValidateTypeScript_Clean(t *testing.T) {
	code := `import { foo } from './utils';

export function bar(): number {
	return foo() + 1;
}
`
	issues := ValidateTypeScript(code)
	if len(issues) > 0 {
		for _, issue := range issues {
			t.Errorf("unexpected issue: %s — %s", issue.Check, issue.Message)
		}
	}
}

func TestFormatValidation_NoIssues(t *testing.T) {
	v := &GenValidation{
		Valid:    true,
		Issues:   nil,
		Language: "go",
		Score:    1.0,
	}
	output := FormatValidation(v)
	if !strings.Contains(output, "no issues") {
		t.Errorf("expected 'no issues' in output, got: %s", output)
	}
	if !strings.Contains(output, "1.00") {
		t.Errorf("expected score 1.00 in output, got: %s", output)
	}
}

func TestFormatValidation_WithIssues(t *testing.T) {
	v := &GenValidation{
		Valid: false,
		Issues: []GenIssue{
			{Check: "syntax", Message: "unclosed brace", Line: 15, Severity: "error", AutoFixable: true},
			{Check: "completeness", Message: "TODO marker left in generated code", Line: 28, Severity: "warning"},
		},
		Language: "go",
		Score:    0.85,
	}
	output := FormatValidation(v)

	if !strings.Contains(output, "2 issues") {
		t.Errorf("expected '2 issues' in output, got: %s", output)
	}
	if !strings.Contains(output, "L15") {
		t.Errorf("expected 'L15' in output, got: %s", output)
	}
	if !strings.Contains(output, "auto-fixable") {
		t.Errorf("expected 'auto-fixable' in output, got: %s", output)
	}
	if !strings.Contains(output, "0.85") {
		t.Errorf("expected score 0.85 in output, got: %s", output)
	}
	if !strings.Contains(output, "proceed with caution") {
		t.Errorf("expected 'proceed with caution' in output, got: %s", output)
	}
}

func TestFormatValidation_LowScore(t *testing.T) {
	v := &GenValidation{
		Valid: false,
		Issues: []GenIssue{
			{Check: "syntax", Message: "unclosed brace", Line: 1, Severity: "error"},
			{Check: "syntax", Message: "unclosed brace", Line: 2, Severity: "error"},
			{Check: "syntax", Message: "unclosed brace", Line: 3, Severity: "error"},
			{Check: "syntax", Message: "unclosed brace", Line: 4, Severity: "error"},
		},
		Language: "go",
		Score:    0.4,
	}
	output := FormatValidation(v)
	if !strings.Contains(output, "significant issues found") {
		t.Errorf("expected 'significant issues found' for low score, got: %s", output)
	}
}

func TestAutoFix_UnclosedBraces(t *testing.T) {
	code := `func main() {
	if true {
		println("hello")
`
	issues := []GenIssue{
		{Check: "syntax", Message: "unclosed brace", Line: 1, Severity: "error", AutoFixable: true, Fix: "add closing brace"},
		{Check: "syntax", Message: "unclosed brace", Line: 2, Severity: "error", AutoFixable: true, Fix: "add closing brace"},
	}

	fixed := AutoFix(code, issues)
	// Should have added two closing braces
	if strings.Count(fixed, "}") < 2 {
		t.Errorf("expected at least 2 closing braces in fixed code, got: %s", fixed)
	}
}

func TestAutoFix_ExtraClosingBrace(t *testing.T) {
	code := "func main() {\n\tx := 1\n}}\n"
	issues := []GenIssue{
		{Check: "syntax", Message: "unexpected closing brace '}'", Line: 3, Severity: "error", AutoFixable: true, Fix: "remove extra closing brace"},
	}

	fixed := AutoFix(code, issues)
	// The extra brace on line 3 should be removed
	lines := strings.Split(fixed, "\n")
	braceCount := 0
	for _, line := range lines {
		braceCount += strings.Count(line, "}")
	}
	if braceCount != 1 {
		t.Errorf("expected 1 closing brace after fix, got %d in: %q", braceCount, fixed)
	}
}

func TestAutoFix_MixedTabsSpaces(t *testing.T) {
	code := "def foo():\n\tx = 1\n\ty = 2\n"
	issues := []GenIssue{
		{Check: "python-indent", Message: "mixed tabs and spaces", Line: 2, Severity: "error", AutoFixable: true, Fix: "replace tabs with spaces"},
	}

	fixed := AutoFix(code, issues)
	if strings.Contains(fixed, "\t") {
		t.Error("expected tabs to be replaced with spaces")
	}
}

func TestAutoFix_NonFixableIssues(t *testing.T) {
	code := "// TODO: implement\nfunc foo() {}\n"
	issues := []GenIssue{
		{Check: "completeness", Message: "TODO marker", Line: 1, Severity: "warning", AutoFixable: false},
	}

	fixed := AutoFix(code, issues)
	if fixed != code {
		t.Errorf("non-fixable issues should not modify code, got: %q", fixed)
	}
}

func TestValidateScore(t *testing.T) {
	gv := NewGenValidator()

	// Code with known issues — only use non-compilation checks
	code := `{
	// TODO: fix this
}`
	result := gv.Validate(code, "javascript")

	if result.Score >= 1.0 {
		t.Errorf("code with issues should have score < 1.0, got: %f", result.Score)
	}
}

func TestValidate_ConcurrentAccess(t *testing.T) {
	gv := NewGenValidator()
	code := `package main

func main() {
	println("hello")
}
`
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			result := gv.Validate(code, "go")
			if result == nil {
				t.Error("nil result from concurrent Validate")
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestCheckDuplicateImports(t *testing.T) {
	code := `package main

import (
	"fmt"
	"os"
	"fmt"
)

func main() {
	fmt.Println(os.Args)
}
`
	issues := checkDuplicateImports(code)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "duplicate import") && strings.Contains(issue.Message, "fmt") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected duplicate import issue for 'fmt'")
	}
}

func TestCheckTypeConsistency_NilReturn(t *testing.T) {
	code := `package main

func foo() int {
	return nil
}
`
	issues := checkTypeConsistency(code)
	found := false
	for _, issue := range issues {
		if issue.Check == "types" && strings.Contains(issue.Message, "nil") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected type issue for returning nil from int function")
	}
}

func TestCheckTypeConsistency_ValidNilReturn(t *testing.T) {
	code := `package main

func foo() error {
	return nil
}
`
	issues := checkTypeConsistency(code)
	for _, issue := range issues {
		if issue.Check == "types" {
			t.Errorf("returning nil from error function should be valid, got: %s", issue.Message)
		}
	}
}
