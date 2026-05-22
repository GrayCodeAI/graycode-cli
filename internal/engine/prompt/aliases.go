// Package prompt provides prompt-construction and prompt-optimisation types.
package prompt

// Optimizer learns better prompts via DSPy-style example mining.
type Optimizer = PromptOptimizer

// DSPyExample is one (input, output) demonstration backing the optimizer.
type DSPyExample = PromptExample

// DSPyVariant is a candidate prompt being evaluated.
type DSPyVariant = PromptVariant

// ABTest pits two prompt variants against each other on incoming traffic.
type ABTest struct {
	A, B DSPyVariant
}

// Tuner is a lighter-weight prompt-tuning helper for online adjustments.
type Tuner = PromptTuner

// Variant is a tuned prompt the Tuner emits.
type Variant = PromptVariant

// NewOptimizer returns a fresh prompt optimizer.
func NewOptimizer() *Optimizer {
	return NewPromptOptimizer()
}

// NewABTest builds an A/B test between two variants.
func NewABTest(a, b DSPyVariant) *ABTest {
	return &ABTest{A: a, B: b}
}

// NewTuner returns a fresh prompt tuner.
func NewTuner() *Tuner {
	return NewPromptTuner()
}
