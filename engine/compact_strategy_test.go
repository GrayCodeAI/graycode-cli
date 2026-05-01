package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/hawk/eyrie/client"

	"github.com/GrayCodeAI/hawk/logger"
	"github.com/GrayCodeAI/hawk/metrics"
)

func TestEstimateTokens(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "Hello world"},
		{Role: "assistant", Content: strings.Repeat("x", 400)},
	}
	tokens := EstimateTokens(msgs)
	if tokens < 100 {
		t.Errorf("expected at least 100 tokens, got %d", tokens)
	}
}

func TestAdjustIndexToPreserveAPIInvariants(t *testing.T) {
	tests := []struct {
		name     string
		msgs     []client.EyrieMessage
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
			msgs: []client.EyrieMessage{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "hi"},
				{Role: "user", Content: "bye"},
			},
			startIdx: 1,
			wantIdx:  1,
		},
		{
			name: "tool_result at startIdx - moves back past tool_use",
			msgs: []client.EyrieMessage{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "", ToolUse: []client.ToolCall{{ID: "t1", Name: "Bash"}}},
				{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t1", Content: "output"}},
				{Role: "assistant", Content: "done"},
			},
			startIdx: 2,
			wantIdx:  1,
		},
		{
			name: "at boundary already",
			msgs: []client.EyrieMessage{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "response"},
			},
			startIdx: 1,
			wantIdx:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustIndexToPreserveAPIInvariants(tt.msgs, tt.startIdx)
			if got != tt.wantIdx {
				t.Errorf("adjustIndexToPreserveAPIInvariants() = %d, want %d", got, tt.wantIdx)
			}
		})
	}
}

func TestMicrocompactMessages(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "read file.go"},
		{Role: "assistant", ToolUse: []client.ToolCall{{ID: "t1", Name: "Read"}}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t1", Content: "package main\nfunc main() {}"}},
		{Role: "assistant", Content: "Here's the file content"},
		{Role: "user", Content: "now read another"},
		{Role: "assistant", ToolUse: []client.ToolCall{{ID: "t2", Name: "Read"}}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t2", Content: "package utils\nfunc Helper() {}"}},
		{Role: "assistant", Content: "Here's the second file"},
		{Role: "user", Content: "and another"},
		{Role: "assistant", ToolUse: []client.ToolCall{{ID: "t3", Name: "Read"}}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t3", Content: "package config\nfunc Load() {}"}},
		{Role: "assistant", Content: "Here's the third"},
		{Role: "user", Content: "one more"},
		{Role: "assistant", ToolUse: []client.ToolCall{{ID: "t4", Name: "Read"}}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t4", Content: "package api\nfunc Serve() {}"}},
		{Role: "assistant", Content: "Here's the fourth"},
	}

	cfg := MicroCompactConfig{
		CompactableTools: compactableTools,
		TimeGapMins:      0,
		KeepRecent:       2,
	}

	result := microcompactMessages(msgs, cfg)
	if len(result) != len(msgs) {
		t.Fatalf("message count changed: got %d, want %d", len(result), len(msgs))
	}

	clearedCount := 0
	for _, m := range result {
		if m.ToolResult != nil && m.ToolResult.Content == "[Old tool result content cleared]" {
			clearedCount++
		}
	}
	if clearedCount != 2 {
		t.Errorf("expected 2 cleared results, got %d", clearedCount)
	}

	// Last 2 results should be preserved
	if result[10].ToolResult.Content == "[Old tool result content cleared]" {
		t.Error("third-to-last result should be preserved")
	}
	if result[14].ToolResult.Content == "[Old tool result content cleared]" {
		t.Error("last result should be preserved")
	}
}

func TestSessionMemoryStrategy_ShouldTrigger(t *testing.T) {
	s := &SessionMemoryStrategy{}
	msgs := makeMessages(50)
	// Without a memory file, should not trigger
	if s.ShouldTrigger(msgs, 200000, 150000) {
		t.Error("should not trigger without memory file")
	}
}

func TestAPICompactMessages(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", ToolUse: []client.ToolCall{{ID: "t1", Name: "Bash", Arguments: map[string]interface{}{"command": strings.Repeat("x", 1000)}}}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t1", Content: strings.Repeat("output ", 1000)}},
		{Role: "assistant", Content: "done"},
	}

	cfg := APICompactConfig{
		TriggerTokens:    0,
		KeepTargetTokens: 100,
		ClearToolInputs:  true,
		ClearThinking:    true,
		PreserveMutating: true,
	}

	result := apiCompactMessages(msgs, cfg)
	if len(result) != len(msgs) {
		t.Fatalf("message count changed")
	}

	if result[2].ToolResult.Content != "[Old tool result content cleared]" {
		t.Error("expected tool result to be cleared")
	}
}

