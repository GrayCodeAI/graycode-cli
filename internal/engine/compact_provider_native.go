package engine

import (
	"context"
	"fmt"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// ProviderNativeCompactStrategy delegates provider-specific compaction to
// Eyrie. Hawk owns when conversation state is compacted and how the resulting
// summary is inserted; Eyrie owns credentials and provider transport details.
type ProviderNativeCompactStrategy struct{}

func (s *ProviderNativeCompactStrategy) Name() string { return "provider_native" }

func (s *ProviderNativeCompactStrategy) ShouldTrigger(msgs []types.EyrieMessage, tokenCount, threshold int) bool {
	return tokenCount >= threshold && len(msgs) >= 8
}

func (s *ProviderNativeCompactStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	if sess == nil {
		return nil, fmt.Errorf("no session")
	}
	client, ok := sess.engineFacadeClient()
	if !ok || !client.engine.SupportsNativeCompaction(ctx, sess.provider, sess.model) {
		return nil, fmt.Errorf("provider native compaction not available")
	}

	tokensBefore := EstimateTokens(sess.messages)
	summary, err := client.engine.CompactNative(ctx, eyrieengine.NativeCompactionRequest{
		Provider:        sess.provider,
		Model:           sess.model,
		Messages:        toEngineMessages(sess.messages),
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
	messages := append([]types.EyrieMessage{{Role: "user", Content: FormatCompactSummary(summary)}}, tail...)
	compact := &CompactResult{
		Messages:     messages,
		TokensBefore: tokensBefore,
		TokensAfter:  EstimateTokens(messages),
		Strategy:     "provider_native",
	}
	sess.messages = compact.Messages
	return compact, nil
}

func (s *Session) engineFacadeClient() (*eyrieEngineClient, bool) {
	if s == nil || s.ChatLLM() == nil {
		return nil, false
	}
	client, ok := s.ChatLLM().Client().(*eyrieEngineClient)
	return client, ok && client != nil && client.engine != nil
}

func (s *Session) supportsAnthropicNativeCompaction() bool {
	client, ok := s.engineFacadeClient()
	return ok && client.engine.SupportsNativeCompaction(context.Background(), s.provider, s.model)
}
