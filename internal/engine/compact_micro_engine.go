package engine

import (
	"context"
	"time"

	"github.com/GrayCodeAI/eyrie/client"

	"github.com/GrayCodeAI/hawk/internal/engine/compact"
)

type MicroCompactStrategy struct{}

func (s *MicroCompactStrategy) Name() string { return "micro" }

func (s *MicroCompactStrategy) ShouldTrigger(msgs []client.EyrieMessage, tokenCount, threshold int) bool {
	if tokenCount < threshold/2 {
		return false
	}
	compactableCount := 0
	for _, m := range msgs {
		if m.ToolResult != nil && compact.IsCompactableTool(compact.ToolNameForResult(m, msgs)) {
			compactableCount++
		}
	}
	if compactableCount < 5 {
		return false
	}
	return compact.HasTimeGap(msgs, 60*time.Minute)
}

func (s *MicroCompactStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	tokensBefore := EstimateTokens(sess.messages)
	result := compact.MicrocompactMessages(sess.messages, DefaultMicroCompactConfig())
	tokensAfter := EstimateTokens(result)

	return &CompactResult{
		Messages:     result,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Strategy:     "micro",
	}, nil
}
