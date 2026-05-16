package engine

import (
	"context"
	"fmt"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// AwaySummary — generates a brief recap when the user returns after stepping
// away from a session.
// ─────────────────────────────────────────────────────────────────────────────

// AwaySummaryConfig controls away summary behavior.
type AwaySummaryConfig struct {
	Enabled       bool
	IdleThreshold time.Duration // minimum idle time to trigger summary
	MaxMessages   int           // number of recent messages to include in context
}

// DefaultAwaySummaryConfig returns sensible defaults.
func DefaultAwaySummaryConfig() AwaySummaryConfig {
	return AwaySummaryConfig{
		Enabled:       true,
		IdleThreshold: 10 * time.Minute,
		MaxMessages:   30,
	}
}

// GenerateAwaySummary creates a brief "while you were away" recap.
// Uses the cheapest model via cascade for cost efficiency.
func GenerateAwaySummary(ctx context.Context, session *Session, cfg AwaySummaryConfig, agentFn func(ctx context.Context, prompt string) (string, error)) (string, error) {
	if !cfg.Enabled {
		return "", nil
	}

	messages := session.RawMessages()
	if len(messages) == 0 {
		return "", nil
	}

	// Get recent messages for context
	start := 0
	if len(messages) > cfg.MaxMessages {
		start = len(messages) - cfg.MaxMessages
	}
	recent := messages[start:]

	// Build conversation summary from recent messages
	var conversationSummary string
	for _, msg := range recent {
		preview := msg.Content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		if msg.Role == "user" {
			conversationSummary += fmt.Sprintf("User: %s\n", preview)
		} else if msg.Role == "assistant" && msg.Content != "" {
			conversationSummary += fmt.Sprintf("Assistant: %s\n", preview)
		}
	}

	prompt := buildAwaySummaryPrompt(conversationSummary)
	summary, err := agentFn(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("generate away summary: %w", err)
	}

	return summary, nil
}

func buildAwaySummaryPrompt(conversationSummary string) string {
	return fmt.Sprintf(`You are generating a brief "while you were away" recap for a coding session.

Rules:
- 1-3 sentences maximum
- Focus on what was accomplished and what's next
- Do NOT include status reports or tool call details
- Be concise and actionable

Recent conversation:
%s

Generate the recap:`, conversationSummary)
}

// ShouldShowAwaySummary checks if enough time has passed to warrant a summary.
func ShouldShowAwaySummary(lastActivity time.Time, cfg AwaySummaryConfig) bool {
	return time.Since(lastActivity) >= cfg.IdleThreshold
}
