package engine

import (
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
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
		s.LoadMessages([]types.GraycodeRouterMessage{
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

func TestPersistenceServiceSnapshotsAreDeepCopies(t *testing.T) {
	ps := NewPersistenceService(nil)
	ps.LoadMessages([]types.GraycodeRouterMessage{{
		Role:   "assistant",
		Images: []string{"data:image/png;base64,abc"},
		ToolUse: []types.ToolCall{{
			ID:   "call-1",
			Name: "Write",
			Arguments: map[string]interface{}{
				"path":   "file.txt",
				"nested": map[string]interface{}{"ok": true},
			},
		}},
	}})

	snapshot := ps.RawMessages()
	snapshot[0].Images[0] = "mutated"
	snapshot[0].ToolUse[0].Arguments["path"] = "evil.txt"
	snapshot[0].ToolUse[0].Arguments["nested"].(map[string]interface{})["ok"] = false

	got := ps.RawMessages()[0]
	if got.Images[0] != "data:image/png;base64,abc" {
		t.Fatalf("image mutation leaked into persistence: %q", got.Images[0])
	}
	if got.ToolUse[0].Arguments["path"] != "file.txt" {
		t.Fatalf("tool argument mutation leaked into persistence: %v", got.ToolUse[0].Arguments["path"])
	}
	if got.ToolUse[0].Arguments["nested"].(map[string]interface{})["ok"] != true {
		t.Fatal("nested tool argument mutation leaked into persistence")
	}
}

func TestPersistenceServiceSetRawMessagesCopiesInput(t *testing.T) {
	ps := NewPersistenceService(nil)
	input := []types.GraycodeRouterMessage{{
		Role: "assistant",
		ToolUse: []types.ToolCall{{
			Arguments: map[string]interface{}{"nested": map[string]interface{}{"value": "safe"}},
		}},
	}}
	ps.SetRawMessages(input)
	input[0].ToolUse[0].Arguments["nested"].(map[string]interface{})["value"] = "mutated"

	got := ps.RawMessages()[0].ToolUse[0].Arguments["nested"].(map[string]interface{})["value"]
	if got != "safe" {
		t.Fatalf("SetRawMessages retained caller alias: got %v", got)
	}
}

// TestPersistenceServiceRawMessagesView verifies the read-only view (M15):
// it reflects live transcript state without cloning, so per-turn reads are
// O(1). It is a view — the caller must not mutate or retain it.
func TestPersistenceServiceRawMessagesView(t *testing.T) {
	ps := NewPersistenceService(nil)
	ps.LoadMessages([]types.GraycodeRouterMessage{{Role: "user", Content: "a"}})

	view := ps.RawMessagesView()
	if len(view) != 1 || view[0].Content != "a" {
		t.Fatalf("unexpected view content: %+v", view)
	}

	// The view must see subsequent mutations (no stale clone).
	ps.AddUser("b")
	view2 := ps.RawMessagesView()
	if len(view2) != 2 {
		t.Fatalf("view did not reflect mutation: %d messages", len(view2))
	}

	// The view must not alias the deep-copy snapshot path.
	snap := ps.RawMessages()
	if &snap[0] == &view2[0] {
		t.Fatal("RawMessages must return a copy, not the view")
	}
}
