package engine

import (
	"context"
	"fmt"

	"github.com/GrayCodeAI/hawk-core-contracts/llm"
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
	if sess.ChatLLM() == nil {
		return nil, fmt.Errorf("no session client")
	}
	compactor, ok := sess.ChatLLM().Client().(nativeCompactionCapable)
	if !ok || !compactor.NativeCompaction(ctx, sess.ChatLLM().Provider(), sess.ChatLLM().Model()) {
		return nil, fmt.Errorf("provider native compaction not available")
	}

	messagesBefore := sess.Persistence().RawMessages()
	tokensBefore := EstimateTokens(messagesBefore)
	summary, err := compactor.CompactNative(ctx, llm.NativeCompactionRequest{
		Provider:        sess.ChatLLM().Provider(),
		Model:           sess.ChatLLM().Model(),
		Messages:        messagesBefore,
		ContextWindow:   sess.ContextWindowSize(),
		ThresholdPct:    sess.compactThresholdPct(),
		MaxOutputTokens: 8192,
	})
	if err != nil {
		return nil, err
	}

	keepEnd := 6
	if keepEnd > len(messagesBefore) {
		keepEnd = len(messagesBefore)
	}
	tail := append([]types.EyrieMessage(nil), messagesBefore[len(messagesBefore)-keepEnd:]...)
	messages := append([]types.EyrieMessage{{Role: "user", Content: FormatCompactSummary(summary)}}, tail...)
	compact := &CompactResult{
		Messages:     messages,
		TokensBefore: tokensBefore,
		TokensAfter:  EstimateTokens(messages),
		Strategy:     "provider_native",
	}
	sess.Persistence().SetMessages(compact.Messages)
	return compact, nil
}

func (s *Session) supportsNativeCompaction() bool {
	if s == nil || s.ChatLLM() == nil {
		return false
	}
	return clientNativeCompaction(s.ChatLLM().Client(), context.Background(), s.ChatLLM().Provider(), s.ChatLLM().Model())
}
