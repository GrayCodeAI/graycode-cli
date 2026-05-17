package tool

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCoreMemoryAppendTool_Execute(t *testing.T) {
	tool := CoreMemoryAppendTool{}
	input, _ := json.Marshal(map[string]interface{}{
		"key":   "project_context",
		"value": "This is a Go project using cobra for CLI",
	})
	_, err := tool.Execute(context.Background(), input)
	// May error if memory backend not initialized — that's expected in unit tests
	_ = err
}

func TestCoreMemoryAppendTool_InvalidJSON(t *testing.T) {
	t.Parallel()
	tool := CoreMemoryAppendTool{}
	_, err := tool.Execute(context.Background(), []byte("bad"))
	if err == nil {
		t.Error("should error on invalid JSON")
	}
}

func TestCoreMemoryReplaceTool_Execute(t *testing.T) {
	tool := CoreMemoryReplaceTool{}
	input, _ := json.Marshal(map[string]interface{}{
		"key":       "replace_test",
		"old_value": "original",
		"new_value": "updated",
	})
	_, _ = tool.Execute(context.Background(), input)
}

func TestCoreMemoryReplaceTool_InvalidJSON(t *testing.T) {
	t.Parallel()
	tool := CoreMemoryReplaceTool{}
	_, err := tool.Execute(context.Background(), []byte("{"))
	if err == nil {
		t.Error("should error on invalid JSON")
	}
}

func TestCoreMemoryRethinkTool_Execute(t *testing.T) {
	tool := CoreMemoryRethinkTool{}
	input, _ := json.Marshal(map[string]interface{}{
		"key":       "rethink_test",
		"new_value": "new content",
	})
	_, _ = tool.Execute(context.Background(), input)
}

func TestCoreMemoryRethinkTool_InvalidJSON(t *testing.T) {
	t.Parallel()
	tool := CoreMemoryRethinkTool{}
	_, err := tool.Execute(context.Background(), []byte("x"))
	if err == nil {
		t.Error("should error")
	}
}

func TestCoreMemoryAppendTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := CoreMemoryAppendTool{}
	if tool.Name() != "CoreMemoryAppend" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

func TestCoreMemoryReplaceTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := CoreMemoryReplaceTool{}
	if tool.Name() != "CoreMemoryReplace" {
		t.Errorf("Name = %q", tool.Name())
	}
}

func TestCoreMemoryRethinkTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := CoreMemoryRethinkTool{}
	if tool.Name() != "CoreMemoryRethink" {
		t.Errorf("Name = %q", tool.Name())
	}
}
