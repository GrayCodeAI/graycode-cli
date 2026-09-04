package session

import (
	"reflect"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestRuntimeMessageRoundTrip(t *testing.T) {
	in := []types.GraycodeRouterMessage{
		{
			Role:    "assistant",
			Content: "working",
			ToolUse: []types.ToolCall{{
				ID:   "tool_1",
				Name: "Read",
				Arguments: map[string]interface{}{
					"path": "main.go",
				},
			}},
		},
		{
			Role:    "user",
			Content: "file contents",
			ToolResults: []types.ToolResult{{
				ToolUseID: "tool_1",
				Content:   "package main",
				IsError:   false,
			}},
		},
	}

	persisted := FromRuntimeMessages(in)
	out := ToRuntimeMessages(persisted)

	if !reflect.DeepEqual(out, in) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", out, in)
	}
}
