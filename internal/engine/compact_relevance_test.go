package engine

import (
	"context"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestRelevancePruneStrategy_ShouldTrigger(t *testing.T) {
	s := &RelevancePruneStrategy{}
	if s.ShouldTrigger(makeMessages(5), 200000, 100000) {
		t.Error("should not trigger with few messages")
	}
	if !s.ShouldTrigger(makeMessages(30), 200000, 100000) {
		t.Error("should trigger with many messages over threshold")
	}
}

func TestRelevancePruneStrategy_Compact(t *testing.T) {
	sess := NewSessionWithClient(NewMockClientForTest(), "test", "test-model", "", nil, false)
	// A long transcript; last user message is the task context.
	var msgs []types.GraycodeRouterMessage
	for i := 0; i < 60; i++ {
		msgs = append(msgs, types.GraycodeRouterMessage{Role: "user", Content: "unrelated filler content that has no shared keywords"})
	}
	msgs = append(msgs, types.GraycodeRouterMessage{Role: "user", Content: "refactor the payment module and update all tests"})
	sess.Persistence().SetRawMessages(msgs)

	s := &RelevancePruneStrategy{TargetTokens: 200}
	result, err := s.Compact(context.Background(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if result.Strategy != "relevance" {
		t.Fatalf("strategy = %q, want relevance", result.Strategy)
	}
	// It must not crash and must return a sane result; the final user message
	// should survive.
	after := sess.Persistence().RawMessages()
	if len(after) == 0 {
		t.Fatal("no messages after pruning")
	}
	found := false
	for _, m := range after {
		if m.Content == "refactor the payment module and update all tests" {
			found = true
		}
	}
	if !found {
		t.Fatal("task-context user message was dropped")
	}
}

func TestToPruneMessagesAdapter(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "ok", ToolUse: []types.ToolCall{{Name: "Bash"}}},
		{Role: "user", Content: "err", ToolResults: []types.ToolResult{{IsError: true}}},
	}
	pruned := toPruneMessages(msgs, time.Now())
	if len(pruned) != 3 {
		t.Fatalf("len = %d", len(pruned))
	}
	if !pruned[1].HasToolCall {
		t.Fatal("tool-use message should be flagged as tool call")
	}
	if !pruned[2].IsError {
		t.Fatal("error result should be flagged as error")
	}
}
