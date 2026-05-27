//go:build cgo

package codegraph

import (
	"crypto/sha256"
	"math"
	"sort"
	"strings"
)

// EmbeddingDimension is the size of the hash-based embedding vector.
// 128 dimensions provides good separation for code symbols without
// requiring external ML models.
const EmbeddingDimension = 128

// Embedding represents a dense vector for a code symbol.
type Embedding struct {
	NodeID  string    `json:"node_id"`
	Vector  []float32 `json:"vector"`
	Model   string    `json:"model"`   // "hash" or external model name
	Symbols []string  `json:"symbols"` // extracted symbols used to generate embedding
}

// GenerateEmbedding creates a hash-based embedding for a code symbol.
// Uses feature hashing (the "hashing trick") to map code features to a
// fixed-size vector without requiring a trained model.
//
// Features extracted:
// - Symbol name (split on camelCase/snake_case)
// - Qualified name parts
// - Docstring tokens
// - Signature tokens
// - File path components
// - Kind (function, class, etc.)
func GenerateEmbedding(node Node) []float32 {
	vec := make([]float32, EmbeddingDimension)

	// Extract all features
	features := extractFeatures(node)

	// Hash each feature to a dimension and accumulate
	for _, feature := range features {
		dim := hashToDimension(feature, EmbeddingDimension)
		// Use hash-based sign (+1 or -1) for better separation
		sign := hashToSign(feature)
		vec[dim] += sign
	}

	// L2 normalize
	return l2Normalize(vec)
}

// extractFeatures extracts all text features from a code symbol.
func extractFeatures(node Node) []string {
	var features []string

	// Name tokens (camelCase and snake_case splitting)
	features = append(features, splitIdentifier(node.Name)...)

	// Qualified name parts
	parts := strings.Split(node.QualifiedName, "::")
	for _, part := range parts {
		features = append(features, splitIdentifier(part)...)
	}

	// Kind as feature
	features = append(features, "kind:"+node.Kind)

	// Language as feature
	features = append(features, "lang:"+node.Language)

	// Docstring tokens
	if node.Docstring != "" {
		docTokens := tokenize(node.Docstring)
		features = append(features, docTokens...)
	}

	// Signature tokens
	if node.Signature != "" {
		sigTokens := tokenize(node.Signature)
		features = append(features, sigTokens...)
	}

	// File path components
	pathParts := strings.Split(node.FilePath, "/")
	for _, part := range pathParts {
		if part != "" && part != "." {
			features = append(features, "path:"+strings.ToLower(part))
		}
	}

	return features
}

// splitIdentifier splits a camelCase or snake_case identifier into tokens.
func splitIdentifier(s string) []string {
	var tokens []string

	// Split on snake_case
	if strings.Contains(s, "_") {
		for _, part := range strings.Split(s, "_") {
			if part != "" {
				tokens = append(tokens, strings.ToLower(part))
			}
		}
		return tokens
	}

	// Split on camelCase
	var current strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if current.Len() > 0 {
				tokens = append(tokens, strings.ToLower(current.String()))
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		tokens = append(tokens, strings.ToLower(current.String()))
	}

	return tokens
}

// tokenize splits text into lowercase alphanumeric tokens.
func tokenize(text string) []string {
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			current.WriteRune(r)
		} else {
			if current.Len() > 2 {
				tokens = append(tokens, strings.ToLower(current.String()))
			}
			current.Reset()
		}
	}
	if current.Len() > 2 {
		tokens = append(tokens, strings.ToLower(current.String()))
	}

	return tokens
}

// hashToDimension maps a string to a dimension index using SHA-256.
func hashToDimension(s string, dim int) int {
	h := sha256.Sum256([]byte(s))
	// Use first 4 bytes as uint32, then mod by dimension
	val := uint32(h[0])<<24 | uint32(h[1])<<16 | uint32(h[2])<<8 | uint32(h[3])
	return int(val % uint32(dim))
}

// hashToSign returns +1 or -1 based on hash for bipolar hashing trick.
func hashToSign(s string) float32 {
	h := sha256.Sum256([]byte(s))
	if h[4]%2 == 0 {
		return 1.0
	}
	return -1.0
}

// l2Normalize normalizes a vector to unit length.
func l2Normalize(vec []float32) []float32 {
	var norm float32
	for _, v := range vec {
		norm += v * v
	}
	norm = float32(math.Sqrt(float64(norm)))
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec
}

// CosineSimilarity computes cosine similarity between two vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
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
	rows, err := cg.db.Query(
		`SELECT id, kind, name, qualified_name, file_path, language,
		        start_line, end_line, signature, docstring, visibility, is_exported
		 FROM nodes WHERE kind IN ('function', 'method', 'class', 'interface', 'struct')`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	allNodes, _ := scanNodes(rows)

	// Compute similarities
	type scored struct {
		node Node
		sim  float32
	}
	var scoredNodes []scored

	for _, n := range allNodes {
		vec := GenerateEmbedding(n)
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
