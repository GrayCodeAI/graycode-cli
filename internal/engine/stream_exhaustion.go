package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// synthesisTailMessages is how many recent messages feed the exhaustion prompt.
const synthesisTailMessages = 8

// SynthesisForExhaustion produces a coherent final completion when the agent
// loop exhausts its budget (turn/token/time limits), using ONE final
// tools-disabled LLM call — the "graceful exhaustion" idea from herm. Instead
// of stopping with a bare "limit reached" line, the model synthesizes a concise
// summary of what was done, what remains, and next steps. Returns "" when the
// session has no LLM, no conversation, the call fails, or the context is
// cancelled — callers fall back to their static stop message.
func (s *Session) SynthesisForExhaustion(ctx context.Context, reason string) string {
	if s == nil || s.ChatLLM() == nil {
		return ""
	}
	raw := s.Persistence().RawMessages()
	if len(raw) == 0 {
		return ""
	}
	if err := ctx.Err(); err != nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("You are completing a coding-agent session that must stop now because its execution budget is exhausted.\n")
	fmt.Fprintf(&b, "Reason: %s\n\n", reason)
	b.WriteString("Do not call any tools. Write a concise final message covering: what was accomplished, what is left to do, and suggested next steps.\n\nRecent conversation (most recent first):\n")
	start := 0
	if len(raw) > synthesisTailMessages {
		start = len(raw) - synthesisTailMessages
	}
	for i := len(raw) - 1; i >= start; i-- {
		m := raw[i]
		fmt.Fprintf(&b, "[%s] %s\n", m.Role, truncateRunes(m.Content, 300))
	}

	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := s.ChatLLM().Chat(callCtx, []types.GraycodeRouterMessage{
		{Role: "user", Content: b.String()},
	}, types.ChatOptions{
		Provider:  s.ChatLLM().Provider(),
		Model:     s.ChatLLM().Model(),
		MaxTokens: 800,
	})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(resp.Content)
}
