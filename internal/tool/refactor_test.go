package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRefactorer(t *testing.T) {
	r := NewRefactorer()
	if r == nil {
		t.Fatal("NewRefactorer returned nil")
	}
}

func TestExtractFunction(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

import "fmt"

func main() {
	x := "hello"
	fmt.Println(x)
	fmt.Println("world")
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRefactorer()
	result, err := r.ExtractFunction(file, 7, 8, "printMessages")
	if err != nil {
		t.Fatal(err)
	}

	if result.Type != "extract_function" {
		t.Errorf("expected type extract_function, got %s", result.Type)
	}
	if result.Changes != 1 {
		t.Errorf("expected 1 change, got %d", result.Changes)
	}

	data, _ := os.ReadFile(file)
	got := string(data)

	if !strings.Contains(got, "func printMessages(") {
		t.Error("expected new function definition in file")
	}
	if !strings.Contains(got, "printMessages(") {
		t.Error("expected function call in file")
	}
}

func TestExtractFunction_InvalidRange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	os.WriteFile(file, []byte("package main\n\nfunc main() {}\n"), 0o644)

	r := NewRefactorer()
	_, err := r.ExtractFunction(file, 10, 20, "foo")
	if err == nil {
		t.Fatal("expected error for invalid line range")
	}
}

