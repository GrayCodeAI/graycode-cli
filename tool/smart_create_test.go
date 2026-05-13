package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateBoilerplate_GoFile(t *testing.T) {
	sc := NewSmartCreator("/tmp/testproject")

	got := sc.GenerateBoilerplate("/tmp/testproject/internal/auth/handler.go")
	if !strings.Contains(got, "package auth") {
		t.Errorf("Go boilerplate should contain correct package, got:\n%s", got)
	}
}

func TestGenerateBoilerplate_GoTestFile(t *testing.T) {
	sc := NewSmartCreator("/tmp/testproject")

	got := sc.GenerateBoilerplate("/tmp/testproject/pkg/auth/auth_test.go")
	if !strings.Contains(got, "package auth") {
		t.Errorf("Go test boilerplate should contain package, got:\n%s", got)
	}
	if !strings.Contains(got, "\"testing\"") {
		t.Errorf("Go test boilerplate should import testing, got:\n%s", got)
	}
	if !strings.Contains(got, "func Test") {
		t.Errorf("Go test boilerplate should have test function, got:\n%s", got)
	}
}

func TestGenerateBoilerplate_Python(t *testing.T) {
	sc := NewSmartCreator("/tmp/testproject")

	got := sc.GenerateBoilerplate("/tmp/testproject/src/utils.py")
	if !strings.Contains(got, "\"\"\"utils module.\"\"\"") {
		t.Errorf("Python boilerplate should have docstring, got:\n%s", got)
	}
	if !strings.Contains(got, "if __name__ == \"__main__\":") {
		t.Errorf("Python boilerplate should have main guard, got:\n%s", got)
	}
}

func TestGenerateBoilerplate_TypeScript(t *testing.T) {
	sc := NewSmartCreator("/tmp/testproject")

	got := sc.GenerateBoilerplate("/tmp/testproject/src/helper.ts")
	if !strings.Contains(got, "export default function") {
		t.Errorf("TS boilerplate should have export default, got:\n%s", got)
	}
	if !strings.Contains(got, "helper") {
		t.Errorf("TS boilerplate should reference file name, got:\n%s", got)
	}
}

func TestGenerateBoilerplate_TSX(t *testing.T) {
	sc := NewSmartCreator("/tmp/testproject")

	got := sc.GenerateBoilerplate("/tmp/testproject/src/Button.tsx")
	if !strings.Contains(got, "import React") {
		t.Errorf("TSX boilerplate should import React, got:\n%s", got)
	}
	if !strings.Contains(got, "export default function") {
		t.Errorf("TSX boilerplate should export default component, got:\n%s", got)
	}
	if !strings.Contains(got, "ButtonProps") {
		t.Errorf("TSX boilerplate should define Props interface, got:\n%s", got)
	}
}

func TestGenerateBoilerplate_Dockerfile(t *testing.T) {
	sc := NewSmartCreator("/tmp/testproject")

	got := sc.GenerateBoilerplate("/tmp/testproject/Dockerfile")
	if !strings.Contains(got, "FROM") {
		t.Errorf("Dockerfile boilerplate should have FROM, got:\n%s", got)
	}
	if !strings.Contains(got, "AS builder") {
		t.Errorf("Dockerfile boilerplate should be multi-stage, got:\n%s", got)
	}
	if !strings.Contains(got, "COPY --from=builder") {
		t.Errorf("Dockerfile boilerplate should copy from builder stage, got:\n%s", got)
	}
}

func TestGenerateBoilerplate_Makefile(t *testing.T) {
	sc := NewSmartCreator("/tmp/testproject")

	got := sc.GenerateBoilerplate("/tmp/testproject/Makefile")
	if !strings.Contains(got, "build:") {
		t.Errorf("Makefile boilerplate should have build target, got:\n%s", got)
	}
	if !strings.Contains(got, "test:") {
		t.Errorf("Makefile boilerplate should have test target, got:\n%s", got)
	}
	if !strings.Contains(got, "lint:") {
		t.Errorf("Makefile boilerplate should have lint target, got:\n%s", got)
	}
	if !strings.Contains(got, "clean:") {
		t.Errorf("Makefile boilerplate should have clean target, got:\n%s", got)
	}
}

