package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestToolsetToolList(t *testing.T) {
	out, err := (ToolsetTool{}).Execute(context.Background(), json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"research", "dev", "ops", "full_stack"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list missing %q: %s", want, out)
		}
	}
}

func TestToolsetToolResolve(t *testing.T) {
	out, err := (ToolsetTool{}).Execute(context.Background(), json.RawMessage(`{"action":"resolve","name":"research"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "WebSearch") || !strings.Contains(out, "CodeMatch") {
		t.Fatalf("resolve output missing expected tools: %s", out)
	}
	var resp struct {
		Toolset string   `json:"toolset"`
		Tools   []string `json:"tools"`
		Count   int      `json:"count"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Toolset != "research" || resp.Count != len(resp.Tools) || resp.Count == 0 {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestToolsetToolResolveFullStackComposes(t *testing.T) {
	out, err := (ToolsetTool{}).Execute(context.Background(), json.RawMessage(`{"action":"resolve","name":"full_stack"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Bash", "Edit", "Write", "CronCreate", "WebFetch"} {
		if !strings.Contains(out, `"`+want+`"`) {
			t.Fatalf("full_stack missing %q: %s", want, out)
		}
	}
}

func TestToolsetToolUnknown(t *testing.T) {
	if _, err := (ToolsetTool{}).Execute(context.Background(), json.RawMessage(`{"action":"resolve","name":"nope"}`)); err == nil {
		t.Fatal("expected error for unknown toolset")
	}
}
