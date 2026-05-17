package engine

import (
	"context"
	"time"

	"github.com/GrayCodeAI/eyrie/client"
)

// MicroCompactStrategy clears old tool result content while preserving message structure.
type MicroCompactStrategy struct{}

func (s *MicroCompactStrategy) Name() string { return "micro" }

// ShouldTrigger fires when there are enough messages with compactable tool results
// and sufficient time has passed since the last assistant message (cache is cold).
func (s *MicroCompactStrategy) ShouldTrigger(msgs []client.EyrieMessage, tokenCount, threshold int) bool {
	if tokenCount < threshold/2 {
		return false
	}
	compactableCount := 0
	for _, m := range msgs {
		if m.ToolResult != nil && isCompactableTool(toolNameForResult(m, msgs)) {
			compactableCount++
		}
	}
	if compactableCount < 5 {
		return false
	}
	return hasTimeGap(msgs, 60*time.Minute)
}

func (s *MicroCompactStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	tokensBefore := EstimateTokens(sess.messages)
	result := microcompactMessages(sess.messages, DefaultMicroCompactConfig())
	tokensAfter := EstimateTokens(result)

	return &CompactResult{
		Messages:     result,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Strategy:     "micro",
	}, nil
}

// MicroCompactConfig controls micro-compaction behavior.
type MicroCompactConfig struct {
	CompactableTools map[string]bool
	TimeGapMins      float64
	KeepRecent       int
}

// DefaultMicroCompactConfig returns the default micro-compaction settings.
func DefaultMicroCompactConfig() MicroCompactConfig {
	return MicroCompactConfig{
		CompactableTools: compactableTools,
		TimeGapMins:      60,
		KeepRecent:       3,
	}
}

// microcompactMessages clears old tool result content from compactable tools,
// keeping the most recent N results intact.
func microcompactMessages(msgs []client.EyrieMessage, cfg MicroCompactConfig) []client.EyrieMessage {
	type resultInfo struct {
		index    int
		toolName string
	}

	var compactableResults []resultInfo
	for i, m := range msgs {
		if m.ToolResult == nil {
			continue
		}
		toolName := toolNameForResult(m, msgs)
		if cfg.CompactableTools[toolName] {
			compactableResults = append(compactableResults, resultInfo{index: i, toolName: toolName})
		}
	}

	if len(compactableResults) <= cfg.KeepRecent {
		return msgs
	}

	toClear := len(compactableResults) - cfg.KeepRecent
	clearSet := make(map[int]bool, toClear)
	for i := 0; i < toClear; i++ {
		clearSet[compactableResults[i].index] = true
	}

	result := make([]client.EyrieMessage, len(msgs))
	copy(result, msgs)
	for idx := range clearSet {
		result[idx] = client.EyrieMessage{
			Role: result[idx].Role,
			ToolResult: &client.ToolResult{
				ToolUseID: result[idx].ToolResult.ToolUseID,
				Content:   "[Old tool result content cleared]",
				IsError:   result[idx].ToolResult.IsError,
			},
		}
	}

	return result
}

// toolNameForResult finds the tool name for a tool_result message by scanning
// backward for the matching tool_use.
func toolNameForResult(m client.EyrieMessage, msgs []client.EyrieMessage) string {
	if m.ToolResult == nil {
		return ""
	}
	targetID := m.ToolResult.ToolUseID
	for i := len(msgs) - 1; i >= 0; i-- {
		for _, tc := range msgs[i].ToolUse {
			if tc.ID == targetID {
				return tc.Name
			}
		}
	}
	return ""
}

// hasTimeGap checks if there's a gap >= threshold since the last assistant message,
// indicating the cache is likely cold.
func hasTimeGap(msgs []client.EyrieMessage, threshold time.Duration) bool {
	// In the absence of timestamps on messages, use message count as a proxy.
	// More than 20 messages since last meaningful text exchange suggests a cold cache.
	lastTextIdx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if hasTextContent(msgs[i]) && msgs[i].Role == "assistant" {
			lastTextIdx = i
			break
		}
	}
	if lastTextIdx < 0 {
		return false
	}
	messagesSinceText := len(msgs) - lastTextIdx - 1
	return messagesSinceText > 20 || threshold == 0
}
