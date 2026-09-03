package engine

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/intelligence/memory"
	"github.com/GrayCodeAI/graycode-cli/internal/resilience/retry"
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

// isRetryableStreamError reports whether a streaming error is worth retrying at
// the stream layer. Network blips (connection reset / EOF) count, as do HTTP
// 429 (rate limited) and 503 (service unavailable) surfaced by the provider —
// the latter two may carry a Retry-After hint (see streamRetryDelay).
func isRetryableStreamError(err error) bool {
	if err == nil {
		return false
	}
	if retry.IsRetryable(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "eof")
}

// retryDelayRe recognises a "retry in|after <num> [ms]" hint embedded in an
// error or provider message, so the stream retry can honor an explicit
// Retry-After instead of always falling back to a fixed 1–3s delay.
var retryDelayRe = regexp.MustCompile(`(?i)(?:retry|try again)\s+(?:in|after)\s+(\d+(?:\.\d+)?)\s*(ms|milliseconds?|s|seconds?)`)

// maxStreamRetryDelay caps a parsed Retry-After value so a misbehaving or
// hostile provider cannot stall a session indefinitely.
const maxStreamRetryDelay = 60 * time.Second

// streamRetryDelay picks the retry delay for a stream error. When the error
// carries an explicit "retry after Ns" hint (e.g. an HTTP 429 from the
// provider), that value is honored — capped at maxStreamRetryDelay — instead
// of the default linear 1–3s backoff.
func streamRetryDelay(err error, attempt int) time.Duration {
	if err != nil {
		if parsed := parseRetryDelayHint(err.Error()); parsed > 0 {
			if parsed > maxStreamRetryDelay {
				return maxStreamRetryDelay
			}
			return parsed
		}
	}
	d := time.Duration(attempt+1) * time.Second
	if d > maxStreamRetryDelay {
		return maxStreamRetryDelay
	}
	return d
}

// parseRetryDelayHint extracts a delay hint from an error message, returning 0
// when no hint is present (mirroring eyrie's parseRetryDelay, but kept local so
// the engine package does not reach past the eyrie engine facade boundary).
func parseRetryDelayHint(errMsg string) time.Duration {
	m := retryDelayRe.FindStringSubmatch(errMsg)
	if m == nil {
		return 0
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	switch {
	case len(m[2]) > 0 && m[2][0] == 'm':
		return time.Duration(val * float64(time.Millisecond))
	default:
		return time.Duration(val * float64(time.Second))
	}
}

// shouldRemember returns true if the assistant response contains language that
// suggests a correction, learning, or noteworthy insight worth persisting.
func shouldRemember(content string) bool {
	return memory.ShouldAutoRemember(content)
}
