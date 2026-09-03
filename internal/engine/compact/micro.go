package compact

import (
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

type MicroCompactConfig struct {
	CompactableTools map[string]bool
	TimeGapMins      float64
	KeepRecent       int
}

func DefaultMicroCompactConfig() MicroCompactConfig {
	return MicroCompactConfig{
		CompactableTools: compactableTools,
		TimeGapMins:      60,
		KeepRecent:       3,
	}
}

type resultInfo struct {
	index    int
	toolName string
}

func MicrocompactMessages(msgs []types.EyrieMessage, cfg MicroCompactConfig) []types.EyrieMessage {
	var compactableResults []resultInfo
	for i, m := range msgs {
		if len(m.ToolResults) == 0 {
			continue
		}
		toolName := ToolNameForResult(m, msgs)
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

	result := make([]types.EyrieMessage, len(msgs))
	copy(result, msgs)
	for idx := range clearSet {
		oldResults := result[idx].ToolResults
		newResults := make([]types.ToolResult, len(oldResults))
		for j, tr := range oldResults {
			newResults[j] = types.ToolResult{
				ToolUseID: tr.ToolUseID,
				Content:   "[Old tool result content cleared]",
				IsError:   tr.IsError,
			}
		}
		result[idx] = types.EyrieMessage{
			Role:        result[idx].Role,
			ToolResults: newResults,
		}
	}

	return result
}

func ToolNameForResult(m types.EyrieMessage, msgs []types.EyrieMessage) string {
	if len(m.ToolResults) == 0 {
		return ""
	}
	targetID := m.ToolResults[0].ToolUseID
	for i := len(msgs) - 1; i >= 0; i-- {
		for _, tc := range msgs[i].ToolUse {
			if tc.ID == targetID {
				return tc.Name
			}
		}
	}
	return ""
}

func HasTimeGap(msgs []types.EyrieMessage, threshold time.Duration) bool {
	lastTextIdx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if HasTextContent(msgs[i]) && msgs[i].Role == "assistant" {
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
