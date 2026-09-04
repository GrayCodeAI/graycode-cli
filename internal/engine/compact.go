package engine

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/engine/compact"
	"github.com/GrayCodeAI/graycode-cli/internal/engine/token"
	"github.com/GrayCodeAI/graycode-cli/internal/types"

	modelPkg "github.com/GrayCodeAI/graycode-cli/internal/provider/routing"
)

// ShouldAutoCompact returns true if the conversation is approaching context limits.
func (s *Session) ShouldAutoCompact() bool {
	// Check message count
	messages := s.Persistence().RawMessagesView()
	if len(messages) >= maxContextMessages {
		return true
	}
	// Check token count using full message estimation (includes tool payloads).
	totalTokens := 0
	for _, msg := range messages {
		totalTokens += token.EstimateMessageTokens(msg)
	}
	window := s.ContextWindowSize()
	threshold := window * s.compactThresholdPct() / 100
	return totalTokens > threshold
}

// AutoCompactIfNeeded runs compaction when the conversation exceeds the threshold.
func (s *Session) AutoCompactIfNeeded(ctx context.Context) bool {
	if !s.ShouldAutoCompact() {
		return false
	}
	s.smartCompact(ctx)
	return true
}

// smartCompact uses the LLM to generate a summary of the conversation being compacted.
func (s *Session) smartCompact(ctx context.Context) {
	if len(s.Persistence().RawMessages()) <= 20 {
		return
	}

	// Check for split-turn condition first
	if s.SplitTurnNeeded(smartCompactKeepEnd) {
		s.splitTurnCompact(ctx)
		return
	}

	s.smartCompactBody(ctx)
}

// smartCompactBody performs standard summary-based compaction. It is shared by
// smartCompact and smartCompactFallback (which is reached when split-turn
// compaction finds no oversized turn).
func (s *Session) smartCompactBody(ctx context.Context) {
	raw := s.Persistence().RawMessages()
	if len(raw) <= 20 {
		return
	}

	// Keep last N messages + summary, respecting pinned count
	keepEnd := smartCompactKeepEnd
	if pinned := s.Persistence().PinnedMessages(); pinned > keepEnd {
		keepEnd = pinned
	}
	// Guard against raw[:len-keepEnd] panicking with a negative index when
	// pinned exceeds the message count (mirrors splitTurnCompact's guard).
	if len(raw) <= keepEnd {
		return
	}

	// Extract file tracking from messages being compacted
	if s.Persistence().Files() == nil {
		s.Persistence().SetFiles(NewFileTracker())
	}
	files := s.Persistence().Files()
	compactedMsgs := raw[:len(raw)-keepEnd]
	files.ExtractFromMessages(compactedMsgs)
	// Also parse any previous tracked-files from existing summary
	if len(compactedMsgs) > 0 && strings.Contains(compactedMsgs[0].Content, "<tracked-files>") {
		files.ParseFromSummary(compactedMsgs[0].Content)
	}

	// Try LLM-based summary first, fall back to truncation
	summary := s.generateSummary(ctx, raw)
	if summary == "" {
		s.compact(ctx) // fallback to boundary-aware truncation
		return
	}

	// Append file tracking to summary
	fileBlock := files.FormatForSummary()
	if fileBlock != "" {
		summary += "\n\n" + fileBlock
	}

	// Persist the verbatim compacted turns as a retrievable transcript
	// segment before they leave the live context. Best-effort: a persistence
	// failure must never block or corrupt compaction itself.
	if sessionID := s.executionGraphSessionID(); sessionID != "" && len(compactedMsgs) > 0 {
		detail, _ := compact.ParseCompactionDetail(os.Getenv("GRAYCODE_COMPACTION_SEGMENT_DETAIL"))
		if _, err := compact.WriteCompactionSegment(sessionID, compactedMsgs, detail); err != nil {
			slog.Debug("compaction segment persistence skipped", "error", err)
		}
	}

	tail := raw[len(raw)-keepEnd:]
	keep := make([]types.GraycodeRouterMessage, 0, len(tail)+2)
	keep = append(keep, types.GraycodeRouterMessage{
		Role:    "user",
		Content: "[Conversation summary]\n" + summary + "\n\n[Continue from the recent messages below.]",
	})
	// Only emit the assistant ack when the tail starts with a user message;
	// two adjacent assistant messages are rejected by OpenAI-compat providers.
	if len(tail) == 0 || tail[0].Role != "assistant" {
		keep = append(keep, types.GraycodeRouterMessage{
			Role:    "assistant",
			Content: "Understood. I have the context from the summary above. Continuing.",
		})
	}
	keep = append(keep, tail...)
	s.Persistence().ApplyCompaction(keep, len(raw))
}

