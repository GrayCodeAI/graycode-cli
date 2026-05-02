package engine

import "github.com/GrayCodeAI/tok"

// CountTokens returns a precise BPE-based token count for the given text.
func CountTokens(text string) int { return tok.EstimateTokensPrecise(text) }

// CountTokensFast returns a fast heuristic token estimate for the given text.
func CountTokensFast(text string) int { return tok.EstimateTokens(text) }

// CompressForContext compresses text to fit within a token budget,
// returning the compressed text and the final token count.
func CompressForContext(text string, budget int) (string, int) {
	compressed, stats := tok.Compress(text, tok.WithBudget(budget))
	return compressed, stats.FinalTokens
}
