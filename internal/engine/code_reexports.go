package engine

import "github.com/GrayCodeAI/hawk/internal/engine/code"

type (
	CodeSnippet        = code.CodeSnippet
	CodeContext        = code.CodeContext
	ContextExtractor   = code.ContextExtractor
	CodeLens           = code.CodeLens
	LensGenerator      = code.LensGenerator
	CodeLensProvider   = code.CodeLensProvider
	CodeAction         = code.CodeAction
	ActionDetector     = code.ActionDetector
	ActionRule         = code.ActionRule
	CodeExplanation    = code.CodeExplanation
	ExplanationSection = code.ExplanationSection
	CodeExplainer      = code.CodeExplainer
)

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
