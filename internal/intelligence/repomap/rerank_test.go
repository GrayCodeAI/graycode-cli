package repomap

import (
	"testing"
)

func TestRerank_BasicOrdering(t *testing.T) {
	candidates := []CodeSearchResult{
		{Path: "a.go", Content: "func FormatDate(t time.Time) string { return t.String() }", Symbol: "FormatDate"},
		{Path: "b.go", Content: "func Authenticate(token string) bool { return validateToken(token) }", Symbol: "Authenticate"},
		{Path: "c.go", Content: "func HandleRequest(w http.ResponseWriter, r *http.Request) { authenticate(r) }", Symbol: "HandleRequest"},
	}

	results := Rerank("authenticate token", candidates, 3)
	if len(results) == 0 {
		t.Fatal("expected results from Rerank")
	}

	// The Authenticate function should rank highest for "authenticate token"
	if results[0].Chunk.Symbol != "Authenticate" {
		t.Errorf("expected Authenticate to rank first, got %q", results[0].Chunk.Symbol)
	}

	// All scores should be non-negative
	for i, r := range results {
		if r.Score < 0 {
			t.Errorf("result %d has negative score: %f", i, r.Score)
		}
	}
}

func TestRerank_TopK(t *testing.T) {
	candidates := []CodeSearchResult{
		{Path: "a.go", Content: "func Alpha() {}", Symbol: "Alpha"},
		{Path: "b.go", Content: "func Beta() {}", Symbol: "Beta"},
		{Path: "c.go", Content: "func Gamma() {}", Symbol: "Gamma"},
		{Path: "d.go", Content: "func Delta() {}", Symbol: "Delta"},
	}

	results := Rerank("alpha beta gamma delta", candidates, 2)
	if len(results) != 2 {
		t.Errorf("expected 2 results with topK=2, got %d", len(results))
	}
}

func TestRerank_EmptyCandidates(t *testing.T) {
	results := Rerank("some query", nil, 5)
	if results != nil {
		t.Errorf("expected nil for empty candidates, got %v", results)
	}

	results = Rerank("some query", []CodeSearchResult{}, 5)
	if results != nil {
		t.Errorf("expected nil for empty candidates slice, got %v", results)
	}
}
