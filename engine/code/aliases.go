// Package code is the Stage-1 namespace for code-aware features
// (context extraction, lenses, actions, explainer). See ../REFACTOR_PLAN.md.
package code

import "github.com/GrayCodeAI/hawk/engine"

type Snippet = engine.CodeSnippet
type Context = engine.CodeContext
type ContextExtractor = engine.ContextExtractor
type Lens = engine.CodeLens
type LensGenerator = engine.LensGenerator
type LensProvider = engine.CodeLensProvider
type Action = engine.CodeAction
type ActionDetector = engine.ActionDetector
type ActionRule = engine.ActionRule
type Explanation = engine.CodeExplanation
type ExplanationSection = engine.ExplanationSection
type Explainer = engine.CodeExplainer

func NewContextExtractor(projectDir string, maxTokens int) *ContextExtractor {
	return engine.NewContextExtractor(projectDir, maxTokens)
}
func FormatContext(ctx *Context) string                    { return engine.FormatContext(ctx) }
func NewLensProvider() *LensProvider                       { return engine.NewCodeLensProvider() }
func NewActionDetector() *ActionDetector                   { return engine.NewActionDetector() }
func NewExplainer() *Explainer                             { return engine.NewCodeExplainer() }
func FormatExplanation(exp *Explanation) string            { return engine.FormatExplanation(exp) }
func FormatSuggestions(actions []Action, max int) string   { return engine.FormatSuggestions(actions, max) }
func ApplyFix(action Action, content string) (string, error) { return engine.ApplyFix(action, content) }
