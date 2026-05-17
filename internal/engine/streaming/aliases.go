// Package streaming is the Stage-1 namespace for response caching,
// formatting, stream optimisation, thinking protocol, and steering.
// See ../REFACTOR_PLAN.md.
package streaming

import (
	"time"

	"github.com/GrayCodeAI/hawk/engine"
)

type (
	CacheEntry        = engine.CacheEntry
	CacheStats        = engine.CacheStats
	ResponseCache     = engine.ResponseCache
	FormatRule        = engine.FormatRule
	FormattedResponse = engine.FormattedResponse
	ResponseFormatter = engine.ResponseFormatter
	StreamOptimizer   = engine.StreamOptimizer
	StreamStats       = engine.StreamStats
	ThinkingPhase     = engine.ThinkingPhase
	ThinkingStep      = engine.ThinkingStep
	ThinkingProtocol  = engine.ThinkingProtocol
	SteeringQueue     = engine.SteeringQueue
	SteeringMessage   = engine.SteeringMessage
)

const DefaultMaxEntries = engine.DefaultMaxEntries

func NewResponseCache(maxEntries int, maxAge time.Duration) *ResponseCache {
	return engine.NewResponseCache(maxEntries, maxAge)
}
func NewResponseFormatter() *ResponseFormatter { return engine.NewResponseFormatter() }
func NewStreamOptimizer() *StreamOptimizer     { return engine.NewStreamOptimizer() }
func NewThinkingProtocol() *ThinkingProtocol   { return engine.NewThinkingProtocol() }
func NewSteeringQueue() *SteeringQueue         { return engine.NewSteeringQueue() }
func HashPrompt(prompt string) string          { return engine.HashPrompt(prompt) }
func ShouldCache(prompt string) bool           { return engine.ShouldCache(prompt) }
func FixCodeFences(text string) string         { return engine.FixCodeFences(text) }
func RemoveFluff(text string) string           { return engine.RemoveFluff(text) }
func FixMarkdown(text string) string           { return engine.FixMarkdown(text) }
