package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestSessionMemoryStrategy_ShouldTrigger(t *testing.T) {
	s := &SessionMemoryStrategy{}
	msgs := makeMessages(50)
	if s.ShouldTrigger(msgs, 200000, 150000) {
		t.Error("should not trigger without memory file")
	}
}

func TestAutoCompactor_CircuitBreaker(t *testing.T) {
	cfg := DefaultCompactConfig()
	cfg.MaxFailures = 2
	cfg.ContextWindowSize = 1000
	cfg.AutoCompactBuffer = 100
	cfg.MaxOutputTokens = 100

	ac := NewAutoCompactor(cfg)
	now := time.Now()
	ac.breaker.RecordFailure(now)
	ac.breaker.RecordFailure(now)

	sess := NewSessionWithClient(NewMockClientForTest(), "test", "test-model", "", nil, false)
	sess.Persistence().SetRawMessages(makeMessages(200))

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
	strategy := registry.SelectStrategy(nil, msgs, cfg.ContextWindowSize)
	if strategy.Name() != "truncate" {
		t.Errorf("expected truncate for high token count with few messages, got %s", strategy.Name())
	}
}

func TestTruncateStrategy(t *testing.T) {
	sess := NewSessionWithClient(NewMockClientForTest(), "test", "test-model", "", nil, false)
	sess.Persistence().SetRawMessages(makeMessages(100))

	s := &TruncateStrategy{}
	result, err := s.Compact(context.Background(), sess)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Strategy != "truncate" {
		t.Errorf("expected strategy 'truncate', got %q", result.Strategy)
	}
	if len(sess.Persistence().RawMessages()) >= 100 {
		t.Error("messages should have been reduced")
	}
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"short string passes through", "hello", 10, "hello"},
		{"ascii truncation", "hello world", 5, "hello..."},
		{"exact length passes through", "hello", 5, "hello"},
		{"empty string", "", 5, ""},
		{"non-positive max returns input", "hello", 0, "hello"},
		{"multi-byte runes not split", "héllo wörld", 6, "héllo ..."},
		{"emoji not split", "🚀🚀🚀", 2, "🚀🚀..."},
		{"ascii bytes shorter than max but more runes", "éééé", 4, "éééé"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateRunes(tt.in, tt.max); got != tt.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}

func TestTurnTokenBudget(t *testing.T) {
	if got := turnTokenBudget(nil); got != 0 {
		t.Errorf("turnTokenBudget(nil) = %d, want 0", got)
	}

	small := []types.EyrieMessage{{Role: "user", Content: "short"}}
	if got := turnTokenBudget(small); got != 2000 {
		t.Errorf("turnTokenBudget(small) = %d, want minimum floor 2000", got)
	}

	big := make([]types.EyrieMessage, 10)
	for i := range big {
		big[i] = types.EyrieMessage{Role: "user", Content: strings.Repeat("x", 10000)}
	}
	avg := EstimateMessageTokens(big[0])
	if avg*3 <= 2000 {
		t.Fatalf("test messages too small to exceed the 2000 floor: avg=%d", avg)
	}
	if got, want := turnTokenBudget(big), avg*3; got != want {
		t.Errorf("turnTokenBudget(big) = %d, want %d (3x average)", got, want)
	}
}

func makeMessages(n int) []types.EyrieMessage {
	msgs := make([]types.EyrieMessage, n)
	for i := range msgs {
		if i%2 == 0 {
			msgs[i] = types.EyrieMessage{Role: "user", Content: strings.Repeat("message ", 50)}
		} else {
			msgs[i] = types.EyrieMessage{Role: "assistant", Content: strings.Repeat("response ", 50)}
		}
	}
	return msgs
}
