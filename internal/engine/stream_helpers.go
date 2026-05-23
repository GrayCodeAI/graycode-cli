package engine

import (
	"strings"
	"time"
)

// toolTimeout returns a per-tool timeout duration based on the tool name.
// Fast file operations get a shorter deadline while Bash gets a longer one.
func toolTimeout(name string) time.Duration {
	switch name {
	case "Read", "Edit", "Write":
		return 30 * time.Second
	case "Bash":
		return 120 * time.Second
	default:
		return 60 * time.Second
	}
}

// isRetryableStreamError checks if a streaming error is transient and worth retrying.
func isRetryableStreamError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "broken pipe")
}

// shouldRemember returns true if the assistant response contains language that
// suggests a correction, learning, or noteworthy insight worth persisting.
func shouldRemember(content string) bool {
	// Require 2+ distinct trigger matches to avoid noise from common words.
	// Single words like "actually" or "don't" appear in normal conversation.
	triggers := []string{"actually", "correction", "instead", "don't", "mistake", "should have", "better approach", "wrong", "fix", "note to self"}
	lower := strings.ToLower(content)
	hits := 0
	for _, t := range triggers {
		if strings.Contains(lower, t) {
			hits++
			if hits >= 2 {
				return true
			}
		}
	}
	return false
}
