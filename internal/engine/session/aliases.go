// Package session is the Stage-1 namespace for session-lifecycle types in
// package engine. See ../REFACTOR_PLAN.md.
package session

import "github.com/GrayCodeAI/hawk/internal/engine"

// Services bundles the dependencies a session uses (provider, tools, memory).
type Services = engine.SessionServices

// ServiceOption configures a Services bundle.
type ServiceOption = engine.ServiceOption

// Timeline records significant session events for replay/debugging.
type Timeline = engine.Timeline

// TimelineEvent is one entry in a Timeline.
type TimelineEvent = engine.TimelineEvent

// Compressor reduces a session's message history while preserving meaning.
type Compressor = engine.SessionCompressor

// CompressStrategy names a compression algorithm.
type CompressStrategy = engine.CompressStrategy

// CompressMessage is the per-message input to compression.
type CompressMessage = engine.CompressMessage

// CompressedBlock is one block of compressed history.
type CompressedBlock = engine.CompressedBlock

// CompressionResult is what a Compressor returns.
type CompressionResult = engine.CompressionResult

// CrossSessionLearner mines insights from past sessions to inform new ones.
type CrossSessionLearner = engine.CrossSessionLearner

// Insight is one cross-session learning, e.g. "tests fail on Linux".
type Insight = engine.Insight

// FailurePattern is a recurring failure mode the learner has seen.
type FailurePattern = engine.FailurePattern

// SessionConvention is a project-specific habit (naming, layout, etc.).
type SessionConvention = engine.SessionConvention

// LearnerStats summarises cross-session learner state.
type LearnerStats = engine.LearnerStats

// NewTimeline returns a new timeline scoped to sessionID.
func NewTimeline(sessionID string) *Timeline { return engine.NewTimeline(sessionID) }

// NewCompressor returns a session compressor using the named strategy.
func NewCompressor(strategy CompressStrategy) *Compressor {
	return engine.NewSessionCompressor(strategy)
}

// NewCrossSessionLearner returns a learner that persists state under dir.
func NewCrossSessionLearner(dir string) *CrossSessionLearner {
	return engine.NewCrossSessionLearner(dir)
}
