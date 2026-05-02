package engine

import (
	"context"

	"github.com/GrayCodeAI/eyrie/client"
)

// APICompactStrategy uses API-level context edits to clear thinking blocks
// and old tool inputs without mutating local message content.
type APICompactStrategy struct{}

func (s *APICompactStrategy) Name() string { return "api_compact" }

func (s *APICompactStrategy) ShouldTrigger(msgs []client.EyrieMessage, tokenCount, threshold int) bool {
	if tokenCount < 180000 {
		return false
	}
	return countClearableToolResults(msgs) > 5
}

func (s *APICompactStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	tokensBefore := EstimateTokens(sess.messages)
	result := apiCompactMessages(sess.messages, DefaultAPICompactConfig())
	tokensAfter := EstimateTokens(result)

	return &CompactResult{
		Messages:     result,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Strategy:     "api_compact",
	}, nil
}

// APICompactConfig controls API-level compaction.
type APICompactConfig struct {
	TriggerTokens  int
	KeepTargetTokens int
	ClearToolInputs  bool
	ClearThinking    bool
	PreserveMutating bool
}

// DefaultAPICompactConfig returns defaults matching the archive.
func DefaultAPICompactConfig() APICompactConfig {
	return APICompactConfig{
		TriggerTokens:    180000,
		KeepTargetTokens: 40000,
		ClearToolInputs:  true,
		ClearThinking:    true,
		PreserveMutating: true,
	}
}

// mutatingTools are tools whose inputs should not be cleared (they modify state).
var mutatingTools = map[string]bool{
	"Edit":         true,
	"Write":        true,
	"NotebookEdit": true,
}

// apiCompactMessages clears thinking content and old tool inputs/results
// for non-mutating tools when context is very large.
func apiCompactMessages(msgs []client.EyrieMessage, cfg APICompactConfig) []client.EyrieMessage {
	totalTokens := EstimateTokens(msgs)
	if totalTokens < cfg.TriggerTokens {
		return msgs
	}

	tokensToFree := totalTokens - cfg.KeepTargetTokens
	if tokensToFree <= 0 {
		return msgs
	}

	result := make([]client.EyrieMessage, len(msgs))
	copy(result, msgs)

	freed := 0
	keepFromEnd := len(msgs) / 4

	for i := 0; i < len(result)-keepFromEnd && freed < tokensToFree; i++ {
		m := &result[i]

		if cfg.ClearThinking && m.Role == "assistant" && isThinkingMessage(*m) {
			before := estimateMessageTokens(*m)
			m.Content = "[Thinking content cleared]"
			freed += before - estimateMessageTokens(*m)
			continue
		}

		if cfg.ClearToolInputs && m.Role == "assistant" && len(m.ToolUse) > 0 {
			for j := range m.ToolUse {
				if cfg.PreserveMutating && mutatingTools[m.ToolUse[j].Name] {
					continue
				}
				before := 0
				for _, v := range m.ToolUse[j].Arguments {
					if s, ok := v.(string); ok {
						before += len(s) / 4
					}
				}
				if before > 100 {
					m.ToolUse[j].Arguments = map[string]interface{}{
						"_cleared": true,
					}
					freed += before
				}
			}
		}

		if m.ToolResult != nil && m.ToolResult.Content != "[Old tool result content cleared]" {
			toolName := toolNameForResult(*m, result)
			if !mutatingTools[toolName] {
				before := len(m.ToolResult.Content) / 4
				if before > 100 {
					m.ToolResult = &client.ToolResult{
						ToolUseID: m.ToolResult.ToolUseID,
						Content:   "[Old tool result content cleared]",
						IsError:   m.ToolResult.IsError,
					}
					freed += before
				}
			}
		}
	}

	return result
}

func countClearableToolResults(msgs []client.EyrieMessage) int {
	count := 0
	for _, m := range msgs {
		if m.ToolResult != nil && m.ToolResult.Content != "[Old tool result content cleared]" {
			toolName := toolNameForResult(m, msgs)
			if !mutatingTools[toolName] {
				count++
			}
		}
	}
	return count
}

func isThinkingMessage(m client.EyrieMessage) bool {
	return len(m.Content) > 0 && m.Content[0] == '<' && len(m.ToolUse) == 0
}
