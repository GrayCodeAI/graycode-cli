package cmd

// turnHadThinkingOnly reports whether the latest user turn ended with internal
// reasoning visible but no assistant reply or tool activity. This is the TUI
// symptom of graycode-router's ResponseErrorOnlyReasoning health check.
func turnHadThinkingOnly(messages []displayMsg) bool {
	if len(messages) == 0 {
		return false
	}
	var sawThinking, sawAssistant, sawTool bool
	for i := len(messages) - 1; i >= 0; i-- {
		switch messages[i].role {
		case "user":
			return sawThinking && !sawAssistant && !sawTool
		case "thinking":
			sawThinking = true
		case "assistant":
			sawAssistant = true
		case "tool_use", "tool_result":
			sawTool = true
		}
	}
	return false
}

// stripCurrentTurnThinking removes thinking messages from the latest user turn.
// Used when the engine retries after a reasoning-only response.
func stripCurrentTurnThinking(messages []displayMsg) []displayMsg {
	if len(messages) == 0 {
		return messages
	}
	lastUser := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].role == "user" {
			lastUser = i
			break
		}
	}
	if lastUser < 0 {
		return messages
	}
	out := make([]displayMsg, 0, len(messages))
	for i, msg := range messages {
		if i > lastUser && msg.role == "thinking" {
			continue
		}
		out = append(out, msg)
	}
	return out
}
