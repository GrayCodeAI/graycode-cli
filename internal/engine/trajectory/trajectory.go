// Package trajectory implements trajectory compression for agent runs,
// adopting Hermes Agent's trajectory_compressor: protect the first and last
// N turns verbatim (including their tool calls), compress only the middle into
// a single human summary message, and keep the surviving tool calls intact so
// training/eval signal is preserved.
package trajectory

import (
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// bytesPerToken is the rough character-based token estimate.
const bytesPerToken = 4

// estTokens estimates the token cost of a message from its text plus tool
// payload sizes.
func estTokens(m types.EyrieMessage) int {
	n := len(m.Content)
	for _, tu := range m.ToolUse {
		if s, ok := tu.Arguments["_raw"]; ok {
			if str, ok := s.(string); ok {
				n += len(str)
			}
		}
		for _, v := range tu.Arguments {
			if str, ok := v.(string); ok {
				n += len(str)
			}
		}
	}
	for _, tr := range m.ToolResults {
		n += len(tr.Content)
	}
	return n / bytesPerToken
}

// CompressTrajectory returns a trajectory where the first protectFirst turns
// and the last protectLast turns are kept verbatim and the middle is replaced
// by a single human summary message, only when the total exceeds
// targetTokens. The summary is a plain text "user" message so downstream
// consumers treat it as a faithful human-supplied checkpoint. Tool calls in
// the kept head/tail remain intact.
//
// Returns (msgs, false) unchanged when already under budget or there is no
// compressible middle.
func CompressTrajectory(msgs []types.EyrieMessage, targetTokens, protectFirst, protectLast int) ([]types.EyrieMessage, bool) {
	if len(msgs) == 0 || targetTokens <= 0 {
		return msgs, false
	}
	total := 0
	for _, m := range msgs {
		total += estTokens(m)
	}
	if total <= targetTokens {
		return msgs, false
	}
	if protectFirst < 0 {
		protectFirst = 0
	}
	if protectLast < 0 {
		protectLast = 0
	}
	// Ensure head and tail do not overlap.
	if protectFirst+protectLast >= len(msgs) {
		return msgs, false // no middle to compress
	}

	head := msgs[:protectFirst]
	tail := msgs[len(msgs)-protectLast:]
	middle := msgs[protectFirst : len(msgs)-protectLast]

	summary := summarizeMiddle(middle)
	out := make([]types.EyrieMessage, 0, len(head)+len(tail)+1)
	out = append(out, head...)
	out = append(out, types.EyrieMessage{
		Role:    "user",
		Content: "[compressed middle: " + summary + "]",
	})
	out = append(out, tail...)
	return out, true
}

// summarizeMiddle reduces the middle region to a compact one-line digest. It
// counts turns and tool calls and captures the last user request, which is the
// most task-relevant signal for a human checkpoint.
func summarizeMiddle(middle []types.EyrieMessage) string {
	turns := len(middle)
	toolCalls := 0
	lastUser := ""
	for _, m := range middle {
		toolCalls += len(m.ToolUse)
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			lastUser = strings.TrimSpace(m.Content)
		}
	}
	var b strings.Builder
	b.WriteString("summarized ")
	b.WriteString(itoa(turns))
	b.WriteString(" turns, ")
	b.WriteString(itoa(toolCalls))
	b.WriteString(" tool calls")
	if lastUser != "" {
		lastUser = strings.ReplaceAll(lastUser, "\n", " ")
		if len(lastUser) > 160 {
			lastUser = lastUser[:160] + "…"
		}
		b.WriteString("; latest request: ")
		b.WriteString(lastUser)
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
