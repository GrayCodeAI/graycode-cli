package tools_test

import (
	"encoding/json"
	"testing"

	"github.com/GrayCodeAI/hawk-core-contracts/tools"
)

func TestToolCallJSONRoundTrip(t *testing.T) {
	in := tools.ToolCall{
		ID:   "call_123",
		Name: "exec",
		Arguments: map[string]interface{}{
			"cmd": "go test ./...",
			"tty": true,
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var out tools.ToolCall
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if out.ID != in.ID {
		t.Fatalf("ID = %q, want %q", out.ID, in.ID)
	}
	if out.Name != in.Name {
		t.Fatalf("Name = %q, want %q", out.Name, in.Name)
	}
	if got, ok := out.Arguments["cmd"].(string); !ok || got != "go test ./..." {
		t.Fatalf("Arguments[cmd] = %#v, want %q", out.Arguments["cmd"], "go test ./...")
	}
	if got, ok := out.Arguments["tty"].(bool); !ok || !got {
		t.Fatalf("Arguments[tty] = %#v, want true", out.Arguments["tty"])
	}
}

func TestToolResultJSONOmitsFalseIsError(t *testing.T) {
	result := tools.ToolResult{
		ToolUseID: "toolu_123",
		Content:   "ok",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if _, exists := got["is_error"]; exists {
		t.Fatalf("unexpected is_error field in %#v", got)
	}
}
