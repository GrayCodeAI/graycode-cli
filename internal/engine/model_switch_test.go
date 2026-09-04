package engine

import (
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// TestSetModelPreservesHistory verifies that a mid-session model switch re-routes
// subsequent requests to the new model WITHOUT dropping conversation history.
// This is the invariant the /model TUI command relies on.
func TestSetModelPreservesHistory(t *testing.T) {
	s := NewSession("anthropic", "claude-sonnet-4-6", "system", nil)

	history := []types.GraycodeRouterMessage{
		{Role: "user", Content: "what is 2+2?"},
		{Role: "assistant", Content: "4"},
		{Role: "user", Content: "and 3+3?"},
		{Role: "assistant", Content: "6"},
	}
	s.LoadMessages(history)

	if got := s.MessageCount(); got != len(history) {
		t.Fatalf("precondition: message count = %d, want %d", got, len(history))
	}

	// Switch the active model mid-session.
	s.SetModel("claude-opus-4-8")

	// The new model must be active for subsequent requests.
	if got := s.Model(); got != "claude-opus-4-8" {
		t.Errorf("model after switch = %q, want claude-opus-4-8", got)
	}

	// History must be fully preserved — same count and same content/order.
	after := s.Persistence().RawMessages()
	if len(after) != len(history) {
		t.Fatalf("history length after switch = %d, want %d (context was dropped!)", len(after), len(history))
	}
	for i := range history {
		if after[i].Role != history[i].Role || after[i].Content != history[i].Content {
			t.Errorf("message %d changed after model switch: got %+v, want %+v", i, after[i], history[i])
		}
	}

	// Cost tracking should reflect the new model so spend is attributed correctly.
	if s.Cost.Model != "claude-opus-4-8" {
		t.Errorf("Cost.Model = %q, want claude-opus-4-8", s.Cost.Model)
	}
}

// TestSetModelTrimsWhitespace verifies SetModel normalizes input, matching the
// /model command which passes a possibly-padded argument.
func TestSetModelTrimsWhitespace(t *testing.T) {
	s := NewSession("anthropic", "a", "sys", nil)
	s.SetModel("  claude-opus-4-8  ")
	if s.Model() != "claude-opus-4-8" {
		t.Errorf("model = %q, want trimmed claude-opus-4-8", s.Model())
	}
}
