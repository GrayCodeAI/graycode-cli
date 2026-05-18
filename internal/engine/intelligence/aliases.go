// Package intelligence is the Stage-1 namespace for intent classification, capabilities, language support, tool selection.
// See ../REFACTOR_PLAN.md.
package intelligence

import "github.com/GrayCodeAI/hawk/internal/engine"

type (
	Intent             = engine.Intent
	IntentRule         = engine.IntentRule
	ClassifiedInput    = engine.ClassifiedInput
	IntentClassifier   = engine.IntentClassifier
	Capability         = engine.Capability
	CapabilityRegistry = engine.CapabilityRegistry
	LanguageConfig     = engine.LanguageConfig
	LanguageRegistry   = engine.LanguageRegistry
	ToolInfo           = engine.ToolInfo
	ToolSelection      = engine.ToolSelection
	ToolSelector       = engine.ToolSelector
	CommandSuggestion  = engine.CommandSuggestion
	SuggestionRule     = engine.SuggestionRule
	SuggestionEngine   = engine.SuggestionEngine
)

var (
	NewIntentClassifier      = engine.NewIntentClassifier
	FormatIntent             = engine.FormatIntent
	NewCapabilityRegistry    = engine.NewCapabilityRegistry
	NewLanguageRegistry      = engine.NewLanguageRegistry
	FormatLanguages          = engine.FormatLanguages
	NewToolSelector          = engine.NewToolSelector
	FormatToolSelection      = engine.FormatToolSelection
	NewSuggestionEngine      = engine.NewSuggestionEngine
	FormatCommandSuggestions = engine.FormatCommandSuggestions
)
