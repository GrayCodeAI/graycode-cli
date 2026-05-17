package session

import (
	"fmt"
	"strings"
)

// Diff compares two sessions and returns the differences as a human-readable string.
func Diff(a, b *Session) string {
	if a == nil && b == nil {
		return "Both sessions are nil."
	}
	if a == nil {
		return "First session is nil."
	}
	if b == nil {
		return "Second session is nil."
	}

	var out strings.Builder

	out.WriteString(fmt.Sprintf("Session A: %s (%d messages)\n", a.ID, len(a.Messages)))
	out.WriteString(fmt.Sprintf("Session B: %s (%d messages)\n", b.ID, len(b.Messages)))

	if a.Model != b.Model {
		out.WriteString(fmt.Sprintf("Model: %s -> %s\n", a.Model, b.Model))
	}
	if a.Provider != b.Provider {
		out.WriteString(fmt.Sprintf("Provider: %s -> %s\n", a.Provider, b.Provider))
	}

	out.WriteString("\n")

	// Find the common prefix length
	commonLen := len(a.Messages)
	if len(b.Messages) < commonLen {
		commonLen = len(b.Messages)
	}

	divergeAt := -1
	for i := 0; i < commonLen; i++ {
		if a.Messages[i].Role != b.Messages[i].Role || a.Messages[i].Content != b.Messages[i].Content {
			divergeAt = i
			break
		}
	}

	if divergeAt < 0 && len(a.Messages) == len(b.Messages) {
		out.WriteString("Sessions are identical.\n")
		return out.String()
	}

	if divergeAt < 0 {
		divergeAt = commonLen
	}

	out.WriteString(fmt.Sprintf("Common messages: %d\n", divergeAt))
	out.WriteString(fmt.Sprintf("Diverge at index: %d\n\n", divergeAt))

	// Show messages unique to A
	if divergeAt < len(a.Messages) {
		out.WriteString(fmt.Sprintf("--- Session A (messages %d..%d) ---\n", divergeAt, len(a.Messages)-1))
		for i := divergeAt; i < len(a.Messages); i++ {
			msg := a.Messages[i]
			preview := truncatePreview(msg.Content, 100)
			out.WriteString(fmt.Sprintf("  [%d] %s: %s\n", i, msg.Role, preview))
		}
		out.WriteString("\n")
	}

	// Show messages unique to B
	if divergeAt < len(b.Messages) {
		out.WriteString(fmt.Sprintf("+++ Session B (messages %d..%d) +++\n", divergeAt, len(b.Messages)-1))
		for i := divergeAt; i < len(b.Messages); i++ {
			msg := b.Messages[i]
			preview := truncatePreview(msg.Content, 100)
			out.WriteString(fmt.Sprintf("  [%d] %s: %s\n", i, msg.Role, preview))
		}
	}

	return out.String()
}
