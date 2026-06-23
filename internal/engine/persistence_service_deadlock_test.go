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
