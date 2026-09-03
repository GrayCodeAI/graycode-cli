package engine

import "github.com/GrayCodeAI/graycode-cli/internal/engine/review"

// Types from review sub-package.

type (
	ReviewBot        = review.ReviewBot
	ReviewRule       = review.ReviewRule
	ReviewComment    = review.ReviewComment
	ReviewReport     = review.ReviewReport
	PatchVerdict     = review.PatchVerdict
	Critic           = review.Critic
	Assessment       = review.Assessment
	SelfAssessor     = review.SelfAssessor
	TaskContext      = review.TaskContext
	ConsensusSampler = review.ConsensusSampler
	Sample           = review.Sample
	ConsensusResult  = review.ConsensusResult
	QualityScorer    = review.QualityScorer
	ScoreWeights     = review.ScoreWeights
	ScoredResponse   = review.ScoredResponse
	ResponseContext  = review.ResponseContext
	SolutionReviewer = review.SolutionReviewer
	Solution         = review.Solution
	ReviewResult     = review.ReviewResult
)

// Short-name aliases.

type (
	Bot     = review.ReviewBot
	Rule    = review.ReviewRule
	Comment = review.ReviewComment
	Report  = review.ReviewReport
)

// Functions.

var (
	NewReviewBot         = review.NewReviewBot
	NewCritic            = review.NewCritic
	NewSelfAssessor      = review.NewSelfAssessor
	NewConsensusSampler  = review.NewConsensusSampler
	NewQualityScorer     = review.NewQualityScorer
	NewSolutionReviewer  = review.NewSolutionReviewer
	DefaultWeights       = review.DefaultWeights
	FormatSelfAssessment = review.FormatSelfAssessment
	FormatConsensus      = review.FormatConsensus
	CompareApproaches    = review.CompareApproaches
	FormatReview         = review.FormatReview
	ShouldRetry          = review.ShouldRetry
	FormatReport         = review.FormatReport
	FormatInline         = review.FormatInline
	FilterBySeverity     = review.FilterBySeverity
)

// NewBot delegates to review.NewBot.
func NewBot() *Bot { return review.NewBot() }

// ReviewBeforeWrite, LLMClient, SelfReviewResult, ConfidenceThreshold
// are defined directly in package engine (self_review.go, client_interface.go).