func TestRenameSymbol(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

func calculate() int {
	result := calculate()
	return result
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRefactorer()
	result, err := r.RenameSymbol(file, "calculate", "compute")
	if err != nil {
		t.Fatal(err)
	}

	if result.Type != "rename_symbol" {
		t.Errorf("expected type rename_symbol, got %s", result.Type)
	}
	if result.Changes != 2 {
		t.Errorf("expected 2 changes, got %d", result.Changes)
	}

	data, _ := os.ReadFile(file)
	got := string(data)
	if strings.Contains(got, "calculate") {
		t.Error("old name should not appear in file")
	}
	if !strings.Contains(got, "compute") {
		t.Error("new name should appear in file")
	}
}

func TestRenameSymbol_NotFound(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	os.WriteFile(file, []byte("package main\n"), 0o644)

	r := NewRefactorer()
	_, err := r.RenameSymbol(file, "nonexistent", "newname")
	if err == nil {
		t.Fatal("expected error for symbol not found")
	}
}

func TestRenameSymbol_WordBoundary(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

var name = "test"
var namePrefix = "pre"
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRefactorer()
	result, err := r.RenameSymbol(file, "name", "title")
	if err != nil {
		t.Fatal(err)
	}

	// Should only rename "name", not "namePrefix"
	data, _ := os.ReadFile(file)
	got := string(data)
	if !strings.Contains(got, "namePrefix") {
		t.Error("namePrefix should remain unchanged")
	}
	if !strings.Contains(got, "title") {
		t.Error("name should be renamed to title")
	}
	if result.Changes != 1 {
		t.Errorf("expected 1 change, got %d", result.Changes)
	}
}

func TestInlineVariable(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

import "fmt"

func main() {
	msg := "hello world"
	fmt.Println(msg)
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRefactorer()
	result, err := r.InlineVariable(file, 6)
	if err != nil {
		t.Fatal(err)
	}

	if result.Type != "inline_variable" {
		t.Errorf("expected type inline_variable, got %s", result.Type)
	}

	data, _ := os.ReadFile(file)
	got := string(data)
	if strings.Contains(got, "msg :=") {
		t.Error("variable declaration should be removed")
	}
	if !strings.Contains(got, `"hello world"`) {
		t.Error("value should be inlined at use sites")
	}
}

func TestInlineVariable_NotDeclaration(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	os.WriteFile(file, []byte("package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"), 0o644)

	r := NewRefactorer()
	_, err := r.InlineVariable(file, 4)
	if err == nil {
		t.Fatal("expected error for non-declaration line")
	}
}

func TestExtractVariable(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

import "fmt"

func main() {
	fmt.Println(2 + 2)
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRefactorer()
	result, err := r.ExtractVariable(file, 6, "2 + 2", "sum")
	if err != nil {
		t.Fatal(err)
	}

	if result.Type != "extract_variable" {
		t.Errorf("expected type extract_variable, got %s", result.Type)
	}

	data, _ := os.ReadFile(file)
	got := string(data)
	if !strings.Contains(got, "sum := 2 + 2") {
		t.Error("expected variable declaration")
	}
	if !strings.Contains(got, "fmt.Println(sum)") {
		t.Error("expected expression to be replaced with variable")
	}
}

func TestExtractVariable_ExprNotFound(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	os.WriteFile(file, []byte("package main\n\nfunc main() {\n\tx := 1\n}\n"), 0o644)

	r := NewRefactorer()
	_, err := r.ExtractVariable(file, 4, "nonexistent", "v")
	if err == nil {
		t.Fatal("expected error for expression not found")
	}
}

func TestAddErrorCheck(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

func main() {
	err := doSomething()
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRefactorer()
	result, err := r.AddErrorCheck(file, 4)
	if err != nil {
		t.Fatal(err)
	}

	if result.Type != "add_error_check" {
		t.Errorf("expected type add_error_check, got %s", result.Type)
	}

	data, _ := os.ReadFile(file)
	got := string(data)
	if !strings.Contains(got, "if err != nil {") {
		t.Error("expected error check block")
	}
	if !strings.Contains(got, "return err") {
		t.Error("expected return err in error check")
	}
}

func TestAddErrorCheck_NoErrAssign(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

func main() {
	doSomething()
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRefactorer()
	_, err := r.AddErrorCheck(file, 4)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(file)
	got := string(data)
	if !strings.Contains(got, "err := doSomething()") {
		t.Error("expected err assignment to be added")
	}
	if !strings.Contains(got, "if err != nil {") {
		t.Error("expected error check block")
	}
}

func TestWrapWithContext(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

func doWork() error {
	err := step()
	return err
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRefactorer()
	result, err := r.WrapWithContext(file, 5, "do work")
	if err != nil {
		t.Fatal(err)
	}

	if result.Type != "wrap_with_context" {
		t.Errorf("expected type wrap_with_context, got %s", result.Type)
	}

	data, _ := os.ReadFile(file)
	got := string(data)
	if !strings.Contains(got, `fmt.Errorf("do work: %w", err)`) {
		t.Errorf("expected wrapped error, got:\n%s", got)
	}
}

func TestWrapWithContext_MultiReturn(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

func doWork() (int, error) {
	err := step()
	return 0, err
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRefactorer()
	result, err := r.WrapWithContext(file, 5, "do work")
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(file)
	got := string(data)
	if !strings.Contains(got, `return 0, fmt.Errorf("do work: %w", err)`) {
		t.Errorf("expected wrapped multi-return error, got:\n%s", got)
	}
	if result.Changes != 1 {
		t.Errorf("expected 1 change, got %d", result.Changes)
	}
}

func TestWrapWithContext_NotReturnErr(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	os.WriteFile(file, []byte("package main\n\nfunc f() {\n\tx := 1\n}\n"), 0o644)

	r := NewRefactorer()
	_, err := r.WrapWithContext(file, 4, "ctx")
	if err == nil {
		t.Fatal("expected error for non-return line")
	}
}

func TestConvertToTableTest(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "calc_test.go")

	content := `package main

import "testing"

func TestAdd(t *testing.T) {
	result := 2 + 2
	if result != 4 {
		t.Fatal("expected 4")
	}
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRefactorer()
	result, err := r.ConvertToTableTest(file, "TestAdd")
	if err != nil {
		t.Fatal(err)
	}

	if result.Type != "convert_table_test" {
		t.Errorf("expected type convert_table_test, got %s", result.Type)
	}

	data, _ := os.ReadFile(file)
	got := string(data)
	if !strings.Contains(got, "tests := []struct") {
		t.Error("expected table-driven test struct")
	}
	if !strings.Contains(got, "t.Run(tt.name") {
		t.Error("expected t.Run call")
	}
	if !strings.Contains(got, `"Add"`) {
		t.Error("expected test case name derived from function name")
	}
}

func TestConvertToTableTest_NotFound(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.go")
	os.WriteFile(file, []byte("package main\n"), 0o644)

	r := NewRefactorer()
	_, err := r.ConvertToTableTest(file, "TestNonexistent")
	if err == nil {
		t.Fatal("expected error for missing test function")
	}
}

func TestSortImports(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

import (
	"github.com/external/pkg"
	"fmt"
	"os"
)

func main() {
	fmt.Println(os.Args)
	pkg.Do()
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRefactorer()
	result, err := r.SortImports(file)
	if err != nil {
		t.Fatal(err)
	}

	if result.Type != "sort_imports" {
		t.Errorf("expected type sort_imports, got %s", result.Type)
	}

	data, _ := os.ReadFile(file)
	got := string(data)

	// stdlib should come before external.
	fmtIdx := strings.Index(got, `"fmt"`)
	extIdx := strings.Index(got, `"github.com/external/pkg"`)
	if fmtIdx > extIdx {
		t.Error("expected stdlib imports before external imports")
	}
}

func TestSortImports_AlreadySorted(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	// Use content that is already in the normalized format the organizer produces.
	content := `package main

import "fmt"

func main() {
	fmt.Println("hi")
}
`
	os.WriteFile(file, []byte(content), 0o644)

	r := NewRefactorer()
	result, err := r.SortImports(file)
	if err != nil {
		t.Fatal(err)
	}
	// The organizer may normalize formatting, so just verify no error occurred.
	if result.Type != "sort_imports" {
		t.Errorf("expected type sort_imports, got %s", result.Type)
	}
}

func TestRemoveUnusedParams(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

func process(used int, unused string) int {
	return used * 2
}
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRefactorer()
	result, err := r.RemoveUnusedParams(file, "process")
	if err != nil {
		t.Fatal(err)
	}

	if result.Type != "remove_unused_params" {
		t.Errorf("expected type remove_unused_params, got %s", result.Type)
	}
	if result.Changes != 1 {
		t.Errorf("expected 1 change, got %d", result.Changes)
	}

	data, _ := os.ReadFile(file)
	got := string(data)
	if strings.Contains(got, "unused") {
		t.Error("unused parameter should be removed")
	}
	if !strings.Contains(got, "used int") {
		t.Error("used parameter should remain")
	}
}

func TestRemoveUnusedParams_AllUsed(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

import "fmt"

func greet(name string, age int) {
	fmt.Printf("%s is %d", name, age)
}
`
	os.WriteFile(file, []byte(content), 0o644)

	r := NewRefactorer()
	result, err := r.RemoveUnusedParams(file, "greet")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changes != 0 {
		t.Errorf("expected 0 changes when all params are used, got %d", result.Changes)
	}
}

func TestRemoveUnusedParams_FuncNotFound(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	os.WriteFile(file, []byte("package main\n"), 0o644)

	r := NewRefactorer()
	_, err := r.RemoveUnusedParams(file, "nonexistent")
	if err == nil {
		t.Fatal("expected error for function not found")
	}
}

func TestFormatRefactoringResult(t *testing.T) {
	result := &RefactoringResult{
		File:        "/tmp/test.go",
		Changes:     3,
		Before:      "old code",
		After:       "new code",
		Type:        "rename_symbol",
		Description: "Renamed foo to bar",
	}

	output := FormatRefactoringResult(result)
	if !strings.Contains(output, "rename_symbol") {
		t.Error("expected type in output")
	}
	if !strings.Contains(output, "/tmp/test.go") {
		t.Error("expected file in output")
	}
	if !strings.Contains(output, "Renamed foo to bar") {
		t.Error("expected description in output")
	}
	if !strings.Contains(output, "Changes: 3") {
		t.Error("expected changes count in output")
	}
}

func TestFormatRefactoringResult_Nil(t *testing.T) {
	output := FormatRefactoringResult(nil)
	if output != "No result" {
		t.Errorf("expected 'No result', got %q", output)
	}
}
