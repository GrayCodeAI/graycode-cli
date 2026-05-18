// Package review is the Stage-1 namespace for self-review / critique / quality
// scoring types in package engine. See ../REFACTOR_PLAN.md.
package review

import (
	"context"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// LLMClient is the minimal interface review components use to call models.
type LLMClient = engine.LLMClient

// Bot is the rule-driven review bot for diffs.
type Bot = engine.ReviewBot

// Rule is a single check in a Bot's rule set.
type Rule = engine.ReviewRule

// Comment is one finding emitted by a Bot.
type Comment = engine.ReviewComment

// Report aggregates Comments for a single review run.
type Report = engine.ReviewReport

// PatchVerdict is the Critic's accept/reject decision on a patch.
type PatchVerdict = engine.PatchVerdict

// Critic is the LLM-driven patch reviewer.
type Critic = engine.Critic

// Assessment is a structured self-review of an in-progress task.
type Assessment = engine.Assessment

// SelfAssessor produces Assessments mid-loop.
type SelfAssessor = engine.SelfAssessor

// TaskContext is the input to a SelfAssessor.
type TaskContext = engine.TaskContext

// SelfReviewResult is the output of ReviewBeforeWrite.
type SelfReviewResult = engine.SelfReviewResult

// ConfidenceThreshold is the minimum confidence at which self-review
// approves a write without asking for human input.
const ConfidenceThreshold = engine.ConfidenceThreshold

// ConsensusSampler runs N samples of an LLM prompt and reduces them.
type ConsensusSampler = engine.ConsensusSampler

// Sample is one raw LLM sample from a ConsensusSampler.
type Sample = engine.Sample

// ConsensusResult is the reduced output of N samples.
type ConsensusResult = engine.ConsensusResult

// QualityScorer ranks candidate responses against a rubric.
type QualityScorer = engine.QualityScorer

// ScoreWeights configures a QualityScorer.
type ScoreWeights = engine.ScoreWeights

// ScoredResponse pairs a candidate with its score breakdown.
type ScoredResponse = engine.ScoredResponse

// ResponseContext is the input the scorer scores against.
type ResponseContext = engine.ResponseContext

// SolutionReviewer evaluates multiple proposed solutions and picks one.
type SolutionReviewer = engine.SolutionReviewer

// Solution is a candidate solution submitted to a SolutionReviewer.
type Solution = engine.Solution

// ReviewResult is the SolutionReviewer's verdict.
type ReviewResult = engine.ReviewResult

// NewBot returns a fresh review bot with the default rule set.
func NewBot() *Bot { return engine.NewReviewBot() }

// NewCritic returns a Critic that uses the given model name.
func NewCritic(model string) *Critic { return engine.NewCritic(model) }

// NewSelfAssessor returns a fresh self-assessor.
func NewSelfAssessor() *SelfAssessor { return engine.NewSelfAssessor() }

// NewConsensusSampler returns a sampler configured for numSamples draws.
func NewConsensusSampler(numSamples int) *ConsensusSampler {
	return engine.NewConsensusSampler(numSamples)
}

// NewQualityScorer returns a scorer with default weights.
func NewQualityScorer() *QualityScorer { return engine.NewQualityScorer() }

// DefaultWeights returns the default scoring weights for QualityScorer.
func DefaultWeights() ScoreWeights { return engine.DefaultWeights() }

// NewSolutionReviewer returns a reviewer capped at maxAttempts iterations.
func NewSolutionReviewer(maxAttempts int) *SolutionReviewer {
	return engine.NewSolutionReviewer(maxAttempts)
}

// ReviewBeforeWrite runs an LLM-driven self-review on a candidate write.
func ReviewBeforeWrite(ctx context.Context, llm LLMClient, model,
	intent, filePath, oldContent, newContent string,
) (*SelfReviewResult, error) {
	return engine.ReviewBeforeWrite(ctx, llm, model, intent, filePath, oldContent, newContent)
}
