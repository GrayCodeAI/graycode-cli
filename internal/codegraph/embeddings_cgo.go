//go:build cgo

package codegraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// maxEmbeddingCacheEntries bounds the in-memory embedding cache so long-lived
// CodeGraph instances (daemon) cannot grow it without limit.
var maxEmbeddingCacheEntries = 200_000

// embeddingFor returns the embedding for a node, computing it once and
// memoizing it keyed by a content hash (H8). The key covers every field
// extractFeatures reads, so a node edit invalidates the entry naturally.
// The cache is bounded: once full it is reset (cheap for hash-based
// embeddings — a recompute after reset is far cheaper than the unbounded
// per-query recomputation this replaces).
func (cg *CodeGraph) embeddingFor(node Node) []float32 {
	key := embeddingCacheKey(node)

	cg.embedMu.Lock()
	defer cg.embedMu.Unlock()
	if vec, ok := cg.embeddingCache[key]; ok {
		return vec
	}
	vec := GenerateEmbedding(node)
	if len(cg.embeddingCache) >= maxEmbeddingCacheEntries {
		cg.embeddingCache = make(map[string][]float32, maxEmbeddingCacheEntries/2)
	}
	cg.embeddingCache[key] = vec
	return vec
}

func embeddingCacheKey(node Node) string {
	h := sha256.New()
	h.Write([]byte(node.Name))
	h.Write([]byte{0})
	h.Write([]byte(node.QualifiedName))
	h.Write([]byte{0})
	h.Write([]byte(node.Kind))
	h.Write([]byte{0})
	h.Write([]byte(node.Language))
	h.Write([]byte{0})
	h.Write([]byte(node.Docstring))
	h.Write([]byte{0})
	h.Write([]byte(node.Signature))
	h.Write([]byte{0})
	h.Write([]byte(node.FilePath))
	return hex.EncodeToString(h.Sum(nil))
}

// SemanticSearch performs embedding-based semantic search.
// It generates embeddings for all nodes and finds the most similar
// to the query embedding.
func (cg *CodeGraph) SemanticSearch(query string, limit int) ([]Node, error) {
	if limit <= 0 {
		limit = 20
	}

	cg.mu.RLock()
	defer cg.mu.RUnlock()

	// Generate query embedding from the query string
	queryNode := Node{
		Name:          query,
		QualifiedName: query,
		Kind:          "query",
		Language:      "query",
	}
	queryVec := GenerateEmbedding(queryNode)

	// Get all nodes
	rows, err := cg.db.QueryContext(
		context.Background(),
		`SELECT id, kind, name, qualified_name, file_path, language,
		        start_line, end_line, signature, docstring, visibility, is_exported
		 FROM nodes WHERE kind IN ('function', 'method', 'class', 'interface', 'struct')`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // deferred Close on read-only rows is cleanup-only

	allNodes, _ := scanNodes(rows)

	// Compute similarities
	type scored struct {
		node Node
		sim  float32
	}
	var scoredNodes []scored

	for _, n := range allNodes {
		vec := cg.embeddingFor(n)
		sim := CosineSimilarity(queryVec, vec)
		if sim > 0.1 { // threshold
			scoredNodes = append(scoredNodes, scored{n, sim})
		}
	}

	// Sort by similarity descending
	sort.Slice(scoredNodes, func(i, j int) bool {
		return scoredNodes[i].sim > scoredNodes[j].sim
	})

	// Return top results
	var results []Node
	l := limit
	if l > len(scoredNodes) {
		l = len(scoredNodes)
	}
	for _, s := range scoredNodes[:l] {
		results = append(results, s.node)
	}

	return results, nil
}

// HybridSearch combines FTS5 keyword search with embedding-based semantic search.
// Uses Reciprocal Rank Fusion (RRF) to merge results from both methods.
func (cg *CodeGraph) HybridSearch(query string, limit int) ([]Node, error) {
	if limit <= 0 {
		limit = 20
	}

	// Get FTS5 results
	ftsResults, _ := cg.Search(query, limit*2)

	// Get semantic results
	semResults, _ := cg.SemanticSearch(query, limit*2)

	// RRF fusion
	k := 60.0 // RRF constant
	scores := make(map[string]float64)
	nodeMap := make(map[string]Node)

	for rank, n := range ftsResults {
		scores[n.ID] += 1.0 / (k + float64(rank+1))
		nodeMap[n.ID] = n
	}

	for rank, n := range semResults {
		scores[n.ID] += 1.0 / (k + float64(rank+1))
		nodeMap[n.ID] = n
	}

	// Sort by RRF score
	type scored struct {
		id    string
		score float64
	}
	var sorted []scored
	for id, score := range scores {
		sorted = append(sorted, scored{id, score})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	// Return top results
	var results []Node
	l := limit
	if l > len(sorted) {
		l = len(sorted)
	}
	for _, s := range sorted[:l] {
		if n, ok := nodeMap[s.id]; ok {
			results = append(results, n)
		}
	}

	return results, nil
}
