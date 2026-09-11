package engine

import (
	"context"
	"time"

	"github.com/GrayCodeAI/hawk/internal/relevanceprune"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// RelevancePruneStrategy prunes context by relevance: it scores older messages
// against the most recent user turn, keeps the highest-relevance groups up to a
// token budget, and always preserves the recent tail plus tool calls and errors.
// It slots into the compaction registry as a deterministic alternative to the
// LLM-summary (smart) and boundary-truncation strategies, and degrades to
// truncation when it cannot reduce the transcript. Port of OpenClaude's
// relevance-based context pruning.
type RelevancePruneStrategy struct {
	// TargetTokens is the budget to fit the kept history under. Zero means the
	// package default (5000).
	TargetTokens int
}

func (s *RelevancePruneStrategy) Name() string { return "relevance" }

func (s *RelevancePruneStrategy) ShouldTrigger(msgs []types.EyrieMessage, tokenCount, threshold int) bool {
	return tokenCount >= threshold && len(msgs) >= 20
}

func (s *RelevancePruneStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	tokensBefore := EstimateTokens(sess.Persistence().RawMessages())
	raw := sess.Persistence().RawMessages()

	taskContext := lastUserText(raw)
	pruned := relevanceprune.PruneByRelevance(
		toPruneMessages(raw, time.Now()),
		relevanceprune.Options{
			TargetTokens:   s.TargetTokens,
			TaskContext:    taskContext,
			PreserveRecent: relevanceprune.DefaultCompactTailTurns,
			PreserveTools:  true,
			PreserveErrors: true,
		},
	)

	out := toEyrieMessages(pruned)
	sess.Persistence().SetMessages(out)
	tokensAfter := EstimateTokens(sess.Persistence().RawMessages())
	return &CompactResult{
		Messages:     out,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Strategy:     "relevance",
	}, nil
}

// toPruneMessages adapts raw session messages to the relevance-prune shape.
func toPruneMessages(raw []types.EyrieMessage, now time.Time) []relevanceprune.Message {
	out := make([]relevanceprune.Message, len(raw))
	for i, m := range raw {
		out[i] = relevanceprune.Message{
			Role:        m.Role,
			Content:     m.Content,
			Timestamp:   now,
			HasToolCall: len(m.ToolUse) > 0 || len(m.ToolResults) > 0,
			IsError:     hasErrorResult(m),
		}
	}
	return out
}

func toEyrieMessages(msgs []relevanceprune.Message) []types.EyrieMessage {
	out := make([]types.EyrieMessage, len(msgs))
	for i, m := range msgs {
		out[i] = types.EyrieMessage{Role: m.Role, Content: m.Content}
	}
	return out
}

func hasErrorResult(m types.EyrieMessage) bool {
	for _, tr := range m.ToolResults {
		if tr.IsError {
			return true
		}
	}
	return false
}

func lastUserText(raw []types.EyrieMessage) string {
	for i := len(raw) - 1; i >= 0; i-- {
		if raw[i].Role == "user" {
			return raw[i].Content
		}
	}
	return ""
}