// summaryInputRuneCap bounds the total rune budget fed into the summarizer so
// the LLM fallback in generateSummary can never overflow the compact model's
// window on very long transcripts.
const summaryInputRuneCap = 32_000

func (s *Session) generateSummary(ctx context.Context, raw []types.GraycodeRouterMessage) string {
	// Incremental compaction: if a prior summary was persisted by an earlier
	// compaction, merge the NEW messages into it rather than re-summarizing the
	// entire conversation from scratch. This preserves already-captured context
	// and avoids the cost of re-deriving it.
	prior := ExtractPriorSummary(raw)
	newMsgs := raw
	if prior != "" {
		// Only the messages that came after the persisted summary are "new".
		for i, m := range raw {
			if m.Role == "user" && strings.HasPrefix(m.Content, PriorSummaryPrefix) {
				newMsgs = raw[i+1:]
				break
			}
		}
	}

	// Build a compact version of the conversation for summarization
	// using the structured compaction prompt from compact_prompt.go
	var summaryMsgs []types.GraycodeRouterMessage
	compactPrompt := BuildIncrementalCompactPrompt(prior)
	summaryMsgs = append(summaryMsgs, types.GraycodeRouterMessage{
		Role:    "user",
		Content: compactPrompt + "\n\nConversation:\n",
	})

	// Add a condensed version of messages, capped at a bounded total size
	budget := summaryInputRuneCap
	for _, m := range newMsgs {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		if budget <= 0 {
			break
		}
		part := m.Role + ": " + truncateRunes(m.Content, 500) + "\n"
		if runes := len([]rune(part)); runes > budget {
			part = truncateRunes(part, budget)
		}
		budget -= len([]rune(part))
		summaryMsgs[0].Content += part
	}

	// Try shrike compression first as a fast, zero-cost alternative
	conversationText := summaryMsgs[0].Content
	targetBudget := 1000 // Keep summary under 1K tokens
	compressed, stats := token.Compress(conversationText, targetBudget)
	s.recordShrikeCompressionObservation(conversationText, "context-compaction", stats)
	reductionRatio := float64(stats.FinalTokens) / float64(stats.OriginalTokens)
	// Only accept the shrike path when the reduction is structural — a
	// budget-enforcer hard truncation (HardTruncated) would return the head
	// of the conversation cut mid-stream, which is not a summary.
	if reductionRatio < 0.5 && stats.OriginalTokens > targetBudget*2 && !stats.HardTruncated() {
		// shrike achieved >50% reduction, use compressed output directly
		// Extract key facts from compressed text for summary format
		return extractSummaryFromCompressed(compressed)
	}

	// Fall back to LLM-based summarization if shrike compression insufficient
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
	// Extract structured summary, stripping analysis block
	return FormatCompactSummary(resp.Content)
}

// extractSummaryFromCompressed pulls key information from shrike-compressed text
// to create a usable summary for the conversation context.
func extractSummaryFromCompressed(compressed string) string {
	// A meta-token digest ([META:...]) means the pipeline replaced content
	// with placeholders rather than summarizing it — never treat that as a
	// summary (defense in depth; the gate on HardTruncated already rejects
	// budget-only reductions).
	if strings.HasPrefix(compressed, "[META:") {
		return ""
	}
	// shrike compression preserves semantic meaning; extract actionable summary
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

// truncateRunes truncates s to at most max runes, appending "..." when it was
// truncated. Unlike byte slicing, this never splits a multi-byte UTF-8 rune,
// so the result is always valid UTF-8.
func truncateRunes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

// CompressMessageContent compresses a single message's content if it exceeds the limit.
// Uses shrike for fast, zero-cost compression. Returns the original if already short enough.
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
// Queries graycode-router's catalog at runtime — no hardcoded model names.
// Summarization doesn't need frontier reasoning, so the cheapest model suffices.
func (s *Session) compactModel() string {
	provider := strings.ToLower(s.ChatLLM().Provider())
	models := modelPkg.ByProvider(provider)
	if len(models) == 0 {
		return s.ChatLLM().Model()
	}

	// Find the cheapest model by input price. Free/local models (price 0) are
	// preferred for summarization, so they must not be skipped.
	cheapest := models[0]
	for _, m := range models[1:] {
		if m.InputPrice < cheapest.InputPrice {
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