func TestGenerateBoilerplate_YAML(t *testing.T) {
	sc := NewSmartCreator("/tmp/testproject")

	got := sc.GenerateBoilerplate("/tmp/testproject/config.yaml")
	if !strings.Contains(got, "# config") {
		t.Errorf("YAML boilerplate should have header comment, got:\n%s", got)
	}
	if !strings.Contains(got, "---") {
		t.Errorf("YAML boilerplate should have document start marker, got:\n%s", got)
	}
}

func TestGenerateBoilerplate_Rust(t *testing.T) {
	sc := NewSmartCreator("/tmp/testproject")

	got := sc.GenerateBoilerplate("/tmp/testproject/src/parser.rs")
	if !strings.Contains(got, "pub mod parser") {
		t.Errorf("Rust boilerplate should have mod declaration, got:\n%s", got)
	}
}

func TestInferPackageName_StandardDir(t *testing.T) {
	sc := NewSmartCreator("/tmp/project")

	tests := []struct {
		path string
		want string
	}{
		{"/tmp/project/internal/auth/handler.go", "auth"},
		{"/tmp/project/pkg/server/server.go", "server"},
		{"/tmp/project/tool/smart_create.go", "tool"},
	}

	for _, tt := range tests {
		got := sc.InferPackageName(tt.path)
		if got != tt.want {
			t.Errorf("InferPackageName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestInferPackageName_CmdDirectory(t *testing.T) {
	sc := NewSmartCreator("/tmp/project")

	tests := []struct {
		path string
		want string
	}{
		{"/tmp/project/cmd/server/main.go", "main"},
		{"/tmp/project/cmd/cli/root.go", "main"},
	}

	for _, tt := range tests {
		got := sc.InferPackageName(tt.path)
		if got != tt.want {
			t.Errorf("InferPackageName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestInferPackageName_EmptyPath(t *testing.T) {
	sc := NewSmartCreator("/tmp/project")

	got := sc.InferPackageName("")
	if got != "main" {
		t.Errorf("InferPackageName(\"\") = %q, want \"main\"", got)
	}
}

func TestDetectCopyright_FindsExistingHeader(t *testing.T) {
	// Create a temporary directory with a file containing a copyright header.
	dir := t.TempDir()
	content := "// Copyright 2024 Example Corp. All rights reserved.\n// Use of this source code is governed by a BSD-style license.\n\npackage example\n"
	err := os.WriteFile(filepath.Join(dir, "example.go"), []byte(content), 0o644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	sc := NewSmartCreator(dir)
	got := sc.DetectCopyright(dir)

	if !strings.Contains(got, "Copyright 2024 Example Corp") {
		t.Errorf("DetectCopyright should find copyright header, got:\n%s", got)
	}
}

func TestDetectCopyright_NoHeader(t *testing.T) {
	dir := t.TempDir()
	content := "package example\n\nfunc Hello() string { return \"hello\" }\n"
	err := os.WriteFile(filepath.Join(dir, "example.go"), []byte(content), 0o644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	sc := NewSmartCreator(dir)
	got := sc.DetectCopyright(dir)

	if got != "" {
		t.Errorf("DetectCopyright should return empty for files without header, got:\n%s", got)
	}
}

func TestGenerateTestFile_GoSource(t *testing.T) {
	// Create a temporary Go source file.
	dir := t.TempDir()
	content := `package auth

func NewAuthenticator() *Authenticator {
	return &Authenticator{}
}

func ValidateToken(token string) error {
	return nil
}

func privateHelper() {}
`
	srcPath := filepath.Join(dir, "auth.go")
	err := os.WriteFile(srcPath, []byte(content), 0o644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	sc := NewSmartCreator(dir)
	got := sc.GenerateTestFile(srcPath)

	if !strings.Contains(got, "package") {
		t.Errorf("generated test should have package declaration, got:\n%s", got)
	}
	if !strings.Contains(got, "\"testing\"") {
		t.Errorf("generated test should import testing, got:\n%s", got)
	}
	if !strings.Contains(got, "TestNewAuthenticator") {
		t.Errorf("generated test should have TestNewAuthenticator, got:\n%s", got)
	}
	if !strings.Contains(got, "TestValidateToken") {
		t.Errorf("generated test should have TestValidateToken, got:\n%s", got)
	}
	if strings.Contains(got, "TestprivateHelper") {
		t.Errorf("generated test should NOT include private functions, got:\n%s", got)
	}
}

func TestGenerateTestFile_EmptyPath(t *testing.T) {
	sc := NewSmartCreator("/tmp/project")

	got := sc.GenerateTestFile("")
	if got != "" {
		t.Errorf("GenerateTestFile(\"\") should return empty, got:\n%s", got)
	}
}

func TestGenerateBoilerplate_EmptyPath(t *testing.T) {
	sc := NewSmartCreator("/tmp/project")

	got := sc.GenerateBoilerplate("")
	if got != "" {
		t.Errorf("GenerateBoilerplate(\"\") should return empty, got:\n%s", got)
	}
}

func TestGenerateInterface(t *testing.T) {
	sc := NewSmartCreator("/tmp/project")

	functions := []string{
		"Get(id string) (*Item, error)",
		"List() ([]*Item, error)",
		"Delete(id string) error",
	}

	got := sc.GenerateInterface(functions)
	if !strings.Contains(got, "type Interface interface {") {
		t.Errorf("GenerateInterface should produce interface block, got:\n%s", got)
	}
	if !strings.Contains(got, "Get(id string) (*Item, error)") {
		t.Errorf("GenerateInterface should include function signatures, got:\n%s", got)
	}
}

func TestGenerateInterface_Empty(t *testing.T) {
	sc := NewSmartCreator("/tmp/project")

	got := sc.GenerateInterface(nil)
	if got != "" {
		t.Errorf("GenerateInterface(nil) should return empty, got:\n%s", got)
	}
}

func TestSmartCreateTool_Name(t *testing.T) {
	sc := NewSmartCreator("/tmp/project")
	tool := &SmartCreateTool{Creator: sc}

	if tool.Name() != "SmartCreate" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "SmartCreate")
	}
}

func TestSmartCreateTool_Parameters(t *testing.T) {
	sc := NewSmartCreator("/tmp/project")
	tool := &SmartCreateTool{Creator: sc}

	params := tool.Parameters()
	if params == nil {
		t.Fatal("Parameters() returned nil")
	}
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Parameters() should have properties")
	}
	if _, ok := props["path"]; !ok {
		t.Error("Parameters() should have path property")
	}
}

func TestDetectImportStyle_Go(t *testing.T) {
	dir := t.TempDir()
	content := `package example

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

func Hello() { fmt.Println(strings.Join(nil, "")) }
`
	err := os.WriteFile(filepath.Join(dir, "example.go"), []byte(content), 0o644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	sc := NewSmartCreator(dir)
	got := sc.DetectImportStyle(dir, "go")

	if !strings.Contains(got, "import") {
		t.Errorf("DetectImportStyle should find import block, got:\n%s", got)
	}
	if !strings.Contains(got, "fmt") {
		t.Errorf("DetectImportStyle should include imports, got:\n%s", got)
	}
}

func TestNewSmartCreator(t *testing.T) {
	sc := NewSmartCreator("/tmp/myproject")
	if sc.ProjectDir != "/tmp/myproject" {
		t.Errorf("ProjectDir = %q, want %q", sc.ProjectDir, "/tmp/myproject")
	}
	if sc.Conventions == nil {
		t.Error("Conventions should be initialized")
	}
}

func TestGenerateBoilerplate_GoWithCopyright(t *testing.T) {
	dir := t.TempDir()
	content := "// Copyright 2024 TestCorp. All rights reserved.\n\npackage existing\n"
	err := os.WriteFile(filepath.Join(dir, "existing.go"), []byte(content), 0o644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	sc := NewSmartCreator(dir)
	got := sc.GenerateBoilerplate(filepath.Join(dir, "pkg", "newfile.go"))

	if !strings.Contains(got, "Copyright 2024 TestCorp") {
		t.Errorf("Go boilerplate should include detected copyright, got:\n%s", got)
	}
	if !strings.Contains(got, "package pkg") {
		t.Errorf("Go boilerplate should have correct package, got:\n%s", got)
	}
}
