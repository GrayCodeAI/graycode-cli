package repomap

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ShapleyScore represents the computed marginal contribution of a code chunk.
type ShapleyScore struct {
	Path      string
	StartLine int
	EndLine   int
	Score     float64 // higher = more helpful in context
	Content   string
}

// ShapleyRanker scores code chunks by their marginal contribution to
// generation quality using an approximate Shapley value approach.
type ShapleyRanker struct {
	chunks []CodeChunk
	scores map[string]float64 // chunk_id -> shapley score
}

// NewShapleyRanker creates a ranker from the given code chunks.
func NewShapleyRanker(chunks []CodeChunk) *ShapleyRanker {
	return &ShapleyRanker{
		chunks: chunks,
		scores: make(map[string]float64),
	}
}

// chunkID generates a unique identifier for a code chunk.
func chunkID(c CodeChunk) string {
	return fmt.Sprintf("%s:%d-%d", c.Path, c.StartLine, c.EndLine)
}

// ComputeScores calculates approximate Shapley values for each chunk.
//
// Score = relevance_to_query * centrality_in_graph * recency_bonus - redundancy_penalty
//
//   - relevance: keyword overlap with query
//   - centrality: how many other chunks reference symbols in this chunk
//   - recency: files in relevantPaths get a boost
//   - redundancy: chunks similar to already-scored chunks get penalized
func (sr *ShapleyRanker) ComputeScores(relevantPaths []string, query string) []ShapleyScore {
	if len(sr.chunks) == 0 {
		return nil
	}

	queryTokens := tokenize(query)
	relevantSet := make(map[string]bool, len(relevantPaths))
	for _, p := range relevantPaths {
		relevantSet[p] = true
	}

	// Build a symbol reference map for centrality computation.
	// For each chunk, extract identifiers and count how many other chunks reference them.
	chunkSymbols := make(map[string][]string) // chunkID -> extracted symbols
	allSymbols := make(map[string]int)         // symbol -> reference count across chunks

	for _, c := range sr.chunks {
		id := chunkID(c)
		symbols := extractIdentifiers(c.Content)
		chunkSymbols[id] = symbols
		for _, sym := range symbols {
			allSymbols[sym]++
		}
	}

	// Compute per-chunk scores.
	var results []ShapleyScore

	for _, c := range sr.chunks {
		id := chunkID(c)

		// 1. Relevance: TF-IDF-like overlap with query.
		relevance := computeRelevance(c.Content, queryTokens)

		// 2. Centrality: how many other chunks reference symbols defined in this chunk.
		centrality := computeCentrality(chunkSymbols[id], allSymbols, len(sr.chunks))

		// 3. Recency bonus: files in relevantPaths get a boost.
		recencyBonus := 0.0
		if relevantSet[c.Path] {
			recencyBonus = 0.3
		}

		// 4. Redundancy penalty: penalize chunks very similar to high-scoring ones.
		redundancy := 0.0
		for _, prev := range results {
			sim := contentSimilarity(c.Content, prev.Content)
			if sim > 0.7 {
				redundancy += sim * 0.5
			}
		}

		score := (relevance*0.5 + centrality*0.3 + recencyBonus) - redundancy
		if score < 0 {
			score = 0
		}

		sr.scores[id] = score
		results = append(results, ShapleyScore{
			Path:      c.Path,
			StartLine: c.StartLine,
			EndLine:   c.EndLine,
			Score:     score,
			Content:   c.Content,
		})
	}

	// Sort by score descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// SelectOptimalContext greedily selects chunks that maximize information within
// the token budget, recomputing redundancy penalties after each addition.
func (sr *ShapleyRanker) SelectOptimalContext(query string, tokenBudget int) []CodeChunk {
	if tokenBudget <= 0 {
		tokenBudget = 4000
	}

	scores := sr.ComputeScores(nil, query)
	if len(scores) == 0 {
		return nil
	}

	var selected []CodeChunk
	usedTokens := 0
	selectedContents := make([]string, 0)

	for _, s := range scores {
		chunkTokens := estimateChunkTokens(s.Content)
		if usedTokens+chunkTokens > tokenBudget {
			continue
		}

		// Recompute redundancy against already-selected chunks.
		redundant := false
		for _, prev := range selectedContents {
			if contentSimilarity(s.Content, prev) > 0.8 {
				redundant = true
				break
			}
		}
		if redundant {
			continue
		}

		selected = append(selected, CodeChunk{
			Path:      s.Path,
			StartLine: s.StartLine,
			EndLine:   s.EndLine,
			Content:   s.Content,
		})
		selectedContents = append(selectedContents, s.Content)
		usedTokens += chunkTokens
	}

	return selected
}

// Format renders selected chunks as a text block for prompt injection.
func (sr *ShapleyRanker) Format(chunks []CodeChunk) string {
	if len(chunks) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Relevant Code Context\n\n")
	for _, c := range chunks {
		b.WriteString(fmt.Sprintf("### %s (lines %d-%d)\n", c.Path, c.StartLine, c.EndLine))
		b.WriteString("```\n")
		b.WriteString(c.Content)
		if !strings.HasSuffix(c.Content, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString("```\n\n")
	}
	return b.String()
}

// computeRelevance scores how relevant a chunk's content is to query tokens.
func computeRelevance(content string, queryTokens []string) float64 {
	if len(queryTokens) == 0 {
		return 0
	}

	contentTokens := tokenize(content)
	if len(contentTokens) == 0 {
		return 0
	}

	contentSet := make(map[string]bool, len(contentTokens))
	for _, t := range contentTokens {
		contentSet[t] = true
	}

	matches := 0
	for _, qt := range queryTokens {
		if contentSet[qt] {
			matches++
		}
	}
	return float64(matches) / float64(len(queryTokens))
}

// computeCentrality estimates how central a chunk is based on how widely
// its symbols are referenced across the codebase.
func computeCentrality(symbols []string, allSymbols map[string]int, totalChunks int) float64 {
	if len(symbols) == 0 || totalChunks == 0 {
		return 0
	}

	var totalRefs float64
	for _, sym := range symbols {
		refs := allSymbols[sym]
		if refs > 1 {
			// Use log to dampen very common symbols.
			totalRefs += math.Log(float64(refs))
		}
	}
	// Normalize by number of symbols.
	return totalRefs / float64(len(symbols))
}

// contentSimilarity computes Jaccard similarity between two content strings.
func contentSimilarity(a, b string) float64 {
	tokA := tokenize(a)
	tokB := tokenize(b)
	if len(tokA) == 0 || len(tokB) == 0 {
		return 0
	}

	setA := make(map[string]bool, len(tokA))
	for _, t := range tokA {
		setA[t] = true
	}
	setB := make(map[string]bool, len(tokB))
	for _, t := range tokB {
		setB[t] = true
	}

	intersection := 0
	for t := range setA {
		if setB[t] {
			intersection++
		}
	}
	union := len(setA)
	for t := range setB {
		if !setA[t] {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// extractIdentifiers extracts likely identifier tokens from source code.
func extractIdentifiers(content string) []string {
	tokens := tokenize(content)
	var identifiers []string
	seen := make(map[string]bool)

	for _, t := range tokens {
		// Skip very short or very common tokens.
		if len(t) < 3 {
			continue
		}
		if isCommonKeyword(t) {
			continue
		}
		if !seen[t] {
			seen[t] = true
			identifiers = append(identifiers, t)
		}
	}
	return identifiers
}

// isCommonKeyword returns true for language keywords that don't indicate a reference.
func isCommonKeyword(t string) bool {
	keywords := map[string]bool{
		"func": true, "var": true, "const": true, "type": true, "return": true,
		"if": true, "else": true, "for": true, "range": true, "switch": true,
		"case": true, "default": true, "import": true, "package": true,
		"struct": true, "interface": true, "map": true, "string": true,
		"int": true, "bool": true, "error": true, "nil": true, "true": true,
		"false": true, "the": true, "and": true, "from": true, "this": true,
		"that": true, "with": true, "not": true,
		"def": true, "class": true, "self": true, "none": true, "pass": true,
		"function": true, "let": true, "new": true, "null": true,
	}
	return keywords[t]
}

// estimateChunkTokens estimates token count for a chunk (~4 chars per token).
func estimateChunkTokens(content string) int {
	return len(content)/4 + 1
}
