package engine

import (
	"context"
	"log"
	"sync"

	"github.com/GrayCodeAI/eyrie/client"
)

// AutoCompactor orchestrates compaction with circuit breaker protection.
type AutoCompactor struct {
	mu                 sync.Mutex
	registry           *StrategyRegistry
	config             CompactConfig
	consecutiveFailures int
	lastStrategy       string
}

// NewAutoCompactor creates an auto-compactor with the given config.
func NewAutoCompactor(config CompactConfig) *AutoCompactor {
	return &AutoCompactor{
		registry: NewStrategyRegistry(config),
		config:   config,
	}
}

// GetAutoCompactThreshold returns the token count at which auto-compaction triggers.
func (ac *AutoCompactor) GetAutoCompactThreshold() int {
	return ac.config.ContextWindowSize - ac.config.AutoCompactBuffer - ac.config.MaxOutputTokens
}

// ShouldAutoCompact determines if compaction is needed based on current state.
func (ac *AutoCompactor) ShouldAutoCompact(sess *Session) bool {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if !ac.config.AutoEnabled {
		return false
	}

	if ac.consecutiveFailures >= ac.config.MaxFailures {
		log.Printf("Auto-compact paused after %d consecutive failures.", ac.consecutiveFailures)
		return false
	}

	tokenCount := EstimateTokens(sess.messages)
	threshold := ac.GetAutoCompactThreshold()
	return tokenCount >= threshold
}

// AutoCompactIfNeeded runs compaction if threshold is met.
// Returns the strategy name used and whether compaction occurred.
func (ac *AutoCompactor) AutoCompactIfNeeded(ctx context.Context, sess *Session) (string, bool) {
	if !ac.ShouldAutoCompact(sess) {
		return "", false
	}

	strategy, err := ac.RunCompaction(ctx, sess)
	if err != nil {
		ac.mu.Lock()
		ac.consecutiveFailures++
		ac.mu.Unlock()
		sess.log.Warn("auto-compact failed", map[string]interface{}{
			"error":    err.Error(),
			"failures": ac.consecutiveFailures,
		})
		sess.compact()
		return "truncate_fallback", true
	}

	ac.mu.Lock()
	ac.consecutiveFailures = 0
	ac.mu.Unlock()
	return strategy, true
}

// RunCompaction selects and executes the best compaction strategy.
func (ac *AutoCompactor) RunCompaction(ctx context.Context, sess *Session) (string, error) {
	tokenCount := EstimateTokens(sess.messages)
	strategy := ac.registry.SelectStrategy(sess.messages, tokenCount)

	sess.log.Info("running compaction", map[string]interface{}{
		"strategy": strategy.Name(),
		"tokens":   tokenCount,
	})

	result, err := strategy.Compact(ctx, sess)
	if err != nil {
		return strategy.Name(), err
	}

	sess.messages = result.Messages
	ac.mu.Lock()
	ac.lastStrategy = result.Strategy
	ac.mu.Unlock()

	sess.log.Info("compaction complete", map[string]interface{}{
		"strategy":      result.Strategy,
		"tokens_before": result.TokensBefore,
		"tokens_after":  result.TokensAfter,
		"reduction":     result.TokensBefore - result.TokensAfter,
	})

	return result.Strategy, nil
}

// LastStrategy returns the name of the last strategy used.
func (ac *AutoCompactor) LastStrategy() string {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.lastStrategy
}

// ResetFailures resets the circuit breaker failure count.
func (ac *AutoCompactor) ResetFailures() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.consecutiveFailures = 0
}

// SmartCompactStrategy uses LLM to generate a conversation summary.
type SmartCompactStrategy struct{}

func (s *SmartCompactStrategy) Name() string { return "smart" }

func (s *SmartCompactStrategy) ShouldTrigger(msgs []client.EyrieMessage, tokenCount, threshold int) bool {
	return tokenCount >= threshold && len(msgs) > 20
}

func (s *SmartCompactStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	tokensBefore := EstimateTokens(sess.messages)
	sess.smartCompact()
	tokensAfter := EstimateTokens(sess.messages)

	return &CompactResult{
		Messages:     sess.messages,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Strategy:     "smart",
	}, nil
}

// TruncateStrategy is the fallback that does boundary-aware truncation.
type TruncateStrategy struct{}

func (s *TruncateStrategy) Name() string { return "truncate" }

func (s *TruncateStrategy) ShouldTrigger(_ []client.EyrieMessage, tokenCount, threshold int) bool {
	return tokenCount >= threshold
}

func (s *TruncateStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	tokensBefore := EstimateTokens(sess.messages)
	sess.compact()
	tokensAfter := EstimateTokens(sess.messages)

	return &CompactResult{
		Messages:     sess.messages,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Strategy:     "truncate",
	}, nil
}
