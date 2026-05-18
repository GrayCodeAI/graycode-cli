package engine

import (
	"context"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/tok"

	modelPkg "github.com/GrayCodeAI/hawk/internal/provider/routing"
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

	// Keep last N messages + summary, respecting pinned count
	keepEnd := 10
	if s.PinnedMessages > keepEnd {
		keepEnd = s.PinnedMessages
	}

	// Check for split-turn condition first
	if s.SplitTurnNeeded(keepEnd) {
		s.splitTurnCompact()
		return
	}

	// Extract file tracking from messages being compacted
	if s.Files == nil {
		s.Files = NewFileTracker()
	}
	compactedMsgs := s.messages[:len(s.messages)-keepEnd]
	s.Files.ExtractFromMessages(compactedMsgs)
	// Also parse any previous tracked-files from existing summary
	if len(compactedMsgs) > 0 && strings.Contains(compactedMsgs[0].Content, "<tracked-files>") {
		s.Files.ParseFromSummary(compactedMsgs[0].Content)
	}

	// Try LLM-based summary first, fall back to truncation
	summary := s.generateSummary()
	if summary == "" {
		s.compact() // fallback to boundary-aware truncation
		return
	}

	// Append file tracking to summary
	fileBlock := s.Files.FormatForSummary()
	if fileBlock != "" {
		summary += "\n\n" + fileBlock
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

	// Try tok compression first as a fast, zero-cost alternative
	conversationText := summaryMsgs[0].Content
	targetBudget := 1000 // Keep summary under 1K tokens
	compressed, stats := tok.Compress(conversationText, tok.WithBudget(targetBudget))
	reductionRatio := float64(stats.FinalTokens) / float64(stats.OriginalTokens)
	if reductionRatio < 0.5 && stats.OriginalTokens > targetBudget*2 {
		// tok achieved >50% reduction, use compressed output directly
		// Extract key facts from compressed text for summary format
		return extractSummaryFromCompressed(compressed)
	}

	// Fall back to LLM-based summarization if tok compression insufficient
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

// extractSummaryFromCompressed pulls key information from tok-compressed text
// to create a usable summary for the conversation context.
func extractSummaryFromCompressed(compressed string) string {
	// tok compression preserves semantic meaning; extract actionable summary
	lines := strings.Split(compressed, "\n")
	var keyPoints []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || len(line) < 20 {
			continue
		}
		// Keep lines that look like substantive content
		if strings.Contains(line, ":") || strings.Contains(line, ".") || strings.Contains(line, "function") || strings.Contains(line, "error") {
			keyPoints = append(keyPoints, line)
		}
	}
	if len(keyPoints) == 0 {
		return ""
	}
	return strings.Join(keyPoints, "\n")
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
