package engine

import (
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/resilience/retry"
)

// truncate shortens a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

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
	if retry.IsRetryable(err) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "EOF")
}

// shouldRemember returns true if the assistant response contains language that
// suggests a correction, learning, or noteworthy insight worth persisting.
func shouldRemember(content string) bool {
	return memory.ShouldAutoRemember(content)
}
