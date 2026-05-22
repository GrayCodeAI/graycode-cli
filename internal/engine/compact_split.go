package engine

import (
	"context"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/client"
)

// splitTurnCompact handles the edge case where a single turn's messages
// exceed the keep-recent token budget. It generates two summaries:
// 1. History summary (all messages before the oversized turn)
// 2. Turn prefix summary (the first portion of the oversized turn)
// Then merges them into a single compacted context.
//
// This prevents information loss when one tool result (e.g., reading a large file)
// or one assistant response exceeds the normal compaction budget.

// SplitTurnNeeded checks if any single turn in the recent messages exceeds
// the given token budget. The budget is defined as the average token count
// of the keepCount messages multiplied by 3 (a single message uses more than
// 3x the average of the kept tail).
func (s *Session) SplitTurnNeeded(keepCount int) bool {
	if len(s.messages) <= keepCount {
		return false
	}

	// Calculate token budget: average of last keepCount messages * 3
	tail := s.messages[len(s.messages)-keepCount:]
	totalTokens := 0
	for _, msg := range tail {
		totalTokens += EstimateMessageTokens(msg)
	}
	if len(tail) == 0 {
		return false
	}
	avgTokens := totalTokens / len(tail)
	budget := avgTokens * 3
	if budget < 2000 {
		budget = 2000 // minimum budget to avoid false positives
	}

	// Check if any single message in the tail exceeds the budget
	for _, msg := range tail {
		if EstimateMessageTokens(msg) > budget {
			return true
		}
	}
	return false
}

// splitTurnCompact performs the two-phase compaction:
// Phase 1: Summarize history (messages before the split point)
// Phase 2: Summarize the turn prefix (early part of the oversized turn)
// Result: merged summary + tail of oversized turn + any messages after
func (s *Session) splitTurnCompact() {
	keepEnd := 10
	if s.PinnedMessages > keepEnd {
		keepEnd = s.PinnedMessages
	}
	if len(s.messages) <= keepEnd {
		return
	}

	// Find the oversized message in the tail
	tail := s.messages[len(s.messages)-keepEnd:]
	totalTokens := 0
	for _, msg := range tail {
		totalTokens += EstimateMessageTokens(msg)
	}
	avgTokens := totalTokens / len(tail)
	budget := avgTokens * 3
	if budget < 2000 {
		budget = 2000
	}

	oversizedIdx := -1
	for i, msg := range tail {
		if EstimateMessageTokens(msg) > budget {
			oversizedIdx = i
			break
		}
	}

	if oversizedIdx < 0 {
		// No oversized turn found, fall back to normal compaction
		s.smartCompactFallback()
		return
	}

	// Split point in the full message array
	tailStart := len(s.messages) - keepEnd
	splitPoint := tailStart + oversizedIdx

	// Extract file tracking before compaction
	if s.Files == nil {
		s.Files = NewFileTracker()
	}
	s.Files.ExtractFromMessages(s.messages[:splitPoint])

	// Phase 1: Summarize everything before the oversized turn
	phase1Summary := s.generatePartialSummary(s.messages[:splitPoint])

	// Phase 2: Summarize the first half of the oversized turn's content
	oversizedMsg := s.messages[splitPoint]
	phase2Summary := s.summarizeOversizedTurn(oversizedMsg)

	// Build the combined summary
	var combined strings.Builder
	combined.WriteString("[Combined summary]\n")
	if phase1Summary != "" {
		combined.WriteString(phase1Summary)
	}
	combined.WriteString("\n\n[Oversized turn context]\n")
	if phase2Summary != "" {
		combined.WriteString(phase2Summary)
	}

	// Append file tracking
	fileBlock := s.Files.FormatForSummary()
	if fileBlock != "" {
		combined.WriteString("\n\n")
		combined.WriteString(fileBlock)
	}

	// Reconstruct messages: summary + tail from oversized turn onward
	// Keep the second half of the oversized message + everything after
	remainingMessages := s.messages[splitPoint:]

	keep := make([]client.EyrieMessage, 0, len(remainingMessages)+2)
	keep = append(keep, client.EyrieMessage{
		Role:    "user",
		Content: combined.String() + "\n\n[Continue from the recent messages below.]",
	})
	keep = append(keep, client.EyrieMessage{
		Role:    "assistant",
		Content: "Understood. I have the context from the summary above. Continuing.",
	})
	keep = append(keep, remainingMessages...)
	s.messages = keep
}

// generatePartialSummary generates an LLM summary for a subset of messages.
func (s *Session) generatePartialSummary(messages []client.EyrieMessage) string {
	if len(messages) == 0 {
		return ""
	}

	var summaryMsgs []client.EyrieMessage
	compactPrompt := BuildCompactPrompt(CompactPartial)
	var content strings.Builder
	content.WriteString(compactPrompt)
	content.WriteString("\n\nConversation:\n")

	for _, m := range messages {
		if m.Role == "user" || m.Role == "assistant" {
			c := m.Content
			if len(c) > 500 {
				c = c[:500] + "..."
			}
			content.WriteString(m.Role)
			content.WriteString(": ")
			content.WriteString(c)
			content.WriteString("\n")
		}
	}

	summaryMsgs = append(summaryMsgs, client.EyrieMessage{
		Role:    "user",
		Content: content.String(),
	})

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
	return FormatCompactSummary(resp.Content)
}

// summarizeOversizedTurn summarizes the content of a single oversized message.
func (s *Session) summarizeOversizedTurn(msg client.EyrieMessage) string {
	content := msg.Content
	if content == "" {
		// If it's a tool result, use that
		if msg.ToolResult != nil {
			content = msg.ToolResult.Content
		}
	}
	if content == "" {
		return ""
	}

	// Take the first half for summarization
	halfLen := len(content) / 2
	if halfLen > 4000 {
		halfLen = 4000 // cap at 4000 chars for the summarization input
	}
	prefix := content[:halfLen]

	var summaryMsgs []client.EyrieMessage
	summaryMsgs = append(summaryMsgs, client.EyrieMessage{
		Role: "user",
		Content: BuildCompactPrompt(CompactUpTo) + "\n\nSummarize the key information from this content prefix (the rest will be retained verbatim):\n\n" +
			msg.Role + ": " + prefix + "...",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := s.client.Chat(ctx, summaryMsgs, client.ChatOptions{
		Provider:  s.provider,
		Model:     s.compactModel(),
		MaxTokens: 500,
	})
	if err != nil {
		return ""
	}
	return FormatCompactSummary(resp.Content)
}

// smartCompactFallback is the original smartCompact logic used when split-turn
// is detected but no oversized message is found (edge case fallback).
func (s *Session) smartCompactFallback() {
	if len(s.messages) <= 20 {
		return
	}

	keepEnd := 10
	if s.PinnedMessages > keepEnd {
		keepEnd = s.PinnedMessages
	}

	if s.Files == nil {
		s.Files = NewFileTracker()
	}
	compactedMsgs := s.messages[:len(s.messages)-keepEnd]
	s.Files.ExtractFromMessages(compactedMsgs)
	if len(compactedMsgs) > 0 && strings.Contains(compactedMsgs[0].Content, "<tracked-files>") {
		s.Files.ParseFromSummary(compactedMsgs[0].Content)
	}

	summary := s.generateSummary()
	if summary == "" {
		s.compact()
		return
	}

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
