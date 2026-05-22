package engine

import "github.com/GrayCodeAI/hawk/internal/engine/session"

type Timeline = session.Timeline
type TimelineEvent = session.TimelineEvent
type SessionCompressor = session.SessionCompressor
type CompressStrategy = session.CompressStrategy
type CompressMessage = session.CompressMessage
type CompressedBlock = session.CompressedBlock
type CompressionResult = session.CompressionResult
type CrossSessionLearner = session.CrossSessionLearner
type Insight = session.Insight
type FailurePattern = session.FailurePattern
type SessionConvention = session.SessionConvention
type LearnerStats = session.LearnerStats

func NewTimeline(sessionID string) *Timeline { return session.NewTimeline(sessionID) }
func NewSessionCompressor(strategy CompressStrategy) *SessionCompressor {
	return session.NewSessionCompressor(strategy)
}
func NewCrossSessionLearner(dir string) *CrossSessionLearner {
	return session.NewCrossSessionLearner(dir)
}
