package engine

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/client"
)

func TestCollapseRepeatedMessages_NoCollapse(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: "bye"},
	}
	result := CollapseRepeatedMessages(msgs)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
}

func TestCollapseRepeatedMessages_ToolResults(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "run tests", ToolResult: &client.ToolResult{ToolUseID: "Bash", Content: "PASS: all tests passed\ndetails..."}},
		{Role: "user", Content: "run tests", ToolResult: &client.ToolResult{ToolUseID: "Bash", Content: "PASS: all tests passed\nother details"}},
		{Role: "user", Content: "run tests", ToolResult: &client.ToolResult{ToolUseID: "Bash", Content: "PASS: all tests passed\nmore details"}},
		{Role: "user", Content: "run tests", ToolResult: &client.ToolResult{ToolUseID: "Bash", Content: "PASS: all tests passed\nfinal"}},
	}
	result := CollapseRepeatedMessages(msgs)
	// Should collapse to: first + collapsed summary + last = 3
	if len(result) != 3 {
		t.Fatalf("expected 3 messages after collapse, got %d", len(result))
	}
}

func TestCollapseRepeatedMessages_Errors(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t1", Content: "connection refused", IsError: true}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t2", Content: "connection refused", IsError: true}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t3", Content: "connection refused", IsError: true}},
	}
	result := CollapseRepeatedMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 collapsed error message, got %d", len(result))
	}
	if result[0].Content == "" {
		t.Fatal("collapsed message should have content")
	}
}

func TestCollapseRepeatedMessages_MixedContent(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "hello"},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "Bash", Content: "ok\ndetails"}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "Bash", Content: "ok\nother"}},
		{Role: "assistant", Content: "done"},
	}
	// Only 2 consecutive tool results — not enough to collapse
	result := CollapseRepeatedMessages(msgs)
	if len(result) != 4 {
		t.Fatalf("expected 4 messages (no collapse with only 2), got %d", len(result))
	}
}

func TestCollapseRepeatedMessages_ShortInput(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "hi"},
	}
	result := CollapseRepeatedMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
}
