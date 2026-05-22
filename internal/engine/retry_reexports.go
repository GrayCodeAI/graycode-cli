// This file re-exports symbols from the retry sub-package so that existing
// callers of engine.RetryItem, engine.NewRetryQueue, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/retry"

type (
	RetryItem     = retry.RetryItem
	RetryQueue    = retry.RetryQueue
	SmartRetry    = retry.SmartRetry
	RetryStrategy = retry.RetryStrategy
	FailureRecord = retry.FailureRecord
	RetryDecision = retry.RetryDecision
)

var (
	NewRetryQueue = retry.NewRetryQueue
	NewSmartRetry = retry.NewSmartRetry
)
