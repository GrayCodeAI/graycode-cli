// This file re-exports symbols from the streaming sub-package so that existing
// callers of engine.ResponseCache, engine.NewResponseCache, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/streaming"

type CacheEntry = streaming.CacheEntry
type CacheStats = streaming.CacheStats
type ResponseCache = streaming.ResponseCache
type FormatRule = streaming.FormatRule
type FormattedResponse = streaming.FormattedResponse
type ResponseFormatter = streaming.ResponseFormatter
type StreamOptimizer = streaming.StreamOptimizer
type StreamStats = streaming.StreamStats
type ThinkingPhase = streaming.ThinkingPhase
type ThinkingStep = streaming.ThinkingStep
type ThinkingProtocol = streaming.ThinkingProtocol
type SteeringQueue = streaming.SteeringQueue
type SteeringMessage = streaming.SteeringMessage

const DefaultMaxEntries = streaming.DefaultMaxEntries

var NewResponseCache = streaming.NewResponseCache
var NewResponseFormatter = streaming.NewResponseFormatter
var NewStreamOptimizer = streaming.NewStreamOptimizer
var NewThinkingProtocol = streaming.NewThinkingProtocol
var NewSteeringQueue = streaming.NewSteeringQueue
var HashPrompt = streaming.HashPrompt
var ShouldCache = streaming.ShouldCache
var FixCodeFences = streaming.FixCodeFences
var RemoveFluff = streaming.RemoveFluff
var FixMarkdown = streaming.FixMarkdown
