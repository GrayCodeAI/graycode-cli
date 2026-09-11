package session

import "github.com/GrayCodeAI/hawk/internal/types"

// FromRuntimeMessages converts Hawk runtime messages into persisted session messages.
func FromRuntimeMessages(in []types.EyrieMessage) []Message {
	if len(in) == 0 {
		return nil
	}
	out := make([]Message, len(in))
	for i, msg := range in {
		out[i] = Message{
			Role:         msg.Role,
			Content:      msg.Content,
			Thinking:     msg.Thinking,
			ContentParts: msg.ContentParts,
			Images:       msg.Images,
			ToolUse:      FromRuntimeToolCalls(msg.ToolUse),
			ToolResults:  FromRuntimeToolResults(msg.ToolResults),
		}
	}
	return out
}

// FromRuntimeToolCalls converts Hawk runtime tool calls into persisted contracts.
// types.ToolCall and session.ToolCall are identical (both alias tools.ToolCall).
func FromRuntimeToolCalls(in []types.ToolCall) []ToolCall {
	return in
}

// FromRuntimeToolResults converts Hawk runtime tool results into persisted contracts.
func FromRuntimeToolResults(in []types.ToolResult) []ToolResult {
	return in
}

// ToRuntimeMessages converts persisted session messages back into Hawk runtime messages.
func ToRuntimeMessages(in []Message) []types.EyrieMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.EyrieMessage, len(in))
	for i, msg := range in {
		out[i] = types.EyrieMessage{
			Role:         msg.Role,
			Content:      msg.Content,
			Thinking:     msg.Thinking,
			ContentParts: msg.ContentParts,
			Images:       msg.Images,
			ToolUse:      ToRuntimeToolCalls(msg.ToolUse),
			ToolResults:  ToRuntimeToolResults(msg.ToolResults),
		}
	}
	return out
}

// ToRuntimeToolCalls converts persisted contracts back into Hawk runtime tool calls.
func ToRuntimeToolCalls(in []ToolCall) []types.ToolCall {
	return in
}

// ToRuntimeToolResults converts persisted contracts back into Hawk runtime tool results.
func ToRuntimeToolResults(in []ToolResult) []types.ToolResult {
	return in
}
