package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateFile_UnknownExtension(t *testing.T) {
	result := ValidateFile("/tmp/file.xyz")
	if !result.Valid {
		t.Error("unknown extension should return valid")
	}
}

func TestValidateFile_ValidGo(t *testing.T) {
	dir := t.TempDir()

	// Create a valid Go file with go.mod
	modPath := filepath.Join(dir, "go.mod")
	os.WriteFile(modPath, []byte("module test\n\ngo 1.21\n"), 0o644)

	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0o644)

	result := ValidateFile(path)
	if !result.Valid {
		t.Errorf("valid Go file should pass validation, errors: %v", result.Errors)
	}
}

func TestValidateFile_InvalidGo(t *testing.T) {
	dir := t.TempDir()

	// Create go.mod
	modPath := filepath.Join(dir, "go.mod")
	os.WriteFile(modPath, []byte("module test\n\ngo 1.21\n"), 0o644)

	path := filepath.Join(dir, "bad.go")
	os.WriteFile(path, []byte("package main\n\nfunc main( {\n}\n"), 0o644)

	result := ValidateFile(path)
	if result.Valid {
		t.Error("invalid Go file should fail validation")
	}
	if len(result.Errors) == 0 {
		t.Error("should have at least one error")
	}
}

func TestValidateFile_ValidPython(t *testing.T) {
	// Check if python3 is available
	if _, err := os.Stat("/usr/bin/python3"); os.IsNotExist(err) {
		t.Skip("python3 not available")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.py")
	os.WriteFile(path, []byte("def hello():\n    print('hello')\n"), 0o644)

	result := ValidateFile(path)
	if !result.Valid {
		t.Errorf("valid Python file should pass, errors: %v", result.Errors)
	}
}

func TestValidateFile_InvalidPython(t *testing.T) {
	// Check if python3 is available
	if _, err := os.Stat("/usr/bin/python3"); os.IsNotExist(err) {
		t.Skip("python3 not available")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.py")
	os.WriteFile(path, []byte("def hello(\n    print('hello')\n"), 0o644)

	result := ValidateFile(path)
	if result.Valid {
		t.Error("invalid Python file should fail")
	}
}

func TestValidateFile_ValidJS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.js")
	os.WriteFile(path, []byte("function hello() { console.log('hello'); }\n"), 0o644)

	result := ValidateFile(path)
	if !result.Valid {
		t.Errorf("valid JS file should pass, errors: %v", result.Errors)
	}
}

func TestLanguageValidator(t *testing.T) {
	tests := []struct {
		ext      string
		hasValid bool
	}{
		{".go", true},
		{".py", true},
		{".js", true},
		{".ts", true},
		{".txt", false},
		{".md", false},
		{".rs", false},
	}

	for _, tt := range tests {
		v := languageValidator(tt.ext)
		if tt.hasValid && v == nil {
			t.Errorf("expected validator for %s", tt.ext)
		}
		if !tt.hasValid && v != nil {
			t.Errorf("expected no validator for %s", tt.ext)
		}
	}
}

func TestAutoFixPrompt(t *testing.T) {
	errors := []ValidationError{
		{File: "test.go", Line: 10, Column: 5, Message: "syntax error"},
		{File: "test.go", Line: 20, Message: "undefined variable"},
	}

	prompt := AutoFixPrompt("test.go", "package main\n", errors)

	if !strings.Contains(prompt, "test.go") {
		t.Error("prompt should contain file path")
	}
	if !strings.Contains(prompt, "syntax error") {
		t.Error("prompt should contain error messages")
	}
	if !strings.Contains(prompt, "Line 10") {
		t.Error("prompt should contain line numbers")
	}
	if !strings.Contains(prompt, "Col 5") {
		t.Error("prompt should contain column for first error")
	}
	if !strings.Contains(prompt, "package main") {
		t.Error("prompt should contain file content")
	}
	if !strings.Contains(prompt, "Please fix the errors") {
		t.Error("prompt should contain fix instruction")
	}
}

func TestAutoFixPrompt_NoLineNumber(t *testing.T) {
	errors := []ValidationError{
		{File: "test.go", Message: "general error"},
	}
	prompt := AutoFixPrompt("test.go", "content\n", errors)
	if !strings.Contains(prompt, "general error") {
		t.Error("prompt should contain error without line number")
	}
}

func TestParseGoErrors(t *testing.T) {
	output := `./main.go:10:5: syntax error: unexpected {
./main.go:15:1: missing return`

	errors := parseGoErrors(output)
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errors))
	}
	if errors[0].Line != 10 || errors[0].Column != 5 {
		t.Errorf("first error: line=%d col=%d", errors[0].Line, errors[0].Column)
	}
	if errors[1].Line != 15 {
		t.Errorf("second error: line=%d", errors[1].Line)
	}
}

func TestParsePythonErrors(t *testing.T) {
	output := `  File "test.py", line 5
    def hello(
              ^
SyntaxError: unexpected EOF while parsing`

	errors := parsePythonErrors(output, "test.py")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0].Line != 5 {
		t.Errorf("expected line 5, got %d", errors[0].Line)
	}
	if !strings.Contains(errors[0].Message, "SyntaxError") {
		t.Errorf("expected SyntaxError, got %q", errors[0].Message)
	}
}

func TestParseNodeErrors(t *testing.T) {
	output := `/tmp/test.js:3
function hello( {
                ^
SyntaxError: Unexpected token '{'`

	errors := parseNodeErrors(output, "/tmp/test.js")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0].Line != 3 {
		t.Errorf("expected line 3, got %d", errors[0].Line)
	}
}

func TestParseTSErrors(t *testing.T) {
	output := `test.ts(5,10): error TS1005: ',' expected.
test.ts(10,1): error TS2304: Cannot find name 'foo'.`

	errors := parseTSErrors(output, "test.ts")
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errors))
	}
	if errors[0].Line != 5 || errors[0].Column != 10 {
		t.Errorf("first error: line=%d col=%d", errors[0].Line, errors[0].Column)
	}
	if !strings.Contains(errors[0].Message, "',' expected") {
		t.Errorf("unexpected message: %q", errors[0].Message)
	}
}

func TestMaxAutoFixRetries(t *testing.T) {
	if MaxAutoFixRetries != 3 {
		t.Errorf("expected MaxAutoFixRetries=3, got %d", MaxAutoFixRetries)
	}
}
