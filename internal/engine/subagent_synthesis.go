package engine

import (
	"context"
	"fmt"

	"github.com/GrayCodeAI/eyrie/client"
)

// SynthesisPrompt is appended when forcing a sub-agent to summarize.
const SynthesisPrompt = "You have reached your turn limit. Provide a concise summary of what you found and accomplished. Do not request any more tool calls."

// SynthesizeSubAgent forces a final response from a sub-agent with tools disabled.
// Used when ShouldSynthesize() returns true. Returns the synthesized summary text.
//
// The function builds messages from the conversation so far, appends a user
// message with SynthesisPrompt, and calls the provider with Tools=nil (disabled)
// to force a text-only response.
func SynthesizeSubAgent(ctx context.Context, llm LLMClient, model string, conversationSoFar []client.EyrieMessage) (string, error) {
	if llm == nil {
		return "", fmt.Errorf("subagent synthesis: LLM client is nil")
	}

	// Build messages: conversation so far + synthesis prompt.
	msgs := make([]client.EyrieMessage, len(conversationSoFar)+1)
	copy(msgs, conversationSoFar)
	msgs[len(msgs)-1] = client.EyrieMessage{
		Role:    "user",
		Content: SynthesisPrompt,
	}

	// Call with Tools=nil to disable tool use and force a text summary.
	opts := client.ChatOptions{
		Model:     model,
		Tools:     nil,
		MaxTokens: 2048,
	}

	resp, err := llm.Chat(ctx, msgs, opts)
	if err != nil {
		return "", fmt.Errorf("subagent synthesis: %w", err)
	}

	if resp == nil || resp.Content == "" {
		return "", fmt.Errorf("subagent synthesis: empty response from provider")
	}

	return resp.Content, nil
}
