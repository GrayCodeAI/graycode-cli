package engine

import (
	"context"
	"sort"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// Two-tier context management (adopted from herm): before compacting, "clear"
// the content of old tool results into a short placeholder to reclaim context
// gently. The tool_use blocks are left intact, so the model still knows what was
// called, and can re-read files if needed. Only when clearing is insufficient
// does the compaction strategy chain run.

const (
	// clearThresholdFraction is the fraction of the context window at which old
	// tool results start getting cleared. 0.8 = clear when input tokens > 80%.
	clearThresholdFraction = 0.8
	// clearKeepRecent is the number of most-recent tool-result messages kept intact.
	clearKeepRecent = 4
	// outputClearedPlaceholder replaces cleared tool-result content.
	outputClearedPlaceholder = "[output cleared]"
)

// clearOldToolResults returns a copy of msgs with the content of old tool
// results replaced by a short placeholder, biggest-first, keeping the most
// recent clearKeepRecent tool-result messages intact, until estimatedTokens
// drops below threshold. It returns the new messages and the estimated tokens
// freed. Adopted from herm's clearOldToolResults.
func clearOldToolResults(msgs []types.EyrieMessage, estimatedTokens, threshold int) ([]types.EyrieMessage, int) {
	if threshold <= 0 || estimatedTokens <= 0 || len(msgs) == 0 {
		return msgs, 0
	}
	type candidate struct {
		idx  int
		size int
	}
	var candidates []candidate
	for i := range msgs {
		if size := toolResultBytes(msgs[i]); size > 0 {
			candidates = append(candidates, candidate{idx: i, size: size})
		}
	}
	if len(candidates) <= clearKeepRecent {
		return msgs, 0
	}
	clearable := candidates[:len(candidates)-clearKeepRecent]
	sort.Slice(clearable, func(i, j int) bool { return clearable[i].size > clearable[j].size })

	out := cloneToolResults(msgs)
	tokens := estimatedTokens
	freed := 0
	for _, c := range clearable {
		if tokens < threshold {
			break
		}
		f := clearMessageToolResults(&out[c.idx])
		if f == 0 {
			continue
		}
		tokens -= f
		freed += f
	}
	return out, freed
}

// ClearOldToolResults is the session-level entry point: it clears old tool
// results when token usage crosses 80% of the context window. Returns true when
// anything was cleared. It is a gentler tier below compaction.
func (s *Session) ClearOldToolResults(ctx context.Context) bool {
	if s == nil {
		return false
	}
	raw := s.Persistence().RawMessages()
	if len(raw) == 0 {
		return false
	}
	window := s.ContextWindowSize()
	if window <= 0 {
		return false
	}
	tokens := EstimateTokens(raw)
	threshold := int(float64(window) * clearThresholdFraction)
	if tokens < threshold {
		return false
	}
	cleared, freed := clearOldToolResults(raw, tokens, threshold)
	if freed > 0 {
		s.Persistence().SetRawMessages(cleared)
		return true
	}
	return false
}

// toolResultBytes returns the total byte size of a message's tool results.
func toolResultBytes(m types.EyrieMessage) int {
	total := 0
	for _, tr := range m.ToolResults {
		total += len(tr.Content)
	}
	return total
}

// clearMessageToolResults replaces each non-empty tool-result content with the
// placeholder and returns the estimated tokens freed (~4 bytes per token).
func clearMessageToolResults(m *types.EyrieMessage) int {
	freedBytes := 0
	for i := range m.ToolResults {
		c := m.ToolResults[i].Content
		if c == "" || c == outputClearedPlaceholder {
			continue
		}
		freedBytes += len(c) - len(outputClearedPlaceholder)
		m.ToolResults[i].Content = outputClearedPlaceholder
	}
	return freedBytes / 4
}

// cloneToolResults copies msgs, deep-copying the ToolResults slices so callers
// can mutate the copy without touching the persisted originals.
func cloneToolResults(msgs []types.EyrieMessage) []types.EyrieMessage {
	out := make([]types.EyrieMessage, len(msgs))
	for i, m := range msgs {
		out[i] = m
		if m.ToolResults != nil {
			out[i].ToolResults = append([]types.ToolResult(nil), m.ToolResults...)
		}
	}
	return out
}
