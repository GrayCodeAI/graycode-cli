// This file re-exports symbols from the intelligence sub-package so that existing
// callers of engine.Intent, engine.NewIntentClassifier, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/intelligence"

type Intent = intelligence.Intent
type IntentRule = intelligence.IntentRule
type ClassifiedInput = intelligence.ClassifiedInput
type IntentClassifier = intelligence.IntentClassifier
type Capability = intelligence.Capability
type CapabilityRegistry = intelligence.CapabilityRegistry
type LanguageConfig = intelligence.LanguageConfig
type LanguageRegistry = intelligence.LanguageRegistry
type ToolInfo = intelligence.ToolInfo
type ToolSelection = intelligence.ToolSelection
type ToolSelector = intelligence.ToolSelector
type CommandSuggestion = intelligence.CommandSuggestion
type SuggestionRule = intelligence.SuggestionRule
type SuggestionEngine = intelligence.SuggestionEngine

var NewIntentClassifier = intelligence.NewIntentClassifier
var FormatIntent = intelligence.FormatIntent
var NewCapabilityRegistry = intelligence.NewCapabilityRegistry
var NewLanguageRegistry = intelligence.NewLanguageRegistry
var FormatLanguages = intelligence.FormatLanguages
var NewToolSelector = intelligence.NewToolSelector
var FormatToolSelection = intelligence.FormatToolSelection
var NewSuggestionEngine = intelligence.NewSuggestionEngine
var FormatCommandSuggestions = intelligence.FormatCommandSuggestions
