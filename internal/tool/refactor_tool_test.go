package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefactorTool_Interface(t *testing.T) {
	tool := NewRefactorTool()
	if tool.Name() != "Refactor" {
		t.Errorf("expected name Refactor, got %s", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
	params := tool.Parameters()
	if params == nil {
		t.Fatal("expected non-nil parameters")
	}
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map")
	}
	if _, exists := props["action"]; !exists {
		t.Error("expected action property")
	}
	if _, exists := props["file"]; !exists {
		t.Error("expected file property")
	}
}

func TestRefactorTool_Execute_RenameSymbol(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

var counter = 0

func increment() {
	counter++
}
`
	os.WriteFile(file, []byte(content), 0o644)

	tool := NewRefactorTool()
	input, _ := json.Marshal(map[string]interface{}{
		"action":   "rename_symbol",
		"file":     file,
		"old_name": "counter",
		"new_name": "count",
	})

	output, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "rename_symbol") {
		t.Error("expected rename_symbol in output")
	}

	data, _ := os.ReadFile(file)
	got := string(data)
	if strings.Contains(got, "counter") {
		t.Error("old name should be replaced")
	}
}

func TestRefactorTool_Execute_MissingAction(t *testing.T) {
	tool := NewRefactorTool()
	input, _ := json.Marshal(map[string]interface{}{
		"file": "/tmp/test.go",
	})

	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing action")
	}
}

func TestRefactorTool_Execute_MissingFile(t *testing.T) {
	tool := NewRefactorTool()
	input, _ := json.Marshal(map[string]interface{}{
		"action": "sort_imports",
	})

	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRefactorTool_Execute_UnknownAction(t *testing.T) {
	tool := NewRefactorTool()
	input, _ := json.Marshal(map[string]interface{}{
		"action": "unknown_action",
		"file":   "/tmp/test.go",
	})

	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}

func TestRefactorTool_Execute_SortImports(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

import (
	"os"
	"fmt"
)

func main() {
	fmt.Println(os.Args)
}
`
	os.WriteFile(file, []byte(content), 0o644)

	tool := NewRefactorTool()
	input, _ := json.Marshal(map[string]interface{}{
		"action": "sort_imports",
		"file":   file,
	})

	output, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "sort_imports") {
		t.Error("expected sort_imports in output")
	}
}

func TestRefactorTool_Execute_ExtractFunction(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

import "fmt"

func main() {
	fmt.Println("a")
	fmt.Println("b")
}
`
	os.WriteFile(file, []byte(content), 0o644)

	tool := NewRefactorTool()
	input, _ := json.Marshal(map[string]interface{}{
		"action":     "extract_function",
		"file":       file,
		"start_line": 6,
		"end_line":   7,
		"new_name":   "printAB",
	})

	output, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "extract_function") {
		t.Error("expected extract_function in output")
	}
}

func TestRefactorTool_Execute_InlineVariable(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

import "fmt"

func main() {
	val := "test"
	fmt.Println(val)
}
`
	os.WriteFile(file, []byte(content), 0o644)

	tool := NewRefactorTool()
	input, _ := json.Marshal(map[string]interface{}{
		"action": "inline_variable",
		"file":   file,
		"line":   6,
	})

	output, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "inline_variable") {
		t.Error("expected inline_variable in output")
	}
}

func TestRefactorTool_Execute_ExtractVariable(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

import "fmt"

func main() {
	fmt.Println(1 + 1)
}
`
	os.WriteFile(file, []byte(content), 0o644)

	tool := NewRefactorTool()
	input, _ := json.Marshal(map[string]interface{}{
		"action":   "extract_variable",
		"file":     file,
		"line":     6,
		"expr":     "1 + 1",
		"var_name": "result",
	})

	output, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "extract_variable") {
		t.Error("expected extract_variable in output")
	}
}

func TestRefactorTool_Execute_AddErrorCheck(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

func main() {
	err := doThing()
}
`
	os.WriteFile(file, []byte(content), 0o644)

	tool := NewRefactorTool()
	input, _ := json.Marshal(map[string]interface{}{
		"action": "add_error_check",
		"file":   file,
		"line":   4,
	})

	output, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "add_error_check") {
		t.Error("expected add_error_check in output")
	}
}

func TestRefactorTool_Execute_WrapWithContext(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

func work() error {
	err := step()
	return err
}
`
	os.WriteFile(file, []byte(content), 0o644)

	tool := NewRefactorTool()
	input, _ := json.Marshal(map[string]interface{}{
		"action":  "wrap_with_context",
		"file":    file,
		"line":    5,
		"context": "work failed",
	})

	output, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "wrap_with_context") {
		t.Error("expected wrap_with_context in output")
	}
}

func TestRefactorTool_Execute_RemoveUnusedParams(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	content := `package main

func process(used int, unused string) int {
	return used * 2
}
`
	os.WriteFile(file, []byte(content), 0o644)

	tool := NewRefactorTool()
	input, _ := json.Marshal(map[string]interface{}{
		"action":    "remove_unused_params",
		"file":      file,
		"func_name": "process",
	})

	output, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "remove_unused_params") {
		t.Error("expected remove_unused_params in output")
	}
}

func TestRefactorTool_Execute_ConvertTableTest(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "calc_test.go")

	content := `package main

import "testing"

func TestMultiply(t *testing.T) {
	result := 3 * 4
	if result != 12 {
		t.Fatal("expected 12")
	}
}
`
	os.WriteFile(file, []byte(content), 0o644)

	tool := NewRefactorTool()
	input, _ := json.Marshal(map[string]interface{}{
		"action":    "convert_table_test",
		"file":      file,
		"test_func": "TestMultiply",
	})

	output, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "convert_table_test") {
		t.Error("expected convert_table_test in output")
	}
}

func TestParseParamList(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"a int, b string", 2},
		{"a, b int", 2},
		{"", 0},
		{"ctx context.Context, name string", 2},
	}

	for _, tt := range tests {
		params := parseParamList(tt.input)
		if len(params) != tt.expected {
			t.Errorf("parseParamList(%q) returned %d params, expected %d", tt.input, len(params), tt.expected)
		}
	}
}

func TestDetectParameters(t *testing.T) {
	before := []string{
		"\tx := 10",
		"\ty := 20",
		"\tz := 30",
	}
	extracted := []string{
		"\tfmt.Println(x, y)",
	}

	params := detectParameters(before, extracted)
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d: %v", len(params), params)
	}
	// Should be sorted.
	if params[0] != "x" || params[1] != "y" {
		t.Errorf("expected [x, y], got %v", params)
	}
}
