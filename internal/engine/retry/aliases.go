// Package retry is the Stage-1 namespace for retry-queue types in
// package engine. See ../REFACTOR_PLAN.md.
//
// New code in hawk should import this package instead of reaching into
// engine for retry symbols. Implementation will move here in Stage 2.
//
// Note: hawk also has a top-level `github.com/GrayCodeAI/hawk/internal/resilience/retry` package
// for low-level HTTP/transport retry. This sub-package is specifically the
// engine's higher-level retry queue (work items deferred for later attempt).
package retry

import "github.com/GrayCodeAI/hawk/internal/engine"

// Item is a single deferred work item awaiting retry.
type Item = engine.RetryItem

// Queue is the FIFO of pending retry items with backoff and dedup.
type Queue = engine.RetryQueue

// NewQueue returns an empty retry queue with default backoff settings.
func NewQueue() *Queue {
	return engine.NewRetryQueue()
}
