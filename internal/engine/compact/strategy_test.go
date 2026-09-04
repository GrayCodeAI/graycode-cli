package compact

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"

	"github.com/GrayCodeAI/graycode-cli/internal/engine/token"
)

func TestCompactEstimateTokens(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{
		{Role: "user", Content: "Hello world"},
		{Role: "assistant", Content: strings.Repeat("x", 400)},
	}
	tokens := token.EstimateTokens(msgs)
	if tokens < 1 {
		t.Errorf("expected at least 1 token, got %d", tokens)
	}
	shortMsgs := []types.GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}
	shortTokens := token.EstimateTokens(shortMsgs)
	if tokens <= shortTokens {
		t.Errorf("expected more tokens for longer input: %d vs %d", tokens, shortTokens)
	}
}

func TestAdjustIndexToPreserveAPIInvariants(t *testing.T) {
	tests := []struct {
		name     string
		msgs     []types.GraycodeRouterMessage
		startIdx int
		wantIdx  int
	}{
		{
			name:     "empty messages",
			msgs:     nil,
			startIdx: 0,
			wantIdx:  0,
		},
		{
			name: "no tool pairs",
			msgs: []types.GraycodeRouterMessage{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "hi"},
				{Role: "user", Content: "bye"},
			},
			startIdx: 1,
			wantIdx:  1,
		},
		{
			name: "tool_result at startIdx - moves back past tool_use",
			msgs: []types.GraycodeRouterMessage{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "", ToolUse: []types.ToolCall{{ID: "t1", Name: "Bash"}}},
				{Role: "user", ToolResults: []types.ToolResult{{ToolUseID: "t1", Content: "output"}}},
				{Role: "assistant", Content: "done"},
			},
			startIdx: 2,
			wantIdx:  1,
		},
		{
			name: "at boundary already",
			msgs: []types.GraycodeRouterMessage{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "response"},
			},
			startIdx: 1,
			wantIdx:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AdjustIndexToPreserveAPIInvariants(tt.msgs, tt.startIdx)
			if got != tt.wantIdx {
				t.Errorf("AdjustIndexToPreserveAPIInvariants() = %d, want %d", got, tt.wantIdx)
			}
		})
	}
}

func TestMicrocompactMessages(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{
		{Role: "user", Content: "read file.go"},
		{Role: "assistant", ToolUse: []types.ToolCall{{ID: "t1", Name: "Read"}}},
		{Role: "user", ToolResults: []types.ToolResult{{ToolUseID: "t1", Content: "package main\nfunc main() {}"}}},
		{Role: "assistant", Content: "Here's the file content"},
		{Role: "user", Content: "now read another"},
		{Role: "assistant", ToolUse: []types.ToolCall{{ID: "t2", Name: "Read"}}},
		{Role: "user", ToolResults: []types.ToolResult{{ToolUseID: "t2", Content: "package utils\nfunc Helper() {}"}}},
		{Role: "assistant", Content: "Here's the second file"},
		{Role: "user", Content: "and another"},
		{Role: "assistant", ToolUse: []types.ToolCall{{ID: "t3", Name: "Read"}}},
		{Role: "user", ToolResults: []types.ToolResult{{ToolUseID: "t3", Content: "package config\nfunc Load() {}"}}},
		{Role: "assistant", Content: "Here's the third"},
		{Role: "user", Content: "one more"},
		{Role: "assistant", ToolUse: []types.ToolCall{{ID: "t4", Name: "Read"}}},
		{Role: "user", ToolResults: []types.ToolResult{{ToolUseID: "t4", Content: "package api\nfunc Serve() {}"}}},
		{Role: "assistant", Content: "Here's the fourth"},
	}

	cfg := MicroCompactConfig{
		CompactableTools: compactableTools,
		TimeGapMins:      0,
		KeepRecent:       2,
	}

	result := MicrocompactMessages(msgs, cfg)
	if len(result) != len(msgs) {
		t.Fatalf("message count changed: got %d, want %d", len(result), len(msgs))
	}

	clearedCount := 0
	for _, m := range result {
		if len(m.ToolResults) > 0 && m.ToolResults[0].Content == "[Old tool result content cleared]" {
			clearedCount++
		}
	}
	if clearedCount != 2 {
		t.Errorf("expected 2 cleared results, got %d", clearedCount)
	}

	if result[10].ToolResults[0].Content == "[Old tool result content cleared]" {
		t.Error("third-to-last result should be preserved")
	}
	if result[14].ToolResults[0].Content == "[Old tool result content cleared]" {
		t.Error("last result should be preserved")
	}
}