func TestAPICompactPreservesMutatingTools(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "edit file"},
		{Role: "assistant", ToolUse: []client.ToolCall{{ID: "t1", Name: "Edit", Arguments: map[string]interface{}{"old_string": strings.Repeat("x", 1000), "new_string": "y"}}}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t1", Content: strings.Repeat("edited ", 500)}},
		{Role: "assistant", Content: "edited"},
	}

	cfg := APICompactConfig{
		TriggerTokens:    0,
		KeepTargetTokens: 100,
		ClearToolInputs:  true,
		ClearThinking:    true,
		PreserveMutating: true,
	}

	result := apiCompactMessages(msgs, cfg)
	// Edit tool results should NOT be cleared
	if result[2].ToolResult.Content == "[Old tool result content cleared]" {
		t.Error("mutating tool result should be preserved")
	}
}

func TestAutoCompactor_CircuitBreaker(t *testing.T) {
	cfg := DefaultCompactConfig()
	cfg.MaxFailures = 2
	cfg.ContextWindowSize = 1000
	cfg.AutoCompactBuffer = 100
	cfg.MaxOutputTokens = 100

	ac := NewAutoCompactor(cfg)
	ac.consecutiveFailures = 2

	sess := &Session{
		messages: makeMessages(200),
		log:      newTestLogger(),
		metrics:  newTestMetrics(),
	}

	if ac.ShouldAutoCompact(sess) {
		t.Error("should not trigger after max failures reached")
	}

	ac.ResetFailures()
	if !ac.ShouldAutoCompact(sess) {
		t.Error("should trigger after reset")
	}
}

func TestStrategyRegistry_SelectStrategy(t *testing.T) {
	cfg := DefaultCompactConfig()
	registry := NewStrategyRegistry(cfg)

	msgs := makeMessages(5)
	strategy := registry.SelectStrategy(msgs, cfg.ContextWindowSize)
	if strategy.Name() != "truncate" {
		t.Errorf("expected truncate for high token count with few messages, got %s", strategy.Name())
	}
}

func TestCalculateMessagesToKeepIndex(t *testing.T) {
	msgs := []client.EyrieMessage{
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

	idx := calculateMessagesToKeepIndex(msgs, cfg)
	if idx >= len(msgs) {
		t.Errorf("keep index should be within messages range, got %d", idx)
	}
	if idx < 0 {
		t.Errorf("keep index should be non-negative, got %d", idx)
	}
}

func TestFilterCompactBoundaries(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "[Session memory summary]\nold stuff"},
		{Role: "assistant", Content: "Understood."},
		{Role: "user", Content: "real message"},
		{Role: "assistant", Content: "real response"},
	}

	filtered := filterCompactBoundaries(msgs)
	if len(filtered) != 3 {
		t.Errorf("expected 3 messages after filtering, got %d", len(filtered))
	}
	if filtered[0].Content != "Understood." {
		t.Errorf("expected first kept message to be 'Understood.', got %q", filtered[0].Content)
	}
}

func TestTruncateStrategy(t *testing.T) {
	sess := &Session{
		messages: makeMessages(100),
		log:      newTestLogger(),
		metrics:  newTestMetrics(),
		client:   client.NewEyrieClient(&client.EyrieConfig{Provider: "test"}),
	}

	s := &TruncateStrategy{}
	result, err := s.Compact(context.Background(), sess)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Strategy != "truncate" {
		t.Errorf("expected strategy 'truncate', got %q", result.Strategy)
	}
	if len(sess.messages) >= 100 {
		t.Error("messages should have been reduced")
	}
}

// Helper functions

func makeMessages(n int) []client.EyrieMessage {
	msgs := make([]client.EyrieMessage, n)
	for i := range msgs {
		if i%2 == 0 {
			msgs[i] = client.EyrieMessage{Role: "user", Content: strings.Repeat("message ", 50)}
		} else {
			msgs[i] = client.EyrieMessage{Role: "assistant", Content: strings.Repeat("response ", 50)}
		}
	}
	return msgs
}

func newTestLogger() *logger.Logger {
	return logger.Default()
}

func newTestMetrics() *metrics.Registry {
	return metrics.NewRegistry()
}
