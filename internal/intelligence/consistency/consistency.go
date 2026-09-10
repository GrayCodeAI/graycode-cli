// Package consistency implements self-consistency (Wang et al., ICLR 2023,
// arXiv 2203.11171): sampling multiple candidate answers to the same question
// and selecting the consensus. Sampling N diverse solutions and taking the
// majority is a cheap reliability boost over a single greedy answer.
package consistency

import "strings"

// Consensus selects the most frequent candidate answer and returns it along
// with its frequency (confidence in [0,1]). Ties are broken by the candidate
// that appears first. The comparison is case-insensitive and trims whitespace,
// so "Yes" and " yes " count as the same answer.
//
// An empty candidate list yields ("", 0). This is deterministic: the same
// candidates always produce the same consensus.
func Consensus(candidates []string) (string, float64) {
	if len(candidates) == 0 {
		return "", 0
	}
	counts := make(map[string]int, len(candidates))
	order := make([]string, 0, len(candidates))
	for _, c := range candidates {
		key := strings.ToLower(strings.TrimSpace(c))
		if _, ok := counts[key]; !ok {
			order = append(order, key)
		}
		counts[key]++
	}

	bestKey := order[0]
	bestCount := counts[bestKey]
	for _, k := range order[1:] {
		if counts[k] > bestCount {
			bestKey = k
			bestCount = counts[k]
		}
	}

	// Return the original (un-normalized) text of the consensus candidate so
	// callers get a usable answer, not a lowercased one.
	for _, c := range candidates {
		if strings.ToLower(strings.TrimSpace(c)) == bestKey {
			return strings.TrimSpace(c), float64(bestCount) / float64(len(candidates))
		}
	}
	return strings.TrimSpace(candidates[0]), float64(bestCount) / float64(len(candidates))
}

// ConsensusBy splits each candidate into lines and applies Consensus to the
// first non-empty line, useful for answers with a leading verdict line. It is a
// convenience wrapper; for arbitrary answers use Consensus directly.
func ConsensusBy(candidates []string) (string, float64) {
	firstLines := make([]string, 0, len(candidates))
	for _, c := range candidates {
		line := c
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		firstLines = append(firstLines, strings.TrimSpace(line))
	}
	return Consensus(firstLines)
}
