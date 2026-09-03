package compact

import (
	"github.com/GrayCodeAI/graycode-cli/internal/types"

	"github.com/GrayCodeAI/graycode-cli/internal/engine/token"
)

type APICompactConfig struct {
	TriggerTokens    int
	KeepTargetTokens int
	ClearToolInputs  bool
	ClearThinking    bool
	PreserveMutating bool
}

func DefaultAPICompactConfig() APICompactConfig {
	return APICompactConfig{
		TriggerTokens:    180000,
		KeepTargetTokens: 40000,
		ClearToolInputs:  true,
		ClearThinking:    true,
		PreserveMutating: true,
	}
}

var mutatingTools = map[string]bool{
	"Edit":         true,
	"Write":        true,
	"NotebookEdit": true,
}

func APICompactMessages(msgs []types.EyrieMessage, cfg APICompactConfig) []types.EyrieMessage {
	totalTokens := token.EstimateTokens(msgs)
	if totalTokens < cfg.TriggerTokens {
		return msgs
	}

	tokensToFree := totalTokens - cfg.KeepTargetTokens
	if tokensToFree <= 0 {
		return msgs
	}

	result := make([]types.EyrieMessage, len(msgs))
	copy(result, msgs)

	freed := 0
	keepFromEnd := len(msgs) / 4

	for i := 0; i < len(result)-keepFromEnd && freed < tokensToFree; i++ {
		m := &result[i]

		if cfg.ClearThinking && m.Role == "assistant" && isThinkingMessage(*m) {
			before := token.EstimateMessageTokens(*m)
			m.Content = "[Thinking content cleared]"
			freed += before - token.EstimateMessageTokens(*m)
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

		if len(m.ToolResults) > 0 && m.ToolResults[0].Content != "[Old tool result content cleared]" {
			toolName := ToolNameForResult(*m, result)
			if !mutatingTools[toolName] {
				before := len(m.ToolResults[0].Content) / 4
				if before > 100 {
					oldResults := m.ToolResults
					newResults := make([]types.ToolResult, len(oldResults))
					for j, tr := range oldResults {
						newResults[j] = types.ToolResult{
							ToolUseID: tr.ToolUseID,
							Content:   "[Old tool result content cleared]",
							IsError:   tr.IsError,
						}
					}
					m.ToolResults = newResults
					freed += before
				}
			}
		}
	}

	return result
}

func CountClearableToolResults(msgs []types.EyrieMessage) int {
	count := 0
	for _, m := range msgs {
		if len(m.ToolResults) > 0 && m.ToolResults[0].Content != "[Old tool result content cleared]" {
			toolName := ToolNameForResult(m, msgs)
			if !mutatingTools[toolName] {
				count++
			}
		}
	}
	return count
}

func isThinkingMessage(m types.EyrieMessage) bool {
	return len(m.Content) > 0 && m.Content[0] == '<' && len(m.ToolUse) == 0
}
