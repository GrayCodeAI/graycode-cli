// Package compact is the Stage-1 namespace for the engine package's
// compaction-related types and functions. It currently re-exports the
// canonical symbols from package engine as type aliases and var aliases;
// no implementation lives here yet.
//
// New code in hawk should import this package instead of reaching into
// engine for compact symbols. When Stage 2 of the engine split lands,
// the implementation will move into this directory and the engine package
// will become the alias re-exporter (the inverse of the current direction).
//
// See REFACTOR_PLAN.md at the engine package root for the full split plan.
package compact

import "github.com/GrayCodeAI/hawk/engine"

// Strategy is the contract every compaction strategy implements.
type Strategy = engine.CompactStrategy

// Result is the outcome of a compaction pass.
type Result = engine.CompactResult

// Config tunes compaction behaviour.
type Config = engine.CompactConfig

// Variant identifies which compaction prompt variant to render.
type Variant = engine.CompactVariant

// Registry stores strategies by name for runtime selection.
type Registry = engine.StrategyRegistry

// AutoCompactor decides when and how to compact based on context pressure.
type AutoCompactor = engine.AutoCompactor

// SmartCompactStrategy is the default LLM-driven compactor.
type SmartCompactStrategy = engine.SmartCompactStrategy

// TruncateStrategy drops oldest messages first; cheap but lossy.
type TruncateStrategy = engine.TruncateStrategy

// MicroCompactStrategy collapses adjacent short messages.
type MicroCompactStrategy = engine.MicroCompactStrategy

// MicroCompactConfig tunes the micro-compactor.
type MicroCompactConfig = engine.MicroCompactConfig

// SessionMemoryStrategy distils conversation into a compact memory blob.
type SessionMemoryStrategy = engine.SessionMemoryStrategy

// SessionMemoryConfig tunes the session-memory compactor.
type SessionMemoryConfig = engine.SessionMemoryConfig

// APICompactStrategy compacts at the API-call boundary (provider-specific).
type APICompactStrategy = engine.APICompactStrategy

// APICompactConfig tunes the API-boundary compactor.
type APICompactConfig = engine.APICompactConfig

// FileTracker remembers which files have been read/modified during a session;
// used by file-aware compactors to keep the relevant ones.
type FileTracker = engine.FileTracker

// ---------------------------------------------------------------------------
// Constructors / defaults.
// ---------------------------------------------------------------------------

// NewAutoCompactor constructs an auto-compactor with the given config.
func NewAutoCompactor(config Config) *AutoCompactor {
	return engine.NewAutoCompactor(config)
}

// NewFileTracker returns an empty file tracker.
func NewFileTracker() *FileTracker {
	return engine.NewFileTracker()
}

// DefaultConfig returns the default top-level compaction config.
func DefaultConfig() Config {
	return engine.DefaultCompactConfig()
}

// DefaultMicroConfig returns the default micro-compactor config.
func DefaultMicroConfig() MicroCompactConfig {
	return engine.DefaultMicroCompactConfig()
}

// DefaultSessionMemoryConfig returns the default session-memory compactor config.
func DefaultSessionMemoryConfig() SessionMemoryConfig {
	return engine.DefaultSessionMemoryConfig()
}

// DefaultAPIConfig returns the default API-boundary compactor config.
func DefaultAPIConfig() APICompactConfig {
	return engine.DefaultAPICompactConfig()
}

// BuildPrompt renders the compaction prompt template for the given variant.
func BuildPrompt(variant Variant) string {
	return engine.BuildCompactPrompt(variant)
}

// FormatSummary normalises a raw LLM summary for display.
func FormatSummary(raw string) string {
	return engine.FormatCompactSummary(raw)
}
