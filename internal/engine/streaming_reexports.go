// This file re-exports symbols from the streaming sub-package so that existing
// callers of engine.ResponseCache, engine.NewResponseCache, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/streaming"

type (
	CacheEntry        = streaming.CacheEntry
	CacheStats        = streaming.CacheStats
	ResponseCache     = streaming.ResponseCache
	FormatRule        = streaming.FormatRule
	FormattedResponse = streaming.FormattedResponse
	ResponseFormatter = streaming.ResponseFormatter
	StreamOptimizer   = streaming.StreamOptimizer
	StreamStats       = streaming.StreamStats
	ThinkingPhase     = streaming.ThinkingPhase
	ThinkingStep      = streaming.ThinkingStep
	ThinkingProtocol  = streaming.ThinkingProtocol
	SteeringQueue     = streaming.SteeringQueue
	SteeringMessage   = streaming.SteeringMessage
)

const DefaultMaxEntries = streaming.DefaultMaxEntries

var (
	NewResponseCache     = streaming.NewResponseCache
	NewResponseFormatter = streaming.NewResponseFormatter
	NewStreamOptimizer   = streaming.NewStreamOptimizer
	NewThinkingProtocol  = streaming.NewThinkingProtocol
	NewSteeringQueue     = streaming.NewSteeringQueue
	HashPrompt           = streaming.HashPrompt
	ShouldCache          = streaming.ShouldCache
	FixCodeFences        = streaming.FixCodeFences
	RemoveFluff          = streaming.RemoveFluff
	FixMarkdown          = streaming.FixMarkdown
)
