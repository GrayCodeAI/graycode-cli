package engine

import (
	"strings"
)

// Prompt-queue combining, adopted from grok-build's xai-prompt-queue
// combine rules: when several plain queued prompts are waiting, they are
// merged into a single turn instead of burning one full agent loop per
// prompt. Only mergeable prompts combine; anything synthetic, high-priority,
// or carrying structured payload stays its own turn so stop conditions and
// display cannot drift.

const promptCombineSeparator = "\n\n"

// CombineGate describes one queued item's eligibility for merging. It is a
// pure value so the rules stay testable without a live queue.
type CombineGate struct {
	// PlainPrompt is a normal user text turn — not steering, not an
	// interjection, not a command/synthetic origin.
	PlainPrompt bool
	// Synthetic origins (auto-wake, scheduled) never combine.
	Synthetic bool
	// Bash/command turns keep their own turn semantics.
	Bash bool
	// HasImages: followers must be text-only; a front prompt may carry images.
	HasImages bool
	// Text is the non-empty body participating in the join.
	Text string
}

// CanCombineFront reports whether gate can lead a combined run.
func CanCombineFront(g CombineGate) bool {
	return g.PlainPrompt && !g.Synthetic && !g.Bash && g.Text != ""
}

// CanCombineFollower reports whether gate can follow a front in the same
// combined turn.
func CanCombineFollower(g CombineGate) bool {
	return CanCombineFront(g) && !g.HasImages
}

// CombinePrefixLen returns the length of the mergeable prefix of gates,
// including the front. 0 for empty input, 1 when only the front is taken.
func CombinePrefixLen(gates []CombineGate) int {
	if len(gates) == 0 {
		return 0
	}
	if !CanCombineFront(gates[0]) {
		return 1
	}
	n := 1
	for _, g := range gates[1:] {
		if !CanCombineFollower(g) {
			break
		}
		n++
	}
	return n
}

// JoinTexts joins non-empty texts with the combine separator.
func JoinTexts(texts []string) string {
	out := make([]string, 0, len(texts))
	for _, t := range texts {
		if t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, promptCombineSeparator)
}

// IsCombined reports whether at least two original prompts were merged.
func IsCombined(segs []string) bool { return len(segs) >= 2 }

// combineGateFor derives the eligibility gate from an enqueued prompt. Source
// "user" is the plain path; anything else (cron, background notify, /btw
// interjections, bash results) keeps its own turn. Priority above Normal is
// never merged — steering must land promptly, not be diluted.
func combineGateFor(p EnqueuedPrompt) CombineGate {
	plain := p.Source == "user" || p.Source == "" ||
		strings.EqualFold(p.Source, "telegram") || strings.EqualFold(p.Source, "discord") ||
		strings.EqualFold(p.Source, "slack")
	hasImages := false
	if p.Metadata != nil {
		if v, ok := p.Metadata["images"]; ok && v != nil {
			switch imgs := v.(type) {
			case []string:
				hasImages = len(imgs) > 0
			case []interface{}:
				hasImages = len(imgs) > 0
			case bool:
				hasImages = imgs
			}
		}
		if v, ok := p.Metadata["synthetic"].(bool); ok && v {
			return CombineGate{Synthetic: true}
		}
		if v, ok := p.Metadata["bash"].(bool); ok && v {
			return CombineGate{Bash: true}
		}
	}
	return CombineGate{
		PlainPrompt: plain,
		Synthetic:   false,
		Bash:        false,
		HasImages:   hasImages,
		Text:        strings.TrimSpace(p.Text),
	}
}

// DequeueCombined pops the next turn from the queue, merging the run of
// eligible followers behind it into one prompt. It returns the (possibly
// joined) prompt, the IDs consumed by the turn, and whether a merge happened.
func (pq *PromptQueue) DequeueCombined() (EnqueuedPrompt, []string, bool) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		return EnqueuedPrompt{}, nil, false
	}

	gates := make([]CombineGate, len(pq.items))
	for i := range pq.items {
		gates[i] = combineGateFor(pq.items[i])
	}
	n := CombinePrefixLen(gates)

	front := pq.items[0]
	consumedIDs := []string{front.ID}
	if n <= 1 {
		pq.items = pq.items[1:]
		return front, consumedIDs, false
	}

	segs := make([]string, 0, n)
	for i := 0; i < n; i++ {
		segs = append(segs, strings.TrimSpace(pq.items[i].Text))
		if i > 0 {
			consumedIDs = append(consumedIDs, pq.items[i].ID)
		}
	}
	merged := EnqueuedPrompt{
		ID:       front.ID,
		Text:     JoinTexts(segs),
		Priority: front.Priority,
		Source:   front.Source,
		Metadata: map[string]interface{}{
			// Downstream UI can render one bubble per original prompt.
			"combined_display_texts": segs,
			"combined_ids":           consumedIDs,
		},
	}
	pq.items = pq.items[n:]
	return merged, consumedIDs, true
}
