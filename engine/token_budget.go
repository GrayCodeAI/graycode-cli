package engine

import (
	"strings"

	"github.com/GrayCodeAI/eyrie/client"
)

// DynamicMaxTokens calculates the optimal max_tokens for a request based on:
// - Whether the last few turns were tool-call-heavy (reduce to 4096)
// - Whether the user asked a question expecting text (8192)
// - Whether this is a code generation task (16384)
// - The remaining context budget (don't exceed model's limit)
//
// Research basis: Output tokens are 3-5x more expensive than input tokens.
// Most tool-call turns only need 200-2000 tokens of output.
func DynamicMaxTokens(messages []client.EyrieMessage, contextSize int, taskType string) int {
	base := selectBaseTokens(taskType)

	// Check recent messages for tool-call-heavy pattern.
	// If the last several assistant turns all used tools, the next turn
	// likely will too -- cap output tokens to avoid waste.
	if isRecentToolHeavy(messages) {
		base = 4096
	}

	// If the user's last message looks like a question expecting a text
	// answer (not a tool invocation), use a moderate budget.
	if base > 8192 && isTextQuestion(messages) {
		base = 8192
	}

	// Ensure we don't exceed the remaining context budget.
	// Reserve at least 20% of the context window for output, but never
	// allocate more output tokens than the remaining space.
	if contextSize > 0 {
		inputTokens := EstimateTokens(messages)
		remaining := contextSize - inputTokens
		if remaining < 1024 {
			remaining = 1024 // absolute floor so the model can respond at all
		}
		if base > remaining {
			base = remaining
		}
	}

	// Absolute floor: never go below 1024 tokens.
	if base < 1024 {
		base = 1024
	}

	return base
}

// selectBaseTokens returns the starting max_tokens budget based on taskType.
func selectBaseTokens(taskType string) int {
	switch strings.ToLower(strings.TrimSpace(taskType)) {
	case "code", "codegen", "code_generation", "implement":
		return 16384
	case "question", "explain", "text":
		return 8192
	case "tool", "tool_use":
		return 4096
	default:
		return 16384 // default to code-generation budget
	}
}

// isRecentToolHeavy returns true if the last N assistant messages all
// contained tool calls, suggesting the agent is in a tool-use loop where
// large output budgets are wasted.
func isRecentToolHeavy(messages []client.EyrieMessage) bool {
	const lookback = 3
	toolTurns := 0
	assistantSeen := 0

	// Walk backward through messages, counting recent assistant turns.
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

	// Need at least `lookback` assistant turns to conclude a pattern.
	return assistantSeen >= lookback && toolTurns == assistantSeen
}

// isTextQuestion returns true if the last user message looks like a
// question expecting a textual answer rather than a tool invocation.
func isTextQuestion(messages []client.EyrieMessage) bool {
	// Find the last user message (skip tool results).
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

	// Question indicators: starts with a question word or ends with "?"
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
	if strings.HasSuffix(strings.TrimSpace(lower), "?") {
		return true
	}

	return false
}

// classifyPromptForBudget extracts a task type string from the current messages
// for use with DynamicMaxTokens. Returns: "code", "question", "tool", or "code" default.
func classifyPromptForBudget(messages []client.EyrieMessage) string {
	if isRecentToolHeavy(messages) {
		return "tool"
	}
	if isTextQuestion(messages) {
		return "question"
	}
	return "code"
}
