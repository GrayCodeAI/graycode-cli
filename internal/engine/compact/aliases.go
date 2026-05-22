// Package compact provides compaction strategies, types, and helpers
// for context-window management. See ../REFACTOR_PLAN.md.
package compact

// Result is the outcome of a compaction pass.
type Result = CompactResult

// Config tunes compaction behaviour.
type Config = CompactConfig

// Variant identifies which compaction prompt variant to render.
type Variant = CompactVariant

// DefaultConfig returns the default top-level compaction config.
func DefaultConfig() Config {
	return DefaultCompactConfig()
}

// DefaultMicroConfig returns the default micro-compactor config.
func DefaultMicroConfig() MicroCompactConfig {
	return DefaultMicroCompactConfig()
}

// DefaultAPIConfig returns the default API-boundary compactor config.
func DefaultAPIConfig() APICompactConfig {
	return DefaultAPICompactConfig()
}

// BuildPrompt renders the compaction prompt template for the given variant.
func BuildPrompt(variant Variant) string {
	return BuildCompactPrompt(variant)
}

// FormatSummary normalises a raw LLM summary for display.
func FormatSummary(raw string) string {
	return FormatCompactSummary(raw)
}
