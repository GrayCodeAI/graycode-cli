package token

import (
	"encoding/json"

	graycodetoken "github.com/GrayCodeAI/graycode-cli/internal/token"
)

// Stats is the compression result consumed by Graycode's runtime observations.
// The alias preserves the external shrike schema while keeping Shrike imports inside
// this package.
type Stats = graycodetoken.Stats

// UsageTracker and UsageLimits expose the session budget API through Graycode's
// token boundary without changing Shrike's accounting behavior.
type (
	UsageTracker       = graycodetoken.UsageTracker
	UsageLimits        = graycodetoken.UsageLimits
	CodeChunk          = graycodetoken.CodeChunk
	ChunkOptions       = graycodetoken.ChunkOptions
	SecretMatch        = graycodetoken.SecretMatch
	SecretDetector     = graycodetoken.SecretDetector
	BudgetDecision     = graycodetoken.BudgetDecision
	RedactionSummary   = graycodetoken.RedactionSummary
	RuntimeGraphInput  = graycodetoken.RuntimeGraphInput
	RuntimeGraphExport = graycodetoken.RuntimeGraphExport
)

// NewUsageTracker creates an in-memory usage tracker with Shrike's defaults.
func NewUsageTracker() *UsageTracker { return graycodetoken.NewUsageTracker() }

// ChunkCode splits source into semantically meaningful token-bounded chunks.
func ChunkCode(source string, opts ChunkOptions) []CodeChunk {
	return graycodetoken.ChunkCode(source, opts)
}

// DefaultSecretDetector returns Shrike's concurrency-safe built-in detector.
func DefaultSecretDetector() *SecretDetector { return graycodetoken.DefaultSecretDetector() }

func BuildRuntimeGraph(input RuntimeGraphInput) (*RuntimeGraphExport, error) {
	return graycodetoken.BuildRuntimeGraph(input)
}

// Compress applies Shrike's context compression with a fixed token budget.
func Compress(text string, budget int) (string, Stats) {
	return graycodetoken.Compress(text, budget)
}

// JSONInvariants renders verified-fact summaries for elided JSON records.
func JSONInvariants(dropped []json.RawMessage) string { return graycodetoken.JSONInvariants(dropped) }

// ShrinkToolCatalog compresses an OpenAI-style function-tool catalog,
// preserving the selection surface byte-for-byte. Fail-open: unchanged input
// with ok=false when nothing can be safely reduced.
func ShrinkToolCatalog(catalog string) (string, bool) {
	return graycodetoken.ShrinkToolCatalog(catalog)
}

// LintToolCatalog reports per-tool reductions without committing.
func LintToolCatalog(catalog string) ([]graycodetoken.ToolShrinkStats, bool) {
	return graycodetoken.LintToolCatalog(catalog)
}

// LogInvariants renders the level distribution of elided log lines.
func LogInvariants(lines []string) string { return graycodetoken.LogInvariants(lines) }
