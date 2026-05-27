package codegraph

import (
	"math"
	"testing"
)

func TestGenerateEmbedding_Deterministic(t *testing.T) {
	t.Parallel()
	node := Node{
		Name:          "HandleRequest",
		QualifiedName: "server::HandleRequest",
		Kind:          "function",
		Language:      "go",
		Docstring:     "handles incoming HTTP requests",
		Signature:     "func HandleRequest(w http.ResponseWriter, r *http.Request)",
		FilePath:      "server/handler.go",
	}

	vec1 := GenerateEmbedding(node)
	vec2 := GenerateEmbedding(node)

	if len(vec1) != EmbeddingDimension {
		t.Fatalf("expected dimension %d, got %d", EmbeddingDimension, len(vec1))
	}
	if len(vec1) != len(vec2) {
		t.Fatalf("vector lengths differ: %d vs %d", len(vec1), len(vec2))
	}

	for i := range vec1 {
		if vec1[i] != vec2[i] {
			t.Errorf("embedding not deterministic at index %d: %f vs %f", i, vec1[i], vec2[i])
		}
	}
}

func TestGenerateEmbedding_L2Normalized(t *testing.T) {
	t.Parallel()
	node := Node{
		Name:     "ProcessData",
		Kind:     "function",
		Language: "go",
	}

	vec := GenerateEmbedding(node)
	var norm float32
	for _, v := range vec {
		norm += v * v
	}
	norm = float32(math.Sqrt(float64(norm)))

	// Should be approximately 1.0 (L2 normalized)
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("expected L2 norm ~1.0, got %f", norm)
	}
}

func TestGenerateEmbedding_DifferentNodesDiffer(t *testing.T) {
	t.Parallel()
	node1 := Node{Name: "HandleAuth", Kind: "function", Language: "go"}
	node2 := Node{Name: "ProcessPayment", Kind: "function", Language: "go"}

	vec1 := GenerateEmbedding(node1)
	vec2 := GenerateEmbedding(node2)

	// Vectors should be different
	same := true
	for i := range vec1 {
		if vec1[i] != vec2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different nodes should produce different embeddings")
	}
}

func TestCosineSimilarity_Identical(t *testing.T) {
	t.Parallel()
	vec := []float32{1.0, 0.0, 0.0}
	sim := CosineSimilarity(vec, vec)
	if sim < 0.99 || sim > 1.01 {
		t.Errorf("expected ~1.0 for identical vectors, got %f", sim)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	t.Parallel()
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{0.0, 1.0, 0.0}
	sim := CosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("expected 0 for orthogonal vectors, got %f", sim)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	t.Parallel()
	a := []float32{1.0, 0.0}
	b := []float32{-1.0, 0.0}
	sim := CosineSimilarity(a, b)
	if sim > -0.99 || sim < -1.01 {
		t.Errorf("expected ~-1.0 for opposite vectors, got %f", sim)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	t.Parallel()
	a := []float32{1.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}
	sim := CosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("expected 0 for different-length vectors, got %f", sim)
	}
}

func TestCosineSimilarity_Empty(t *testing.T) {
	t.Parallel()
	sim := CosineSimilarity(nil, nil)
	if sim != 0 {
		t.Errorf("expected 0 for nil vectors, got %f", sim)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	t.Parallel()
	a := []float32{0.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}
	sim := CosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("expected 0 for zero vector, got %f", sim)
	}
}

func TestSplitIdentifier_CamelCase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected []string
	}{
		{"HandleRequest", []string{"handle", "request"}},
		{"getUserName", []string{"get", "user", "name"}},
		{"APIKey", []string{"a", "p", "i", "key"}},
		{"simple", []string{"simple"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitIdentifier(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("splitIdentifier(%q) = %v, want %v", tt.input, got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("splitIdentifier(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestSplitIdentifier_SnakeCase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected []string
	}{
		{"handle_request", []string{"handle", "request"}},
		{"get_user_name", []string{"get", "user", "name"}},
		{"api_key", []string{"api", "key"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitIdentifier(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("splitIdentifier(%q) = %v, want %v", tt.input, got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("splitIdentifier(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestL2Normalize_ZeroVector(t *testing.T) {
	t.Parallel()
	vec := []float32{0.0, 0.0, 0.0}
	result := l2Normalize(vec)
	// Should not panic; zero vector stays zero
	for i, v := range result {
		if v != 0 {
			t.Errorf("expected 0 at index %d, got %f", i, v)
		}
	}
}

func TestHashToDimension_Bounds(t *testing.T) {
	t.Parallel()
	dim := 128
	for i := 0; i < 100; i++ {
		idx := hashToDimension("test_feature_"+string(rune(i+'0')), dim)
		if idx < 0 || idx >= dim {
			t.Errorf("hashToDimension returned %d, expected 0-%d", idx, dim-1)
		}
	}
}

func TestHashToSign_Values(t *testing.T) {
	t.Parallel()
	for i := 0; i < 50; i++ {
		sign := hashToSign("feature_" + string(rune(i+'0')))
		if sign != 1.0 && sign != -1.0 {
			t.Errorf("hashToSign returned %f, expected 1.0 or -1.0", sign)
		}
	}
}
