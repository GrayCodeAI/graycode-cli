package token

import tok "github.com/GrayCodeAI/tok"

// Stats is the compression result consumed by Hawk's runtime observations.
// The alias preserves the external tok schema while keeping Tok imports inside
// this package.
type Stats = tok.Stats

// UsageTracker and UsageLimits expose the session budget API through Hawk's
// token boundary without changing Tok's accounting behavior.
type (
	UsageTracker = tok.UsageTracker
	UsageLimits  = tok.UsageLimits
)

// NewUsageTracker creates an in-memory usage tracker with Tok's defaults.
func NewUsageTracker() *UsageTracker { return tok.NewUsageTracker() }

// Compress applies Tok's context compression with a fixed token budget.
func Compress(text string, budget int) (string, Stats) {
	return tok.Compress(text, tok.WithBudget(budget))
}
