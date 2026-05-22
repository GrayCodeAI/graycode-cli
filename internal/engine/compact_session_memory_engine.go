package engine

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/types"

	"github.com/GrayCodeAI/hawk/internal/engine/compact"
)

type SessionMemoryStrategy struct{}

func (s *SessionMemoryStrategy) Name() string { return "session_memory" }

func (s *SessionMemoryStrategy) ShouldTrigger(msgs []types.EyrieMessage, tokenCount, threshold int) bool {
	if tokenCount < threshold {
		return false
	}
	memFile := compact.SessionMemoryPath("")
	info, err := os.Stat(memFile)
	if err != nil || info.Size() < 100 {
		return false
	}
	return true
}

func (s *SessionMemoryStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	memContent, err := compact.ReadSessionMemory("")
	if err != nil {
		return nil, fmt.Errorf("reading session memory: %w", err)
	}
	if strings.TrimSpace(memContent) == "" {
		return nil, fmt.Errorf("session memory is empty")
	}

	tokensBefore := EstimateTokens(sess.messages)

	cfg := DefaultSessionMemoryConfig()
	keepIdx := compact.CalculateMessagesToKeepIndex(sess.messages, cfg)
	keepIdx = compact.AdjustIndexToPreserveAPIInvariants(sess.messages, keepIdx)

	if keepIdx >= len(sess.messages)-2 {
		return nil, fmt.Errorf("not enough messages to compact")
	}

	kept := sess.messages[keepIdx:]
	kept = compact.FilterCompactBoundaries(kept)

	result := make([]types.EyrieMessage, 0, len(kept)+2)
	result = append(result, types.EyrieMessage{
		Role:    "user",
		Content: "[Session memory summary]\n" + memContent + "\n\n[Continue from the recent messages below.]",
	})
	result = append(result, types.EyrieMessage{
		Role:    "assistant",
		Content: "Understood. I have the context from the session memory above. Continuing with the recent conversation.",
	})
	result = append(result, kept...)

	tokensAfter := EstimateTokens(result)

	return &CompactResult{
		Messages:     result,
		Summary:      memContent,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Strategy:     "session_memory",
	}, nil
}
