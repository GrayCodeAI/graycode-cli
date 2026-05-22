// This file re-exports symbols from the retry sub-package so that existing
// callers of engine.RetryItem, engine.NewRetryQueue, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/retry"

type RetryItem = retry.RetryItem
type RetryQueue = retry.RetryQueue
type SmartRetry = retry.SmartRetry
type RetryStrategy = retry.RetryStrategy
type FailureRecord = retry.FailureRecord
type RetryDecision = retry.RetryDecision

var NewRetryQueue = retry.NewRetryQueue
var NewSmartRetry = retry.NewSmartRetry
