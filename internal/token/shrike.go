// Package token is Hawk's dependency boundary for the external Shrike library.
// Generic token counting, compression, chunking, secret detection, and usage
// tracking should enter Hawk through this package.
package token

import (
	"encoding/json"

	shrike "github.com/GrayCodeAI/shrike"
	shrikegraph "github.com/GrayCodeAI/shrike/runtimegraph"
)

type (
	Stats              = shrike.Stats
	UsageTracker       = shrike.UsageTracker
	UsageLimits        = shrike.UsageLimits
	CodeChunk          = shrike.CodeChunk
	ChunkOptions       = shrike.ChunkOptions
	SecretMatch        = shrike.SecretMatch
	SecretDetector     = shrike.SecretDetector
	BudgetDecision     = shrikegraph.BudgetDecision
	RedactionSummary   = shrikegraph.RedactionSummary
	RuntimeGraphInput  = shrikegraph.Input
	RuntimeGraphExport = shrikegraph.Export
)

func CountTokens(text string) int     { return shrike.EstimateTokensPrecise(text) }
func CountTokensFast(text string) int { return shrike.EstimateTokens(text) }

func Compress(text string, budget int) (string, Stats) {
	return shrike.Compress(text, shrike.WithBudget(budget))
}

func NewUsageTracker() *UsageTracker { return shrike.NewUsageTracker() }

// JSONInvariants renders verified-fact summaries for elided JSON records
// (constants, enumerations, ranges, coverage). "" when nothing clears the
// withhold rules.
// ToolShrinkStats reports one tool's catalog reduction.
type ToolShrinkStats = shrike.ToolShrinkStats

func JSONInvariants(dropped []json.RawMessage) string { return shrike.JSONInvariants(dropped) }

// ShrinkToolCatalog compresses an OpenAI-style function-tool catalog,
// preserving the selection surface byte-for-byte. Fail-open: unchanged input
// with ok=false when nothing can be safely reduced.
func ShrinkToolCatalog(catalog string) (string, bool) { return shrike.ShrinkToolCatalog(catalog) }

// LintToolCatalog reports per-tool reductions without committing.
func LintToolCatalog(catalog string) ([]shrike.ToolShrinkStats, bool) {
	return shrike.LintToolCatalog(catalog)
}

// LogInvariants renders the level distribution of elided log lines.
// "" when the lines do not parse as logs.
func LogInvariants(lines []string) string { return shrike.LogInvariants(lines) }

func ChunkCode(source string, opts ChunkOptions) []CodeChunk {
	return shrike.ChunkCode(source, opts)
}

func DefaultSecretDetector() *SecretDetector { return shrike.DefaultSecretDetector() }

func BuildRuntimeGraph(input RuntimeGraphInput) (*RuntimeGraphExport, error) {
	return shrikegraph.Build(input)
}
