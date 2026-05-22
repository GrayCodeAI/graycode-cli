package engine

import (
	"context"

	"github.com/GrayCodeAI/hawk/internal/types"

	"github.com/GrayCodeAI/hawk/internal/engine/compact"
)

type APICompactStrategy struct{}

func (s *APICompactStrategy) Name() string { return "api_compact" }

func (s *APICompactStrategy) ShouldTrigger(msgs []types.EyrieMessage, tokenCount, threshold int) bool {
	if tokenCount < 180000 {
		return false
	}
	return compact.CountClearableToolResults(msgs) > 5
}

func (s *APICompactStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	tokensBefore := EstimateTokens(sess.messages)
	result := compact.APICompactMessages(sess.messages, DefaultAPICompactConfig())
	tokensAfter := EstimateTokens(result)

	return &CompactResult{
		Messages:     result,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Strategy:     "api_compact",
	}, nil
}
