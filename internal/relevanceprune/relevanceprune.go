// Package relevanceprune provides deterministic, token-budgeted context
// pruning that keeps messages relevant to the current task while preserving
// recent turns, tool calls, and error messages verbatim. It is the Go port of
// OpenClaude's relevance-based context pruning — a cheap, predictable
// alternative to pure LLM summarization: score older messages by keyword
// overlap against the task context, keep the highest-relevance groups up to a
// token budget, and always retain the most recent tail.
package relevanceprune

import (
	"sort"
	"strings"
	"time"
)

// DefaultCompactTailTurns is the number of recent messages preserved verbatim.
const DefaultCompactTailTurns = 3

// NormalizeCompactTailTurns floors any finite value >= 1 to an integer and
// falls back to the default for everything else (0, negatives, fractions below
// 1, NaN). UI and runtime must share this single rule.
func NormalizeCompactTailTurns(value int) int {
	if value >= 1 {
		return value
	}
	return DefaultCompactTailTurns
}

// Message is a minimal conversation message the pruner understands.
type Message struct {
	Role        string // "user", "assistant", or "system"
	Content     string
	Timestamp   time.Time
	HasToolCall bool
	IsError     bool
}

// Options configures a pruning pass.
type Options struct {
	TargetTokens   int
	TaskContext    string
	PreserveRecent int
	PreserveTools  bool
	PreserveErrors bool
}

var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "but": true,
	"not": true, "you": true, "all": true, "can": true, "had": true,
	"her": true, "was": true, "one": true, "our": true, "out": true,
	"has": true, "have": true, "they": true, "will": true, "would": true,
}

func extractKeywords(text string) map[string]bool {
	words := strings.Fields(strings.ToLower(text))
	keywords := map[string]bool{}
	for _, w := range words {
		cleaned := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			return -1
		}, w)
		if len(cleaned) > 3 && !stopWords[cleaned] {
			keywords[cleaned] = true
		}
	}
	return keywords
}

func keywordOverlap(a, b string) float64 {
	ka := extractKeywords(a)
	kb := extractKeywords(b)
	overlap := 0
	for k := range ka {
		if kb[k] {
			overlap++
		}
	}
	total := len(ka) + len(kb)
	if total == 0 {
		return 0
	}
	return float64(2*overlap) / float64(total)
}

// CalculateRelevance scores a message in [0,1]. Base 0.5, boosted by keyword
// overlap with the task context, preserved tool calls / errors, recency, and
// user role.
func CalculateRelevance(m Message, o Options) float64 {
	score := 0.5
	if o.TaskContext != "" {
		score += keywordOverlap(m.Content, o.TaskContext) * 0.3
	}
	if m.HasToolCall && o.PreserveTools {
		score += 0.25
	}
	if m.IsError && o.PreserveErrors {
		score += 0.3
	}
	if !m.Timestamp.IsZero() && time.Since(m.Timestamp) < time.Hour {
		score += 0.15
	}
	if m.Role == "user" {
		score += 0.1
	}
	if score > 1 {
		return 1
	}
	return score
}

// PruneByRelevance drops low-relevance history to fit targetTokens while
// preserving the most recent preserveRecent messages verbatim. Older messages
// are grouped by API round and scored by average relevance; the highest-scoring
// groups are kept until the token budget is exhausted, then everything is
// re-sorted chronologically.
func PruneByRelevance(messages []Message, o Options) []Message {
	target := o.TargetTokens
	if target <= 0 {
		target = 5000
	}
	preserve := NormalizeCompactTailTurns(o.PreserveRecent)
	if len(messages) <= preserve {
		return messages
	}
	recent := messages[len(messages)-preserve:]
	older := messages[:len(messages)-preserve]

	groups := groupByRound(older)
	scored := make([]struct {
		group []Message
		score float64
	}, 0, len(groups))
	for _, g := range groups {
		var sum float64
		for _, m := range g {
			sum += CalculateRelevance(m, o)
		}
		scored = append(scored, struct {
			group []Message
			score float64
		}{group: g, score: sum / float64(len(g))})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].group[0].Timestamp.After(scored[j].group[0].Timestamp)
	})

	result := make([]Message, 0, len(recent))
	result = append(result, recent...)
	totalTokens := 0
	for _, s := range scored {
		var content strings.Builder
		for _, m := range s.group {
			content.WriteString(m.Content)
		}
		tokens := RoughTokenCount(content.String())
		if totalTokens+tokens > target {
			continue
		}
		result = append(result, s.group...)
		totalTokens += tokens
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Timestamp.Before(result[j].Timestamp) })
	return result
}

// RoughTokenCount estimates tokens as chars/4, matching the source's cheap
// heuristic.
func RoughTokenCount(text string) int {
	return len(text) / 4
}

// GetRelevanceStats summarizes a set of messages.
func GetRelevanceStats(messages []Message, o Options) (average float64, highCount, toolCalls, errors int) {
	var sum float64
	for _, m := range messages {
		score := CalculateRelevance(m, o)
		sum += score
		if score > 0.7 {
			highCount++
		}
		if m.HasToolCall {
			toolCalls++
		}
		if m.IsError {
			errors++
		}
	}
	if len(messages) > 0 {
		average = sum / float64(len(messages))
	}
	return
}

// groupByRound groups consecutive messages that share an assistant response
// round (an assistant message starts a new group unless it repeats the previous
// assistant's id — here we approximate by starting a new group at each assistant
// message).
func groupByRound(messages []Message) [][]Message {
	var groups [][]Message
	var current []Message
	for _, m := range messages {
		if m.Role == "assistant" && len(current) > 0 {
			groups = append(groups, current)
			current = []Message{}
		}
		current = append(current, m)
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}
