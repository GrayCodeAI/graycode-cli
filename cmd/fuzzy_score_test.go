package cmd

import (
	"testing"
)

func TestFuzzyScore_ExactPrefix(t *testing.T) {
	t.Parallel()
	score := FuzzyScore("/com", "/commit")
	if score <= 0 {
		t.Error("exact prefix should have positive score")
	}
	// Should score higher than substring
	subScore := FuzzyScore("/com", "some /command")
	if score <= subScore {
		t.Error("exact prefix should score higher than substring")
	}
}

func TestFuzzyScore_Subsequence(t *testing.T) {
	t.Parallel()
	score := FuzzyScore("cm", "/commit")
	if score <= 0 {
		t.Error("subsequence 'cm' should match '/commit'")
	}
}

func TestFuzzyScore_NoMatch(t *testing.T) {
	t.Parallel()
	score := FuzzyScore("xyz", "/commit")
	if score != -1 {
		t.Errorf("expected -1 for no match, got %d", score)
	}
}

func TestFuzzyScore_EmptyQuery(t *testing.T) {
	t.Parallel()
	score := FuzzyScore("", "/commit")
	if score <= 0 {
		t.Error("empty query should match everything with score > 0")
	}
}

func TestFuzzyScore_WordBoundary(t *testing.T) {
	t.Parallel()
	// "sec" should match "/security-review" well due to word boundary after /
	score1 := FuzzyScore("sec", "/security-review")
	// "sec" matching "asecure" (no boundary)
	score2 := FuzzyScore("sec", "asecure")
	if score1 <= score2 {
		t.Error("word boundary match should score higher")
	}
}

func TestFuzzyScore_CaseInsensitive(t *testing.T) {
	t.Parallel()
	lower := FuzzyScore("commit", "/COMMIT")
	upper := FuzzyScore("COMMIT", "/commit")
	if lower != upper {
		t.Error("scoring should be case-insensitive")
	}
}

func TestRankFuzzyResults(t *testing.T) {
	t.Parallel()
	entries := []CommandPaletteEntry{
		{Name: "/commit", Description: "Auto-commit changes", Category: "Workflow"},
		{Name: "/config", Description: "Open settings", Category: "Core"},
		{Name: "/compact", Description: "Compress conversation", Category: "Core"},
		{Name: "/clear", Description: "Clear display", Category: "Core"},
	}

	ranked := RankFuzzyResults("com", entries)
	if len(ranked) == 0 {
		t.Fatal("expected at least one result")
	}
	// First result should be /commit or /compact (best prefix match)
	if ranked[0].Entry.Name != "/commit" && ranked[0].Entry.Name != "/compact" && ranked[0].Entry.Name != "/config" {
		t.Errorf("expected a /com* command first, got %s", ranked[0].Entry.Name)
	}
}

func TestRankFuzzyResults_NoMatch(t *testing.T) {
	t.Parallel()
	entries := []CommandPaletteEntry{
		{Name: "/commit", Description: "Auto-commit", Category: "Workflow"},
	}
	ranked := RankFuzzyResults("zzz", entries)
	if len(ranked) != 0 {
		t.Errorf("expected 0 results for non-matching query, got %d", len(ranked))
	}
}
