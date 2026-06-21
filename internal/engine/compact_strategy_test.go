package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/observability/metrics"
	"github.com/GrayCodeAI/hawk/internal/types"
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
	strategy := registry.SelectStrategy(nil, msgs, cfg.ContextWindowSize)
	if strategy.Name() != "truncate" {
		t.Errorf("expected truncate for high token count with few messages, got %s", strategy.Name())
	}
}

func TestTruncateStrategy(t *testing.T) {
	sess := &Session{
		messages: makeMessages(100),
		log:      newTestLogger(),
		metrics:  newTestMetrics(),
		client:   types.NewClient(&types.ClientConfig{Provider: "test"}),
	}

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

func newTestLogger() *logger.Logger {
	return logger.Default()
}

func newTestMetrics() *metrics.Registry {
	return metrics.NewRegistry()
}
