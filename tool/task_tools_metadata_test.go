package tool

import (
	"context"
	"encoding/json"
	"testing"
)

func TestTaskOutputTool_Metadata(t *testing.T) {
	t.Parallel()
	tl := TaskOutputTool{}
	if tl.Name() == "" {
		t.Error("Name empty")
	}
	if tl.Description() == "" {
		t.Error("Description empty")
	}
	if tl.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

func TestTaskStopTool_Metadata(t *testing.T) {
	t.Parallel()
	tl := TaskStopTool{}
	if tl.Name() == "" {
		t.Error("Name empty")
	}
	if tl.Description() == "" {
		t.Error("Description empty")
	}
	if tl.Parameters() == nil {
		t.Error("Parameters nil")
	}
}

func TestTaskOutputTool_Execute(t *testing.T) {
	t.Parallel()
	tl := TaskOutputTool{}
	input, _ := json.Marshal(map[string]interface{}{"taskId": "nonexistent"})
	_, _ = tl.Execute(context.Background(), input)
}

func TestTaskStopTool_Execute(t *testing.T) {
	t.Parallel()
	tl := TaskStopTool{}
	input, _ := json.Marshal(map[string]interface{}{"taskId": "nonexistent"})
	_, _ = tl.Execute(context.Background(), input)
}

func TestWorktreeToolMetadata(t *testing.T) {
	t.Parallel()
	tools := []Tool{
		EnterWorktreeTool{},
		ExitWorktreeTool{},
	}
	for _, tl := range tools {
		if tl.Name() == "" {
			t.Errorf("Name empty for %T", tl)
		}
		if tl.Description() == "" {
			t.Errorf("Description empty for %s", tl.Name())
		}
		if tl.Parameters() == nil {
			t.Errorf("Parameters nil for %s", tl.Name())
		}
	}
}

func TestSleepTool_Execute(t *testing.T) {
	t.Parallel()
	tl := SleepTool{}
	if tl.Name() == "" {
		t.Error("Name empty")
	}
	input, _ := json.Marshal(map[string]interface{}{"duration_ms": 1})
	_, _ = tl.Execute(context.Background(), input)
}

func TestDownloadTool_Metadata(t *testing.T) {
	t.Parallel()
	tl := DownloadTool{}
	if tl.Name() == "" {
		t.Error("Name empty")
	}
	if tl.Description() == "" {
		t.Error("Description empty")
	}
}
