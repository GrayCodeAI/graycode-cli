package engine

import (
	"context"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/client"

	modelPkg "github.com/GrayCodeAI/hawk/routing"
)

// ShouldAutoCompact returns true if the conversation is approaching context limits.
func (s *Session) ShouldAutoCompact() bool {
	return len(s.messages) >= maxContextMessages
}

// AutoCompactIfNeeded runs compaction when the conversation exceeds the threshold.
func (s *Session) AutoCompactIfNeeded() bool {
	if !s.ShouldAutoCompact() {
		return false
	}
	s.smartCompact()
	return true
}

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

	// Keep last N messages + summary, respecting pinned count
	keepEnd := 10
	if s.PinnedMessages > keepEnd {
		keepEnd = s.PinnedMessages
	}
	keep := make([]client.EyrieMessage, 0, keepEnd+2)
	keep = append(keep, client.EyrieMessage{
		Role:    "user",
		Content: "[Conversation summary]\n" + summary + "\n\n[Continue from the recent messages below.]",
	})
	keep = append(keep, client.EyrieMessage{
		Role:    "assistant",
		Content: "Understood. I have the context from the summary above. Continuing.",
	})
	keep = append(keep, s.messages[len(s.messages)-keepEnd:]...)
	s.messages = keep
}

func (s *Session) generateSummary() string {
	// Build a compact version of the conversation for summarization
	// using the structured compaction prompt from compact_prompt.go
	var summaryMsgs []client.EyrieMessage
	compactPrompt := BuildCompactPrompt(CompactBase)
	summaryMsgs = append(summaryMsgs, client.EyrieMessage{
		Role:    "user",
		Content: compactPrompt + "\n\nConversation:\n",
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
		Model:     s.compactModel(),
		MaxTokens: 1000,
	})
	if err != nil {
		return ""
	}
	// Extract structured summary, stripping analysis block
	return FormatCompactSummary(resp.Content)
}

// compactModel returns the cheapest available model for the current provider.
// Queries eyrie's catalog at runtime — no hardcoded model names.
// Summarization doesn't need frontier reasoning, so the cheapest model suffices.
func (s *Session) compactModel() string {
	provider := strings.ToLower(s.provider)
	models := modelPkg.ByProvider(provider)
	if len(models) == 0 {
		return s.model
	}

	// Find the cheapest model by input price
	cheapest := models[0]
	for _, m := range models[1:] {
		if m.InputPrice > 0 && m.InputPrice < cheapest.InputPrice {
			cheapest = m
		}
	}

	// Only use a cheaper model if it actually costs less than the session model
	if info, ok := modelPkg.Find(s.model); ok {
		if cheapest.InputPrice >= info.InputPrice {
			return s.model
		}
	}

	return cheapest.Name
}
