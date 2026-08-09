package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestToolHealthToolMetadata(t *testing.T) {
	tool := ToolHealthTool{}
	if tool.Name() != "ToolHealth" || tool.RiskLevel() != "low" {
		t.Fatalf("metadata = %q/%q", tool.Name(), tool.RiskLevel())
	}
}

func TestToolHealthToolReportsWithoutContext(t *testing.T) {
	out, err := (ToolHealthTool{}).Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(out, `"prerequisites"`) || !strings.Contains(out, `"registered_tools"`) {
		t.Fatalf("health output missing sections: %s", out)
	}
}

func TestToolHealthToolReportsRegistry(t *testing.T) {
	registry := NewRegistry(FileReadTool{}, WebFetchTool{})
	registry.EnableLazyModelSurface([]string{"Read"})
	ctx := WithToolContext(context.Background(), &ToolContext{Registry: registry})
	out, err := (ToolHealthTool{}).Execute(ctx, json.RawMessage(`{"include_optional":true}`))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(out, `"WebFetch"`) || !strings.Contains(out, `"Read"`) {
		t.Fatalf("registry tools missing: %s", out)
	}
}
