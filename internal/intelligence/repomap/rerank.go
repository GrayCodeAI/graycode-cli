// rerank.go applies a BM25-over-BM25 reranking pass to a
// candidate set of code chunks. It is used after a faster but less precise
// retrieval step (semantic similarity, PageRank, or import-graph proximity)
// to surface the chunks that are most relevant to the query terms.
package repomap

import (
	"sort"
	"strings"
	"unicode"

	"github.com/GrayCodeAI/hawk/internal/scoring"
)

// RerankResult pairs a search result with a re-ranking score.
type RerankResult struct {
	Chunk CodeSearchResult
	Score float64
}

// Rerank re-scores candidates using BM25 against the query
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

	// Compute average document length and per-document term frequencies
	var totalLen float64
	docLengths := make([]float64, len(candidates))
	docTermFreqs := make([]map[string]int, len(candidates))
	for i, c := range candidates {
		tokens := rerankTokenize(c.Content)
		docLengths[i] = float64(len(tokens))
		totalLen += docLengths[i]
		tf := make(map[string]int)
		for _, t := range tokens {
			tf[t]++
		}
		docTermFreqs[i] = tf
	}
	avgDL := totalLen / float64(len(candidates))
	if avgDL == 0 {
		avgDL = 1
	}

	// Compute document frequency for query terms
	docCount := len(candidates)
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

	scorer := scoring.NewBM25Scorer(0, 0) // defaults: k1=1.2, b=0.75

	// Score each candidate using the shared BM25 scorer
	results := make([]RerankResult, len(candidates))
	for i, c := range candidates {
		s := scorer.Score(queryTerms, docTermFreqs[i], docLengths[i], avgDL, docCount, docFreq)
		results[i] = RerankResult{Chunk: c, Score: s}
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
