package engine

import "github.com/GrayCodeAI/hawk/internal/engine/review"

// Types from review sub-package.

type ReviewBot = review.ReviewBot
type ReviewRule = review.ReviewRule
type ReviewComment = review.ReviewComment
type ReviewReport = review.ReviewReport
type PatchVerdict = review.PatchVerdict
type Critic = review.Critic
type Assessment = review.Assessment
type SelfAssessor = review.SelfAssessor
type TaskContext = review.TaskContext
type ConsensusSampler = review.ConsensusSampler
type Sample = review.Sample
type ConsensusResult = review.ConsensusResult
type QualityScorer = review.QualityScorer
type ScoreWeights = review.ScoreWeights
type ScoredResponse = review.ScoredResponse
type ResponseContext = review.ResponseContext
type SolutionReviewer = review.SolutionReviewer
type Solution = review.Solution
type ReviewResult = review.ReviewResult

// Short-name aliases.

type Bot = review.ReviewBot
type Rule = review.ReviewRule
type Comment = review.ReviewComment
type Report = review.ReviewReport

// Functions.

var NewReviewBot = review.NewReviewBot
var NewCritic = review.NewCritic
var NewSelfAssessor = review.NewSelfAssessor
var NewConsensusSampler = review.NewConsensusSampler
var NewQualityScorer = review.NewQualityScorer
var NewSolutionReviewer = review.NewSolutionReviewer
var DefaultWeights = review.DefaultWeights
var FormatSelfAssessment = review.FormatSelfAssessment
var FormatConsensus = review.FormatConsensus
var CompareApproaches = review.CompareApproaches
var FormatReview = review.FormatReview
var ShouldRetry = review.ShouldRetry
var FormatReport = review.FormatReport
var FormatInline = review.FormatInline
var FilterBySeverity = review.FilterBySeverity

// NewBot delegates to review.NewBot.
func NewBot() *Bot { return review.NewBot() }

// ReviewBeforeWrite, LLMClient, SelfReviewResult, ConfidenceThreshold
// are defined directly in package engine (self_review.go, client_interface.go).
