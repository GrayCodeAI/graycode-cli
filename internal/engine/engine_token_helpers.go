package engine

import (
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func isRecentToolHeavy(messages []types.GraycodeRouterMessage) bool {
	const lookback = 3
	toolTurns := 0
	assistantSeen := 0

	for i := len(messages) - 1; i >= 0 && assistantSeen < lookback; i-- {
		msg := messages[i]
		if msg.Role != "assistant" {
			continue
		}
		assistantSeen++
		if len(msg.ToolUse) > 0 {
			toolTurns++
		}
	}

	return assistantSeen >= lookback && toolTurns == assistantSeen
}

func isTextQuestion(messages []types.GraycodeRouterMessage) bool {
	var lastUserMsg string
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "user" && len(msg.ToolResults) == 0 {
			lastUserMsg = msg.Content
			break
		}
	}
	if lastUserMsg == "" {
		return false
	}

	lower := strings.ToLower(lastUserMsg)

	questionPrefixes := []string{
		"what ", "why ", "how ", "when ", "where ", "who ",
		"can you explain", "explain ", "describe ",
		"tell me", "is it", "are there", "does ",
	}
	for _, prefix := range questionPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return strings.HasSuffix(strings.TrimSpace(lower), "?")
}

func classifyPromptForBudget(messages []types.GraycodeRouterMessage) string {
	if isRecentToolHeavy(messages) {
		return "tool"
	}
	if isTextQuestion(messages) {
		return "question"
	}
	return "code"
}
