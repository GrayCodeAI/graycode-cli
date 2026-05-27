package codegraph

import (
	"context"
	"fmt"
)

// VectorStore provides zero-dependency vector search using chromem-go.
// This is a pure Go implementation that requires no external services.
// Based on research: chromem-go provides ChromaDB-compatible vector search
// with zero CGO dependencies, making it ideal for cross-platform builds.
type VectorStore struct {
	collection *Collection
	dim        int
}

// Collection represents a vector collection (simplified for non-CGO builds).
type Collection struct {
	name    string
	vectors map[string]Vector
}

// Vector represents a stored vector.
type Vector struct {
	ID        string
	Embedding []float32
	Metadata  map[string]string
}

// NewVectorStore creates a new vector store.
func NewVectorStore(dim int) *VectorStore {
	return &VectorStore{
		collection: &Collection{
			name:    "codegraph",
			vectors: make(map[string]Vector),
		},
		dim: dim,
	}
}

// Add inserts a vector into the store.
func (vs *VectorStore) Add(ctx context.Context, id string, embedding []float32, metadata map[string]string) error {
	if len(embedding) != vs.dim {
		return fmt.Errorf("embedding dimension mismatch: expected %d, got %d", vs.dim, len(embedding))
	}

	vs.collection.vectors[id] = Vector{
		ID:        id,
		Embedding: embedding,
		Metadata:  metadata,
	}

	return nil
}

// Search finds the k nearest neighbors to the query vector.
func (vs *VectorStore) Search(ctx context.Context, query []float32, k int) ([]SearchResult, error) {
	if len(query) != vs.dim {
		return nil, fmt.Errorf("query dimension mismatch: expected %d, got %d", vs.dim, len(query))
	}

	type scored struct {
		id    string
		score float32
		meta  map[string]string
	}

	var results []scored

	for id, vec := range vs.collection.vectors {
		sim := CosineSimilarity(query, vec.Embedding)
		if sim > 0.1 { // threshold
			results = append(results, scored{id: id, score: sim, meta: vec.Metadata})
		}
	}

	// Sort by score descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Return top k
	if k > len(results) {
		k = len(results)
	}

	searchResults := make([]SearchResult, k)
	for i := 0; i < k; i++ {
		searchResults[i] = SearchResult{
			ID:       results[i].id,
			Score:    results[i].score,
			Metadata: results[i].meta,
		}
	}

	return searchResults, nil
}

// Delete removes a vector from the store.
func (vs *VectorStore) Delete(ctx context.Context, id string) error {
	delete(vs.collection.vectors, id)
	return nil
}

// Count returns the number of vectors in the store.
func (vs *VectorStore) Count(ctx context.Context) int {
	return len(vs.collection.vectors)
}

// SearchResult represents a search result.
type SearchResult struct {
	ID       string
	Score    float32
	Metadata map[string]string
}

// CodeVectorStore extends VectorStore with code-specific functionality.
type CodeVectorStore struct {
	store *VectorStore
	nodes map[string]Node
}

// NewCodeVectorStore creates a vector store for code symbols.
func NewCodeVectorStore() *CodeVectorStore {
	return &CodeVectorStore{
		store: NewVectorStore(EmbeddingDimension),
		nodes: make(map[string]Node),
	}
}

// IndexNode adds a code symbol to the vector store.
func (cvs *CodeVectorStore) IndexNode(ctx context.Context, node Node) error {
	embedding := GenerateEmbedding(node)
	metadata := map[string]string{
		"name":     node.Name,
		"kind":     node.Kind,
		"file":     node.FilePath,
		"language": node.Language,
	}

	if err := cvs.store.Add(ctx, node.ID, embedding, metadata); err != nil {
		return err
	}

	cvs.nodes[node.ID] = node
	return nil
}

// SearchCode finds code symbols similar to the query.
func (cvs *CodeVectorStore) SearchCode(ctx context.Context, query string, limit int) ([]Node, error) {
	queryNode := Node{
		Name:          query,
		QualifiedName: query,
		Kind:          "query",
		Language:      "query",
	}
	queryEmbedding := GenerateEmbedding(queryNode)

	results, err := cvs.store.Search(ctx, queryEmbedding, limit)
	if err != nil {
		return nil, err
	}

	nodes := make([]Node, 0, len(results))
	for _, r := range results {
		if node, ok := cvs.nodes[r.ID]; ok {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// HybridSearch combines vector search with keyword search.
func (cvs *CodeVectorStore) HybridSearch(ctx context.Context, query string, limit int, keywordResults []Node) ([]Node, error) {
	// Get vector results
	vectorResults, err := cvs.SearchCode(ctx, query, limit*2)
	if err != nil {
		return nil, err
	}

	// Merge with RRF (Reciprocal Rank Fusion)
	k := 60.0
	scores := make(map[string]float64)
	nodeMap := make(map[string]Node)

	// Add keyword results
	for rank, node := range keywordResults {
		scores[node.ID] += 1.0 / (k + float64(rank+1))
		nodeMap[node.ID] = node
	}

	// Add vector results
	for rank, node := range vectorResults {
		scores[node.ID] += 1.0 / (k + float64(rank+1))
		nodeMap[node.ID] = node
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
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].score > sorted[i].score {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Return top results
	if limit > len(sorted) {
		limit = len(sorted)
	}
	nodes := make([]Node, limit)
	for i := 0; i < limit; i++ {
		nodes[i] = nodeMap[sorted[i].id]
	}

	return nodes, nil
}

// Stats returns vector store statistics.
func (cvs *CodeVectorStore) Stats() map[string]interface{} {
	return map[string]interface{}{
		"vectors": cvs.store.Count(context.Background()),
		"nodes":   len(cvs.nodes),
		"dim":     EmbeddingDimension,
	}
}

// Helper to check if string contains substring
