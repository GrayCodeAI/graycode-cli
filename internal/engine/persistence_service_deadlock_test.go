package engine

import (
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// TestPersistenceServiceNoRecursiveLock guards against the recursive-lock
// deadlocks that previously existed in Messages, MessageCount,
// RemoveLastExchange, and LoadMessages. Each held s.mu and then called an
// accessor (RawMessages/SetRawMessages) that tried to take the same lock
// again — RemoveLastExchange and LoadMessages would hang unconditionally.
func TestPersistenceServiceNoRecursiveLock(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		s := NewPersistenceService(nil)
		s.LoadMessages([]types.EyrieMessage{
			{Role: "user", Content: "a"},
			{Role: "assistant", Content: "b"},
		})
		if got := s.MessageCount(); got != 2 {
			t.Errorf("MessageCount = %d, want 2", got)
		}
		if got := len(s.Messages()); got != 2 {
			t.Errorf("Messages len = %d, want 2", got)
		}
		s.RemoveLastExchange()
		if got := s.MessageCount(); got != 0 {
			t.Errorf("after RemoveLastExchange MessageCount = %d, want 0", got)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("PersistenceService deadlocked (recursive lock acquisition)")
	}
}

func TestPersistenceService_MessageSnapshotsDoNotAlias(t *testing.T) {
	ps := NewPersistenceService(nil)
	input := []types.EyrieMessage{{
		Role:    "assistant",
		Images:  []string{"before.png"},
		ToolUse: []types.ToolCall{{Name: "Read", Arguments: map[string]interface{}{"path": "before.txt"}}},
	}}
	ps.SetRawMessages(input)
	input[0].Images[0] = "input-mutated.png"
	input[0].ToolUse[0].Arguments["path"] = "input-mutated.txt"

	snapshot := ps.RawMessages()
	snapshot[0].Images[0] = "snapshot-mutated.png"
	snapshot[0].ToolUse[0].Arguments["path"] = "snapshot-mutated.txt"

	got := ps.Messages()
	if got[0].Images[0] != "before.png" {
		t.Fatalf("persisted image aliased caller snapshot: %q", got[0].Images[0])
	}
	if got[0].ToolUse[0].Arguments["path"] != "before.txt" {
		t.Fatalf("persisted tool arguments aliased caller snapshot: %v", got[0].ToolUse[0].Arguments)
	}
}
