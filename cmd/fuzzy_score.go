package cmd

import (
	"strings"
	"unicode"
)

// FuzzyMatchResult holds a scored fuzzy match result.
type FuzzyMatchResult struct {
	Score    int
	MatchIdx []int // indices of matched characters in the target
}

// FuzzyScore computes a relevance score for how well query matches target.
// Higher scores indicate better matches. Returns -1 if no match.
// Scoring priorities:
//   - Exact prefix match (highest)
//   - Consecutive character bonus
//   - Word boundary bonus (matching after / or _ or -)
//   - CamelCase boundary bonus
func FuzzyScore(query, target string) int {
	if query == "" {
		return 1
	}
	q := strings.ToLower(query)
	t := strings.ToLower(target)

	// Exact prefix match — best possible
	if strings.HasPrefix(t, q) {
		return 100 + len(q)*10
	}

	// Substring match
	if idx := strings.Index(t, q); idx >= 0 {
		score := 50 + len(q)*5 - idx
		// Word boundary bonus for substring starting at a separator
		if idx > 0 {
			prev := rune(t[idx-1])
			if prev == '/' || prev == '_' || prev == '-' || prev == ' ' {
				score += 10
			}
		}
		return score
	}

	// Subsequence match with scoring
	qi := 0
	score := 0
	consecutive := 0
	prevMatchIdx := -2

	for ti := 0; ti < len(t) && qi < len(q); ti++ {
		if t[ti] == q[qi] {
			// Base match score
			matchScore := 1

			// Consecutive bonus
			if ti == prevMatchIdx+1 {
				consecutive++
				matchScore += consecutive * 3
			} else {
				consecutive = 0
			}

			// Word boundary bonus (/ _ - space)
			if ti > 0 {
				prev := rune(t[ti-1])
				if prev == '/' || prev == '_' || prev == '-' || prev == ' ' {
					matchScore += 10
				}
			}

			// CamelCase boundary bonus
			if ti > 0 && ti < len(target) {
				cur := rune(target[ti])
				prev := rune(target[ti-1])
				if unicode.IsUpper(cur) && unicode.IsLower(prev) {
					matchScore += 8
				}
			}

			score += matchScore
			prevMatchIdx = ti
			qi++
		}
	}

	if qi == len(q) {
		return score
	}
	return -1
}

// RankFuzzyResults sorts entries by fuzzy score against the query.
// Only entries with score > 0 are returned.
type RankedEntry struct {
	Entry CommandPaletteEntry
	Score int
}

func RankFuzzyResults(query string, entries []CommandPaletteEntry) []RankedEntry {
	var ranked []RankedEntry
	for _, e := range entries {
		// Try name, description, and category
		nameScore := FuzzyScore(query, e.Name)
		descScore := FuzzyScore(query, e.Description)
		catScore := FuzzyScore(query, e.Category)

		best := nameScore
		if descScore > best {
			best = descScore
		}
		if catScore > best {
			best = catScore
		}

		if best > 0 {
			ranked = append(ranked, RankedEntry{Entry: e, Score: best})
		}
	}

	// Sort by score descending (simple insertion sort for small lists)
	for i := 1; i < len(ranked); i++ {
		for j := i; j > 0 && ranked[j].Score > ranked[j-1].Score; j-- {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
		}
	}

	return ranked
}
