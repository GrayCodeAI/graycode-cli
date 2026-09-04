// This file re-exports symbols from the intelligence sub-package so that existing
// callers of engine.Intent, engine.NewIntentClassifier, etc. keep compiling
// during the Stage 2 migration. See docs/plans/engine-refactor-plan.md.
package engine

import "github.com/GrayCodeAI/graycode-cli/internal/engine/intelligence"

type (
	Intent             = intelligence.Intent
	IntentRule         = intelligence.IntentRule
	ClassifiedInput    = intelligence.ClassifiedInput
	IntentClassifier   = intelligence.IntentClassifier
	Capability         = intelligence.Capability
	CapabilityRegistry = intelligence.CapabilityRegistry
	LanguageConfig     = intelligence.LanguageConfig
	LanguageRegistry   = intelligence.LanguageRegistry
	ToolInfo           = intelligence.ToolInfo
	ToolSelection      = intelligence.ToolSelection
	ToolSelector       = intelligence.ToolSelector
	CommandSuggestion  = intelligence.CommandSuggestion
	SuggestionRule     = intelligence.SuggestionRule
	SuggestionEngine   = intelligence.SuggestionEngine
)

var (
	NewIntentClassifier      = intelligence.NewIntentClassifier
	FormatIntent             = intelligence.FormatIntent
	NewCapabilityRegistry    = intelligence.NewCapabilityRegistry
	NewLanguageRegistry      = intelligence.NewLanguageRegistry
	FormatLanguages          = intelligence.FormatLanguages
	NewToolSelector          = intelligence.NewToolSelector
	FormatToolSelection      = intelligence.FormatToolSelection
	NewSuggestionEngine      = intelligence.NewSuggestionEngine
	FormatCommandSuggestions = intelligence.FormatCommandSuggestions
)
