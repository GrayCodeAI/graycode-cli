// Package retry provides retry-queue and smart-retry types for the
// engine package. See ../REFACTOR_PLAN.md.
//
// Note: hawk also has a top-level `github.com/GrayCodeAI/hawk/internal/resilience/retry` package
// for low-level HTTP/transport retry. This sub-package is specifically the
// engine's higher-level retry queue (work items deferred for later attempt).
package retry

// Item is a single deferred work item awaiting retry.
type Item = RetryItem

// Queue is the FIFO of pending retry items with backoff and dedup.
type Queue = RetryQueue

// NewQueue returns an empty retry queue with default backoff settings.
func NewQueue() *Queue {
	return NewRetryQueue()
}
