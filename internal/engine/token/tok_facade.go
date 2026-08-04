package token

import tok "github.com/GrayCodeAI/tok"

// Stats is the compression result consumed by Hawk's runtime observations.
// The alias preserves the external tok schema while keeping Tok imports inside
// this package.
type Stats = tok.Stats

// Compress applies Tok's context compression with a fixed token budget.
func Compress(text string, budget int) (string, Stats) {
	return tok.Compress(text, tok.WithBudget(budget))
}
