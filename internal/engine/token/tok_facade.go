package token

import (
	"encoding/json"

	hawktoken "github.com/GrayCodeAI/hawk/internal/token"
)

// Stats is the compression result consumed by Hawk's runtime observations.
// The alias preserves the external tok schema while keeping Tok imports inside
// this package.
type Stats = hawktoken.Stats

// UsageTracker and UsageLimits expose the session budget API through Hawk's
// token boundary without changing Tok's accounting behavior.
type (
	UsageTracker       = hawktoken.UsageTracker
	UsageLimits        = hawktoken.UsageLimits
	CodeChunk          = hawktoken.CodeChunk
	ChunkOptions       = hawktoken.ChunkOptions
	SecretMatch        = hawktoken.SecretMatch
	SecretDetector     = hawktoken.SecretDetector
	BudgetDecision     = hawktoken.BudgetDecision
	RedactionSummary   = hawktoken.RedactionSummary
	RuntimeGraphInput  = hawktoken.RuntimeGraphInput
	RuntimeGraphExport = hawktoken.RuntimeGraphExport
)

// NewUsageTracker creates an in-memory usage tracker with Tok's defaults.
func NewUsageTracker() *UsageTracker { return hawktoken.NewUsageTracker() }

// ChunkCode splits source into semantically meaningful token-bounded chunks.
func ChunkCode(source string, opts ChunkOptions) []CodeChunk {
	return hawktoken.ChunkCode(source, opts)
}

// DefaultSecretDetector returns Tok's concurrency-safe built-in detector.
func DefaultSecretDetector() *SecretDetector { return hawktoken.DefaultSecretDetector() }

func BuildRuntimeGraph(input RuntimeGraphInput) (*RuntimeGraphExport, error) {
	return hawktoken.BuildRuntimeGraph(input)
}

// Compress applies Tok's context compression with a fixed token budget.
func Compress(text string, budget int) (string, Stats) {
	return hawktoken.Compress(text, budget)
}

// JSONInvariants renders verified-fact summaries for elided JSON records.
func JSONInvariants(dropped []json.RawMessage) string { return hawktoken.JSONInvariants(dropped) }

// LogInvariants renders the level distribution of elided log lines.
func LogInvariants(lines []string) string { return hawktoken.LogInvariants(lines) }
