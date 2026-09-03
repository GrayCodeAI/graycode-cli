package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// GenerateTitle derives a concise, descriptive title for the session.
// It attempts LLM-based summarization (3–6 words) on the initial conversation turns,
// falling back to the deterministic JournalTitle if LLM titling is unavailable or fails.
func (s *Session) GenerateTitle(ctx context.Context) (string, error) {
	if s == nil {
		return "Untitled Session", nil
	}

	titleCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if j := s.Persistence().Journal(); j != nil && s.ChatLLM() != nil {
		j.AppendSessionTitleLLMRequest(s.ChatLLM().Model())
	}

	title, err := s.generateTitleLLM(titleCtx)
	if err == nil && title != "" {
		if j := s.Persistence().Journal(); j != nil {
			j.AppendSessionTitle(title)
		}
		return title, nil
	}

	// Deterministic fallback
	detTitle := s.JournalTitle()
	if j := s.Persistence().Journal(); j != nil {
		j.AppendSessionTitle(detTitle)
	}
	return detTitle, nil
}

func (s *Session) generateTitleLLM(ctx context.Context) (string, error) {
	msgs := s.Persistence().Messages()
	if len(msgs) == 0 {
		return "", fmt.Errorf("no messages to generate title from")
	}

	// Collect first user message and optional assistant reply
	var userMsg, assistantMsg string
	for _, m := range msgs {
		if m.Role == "user" && userMsg == "" {
			userMsg = m.Content
		} else if m.Role == "assistant" && userMsg != "" && assistantMsg == "" {
			assistantMsg = m.Content
			break
		}
	}

	if userMsg == "" {
		return "", fmt.Errorf("no user message found")
	}

	if len(userMsg) > 500 {
		userMsg = userMsg[:500]
	}
	if len(assistantMsg) > 500 {
		assistantMsg = assistantMsg[:500]
	}

	titlePrompt := fmt.Sprintf(
		"Generate a concise 3 to 6 word title summarizing the user's intent in this session. Return ONLY the title text, with no quotes, no markdown, and no trailing period.\n\nUser request: %s",
		userMsg,
	)
	if assistantMsg != "" {
		titlePrompt += fmt.Sprintf("\nAssistant response summary: %s", assistantMsg)
	}

	llm := s.ChatLLM()
	if llm == nil {
		return "", fmt.Errorf("no chat client available")
	}

	// Fast streaming call for title generation
	reqMsgs := []types.EyrieMessage{
		{
			Role:    "user",
			Content: titlePrompt,
		},
	}

	result, err := llm.Stream(ctx, reqMsgs, types.ChatOptions{})
	if err != nil {
		return "", err
	}
	defer result.Close()

	var sb strings.Builder
	for ev := range result.Events {
		if ev.Type == "content" {
			sb.WriteString(ev.Content)
		}
	}

	raw := strings.TrimSpace(sb.String())
	cleaned := sanitizeTitle(raw)
	if cleaned == "" {
		return "", fmt.Errorf("empty title returned by LLM")
	}

	return cleaned, nil
}

func sanitizeTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`+"`")
	s = strings.TrimPrefix(s, "Title:")
	s = strings.TrimPrefix(s, "title:")
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}
