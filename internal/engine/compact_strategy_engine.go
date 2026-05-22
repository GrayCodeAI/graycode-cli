package engine

import (
	"context"

	"github.com/GrayCodeAI/hawk/internal/types"
)

type CompactStrategy interface {
	Name() string
	ShouldTrigger(msgs []types.EyrieMessage, tokenCount, threshold int) bool
	Compact(ctx context.Context, s *Session) (*CompactResult, error)
}

type StrategyRegistry struct {
	strategies []CompactStrategy
	config     CompactConfig
}

func NewStrategyRegistry(config CompactConfig) *StrategyRegistry {
	r := &StrategyRegistry{config: config}
	r.strategies = []CompactStrategy{
		&MicroCompactStrategy{},
		&SessionMemoryStrategy{},
		&SmartCompactStrategy{},
		&TruncateStrategy{},
	}
	return r
}

func (r *StrategyRegistry) SelectStrategy(msgs []types.EyrieMessage, tokenCount int) CompactStrategy {
	threshold := r.config.ContextWindowSize - r.config.AutoCompactBuffer - r.config.MaxOutputTokens
	for _, s := range r.strategies {
		if s.ShouldTrigger(msgs, tokenCount, threshold) {
			return s
		}
	}
	return &TruncateStrategy{}
}
