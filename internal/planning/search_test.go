package planning

import "testing"

// staticTree is a deterministic Expander backed by a map.
type staticTree map[string][]string

func (t staticTree) Expand(state string) []string { return t[state] }

// scoreMap is a deterministic Scorer backed by a map.
type scoreMap map[string]float64

func (s scoreMap) Score(state string) float64 { return s[state] }

func TestBeamSearch_FindsBestLeaf(t *testing.T) {
	expand := staticTree{
		"root": {"a", "b"},
		"a":    {"a1", "a2"},
		"b":    {"b1"},
	}
	score := scoreMap{
		"root": 0, "a": 1, "b": 1,
		"a1": 10, "a2": 5, "b1": 7,
	}
	best := BeamSearch("root", expand, score, 2, 3)
	if best == nil || best.State != "a1" {
		t.Fatalf("best = %+v, want a1 (score 10)", best)
	}
}

func TestBeamSearch_BacktracksDeadEnd(t *testing.T) {
	// "a" is a dead-end (no expansion); the search must prune it and fall back
	// to "b" instead of returning nil or a stale node.
	expand := staticTree{
		"root": {"a", "b"},
		"a":    {},
		"b":    {"b1"},
	}
	score := scoreMap{"root": 0, "a": 9, "b": 1, "b1": 7}
	best := BeamSearch("root", expand, score, 2, 3)
	if best == nil || best.State != "b1" {
		t.Fatalf("best = %+v, want b1 (dead-end a must be pruned)", best)
	}
}

func TestBeamSearch_BeamWidthLimits(t *testing.T) {
	expand := staticTree{
		"root": {"a", "b", "c"},
		"a":    {"a1"}, "b": {"b1"}, "c": {"c1"},
	}
	score := scoreMap{
		"root": 0, "a": 5, "b": 3, "c": 1,
		"a1": 10, "b1": 9, "c1": 8,
	}
	// Beam width 1 keeps only the top scorer per depth, so only a's branch is
	// explored; best is a1.
	best := BeamSearch("root", expand, score, 1, 3)
	if best == nil || best.State != "a1" {
		t.Fatalf("beam=1 best = %+v, want a1", best)
	}
}

func TestBeamSearch_Deterministic(t *testing.T) {
	expand := staticTree{"root": {"a", "b"}, "a": {"a1"}, "b": {"b1"}}
	score := scoreMap{"root": 0, "a": 1, "b": 1, "a1": 5, "b1": 4}
	x := BeamSearch("root", expand, score, 2, 3)
	y := BeamSearch("root", expand, score, 2, 3)
	if x.State != y.State {
		t.Fatalf("non-deterministic: %q vs %q", x.State, y.State)
	}
}

func TestBeamSearch_NoExpansionReturnsRoot(t *testing.T) {
	expand := staticTree{"root": {}}
	score := scoreMap{"root": 0}
	best := BeamSearch("root", expand, score, 2, 3)
	if best == nil || best.State != "root" {
		t.Fatalf("best = %+v, want root", best)
	}
}
