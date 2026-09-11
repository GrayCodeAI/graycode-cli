package engine

import "github.com/GrayCodeAI/hawk/internal/engine/compression"

type (
	Timeline            = compression.Timeline
	TimelineEvent       = compression.TimelineEvent
	SessionCompressor   = compression.SessionCompressor
	CompressStrategy    = compression.CompressStrategy
	CompressMessage     = compression.CompressMessage
	CompressedBlock     = compression.CompressedBlock
	CompressionResult   = compression.CompressionResult
	CrossSessionLearner = compression.CrossSessionLearner
	Insight             = compression.Insight
	FailurePattern      = compression.FailurePattern
	SessionConvention   = compression.SessionConvention
	LearnerStats        = compression.LearnerStats
)

func NewTimeline(sessionID string) *Timeline { return compression.NewTimeline(sessionID) }
func NewSessionCompressor(strategy CompressStrategy) *SessionCompressor {
	return compression.NewSessionCompressor(strategy)
}

func NewCrossSessionLearner(dir string) *CrossSessionLearner {
	return compression.NewCrossSessionLearner(dir)
}
