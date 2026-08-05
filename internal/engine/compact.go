package engine

import (
	"context"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine/token"
	"github.com/GrayCodeAI/hawk/internal/types"

	modelPkg "github.com/GrayCodeAI/hawk/internal/provider/routing"
)

// ShouldAutoCompact returns true if the conversation is approaching context limits.
func (s *Session) ShouldAutoCompact() bool {
	// Check message count
	if len(s.Persistence().RawMessages()) >= maxContextMessages {
		return true
	}
	// Check token count using tok estimation
	totalTokens := 0
	for _, msg := range s.Persistence().RawMessages() {
		totalTokens += token.CountTokensFast(msg.Content)
	}
	window := s.ContextWindowSize()
	threshold := window * s.compactThresholdPct() / 100
	return totalTokens > threshold
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
	if len(s.Persistence().RawMessages()) <= 20 {
		return
	}

	// Keep last N messages + summary, respecting pinned count
	keepEnd := 10
	if pinned := s.Persistence().PinnedMessages(); pinned > keepEnd {
		keepEnd = pinned
	}

	// Check for split-turn condition first
	if s.SplitTurnNeeded(keepEnd) {
		s.splitTurnCompact()
		return
	}

	// Extract file tracking from messages being compacted
	if s.Persistence().Files() == nil {
		s.Persistence().SetFiles(NewFileTracker())
	}
	files := s.Persistence().Files()
	compactedMsgs := s.Persistence().RawMessages()[:len(s.Persistence().RawMessages())-keepEnd]
	files.ExtractFromMessages(compactedMsgs)
	// Also parse any previous tracked-files from existing summary
	if len(compactedMsgs) > 0 && strings.Contains(compactedMsgs[0].Content, "<tracked-files>") {
		files.ParseFromSummary(compactedMsgs[0].Content)
	}

	// Try LLM-based summary first, fall back to truncation
	summary := s.generateSummary()
	if summary == "" {
		s.compact() // fallback to boundary-aware truncation
		return
	}

	// Append file tracking to summary
	fileBlock := files.FormatForSummary()
	if fileBlock != "" {
		summary += "\n\n" + fileBlock
	}

	keep := make([]types.EyrieMessage, 0, keepEnd+2)
	keep = append(keep, types.EyrieMessage{
		Role:    "user",
		Content: "[Conversation summary]\n" + summary + "\n\n[Continue from the recent messages below.]",
	})
	keep = append(keep, types.EyrieMessage{
		Role:    "assistant",
		Content: "Understood. I have the context from the summary above. Continuing.",
	})
	keep = append(keep, s.Persistence().RawMessages()[len(s.Persistence().RawMessages())-keepEnd:]...)
	s.Persistence().SetRawMessages(keep)
}

func (s *Session) generateSummary() string {
	// Build a compact version of the conversation for summarization
	// using the structured compaction prompt from compact_prompt.go
	var summaryMsgs []types.EyrieMessage
	compactPrompt := BuildCompactPrompt(CompactBase)
	summaryMsgs = append(summaryMsgs, types.EyrieMessage{
		Role:    "user",
		Content: compactPrompt + "\n\nConversation:\n",
	})

	// Add a condensed version of messages
	for _, m := range s.Persistence().RawMessages() {
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
	compressed, stats := token.Compress(conversationText, targetBudget)
	s.recordTokCompressionObservation(conversationText, "context-compaction", stats)
	reductionRatio := float64(stats.FinalTokens) / float64(stats.OriginalTokens)
	if reductionRatio < 0.5 && stats.OriginalTokens > targetBudget*2 {
		// tok achieved >50% reduction, use compressed output directly
		// Extract key facts from compressed text for summary format
		return extractSummaryFromCompressed(compressed)
	}

	// Fall back to LLM-based summarization if tok compression insufficient
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := s.ChatLLM().Chat(ctx, summaryMsgs, types.ChatOptions{
		Provider:  s.ChatLLM().Provider(),
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

// CompressMessageContent compresses a single message's content if it exceeds the limit.
// Uses tok for fast, zero-cost compression. Returns the original if already short enough.
func CompressMessageContent(content string, maxTokens int) string {
	if token.CountTokensFast(content) <= maxTokens {
		return content
	}
	compressed, stats := token.Compress(content, maxTokens)
	if stats.FinalTokens < stats.OriginalTokens {
		return compressed
	}
	return content
}

// compactModel returns the cheapest available model for the current provider.
// Queries eyrie's catalog at runtime — no hardcoded model names.
// Summarization doesn't need frontier reasoning, so the cheapest model suffices.
func (s *Session) compactModel() string {
	provider := strings.ToLower(s.ChatLLM().Provider())
	models := modelPkg.ByProvider(provider)
	if len(models) == 0 {
		return s.ChatLLM().Model()
	}

	// Find the cheapest model by input price
	cheapest := models[0]
	for _, m := range models[1:] {
		if m.InputPrice > 0 && m.InputPrice < cheapest.InputPrice {
			cheapest = m
		}
	}

	// Only use a cheaper model if it actually costs less than the session model
	if info, ok := modelPkg.Find(s.ChatLLM().Model()); ok {
		if cheapest.InputPrice >= info.InputPrice {
			return s.ChatLLM().Model()
		}
	}

	return cheapest.Name
}
