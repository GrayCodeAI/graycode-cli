package engine

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// AutoCompactor orchestrates compaction with circuit breaker protection.
type AutoCompactor struct {
	mu                  sync.Mutex
	registry            *StrategyRegistry
	config              CompactConfig
	consecutiveFailures int
	lastStrategy        string
}

// NewAutoCompactor creates an auto-compactor with the given config.
func NewAutoCompactor(config CompactConfig) *AutoCompactor {
	return &AutoCompactor{
		registry: NewStrategyRegistry(config),
		config:   config,
	}
}

// Configure updates compaction settings and rebuilds the strategy registry.
func (ac *AutoCompactor) Configure(config CompactConfig) {
	if ac == nil {
		return
	}
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.config = config
	ac.registry = NewStrategyRegistry(config)
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

	tokenCount := EstimateTokens(sess.Persistence().RawMessages())
	threshold := ac.GetAutoCompactThreshold()
	return tokenCount >= threshold
}

// AutoCompactIfNeeded runs compaction if threshold is met.
// Returns the strategy name used and whether compaction occurred.
func (ac *AutoCompactor) AutoCompactIfNeeded(ctx context.Context, sess *Session) (string, bool) {
	if !ac.ShouldAutoCompact(sess) {
		return "", false
	}

	tokensBefore := EstimateTokens(sess.Persistence().RawMessages())
	strategy, err := ac.RunCompaction(ctx, sess)
	if err != nil {
		ac.mu.Lock()
		ac.consecutiveFailures++
		ac.mu.Unlock()
		sess.Logger().Warn("auto-compact failed", map[string]any{
			"error":    err.Error(),
			"failures": ac.consecutiveFailures,
		})
		sess.compact(ctx)
		tokensAfter := EstimateTokens(sess.Persistence().RawMessages())
		sess.recordCompaction("truncate_fallback", tokensBefore, tokensAfter, false)
		return "truncate_fallback", true
	}

	tokensAfter := EstimateTokens(sess.Persistence().RawMessages())
	if tokensAfter >= tokensBefore {
		// Strategy ran but produced no reduction (e.g. LLM summary was
		// rejected, or messages were not reduced); fall back to truncation.
		ac.mu.Lock()
		ac.consecutiveFailures++
		ac.mu.Unlock()
		sess.Logger().Warn("auto-compact produced no reduction, falling back to truncation", map[string]any{
			"tokens_before": tokensBefore,
			"tokens_after":  tokensAfter,
		})
		sess.compact(ctx)
		tokensAfter = EstimateTokens(sess.Persistence().RawMessages())
		sess.recordCompaction("truncate_fallback", tokensBefore, tokensAfter, false)
		return "truncate_fallback", true
	}

	ac.mu.Lock()
	ac.consecutiveFailures = 0
	ac.mu.Unlock()
	sess.recordCompaction(strategy, tokensBefore, tokensAfter, false)
	return strategy, true
}

// RunCompaction selects and executes the best compaction strategy.
func (ac *AutoCompactor) RunCompaction(ctx context.Context, sess *Session) (string, error) {
	// Snapshot the registry under the mutex: Configure() swaps the field, and
	// reading it lock-free races with that write.
	ac.mu.Lock()
	registry := ac.registry
	ac.mu.Unlock()

	messages := sess.Persistence().RawMessages()
	tokenCount := EstimateTokens(messages)
	strategy := registry.SelectStrategy(sess, messages, tokenCount)
	if strategy == nil {
		return "", errors.New("no compaction strategy available")
	}

	sess.Logger().Info("running compaction", map[string]any{
		"strategy": strategy.Name(),
		"tokens":   tokenCount,
	})

	result, err := strategy.Compact(ctx, sess)
	if err != nil {
		return strategy.Name(), err
	}

	sess.Persistence().SetMessages(result.Messages)
	ac.mu.Lock()
	ac.lastStrategy = result.Strategy
	ac.mu.Unlock()

	sess.Logger().Info("compaction complete", map[string]any{
		"strategy":      result.Strategy,
		"tokens_before": result.TokensBefore,
		"tokens_after":  result.TokensAfter,
		"reduction":     result.TokensBefore - result.TokensAfter,
	})
	// recordCompaction is called by AutoCompactIfNeeded / CompactConversation callers

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

func (s *SmartCompactStrategy) ShouldTrigger(msgs []types.EyrieMessage, tokenCount, threshold int) bool {
	return tokenCount >= threshold && len(msgs) > 20
}

func (s *SmartCompactStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	tokensBefore := EstimateTokens(sess.Persistence().RawMessages())
	sess.smartCompact(ctx)
	messages := sess.Persistence().RawMessages()
	tokensAfter := EstimateTokens(messages)

	return &CompactResult{
		Messages:     messages,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Strategy:     "smart",
	}, nil
}

// TruncateStrategy is the fallback that does boundary-aware truncation.
type TruncateStrategy struct{}

func (s *TruncateStrategy) Name() string { return "truncate" }

func (s *TruncateStrategy) ShouldTrigger(_ []types.EyrieMessage, tokenCount, threshold int) bool {
	return tokenCount >= threshold
}

func (s *TruncateStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	tokensBefore := EstimateTokens(sess.Persistence().RawMessages())
	sess.compact(ctx)
	messages := sess.Persistence().RawMessages()
	tokensAfter := EstimateTokens(messages)

	return &CompactResult{
		Messages:     messages,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Strategy:     "truncate",
	}, nil
}
