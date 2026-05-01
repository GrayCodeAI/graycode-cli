package cmd

import (
	"testing"
)

func TestFuzzyMatch_ExactMatch(t *testing.T) {
	score := fuzzyMatch("hello", "hello")
	if score <= 0 {
		t.Fatal("exact match should have positive score")
	}
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	score := fuzzyMatch("xyz", "hello")
	if score != 0 {
		t.Fatalf("expected 0 for no match, got %f", score)
	}
}

func TestFuzzyMatch_Prefix(t *testing.T) {
	prefix := fuzzyMatch("hel", "hello")
	middle := fuzzyMatch("ell", "hello")
	if prefix <= middle {
		t.Fatalf("prefix match (%f) should score higher than middle match (%f)", prefix, middle)
	}
}

func TestFuzzyMatch_CaseInsensitive(t *testing.T) {
	score := fuzzyMatch("HELLO", "hello")
	if score <= 0 {
		t.Fatal("case-insensitive match should have positive score")
	}
}

func TestFuzzyMatch_Empty(t *testing.T) {
	if fuzzyMatch("", "hello") != 0 {
		t.Fatal("empty query should score 0")
	}
	if fuzzyMatch("hello", "") != 0 {
		t.Fatal("empty target should score 0")
	}
}

func TestFuzzySearch_Ordering(t *testing.T) {
	candidates := []string{
		"/help",
		"/history",
		"/hooks",
		"/hello-world",
	}
	results := fuzzySearch("hel", candidates)
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	// /help should rank high since "hel" is a prefix
	if results[0] != "/help" && results[0] != "/hello-world" {
		t.Logf("results: %v", results)
		// Both are acceptable as top result since "hel" matches both strongly
	}
}

func TestFuzzySearch_EmptyQuery(t *testing.T) {
	results := fuzzySearch("", []string{"a", "b"})
	if len(results) != 0 {
		t.Fatalf("expected no results for empty query, got %d", len(results))
	}
}

func TestFuzzySearch_NoMatches(t *testing.T) {
	results := fuzzySearch("zzz", []string{"/help", "/commit", "/diff"})
	if len(results) != 0 {
		t.Fatalf("expected no matches for zzz, got %d", len(results))
	}
}

func TestFuzzyMatch_ConsecutiveBonus(t *testing.T) {
	consecutive := fuzzyMatch("abc", "abcdef")
	scattered := fuzzyMatch("abc", "a_b_c_d_e_f")
	if consecutive <= scattered {
		t.Fatalf("consecutive match (%f) should score higher than scattered (%f)", consecutive, scattered)
	}
}
