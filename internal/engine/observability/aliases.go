// Package observability is the Stage-1 namespace for profiling, debug recording, structured logging, feedback.
// See ../REFACTOR_PLAN.md.
package observability

import "github.com/GrayCodeAI/hawk/internal/engine"

type (
	Profiler          = engine.Profiler
	ProfileSpan       = engine.ProfileSpan
	Counter           = engine.Counter
	Timer             = engine.Timer
	DebugSession      = engine.DebugSession
	DebugStep         = engine.DebugStep
	Hypothesis        = engine.Hypothesis
	DebugRecorder     = engine.DebugRecorder
	LogLevel          = engine.LogLevel
	LogEntry          = engine.LogEntry
	StructuredLogger  = engine.StructuredLogger
	AgentLogger       = engine.AgentLogger
	RotatingWriter    = engine.RotatingWriter
	Feedback          = engine.Feedback
	FeedbackCollector = engine.FeedbackCollector
	ImplicitSignal    = engine.ImplicitSignal
)

var (
	NewProfiler          = engine.NewProfiler
	NewDebugRecorder     = engine.NewDebugRecorder
	NewStructuredLogger  = engine.NewStructuredLogger
	ParseLevel           = engine.ParseLevel
	NewRotatingWriter    = engine.NewRotatingWriter
	NewFeedbackCollector = engine.NewFeedbackCollector
)
