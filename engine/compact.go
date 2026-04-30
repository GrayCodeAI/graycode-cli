package engine

import (
	"context"
	"time"

	"github.com/hawk/eyrie/client"
)

// smartCompact uses the LLM to generate a summary of the conversation being compacted.
func (s *Session) smartCompact() {
	if len(s.messages) <= 20 {
		return
	}

	// Try LLM-based summary first, fall back to truncation
	summary := s.generateSummary()
	if summary == "" {
		s.compact() // fallback to boundary-aware truncation
		return
	}

	// Keep last 10 messages + summary
	keep := make([]client.EyrieMessage, 0, 12)
	keep = append(keep, client.EyrieMessage{
		Role:    "user",
		Content: "[Conversation summary]\n" + summary + "\n\n[Continue from the recent messages below.]",
	})
	keep = append(keep, client.EyrieMessage{
		Role:    "assistant",
		Content: "Understood. I have the context from the summary above. Continuing.",
	})
	keep = append(keep, s.messages[len(s.messages)-10:]...)
	s.messages = keep
}

func (s *Session) generateSummary() string {
	// Build a compact version of the conversation for summarization
	var summaryMsgs []client.EyrieMessage
	summaryMsgs = append(summaryMsgs, client.EyrieMessage{
		Role:    "user",
		Content: "Summarize this conversation concisely. Include: what the user asked for, what tools were used, what files were modified, what was accomplished, and any unfinished work. Be brief — 3-5 sentences max.\n\nConversation:\n",
	})

	// Add a condensed version of messages
	for _, m := range s.messages {
		if m.Role == "user" || m.Role == "assistant" {
			content := m.Content
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			summaryMsgs[0].Content += m.Role + ": " + content + "\n"
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := s.client.Chat(ctx, summaryMsgs, client.ChatOptions{
		Provider:  s.provider,
		Model:     s.model,
		MaxTokens: 500,
	})
	if err != nil {
		return ""
	}
	return resp.Content
}
