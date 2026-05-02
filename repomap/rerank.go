package repomap

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// RerankResult pairs a search result with a re-ranking score.
type RerankResult struct {
	Chunk CodeSearchResult
	Score float64
}

// Rerank re-scores candidates using BM25 (k1=1.2, b=0.75) against the query
// and returns the top-K results sorted by descending score.
func Rerank(query string, candidates []CodeSearchResult, topK int) []RerankResult {
	if len(candidates) == 0 {
		return nil
	}
	if topK <= 0 {
		topK = len(candidates)
	}

	queryTerms := rerankTokenize(query)
	if len(queryTerms) == 0 {
		// No usable query terms; return candidates in original order
		out := make([]RerankResult, len(candidates))
		for i, c := range candidates {
			out[i] = RerankResult{Chunk: c, Score: 0}
		}
		if len(out) > topK {
			out = out[:topK]
		}
		return out
	}

	// Compute average document length
	var totalLen float64
	docLengths := make([]float64, len(candidates))
	for i, c := range candidates {
		tokens := rerankTokenize(c.Content)
		docLengths[i] = float64(len(tokens))
		totalLen += docLengths[i]
	}
	avgDL := totalLen / float64(len(candidates))
	if avgDL == 0 {
		avgDL = 1
	}

	// Compute IDF for query terms
	N := float64(len(candidates))
	docFreq := make(map[string]int)
	for _, c := range candidates {
		seen := make(map[string]bool)
		for _, t := range rerankTokenize(c.Content) {
			if !seen[t] {
				seen[t] = true
				docFreq[t]++
			}
		}
	}

	idf := make(map[string]float64)
	for _, qt := range queryTerms {
		df := float64(docFreq[qt])
		// Standard BM25 IDF: log((N - df + 0.5) / (df + 0.5) + 1)
		idf[qt] = math.Log((N-df+0.5)/(df+0.5) + 1.0)
	}

	// BM25 parameters
	const k1 = 1.2
	const b = 0.75

	// Score each candidate
	results := make([]RerankResult, len(candidates))
	for i, c := range candidates {
		docTokens := rerankTokenize(c.Content)
		tf := make(map[string]int)
		for _, t := range docTokens {
			tf[t]++
		}

		var score float64
		dl := docLengths[i]
		for _, qt := range queryTerms {
			freq := float64(tf[qt])
			if freq == 0 {
				continue
			}
			// BM25 formula
			num := freq * (k1 + 1)
			denom := freq + k1*(1-b+b*(dl/avgDL))
			score += idf[qt] * (num / denom)
		}

		results[i] = RerankResult{Chunk: c, Score: score}
	}

	// Sort by score descending
	sort.Slice(results, func(a, b int) bool {
		return results[a].Score > results[b].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

// rerankTokenize splits text into lowercase tokens for BM25 scoring.
func rerankTokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			current.WriteRune(r)
		} else {
			if current.Len() > 1 {
				tokens = append(tokens, current.String())
			}
			current.Reset()
		}
	}
	if current.Len() > 1 {
		tokens = append(tokens, current.String())
	}
	return tokens
}
