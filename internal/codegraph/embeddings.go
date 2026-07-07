package codegraph

import (
	"crypto/sha256"
	"math"
	"strings"
)

// EmbeddingDimension is the size of the hash-based embedding vector.
// 128 dimensions provides good separation for code symbols without
// requiring external ML models.
const EmbeddingDimension = 128

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
	if dim <= 0 {
		return 0
	}
	if dim > math.MaxUint32 {
		// Cannot happen in practice (dim is a small embedding dimension),
		// but guard against silent overflow in the uint32 conversion below.
		dim = math.MaxUint32
	}
	return int(val % uint32(dim)) // #nosec G115 -- dim bounds-checked above, cannot overflow uint32
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
