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

func CountTokens(text string) int {
	if ShrikeAvailable() {
		return shrike.EstimateTokensPrecise(text)
	}
	return fallbackEstimate(text)
}

func CountTokensFast(text string) int {
	if ShrikeAvailable() {
		return shrike.EstimateTokens(text)
	}
	return fallbackEstimate(text)
}

// fallbackEstimate is a self-contained BPE-style token estimate used when the
// shrike engine is the build-harness stub (or otherwise unavailable). It lands
// in the 3-7 chars-per-token band for English prose (the range the rest of the
// codebase expects) and is strictly better than the stub's constant zero, so
// context budgeting and cost accounting degrade gracefully instead of silently
// treating every message as zero tokens. When a real shrike is linked it is
// never used.
func fallbackEstimate(text string) int {
	n := len([]rune(text))
	if n == 0 {
		return 0
	}
	est := (n + 3) / 4 // ~4 chars/token
	if est < 1 {
		est = 1
	}
	return est
}

// ShrikeAvailable reports whether the underlying shrike engine is a real
// implementation rather than a build-harness stub. The
// stub returns zero for every token estimate, so a non-empty text probe
// reliably distinguishes it from the real engine. Consumers use this to report
// honest availability instead of claiming an operational token pipeline.
func ShrikeAvailable() bool {
	return shrike.EstimateTokensPrecise("hawk context compression pipeline") > 0
}

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
