package engine

import "github.com/GrayCodeAI/hawk/internal/engine/session"

type (
	Timeline            = session.Timeline
	TimelineEvent       = session.TimelineEvent
	SessionCompressor   = session.SessionCompressor
	CompressStrategy    = session.CompressStrategy
	CompressMessage     = session.CompressMessage
	CompressedBlock     = session.CompressedBlock
	CompressionResult   = session.CompressionResult
	CrossSessionLearner = session.CrossSessionLearner
	Insight             = session.Insight
	FailurePattern      = session.FailurePattern
	SessionConvention   = session.SessionConvention
	LearnerStats        = session.LearnerStats
)

func NewTimeline(sessionID string) *Timeline { return session.NewTimeline(sessionID) }
func NewSessionCompressor(strategy CompressStrategy) *SessionCompressor {
	return session.NewSessionCompressor(strategy)
}

func NewCrossSessionLearner(dir string) *CrossSessionLearner {
	return session.NewCrossSessionLearner(dir)
}
