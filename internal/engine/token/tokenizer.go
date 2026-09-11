package token

import hawktoken "github.com/GrayCodeAI/hawk/internal/token"

// CountTokens returns a precise BPE-based token count for the given text.
func CountTokens(text string) int { return hawktoken.CountTokens(text) }

// CountTokensFast returns a fast heuristic token estimate for the given text.
func CountTokensFast(text string) int { return hawktoken.CountTokensFast(text) }

// CompressForContext compresses text to fit within a token budget,
// returning the compressed text and the final token count.
func CompressForContext(text string, budget int) (string, int) {
	compressed, stats := hawktoken.Compress(text, budget)
	return compressed, stats.FinalTokens
}
