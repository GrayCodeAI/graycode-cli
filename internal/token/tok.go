// Package token is Hawk's dependency boundary for the external Tok library.
// Generic token counting, compression, chunking, secret detection, and usage
// tracking should enter Hawk through this package.
package token

import (
	"encoding/json"

	tok "github.com/GrayCodeAI/tok"
	tokgraph "github.com/GrayCodeAI/tok/runtimegraph"
)

type (
	Stats              = tok.Stats
	UsageTracker       = tok.UsageTracker
	UsageLimits        = tok.UsageLimits
	CodeChunk          = tok.CodeChunk
	ChunkOptions       = tok.ChunkOptions
	SecretMatch        = tok.SecretMatch
	SecretDetector     = tok.SecretDetector
	BudgetDecision     = tokgraph.BudgetDecision
	RedactionSummary   = tokgraph.RedactionSummary
	RuntimeGraphInput  = tokgraph.Input
	RuntimeGraphExport = tokgraph.Export
)

func CountTokens(text string) int     { return tok.EstimateTokensPrecise(text) }
func CountTokensFast(text string) int { return tok.EstimateTokens(text) }

func Compress(text string, budget int) (string, Stats) {
	return tok.Compress(text, tok.WithBudget(budget))
}

func NewUsageTracker() *UsageTracker { return tok.NewUsageTracker() }

// JSONInvariants renders verified-fact summaries for elided JSON records
// (constants, enumerations, ranges, coverage). "" when nothing clears the
// withhold rules.
func JSONInvariants(dropped []json.RawMessage) string { return tok.JSONInvariants(dropped) }

// LogInvariants renders the level distribution of elided log lines.
// "" when the lines do not parse as logs.
func LogInvariants(lines []string) string { return tok.LogInvariants(lines) }

func ChunkCode(source string, opts ChunkOptions) []CodeChunk {
	return tok.ChunkCode(source, opts)
}

func DefaultSecretDetector() *SecretDetector { return tok.DefaultSecretDetector() }

func BuildRuntimeGraph(input RuntimeGraphInput) (*RuntimeGraphExport, error) {
	return tokgraph.Build(input)
}