func TestAPICompactMessages(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", ToolUse: []types.ToolCall{{ID: "t1", Name: "Bash", Arguments: map[string]interface{}{"command": strings.Repeat("x", 1000)}}}},
		{Role: "user", ToolResults: []types.ToolResult{{ToolUseID: "t1", Content: strings.Repeat("output ", 1000)}}},
		{Role: "assistant", Content: "done"},
	}

	cfg := APICompactConfig{
		TriggerTokens:    0,
		KeepTargetTokens: 100,
		ClearToolInputs:  true,
		ClearThinking:    true,
		PreserveMutating: true,
	}

	result := APICompactMessages(msgs, cfg)
	if len(result) != len(msgs) {
		t.Fatalf("message count changed")
	}

	if result[2].ToolResults[0].Content != "[Old tool result content cleared]" {
		t.Error("expected tool result to be cleared")
	}
}

func TestAPICompactPreservesMutatingTools(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{
		{Role: "user", Content: "edit file"},
		{Role: "assistant", ToolUse: []types.ToolCall{{ID: "t1", Name: "Edit", Arguments: map[string]interface{}{"old_string": strings.Repeat("x", 1000), "new_string": "y"}}}},
		{Role: "user", ToolResults: []types.ToolResult{{ToolUseID: "t1", Content: strings.Repeat("edited ", 500)}}},
		{Role: "assistant", Content: "edited"},
	}

	cfg := APICompactConfig{
		TriggerTokens:    0,
		KeepTargetTokens: 100,
		ClearToolInputs:  true,
		ClearThinking:    true,
		PreserveMutating: true,
	}

	result := APICompactMessages(msgs, cfg)
	if result[2].ToolResults[0].Content == "[Old tool result content cleared]" {
		t.Error("mutating tool result should be preserved")
	}
}

func TestCalculateMessagesToKeepIndex(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{
		{Role: "user", Content: strings.Repeat("hello ", 100)},
		{Role: "assistant", Content: strings.Repeat("response ", 100)},
		{Role: "user", Content: strings.Repeat("follow up ", 100)},
		{Role: "assistant", Content: strings.Repeat("answer ", 100)},
		{Role: "user", Content: strings.Repeat("more ", 100)},
		{Role: "assistant", Content: strings.Repeat("final ", 100)},
	}

	cfg := SessionMemoryConfig{
		MinTokens:            50,
		MinTextBlockMessages: 2,
		MaxTokens:            5000,
	}

	idx := CalculateMessagesToKeepIndex(msgs, cfg)
	if idx >= len(msgs) {
		t.Errorf("keep index should be within messages range, got %d", idx)
	}
	if idx < 0 {
		t.Errorf("keep index should be non-negative, got %d", idx)
	}
}

func TestFilterCompactBoundaries(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{
		{Role: "user", Content: "[Session memory summary]\nold stuff"},
		{Role: "assistant", Content: "Understood."},
		{Role: "user", Content: "real message"},
		{Role: "assistant", Content: "real response"},
	}

	filtered := FilterCompactBoundaries(msgs)
	if len(filtered) != 3 {
		t.Errorf("expected 3 messages after filtering, got %d", len(filtered))
	}
	if filtered[0].Content != "Understood." {
		t.Errorf("expected first kept message to be 'Understood.', got %q", filtered[0].Content)
	}
}
