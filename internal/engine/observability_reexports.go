// This file re-exports symbols from the observability sub-package so that existing
// callers of engine.Profiler, engine.NewProfiler, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/observability"

type Profiler = observability.Profiler
type ProfileSpan = observability.ProfileSpan
type Counter = observability.Counter
type Timer = observability.Timer
type DebugSession = observability.DebugSession
type DebugStep = observability.DebugStep
type Hypothesis = observability.Hypothesis
type DebugRecorder = observability.DebugRecorder
type LogLevel = observability.LogLevel
type LogEntry = observability.LogEntry
type StructuredLogger = observability.StructuredLogger
type AgentLogger = observability.AgentLogger
type RotatingWriter = observability.RotatingWriter
type Feedback = observability.Feedback
type FeedbackCollector = observability.FeedbackCollector
type ImplicitSignal = observability.ImplicitSignal

var NewProfiler = observability.NewProfiler
var NewDebugRecorder = observability.NewDebugRecorder
var NewStructuredLogger = observability.NewStructuredLogger
var ParseLevel = observability.ParseLevel
var NewRotatingWriter = observability.NewRotatingWriter
var NewFeedbackCollector = observability.NewFeedbackCollector
