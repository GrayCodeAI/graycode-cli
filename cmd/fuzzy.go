package cmd

import (
	"sort"
	"strings"
	"unicode"
)

// fuzzyMatch scores how well a query matches a target string on a scale of
// 0 to 1 (higher is better). It uses character-by-character matching with
// bonuses for consecutive matches, matches at the start, and case-sensitive
// matches.
func fuzzyMatch(query, target string) float64 {
	if query == "" {
		return 0
	}
	if target == "" {
		return 0
	}

	queryLower := strings.ToLower(query)
	targetLower := strings.ToLower(target)

	queryRunes := []rune(queryLower)
	targetRunes := []rune(targetLower)
	origTargetRunes := []rune(target)
	origQueryRunes := []rune(query)

	// Check if all query characters exist in order in target
	qi := 0
	for ti := 0; ti < len(targetRunes) && qi < len(queryRunes); ti++ {
		if targetRunes[ti] == queryRunes[qi] {
			qi++
		}
	}
	if qi < len(queryRunes) {
		return 0 // not all characters matched
	}

	// Score the match
	score := 0.0
	maxScore := 0.0

	qi = 0
	lastMatchIdx := -1
	consecutiveBonus := 0.0

	for ti := 0; ti < len(targetRunes) && qi < len(queryRunes); ti++ {
		if targetRunes[ti] == queryRunes[qi] {
			points := 1.0

			// Consecutive match bonus
			if lastMatchIdx == ti-1 {
				consecutiveBonus += 0.5
				points += consecutiveBonus
			} else {
				consecutiveBonus = 0
			}

			// Start of string bonus
			if ti == 0 {
				points += 2.0
			}

			// Start of word bonus (after separator or case boundary)
			if ti > 0 {
				prev := origTargetRunes[ti-1]
				curr := origTargetRunes[ti]
				if prev == '/' || prev == '-' || prev == '_' || prev == ' ' || prev == '.' {
					points += 1.5
				}
				// CamelCase boundary
				if unicode.IsLower(prev) && unicode.IsUpper(curr) {
					points += 1.0
				}
			}

			// Exact case match bonus
			if qi < len(origQueryRunes) && origTargetRunes[ti] == origQueryRunes[qi] {
				points += 0.3
			}

			score += points
			lastMatchIdx = ti
			qi++
		}

		maxScore += 3.3 // theoretical max per position
	}

	// Normalize by the query length and target length
	if maxScore == 0 {
		return 0
	}

	// Base score: ratio of achieved score to max possible score
	normalized := score / (float64(len(queryRunes)) * 4.3)
	if normalized > 1.0 {
		normalized = 1.0
	}

	// Penalize very long targets slightly (prefer shorter matches)
	lengthPenalty := float64(len(queryRunes)) / float64(len(targetRunes))
	if lengthPenalty > 1.0 {
		lengthPenalty = 1.0
	}

	return normalized*0.8 + lengthPenalty*0.2
}

// fuzzySearch returns candidates sorted by match score (best first).
// Only candidates with a non-zero score are included.
func fuzzySearch(query string, candidates []string) []string {
	if query == "" {
		return nil
	}

	type scored struct {
		text  string
		score float64
	}

	var results []scored
	for _, c := range candidates {
		s := fuzzyMatch(query, c)
		if s > 0 {
			results = append(results, scored{text: c, score: s})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.text
	}
	return out
}
