// This file re-exports symbols from the prompt sub-package so that existing
// callers of engine.PromptOptimizer, engine.NewPromptOptimizer, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/prompt"

type PromptOptimizer = prompt.PromptOptimizer
type PromptParameter = prompt.PromptParameter
type PromptGradient = prompt.PromptGradient
type OptimizationStep = prompt.OptimizationStep
type PromptFewShotSelector = prompt.PromptFewShotSelector
type PromptExample = prompt.PromptExample
type PromptTuner = prompt.PromptTuner
type PromptVariant = prompt.PromptVariant

var NewPromptOptimizer = prompt.NewPromptOptimizer
var NewPromptTuner = prompt.NewPromptTuner
var ComputeGradientPrompt = prompt.ComputeGradientPrompt
var FormatPromptExamples = prompt.FormatPromptExamples
