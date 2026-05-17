package session

import (
	"testing"
	"time"
)

func TestListCheckpoints(t *testing.T) {
	sess := &Session{
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
			{Role: "user", Content: "read file.go"},
			{Role: "assistant", ToolUse: []ToolCall{{ID: "t1", Name: "Read"}}},
			{Role: "user", ToolResult: &ToolResult{ToolUseID: "t1", Content: "package main"}},
			{Role: "assistant", Content: "here's the file"},
			{Role: "user", Content: "now edit it"},
		},
	}

	checkpoints := ListCheckpoints(sess)
	if len(checkpoints) == 0 {
		t.Fatal("expected checkpoints")
	}

	// Should have user messages (non-tool-result) and assistant text responses
	userCPs := 0
	assistantCPs := 0
	for _, cp := range checkpoints {
		if cp.Role == "user" {
			userCPs++
		} else {
			assistantCPs++
		}
	}
	if userCPs != 3 {
		t.Errorf("expected 3 user checkpoints, got %d", userCPs)
	}
	if assistantCPs != 2 {
		t.Errorf("expected 2 assistant checkpoints, got %d", assistantCPs)
	}
}

func TestListCheckpoints_Nil(t *testing.T) {
	if cps := ListCheckpoints(nil); cps != nil {
		t.Error("nil session should return nil")
	}
}

func TestRewindTo(t *testing.T) {
	sess := &Session{
		Messages: []Message{
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "response 1"},
			{Role: "user", Content: "second"},
			{Role: "assistant", Content: "response 2"},
			{Role: "user", Content: "third"},
		},
		UpdatedAt: time.Now(),
	}

	err := RewindTo(sess, 2)
	if err != nil {
		t.Fatalf("RewindTo error: %v", err)
	}
	if len(sess.Messages) != 3 {
		t.Errorf("expected 3 messages after rewind to index 2, got %d", len(sess.Messages))
	}
	if sess.Messages[2].Content != "second" {
		t.Errorf("last message should be 'second', got %q", sess.Messages[2].Content)
	}
}

func TestRewindTo_InvalidIndex(t *testing.T) {
	sess := &Session{Messages: []Message{{Role: "user", Content: "hi"}}}

	if err := RewindTo(sess, -1); err == nil {
		t.Error("expected error for negative index")
	}
	if err := RewindTo(sess, 5); err == nil {
		t.Error("expected error for out of bounds index")
	}
}

func TestRewindLastExchange(t *testing.T) {
	sess := &Session{
		Messages: []Message{
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "response 1"},
			{Role: "user", Content: "second"},
			{Role: "assistant", Content: "response 2"},
		},
	}

	err := RewindLastExchange(sess)
	if err != nil {
		t.Fatalf("RewindLastExchange error: %v", err)
	}
	if len(sess.Messages) > 2 {
		t.Errorf("expected at most 2 messages after rewind, got %d", len(sess.Messages))
	}
}

func TestFormatCheckpointList(t *testing.T) {
	cps := []Checkpoint{
		{Index: 0, Role: "user", Preview: "hello world"},
		{Index: 1, Role: "assistant", Preview: "hi there"},
	}
	output := FormatCheckpointList(cps)
	if output == "" {
		t.Error("should not be empty")
	}
	if output == "No checkpoints available." {
		t.Error("should have checkpoints formatted")
	}
}

func TestFormatCheckpointList_Empty(t *testing.T) {
	output := FormatCheckpointList(nil)
	if output != "No checkpoints available." {
		t.Errorf("expected empty message, got %q", output)
	}
}

func TestTruncatePreview(t *testing.T) {
	long := "This is a very long message that should be truncated because it exceeds the maximum length we allow for preview display."
	result := truncatePreview(long, 30)
	if len(result) > 30 {
		t.Errorf("should be truncated to 30 chars, got %d", len(result))
	}
	if result[len(result)-3:] != "..." {
		t.Error("should end with ...")
	}

	short := "short"
	if truncatePreview(short, 30) != short {
		t.Error("short strings should not be modified")
	}
}
