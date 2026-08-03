package codegraph

import (
	"testing"
)

func TestEmbeddingCache_Memoizes(t *testing.T) {
	cg := &CodeGraph{embeddingCache: make(map[string][]float32)}
	n := Node{
		Name:          "LoadConfig",
		QualifiedName: "config::LoadConfig",
		Kind:          "function",
		Language:      "go",
		FilePath:      "internal/config/load.go",
		Signature:     "func LoadConfig(path string) (*Config, error)",
		Docstring:     "Loads configuration from a file.",
	}

	first := cg.embeddingFor(n)
	second := cg.embeddingFor(n)
	if &first[0] != &second[0] {
		t.Error("expected the same cached vector for identical node content")
	}
	if len(cg.embeddingCache) != 1 {
		t.Errorf("cache size = %d, want 1", len(cg.embeddingCache))
	}
}

func TestEmbeddingCache_InvalidatesOnContentChange(t *testing.T) {
	cg := &CodeGraph{embeddingCache: make(map[string][]float32)}
	base := Node{
		Name:          "LoadConfig",
		QualifiedName: "config::LoadConfig",
		Kind:          "function",
		Language:      "go",
		FilePath:      "internal/config/load.go",
	}

	before := cg.embeddingFor(base)

	edited := base
	edited.Docstring = "Loads configuration from a file, with retries."
	after := cg.embeddingFor(edited)

	if len(cg.embeddingCache) != 2 {
		t.Errorf("cache size = %d, want 2 (content change must not reuse the old entry)", len(cg.embeddingCache))
	}
	equal := true
	for i := range before {
		if before[i] != after[i] {
			equal = false
			break
		}
	}
	if equal {
		t.Error("embeddings for different docstrings must differ")
	}
}

func TestEmbeddingCache_Bounded(t *testing.T) {
	original := maxEmbeddingCacheEntries
	maxEmbeddingCacheEntries = 5
	defer func() { maxEmbeddingCacheEntries = original }()

	cg := &CodeGraph{embeddingCache: make(map[string][]float32)}
	for i := 0; i < 20; i++ {
		cg.embeddingFor(Node{
			Name:          "fn_" + string(rune('a'+i)),
			QualifiedName: "pkg::fn_" + string(rune('a'+i)),
			Kind:          "function",
			Language:      "go",
		})
	}
	if len(cg.embeddingCache) > 5 {
		t.Errorf("cache grew past bound: %d entries", len(cg.embeddingCache))
	}
}
