package engine

import (
	"strings"

	"github.com/GrayCodeAI/eyrie/client"
)

func isRecentToolHeavy(messages []client.EyrieMessage) bool {
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

func isTextQuestion(messages []client.EyrieMessage) bool {
	var lastUserMsg string
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "user" && msg.ToolResult == nil {
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

func classifyPromptForBudget(messages []client.EyrieMessage) string {
	if isRecentToolHeavy(messages) {
		return "tool"
	}
	if isTextQuestion(messages) {
		return "question"
	}
	return "code"
}
