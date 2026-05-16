// Package observability is the Stage-1 namespace for profiling, debug recording, structured logging, feedback.
// See ../REFACTOR_PLAN.md.
package observability

import "github.com/GrayCodeAI/hawk/engine"

type Profiler = engine.Profiler
type ProfileSpan = engine.ProfileSpan
type Counter = engine.Counter
type Timer = engine.Timer
type DebugSession = engine.DebugSession
type DebugStep = engine.DebugStep
type Hypothesis = engine.Hypothesis
type DebugRecorder = engine.DebugRecorder
type LogLevel = engine.LogLevel
type LogEntry = engine.LogEntry
type StructuredLogger = engine.StructuredLogger
type AgentLogger = engine.AgentLogger
type RotatingWriter = engine.RotatingWriter
type Feedback = engine.Feedback
type FeedbackCollector = engine.FeedbackCollector
type ImplicitSignal = engine.ImplicitSignal

var NewProfiler = engine.NewProfiler
var NewDebugRecorder = engine.NewDebugRecorder
var NewStructuredLogger = engine.NewStructuredLogger
var ParseLevel = engine.ParseLevel
var NewRotatingWriter = engine.NewRotatingWriter
var NewFeedbackCollector = engine.NewFeedbackCollector
