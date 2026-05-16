// Package intelligence is the Stage-1 namespace for intent classification, capabilities, language support, tool selection.
// See ../REFACTOR_PLAN.md.
package intelligence

import "github.com/GrayCodeAI/hawk/engine"

type Intent = engine.Intent
type IntentRule = engine.IntentRule
type ClassifiedInput = engine.ClassifiedInput
type IntentClassifier = engine.IntentClassifier
type Capability = engine.Capability
type CapabilityRegistry = engine.CapabilityRegistry
type LanguageConfig = engine.LanguageConfig
type LanguageRegistry = engine.LanguageRegistry
type ToolInfo = engine.ToolInfo
type ToolSelection = engine.ToolSelection
type ToolSelector = engine.ToolSelector
type CommandSuggestion = engine.CommandSuggestion
type SuggestionRule = engine.SuggestionRule
type SuggestionEngine = engine.SuggestionEngine

var NewIntentClassifier = engine.NewIntentClassifier
var FormatIntent = engine.FormatIntent
var NewCapabilityRegistry = engine.NewCapabilityRegistry
var NewLanguageRegistry = engine.NewLanguageRegistry
var FormatLanguages = engine.FormatLanguages
var NewToolSelector = engine.NewToolSelector
var FormatToolSelection = engine.FormatToolSelection
var NewSuggestionEngine = engine.NewSuggestionEngine
var FormatCommandSuggestions = engine.FormatCommandSuggestions
