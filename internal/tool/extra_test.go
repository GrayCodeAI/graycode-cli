package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNotebookEditTool_Execute(t *testing.T) {
	t.Parallel()
	tool := NotebookEditTool{}

	// Create a test notebook
	dir := t.TempDir()
	nb := filepath.Join(dir, "test.ipynb")
	notebook := map[string]interface{}{
		"cells": []map[string]interface{}{
			{"cell_type": "code", "source": []string{"print('hello')"}, "outputs": []interface{}{}, "metadata": map[string]interface{}{}},
		},
		"metadata":       map[string]interface{}{},
		"nbformat":       4,
		"nbformat_minor": 5,
	}
	data, _ := json.Marshal(notebook)
	_ = os.WriteFile(nb, data, 0o644)

	input, _ := json.Marshal(map[string]interface{}{
		"path":        nb,
		"cell_number": 0,
		"new_source":  "print('updated')",
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == "" {
		t.Error("result should not be empty")
	}
}

func TestNotebookEditTool_Execute_MissingFile(t *testing.T) {
	t.Parallel()
	tool := NotebookEditTool{}
	input, _ := json.Marshal(map[string]interface{}{
		"path":        "/nonexistent/path.ipynb",
		"cell_number": 0,
		"new_source":  "x",
	})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Error("should error on missing file")
	}
}

func TestNotebookEditTool_Execute_InvalidJSON(t *testing.T) {
	t.Parallel()
	tool := NotebookEditTool{}
	_, err := tool.Execute(context.Background(), []byte("not json"))
	if err == nil {
		t.Error("should error on invalid JSON input")
	}
}

func TestConfigTool_Execute(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	graycodeDir := filepath.Join(dir, ".graycode")
	_ = os.MkdirAll(graycodeDir, 0o755)

	ctx := context.Background()
	ctx = WithToolContext(ctx, &ToolContext{
		SettingsGet: func(key string) (string, bool) {
			if key == "model" {
				return "claude-sonnet-4", true
			}
			return "", false
		},
		SettingsSet: func(key, value string) error { return nil },
	})

	tool := ConfigTool{}
	input, _ := json.Marshal(map[string]interface{}{
		"action": "get",
		"key":    "model",
	})
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "model=claude-sonnet-4" {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestConfigTool_Execute_InvalidInput(t *testing.T) {
	t.Parallel()
	tool := ConfigTool{}
	_, err := tool.Execute(context.Background(), []byte("bad"))
	if err == nil {
		t.Error("should error on invalid input")
	}
}

func TestBriefTool_Execute(t *testing.T) {
	t.Parallel()
	tool := BriefTool{}
	input, _ := json.Marshal(map[string]interface{}{
		"message": "hello user",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == "" {
		t.Error("BriefTool should return the message")
	}
}

func TestBriefTool_Execute_Empty(t *testing.T) {
	t.Parallel()
	tool := BriefTool{}
	input, _ := json.Marshal(map[string]interface{}{})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Error("should error on empty message")
	}
}

func TestBriefTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := BriefTool{}
	if tool.Name() != "SendUserMessage" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}
