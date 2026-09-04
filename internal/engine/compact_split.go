package engine

import (
	"context"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// splitTurnCompact handles the edge case where a single turn's messages
// exceed the keep-recent token budget. It generates two summaries:
// 1. History summary (all messages before the oversized turn)
// 2. Turn prefix summary (the first portion of the oversized turn)
// Then merges them into a single compacted context.
//
// This prevents information loss when one tool result (e.g., reading a large file)
// or one assistant response exceeds the normal compaction budget.

// turnTokenBudget returns the split-turn budget for a message tail: 3x the
// average token count of the tail, with a 2000-token floor to avoid false
// positives. A single message exceeding this budget is treated as an
// oversized turn.
func turnTokenBudget(tail []types.EyrieMessage) int {
	if len(tail) == 0 {
		return 0
	}
	totalTokens := 0
	for _, msg := range tail {
		totalTokens += EstimateMessageTokens(msg)
	}
	budget := totalTokens / len(tail) * 3
	if budget < 2000 {
		budget = 2000 // minimum budget to avoid false positives
	}
	return budget
}

// SplitTurnNeeded checks if any single turn in the recent messages exceeds
// the given token budget. The budget is defined as the average token count
// of the keepCount messages multiplied by 3 (a single message uses more than
// 3x the average of the kept tail).
func (s *Session) SplitTurnNeeded(keepCount int) bool {
	if len(s.Persistence().RawMessages()) <= keepCount {
		return false
	}

	// Calculate token budget: average of last keepCount messages * 3
	tail := s.Persistence().RawMessages()[len(s.Persistence().RawMessages())-keepCount:]
	budget := turnTokenBudget(tail)

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
func (s *Session) splitTurnCompact(ctx context.Context) {
	keepEnd := smartCompactKeepEnd
	if pinned := s.Persistence().PinnedMessages(); pinned > keepEnd {
		keepEnd = pinned
	}
	// Snapshot once so the compaction observes a consistent transcript even
	// if a concurrent AddUser appends mid-way.
	raw := s.Persistence().RawMessages()
	if len(raw) <= keepEnd {
		return
	}

	// Find the oversized message in the tail
	tail := raw[len(raw)-keepEnd:]
	budget := turnTokenBudget(tail)

	oversizedIdx := -1
	for i, msg := range tail {
		if EstimateMessageTokens(msg) > budget {
			oversizedIdx = i
			break
		}
	}

	if oversizedIdx < 0 {
		// No oversized turn found, fall back to normal compaction
		s.smartCompactFallback(ctx)
		return
	}

	// Split point in the full message array
	tailStart := len(raw) - keepEnd
	splitPoint := tailStart + oversizedIdx

	// Extract file tracking before compaction
	if s.Persistence().Files() == nil {
		s.Persistence().SetFiles(NewFileTracker())
	}
	files := s.Persistence().Files()
	files.ExtractFromMessages(raw[:splitPoint])

	// Phase 1: Summarize everything before the oversized turn
	phase1Summary := s.generatePartialSummary(ctx, raw[:splitPoint])

	// Phase 2: Summarize the first half of the oversized turn's content
	oversizedMsg := raw[splitPoint]
	phase2Summary := s.summarizeOversizedTurn(ctx, oversizedMsg)

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
	fileBlock := files.FormatForSummary()
	if fileBlock != "" {
		combined.WriteString("\n\n")
		combined.WriteString(fileBlock)
	}

	// Reconstruct messages: summary + tail from oversized turn onward
	// Keep the second half of the oversized message + everything after
	// (its first half is covered by phase2Summary above).
	remaining := append([]types.EyrieMessage(nil), raw[splitPoint:]...)
	if len(remaining) > 0 {
		first := remaining[0]
		first.Content = secondHalfRunes(first.Content)
		for i := range first.ToolResults {
			first.ToolResults[i].Content = secondHalfRunes(first.ToolResults[i].Content)
		}
		remaining[0] = first
	}

	keep := make([]types.EyrieMessage, 0, len(remaining)+2)
	keep = append(keep, types.EyrieMessage{
		Role:    "user",
		Content: combined.String() + "\n\n[Continue from the recent messages below.]",
	})
	// Only emit the assistant ack when the remaining tail starts with a user
	// message; adjacent assistant messages are rejected by OpenAI-compat
	// providers.
	if len(remaining) == 0 || remaining[0].Role != "assistant" {
		keep = append(keep, types.EyrieMessage{
			Role:    "assistant",
			Content: "Understood. I have the context from the summary above. Continuing.",
		})
	}
	keep = append(keep, remaining...)
	s.Persistence().ApplyCompaction(keep, len(raw))
}

// secondHalfRunes returns the second half of s, split on a rune boundary so
// multi-byte UTF-8 is never cut mid-sequence. An empty string returns itself.
func secondHalfRunes(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	return string(runes[len(runes)/2:])
}

// generatePartialSummary generates an LLM summary for a subset of messages.
func (s *Session) generatePartialSummary(ctx context.Context, messages []types.EyrieMessage) string {
	if len(messages) == 0 {
		return ""
	}

	var summaryMsgs []types.EyrieMessage
	compactPrompt := BuildCompactPrompt(CompactPartial)
	var content strings.Builder
	content.WriteString(compactPrompt)
	content.WriteString("\n\nConversation:\n")

	for _, m := range messages {
		if m.Role == "user" || m.Role == "assistant" {
			c := truncateRunes(m.Content, 500)
			content.WriteString(m.Role)
			content.WriteString(": ")
			content.WriteString(c)
			content.WriteString("\n")
		}
	}

	summaryMsgs = append(summaryMsgs, types.EyrieMessage{
		Role:    "user",
		Content: content.String(),
	})

	if s.ChatLLM() == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := s.ChatLLM().Chat(ctx, summaryMsgs, types.ChatOptions{
		Provider:  s.ChatLLM().Provider(),
		Model:     s.compactModel(),
		MaxTokens: 1000,
	})
	if err != nil {
		return ""
	}
	return FormatCompactSummary(resp.Content)
}

// summarizeOversizedTurn summarizes the content of a single oversized message.
func (s *Session) summarizeOversizedTurn(ctx context.Context, msg types.EyrieMessage) string {
	content := msg.Content
	if content == "" {
		// If it's a tool result, use that
		if len(msg.ToolResults) > 0 {
			content = msg.ToolResults[0].Content
		}
	}
	if content == "" {
		return ""
	}

	// Take the first half for summarization, capped at 4000 runes so the
	// summarization input stays small (the rest is retained verbatim).
	halfLen := len([]rune(content)) / 2
	if halfLen > 4000 {
		halfLen = 4000
	}

	var summaryMsgs []types.EyrieMessage
	summaryMsgs = append(summaryMsgs, types.EyrieMessage{
		Role: "user",
		Content: BuildCompactPrompt(CompactUpTo) + "\n\nSummarize the key information from this content prefix (the rest will be retained verbatim):\n\n" +
			msg.Role + ": " + truncateRunes(content, halfLen),
	})

	if s.ChatLLM() == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := s.ChatLLM().Chat(ctx, summaryMsgs, types.ChatOptions{
		Provider:  s.ChatLLM().Provider(),
		Model:     s.compactModel(),
		MaxTokens: 500,
	})
	if err != nil {
		return ""
	}
	return FormatCompactSummary(resp.Content)
}

// smartCompactFallback is reached when split-turn compaction is detected but
// no oversized message is found (edge case). It runs the standard summary-based
// compaction body directly; the split-turn check is intentionally skipped to
// avoid re-entering splitTurnCompact.
func (s *Session) smartCompactFallback(ctx context.Context) {
	s.smartCompactBody(ctx)
}
