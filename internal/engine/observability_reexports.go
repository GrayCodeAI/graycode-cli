// This file re-exports symbols from the observability sub-package so that existing
// callers of engine.Profiler, engine.NewProfiler, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/observability"

type (
	Profiler          = observability.Profiler
	ProfileSpan       = observability.ProfileSpan
	Counter           = observability.Counter
	Timer             = observability.Timer
	DebugSession      = observability.DebugSession
	DebugStep         = observability.DebugStep
	Hypothesis        = observability.Hypothesis
	DebugRecorder     = observability.DebugRecorder
	LogLevel          = observability.LogLevel
	LogEntry          = observability.LogEntry
	StructuredLogger  = observability.StructuredLogger
	AgentLogger       = observability.AgentLogger
	RotatingWriter    = observability.RotatingWriter
	Feedback          = observability.Feedback
	FeedbackCollector = observability.FeedbackCollector
	ImplicitSignal    = observability.ImplicitSignal
)

var (
	NewProfiler          = observability.NewProfiler
	NewDebugRecorder     = observability.NewDebugRecorder
	NewStructuredLogger  = observability.NewStructuredLogger
	ParseLevel           = observability.ParseLevel
	NewRotatingWriter    = observability.NewRotatingWriter
	NewFeedbackCollector = observability.NewFeedbackCollector
)
