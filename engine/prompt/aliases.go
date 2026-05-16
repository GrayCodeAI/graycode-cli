// Package prompt is the Stage-1 namespace for prompt-construction and
// prompt-optimisation types in package engine. See ../REFACTOR_PLAN.md.
package prompt

import "github.com/GrayCodeAI/hawk/engine"

// Optimizer learns better prompts via DSPy-style example mining.
type Optimizer = engine.PromptOptimizer

// DSPyExample is one (input, output) demonstration backing the optimizer.
type DSPyExample = engine.DSPyExample

// DSPyVariant is a candidate prompt being evaluated.
type DSPyVariant = engine.DSPyVariant

// ABTest pits two prompt variants against each other on incoming traffic.
type ABTest = engine.ABTest

// Tuner is a lighter-weight prompt-tuning helper for online adjustments.
type Tuner = engine.PromptTuner

// Variant is a tuned prompt the Tuner emits.
type Variant = engine.PromptVariant

// NewOptimizer returns a fresh prompt optimizer.
func NewOptimizer() *Optimizer {
	return engine.NewPromptOptimizer()
}

// NewABTest builds an A/B test between two variants.
func NewABTest(a, b DSPyVariant) *ABTest {
	return engine.NewABTest(a, b)
}

// NewTuner returns a fresh prompt tuner.
func NewTuner() *Tuner {
	return engine.NewPromptTuner()
}
