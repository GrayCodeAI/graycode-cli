// This file re-exports symbols from the prompt sub-package so that existing
// callers of engine.PromptOptimizer, engine.NewPromptOptimizer, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/prompt"

type (
	PromptOptimizer       = prompt.PromptOptimizer
	PromptParameter       = prompt.PromptParameter
	PromptGradient        = prompt.PromptGradient
	OptimizationStep      = prompt.OptimizationStep
	PromptFewShotSelector = prompt.PromptFewShotSelector
	PromptExample         = prompt.PromptExample
	PromptTuner           = prompt.PromptTuner
	PromptVariant         = prompt.PromptVariant
)

var (
	NewPromptOptimizer    = prompt.NewPromptOptimizer
	NewPromptTuner        = prompt.NewPromptTuner
	ComputeGradientPrompt = prompt.ComputeGradientPrompt
	FormatPromptExamples  = prompt.FormatPromptExamples
)
