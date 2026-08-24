package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/types"
)

func TestSynthesisForExhaustion(t *testing.T) {
	sess := NewSessionWithClient(NewMockClientForTest(), "test", "test-model", "", nil, false)
	sess.Persistence().SetRawMessages([]types.EyrieMessage{
		{Role: "user", Content: "fix the bug"},
		{Role: "assistant", Content: "I found it"},
	})
	out := sess.SynthesisForExhaustion(context.Background(), "turn limit reached")
	if strings.TrimSpace(out) != "mock test response" {
		t.Fatalf("got %q, want mock test response", out)
	}
}

func TestSynthesisForExhaustionNoSession(t *testing.T) {
	var nilSess *Session
	if out := nilSess.SynthesisForExhaustion(context.Background(), "x"); out != "" {
		t.Fatalf("nil session should return empty, got %q", out)
	}
}

func TestSynthesisForExhaustionNoMessages(t *testing.T) {
	sess := NewSessionWithClient(NewMockClientForTest(), "test", "test-model", "", nil, false)
	if out := sess.SynthesisForExhaustion(context.Background(), "x"); out != "" {
		t.Fatalf("empty conversation should return empty, got %q", out)
	}
}

func TestSynthesisForExhaustionCancelled(t *testing.T) {
	sess := NewSessionWithClient(NewMockClientForTest(), "test", "test-model", "", nil, false)
	sess.Persistence().SetRawMessages([]types.EyrieMessage{{Role: "user", Content: "hi"}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if out := sess.SynthesisForExhaustion(ctx, "x"); out != "" {
		t.Fatalf("cancelled ctx should return empty, got %q", out)
	}
}

func TestEmitExhaustionFallsBack(t *testing.T) {
	// A session with no LLM should emit the static fallback message.
	var nilSess *Session
	ch := make(chan StreamEvent, 2)
	nilSess.emitExhaustion(context.Background(), ch, "reason")
	ev := <-ch
	if ev.Type != "content" || !strings.Contains(ev.Content, "Limit reached") {
		t.Fatalf("expected fallback content, got %+v", ev)
	}
	ev2 := <-ch
	if ev2.Type != "done" {
		t.Fatalf("expected done event, got %+v", ev2)
	}
}
