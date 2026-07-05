package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCodeCompletionTool_Name(t *testing.T) {
	ct := &CodeCompletionTool{}
	if got := ct.Name(); got != "code_completion" {
		t.Errorf("Name() = %q, want %q", got, "code_completion")
	}
}

func TestCodeCompletionTool_Description(t *testing.T) {
	ct := &CodeCompletionTool{}
	if got := ct.Description(); got == "" {
		t.Error("Description() returned empty string")
	}
}

func TestCodeCompletionTool_Parameters(t *testing.T) {
	ct := &CodeCompletionTool{}
	params := ct.Parameters()

	if params == nil {
		t.Fatal("Parameters() returned nil")
	}

	// Check that required fields are present.
	properties := params.(map[string]any)["properties"].(map[string]any)
	required := params.(map[string]any)["required"].([]any)

	expectedRequired := []string{"file_path", "cursor_line", "cursor_col", "prefix", "language"}
	for _, field := range required {
		found := false
		for _, exp := range expectedRequired {
			if field.(string) == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected required field: %s", field)
		}
	}

	// Check that all required fields have type and description.
	for _, field := range required {
		prop, ok := properties[field.(string)]
		if !ok {
			t.Errorf("Missing property for field: %s", field)
			continue
		}
		propMap := prop.(map[string]any)
		if propMap["type"] == nil {
			t.Errorf("Missing type for field: %s", field)
		}
		if propMap["description"] == nil {
			t.Errorf("Missing description for field: %s", field)
		}
	}
}

func TestCodeCompletionTool_Execute(t *testing.T) {
	// Create a mock ChatFn that returns a completion.
	mockChatFn := func(ctx context.Context, prompt string) (string, error) {
		return "\t\"fmt\"", nil
	}

	ct := &CodeCompletionTool{
		ChatFn: mockChatFn,
	}

	// Execute the tool.
	input := `{"file_path": "test.go", "cursor_line": 4, "cursor_col": 1, "prefix": "\tfmt.\n", "language": "go"}`
	result, err := ct.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify the result.
	if result == "" {
		t.Error("Execute() returned empty result")
	}
}

func TestCodeCompletionTool_Execute_InvalidInput(t *testing.T) {
	mockChatFn := func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}
	ct := &CodeCompletionTool{
		ChatFn: mockChatFn,
	}

	// Test with empty prefix.
	input := `{"file_path": "test.go", "cursor_line": 4, "cursor_col": 1, "prefix": "", "language": "go"}`
	_, err := ct.Execute(context.Background(), input)
	if err == nil {
		t.Error("Execute() should return error for empty prefix")
	}

	// Test with missing required fields.
	input = `{"file_path": "test.go"}`
	_, err = ct.Execute(context.Background(), input)
	if err == nil {
		t.Error("Execute() should return error for missing required fields")
	}
}

func TestCodeCompletionTool_Execute_FileNotFound(t *testing.T) {
	ct := &CodeCompletionTool{}

	input := `{"file_path": "nonexistent.go", "cursor_line": 1, "cursor_col": 1, "prefix": "package ", "language": "go"}`
	_, err := ct.Execute(context.Background(), input)
	if err == nil {
		t.Error("Execute() should return error for nonexistent file")
	}
}

func TestCodeCompletionTool_Execute_JSONParsing(t *testing.T) {
	mockChatFn := func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}
	ct := &CodeCompletionTool{
		ChatFn: mockChatFn,
	}

	// Test with invalid JSON.
	input := `invalid json`
	_, err := ct.Execute(context.Background(), input)
	if err == nil {
		t.Error("Execute() should return error for invalid JSON")
	}

	// Test with valid JSON but invalid structure.
	input = `{"invalid": "structure"}`
	_, err = ct.Execute(context.Background(), input)
	if err == nil {
		t.Error("Execute() should return error for missing required fields")
	}
}
