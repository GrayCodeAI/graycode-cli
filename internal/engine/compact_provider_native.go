package engine

import (
	"context"
	"fmt"

	"github.com/GrayCodeAI/eyrie/runtime"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// ProviderNativeCompactStrategy delegates provider-specific compaction to Eyrie.
type ProviderNativeCompactStrategy struct{}

func (s *ProviderNativeCompactStrategy) Name() string { return "provider_native" }

func (s *ProviderNativeCompactStrategy) ShouldTrigger(msgs []types.EyrieMessage, tokenCount, threshold int) bool {
	return tokenCount >= threshold && len(msgs) >= 8
}

func (s *ProviderNativeCompactStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	if sess == nil {
		return nil, fmt.Errorf("no session")
	}
	if !sess.supportsProviderNativeCompaction(ctx) {
		return nil, fmt.Errorf("provider native compaction not available")
	}
	tokensBefore := EstimateTokens(sess.messages)
	result, err := runtime.CompactNativeConversation(ctx, runtime.NativeCompactionOpts{
		Provider:        sess.provider,
		Model:           sess.model,
		Messages:        types.ToClientMessages(sess.messages),
		ContextWindow:   sess.ContextWindowSize(),
		ThresholdPct:    sess.compactThresholdPct(),
		MaxOutputTokens: 8192,
	})
	if err != nil {
		return nil, err
	}

	keepEnd := 6
	if keepEnd > len(sess.messages) {
		keepEnd = len(sess.messages)
	}
	tail := append([]types.EyrieMessage(nil), sess.messages[len(sess.messages)-keepEnd:]...)
	newMessages := append([]types.EyrieMessage{{Role: "user", Content: FormatCompactSummary(result.Summary)}}, tail...)
	compactResult := &CompactResult{
		Messages: newMessages, TokensBefore: tokensBefore, TokensAfter: EstimateTokens(newMessages), Strategy: s.Name(),
	}
	sess.messages = compactResult.Messages
	return compactResult, nil
}

func (s *Session) supportsProviderNativeCompaction(ctx context.Context) bool {
	if s == nil {
		return false
	}
	return runtime.SupportsNativeCompaction(ctx, s.provider, s.model)
}
