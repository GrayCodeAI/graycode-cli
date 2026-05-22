package engine

import "github.com/GrayCodeAI/hawk/internal/engine/code"

type CodeSnippet = code.CodeSnippet
type CodeContext = code.CodeContext
type ContextExtractor = code.ContextExtractor
type CodeLens = code.CodeLens
type LensGenerator = code.LensGenerator
type CodeLensProvider = code.CodeLensProvider
type CodeAction = code.CodeAction
type ActionDetector = code.ActionDetector
type ActionRule = code.ActionRule
type CodeExplanation = code.CodeExplanation
type ExplanationSection = code.ExplanationSection
type CodeExplainer = code.CodeExplainer

func NewContextExtractor(projectDir string, maxTokens int) *ContextExtractor {
	return code.NewContextExtractor(projectDir, maxTokens)
}
func FormatContext(ctx *CodeContext) string         { return code.FormatContext(ctx) }
func NewCodeLensProvider() *CodeLensProvider        { return code.NewCodeLensProvider() }
func NewActionDetector() *ActionDetector            { return code.NewActionDetector() }
func NewCodeExplainer() *CodeExplainer              { return code.NewCodeExplainer() }
func FormatExplanation(exp *CodeExplanation) string { return code.FormatExplanation(exp) }
func FormatSuggestions(actions []CodeAction, max int) string {
	return code.FormatSuggestions(actions, max)
}
func ApplyFix(action CodeAction, content string) (string, error) {
	return code.ApplyFix(action, content)
}
