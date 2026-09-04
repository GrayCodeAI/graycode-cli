package engine

import (
	"encoding/json"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// Prompt-cache segment planning, extending the caveman-style break-even gate
// into a full client-side planner: the stable prefix (system prompt + tool
// catalog) is split into cacheable segments, breakpoints are computed at the
// last tool and last system boundaries, and the whole thing is enabled only
// when the provider economics pay off for the expected reuse count. Full
// wire-format lowering and key sharding across a fleet remain graycode-router-side;
// this is the deterministic client planner.

// Anthropic 5m cache economics: uncached = 1.0x, cache write = 1.25x,
// cache read = 0.1x per reuse. The write premium (0.25x) is paid once.
const (
	cacheWritePremium = 0.25
	cacheReadCost     = 0.1 // per-reuse cost of reading a cached segment
	// cacheMinSegmentBytes: segments below this are not worth a cache write.
	cacheMinSegmentBytes = 8 * 1024
	// cacheDefaultReuse is the expected number of turns reusing the prefix.
	cacheDefaultReuse = 2
	// cacheMinPrefixBytes is the historical name for the minimum stable-prefix
	// size worth a cache write (alias for cacheMinSegmentBytes).
	cacheMinPrefixBytes = cacheMinSegmentBytes
)

// CacheSegment is one cacheable unit of the stable prefix.
type CacheSegment struct {
	Index int    `json:"index"`
	Label string `json:"label"` // "system" | "tools"
	Bytes int    `json:"bytes"`
}

// CachePlan is the outcome of cache planning for one call.
type CachePlan struct {
	Enabled      bool           `json:"enabled"`
	Provider     string         `json:"provider"`
	Segments     []CacheSegment `json:"segments"`
	Breakpoints  int            `json:"breakpoints"` // number of cache breakpoints to emit
	ReuseCount   int            `json:"reuse_count"`
	WriteBytes   int            `json:"write_bytes"`
	ReadSaving   int            `json:"read_saving_bytes"`
	UncachedCost int            `json:"uncached_cost_bytes"`
	CachedCost   int            `json:"cached_cost_bytes"`
	Reason       string         `json:"reason,omitempty"`
}

// planCache computes whether and how to cache the stable prefix for a call.
// Deterministic and pure so it can be tested without a provider connection.
func planCache(provider, systemPrompt string, tools []types.GraycodeRouterTool, expectedReuse int) CachePlan {
	if expectedReuse <= 0 {
		expectedReuse = cacheDefaultReuse
	}
	if !strings.EqualFold(provider, "anthropic") {
		return CachePlan{Provider: provider, ReuseCount: expectedReuse, Reason: "no explicit cache controls for provider"}
	}

	plan := CachePlan{Provider: "anthropic", ReuseCount: expectedReuse}
	total := 0
	if systemPrompt != "" {
		plan.Segments = append(plan.Segments, CacheSegment{Index: 0, Label: "system", Bytes: len(systemPrompt)})
		total += len(systemPrompt)
	}
	if len(tools) > 0 {
		if raw, err := json.Marshal(tools); err == nil && len(raw) > 0 {
			plan.Segments = append(plan.Segments, CacheSegment{Index: len(plan.Segments), Label: "tools", Bytes: len(raw)})
			total += len(raw)
		}
	}
	plan.WriteBytes = total

	if len(plan.Segments) == 0 {
		plan.Reason = "no stable prefix to cache"
		return plan
	}

	// A tiny prefix is not worth the write premium (matches the old gate).
	if total < cacheMinSegmentBytes {
		plan.Reason = "prefix below break-even size"
		return plan
	}

	// Breakpoints: one at the last system boundary and one at the last tool
	// boundary. We emit breakpoints on every segment so a later prefix reuse
	// hits an early breakpoint (Anthropic caches at the nearest breakpoint
	// before reused content). 2 segments -> 2 breakpoints.
	plan.Breakpoints = len(plan.Segments)

	// Economics: break-even reuse R satisfies
	//   R*S > 1.25*S + (R-1)*0.1*S  =>  R > 1.25 + 0.1*(R-1)
	//   => 0.9*R > 1.15 => R > 1.278
	// So caching pays off for reuse >= 2. Compute costs for the given reuse.
	plan.UncachedCost = total * expectedReuse
	write := int(float64(total) * (1 + cacheWritePremium))
	reads := int(float64(total) * cacheReadCost * float64(expectedReuse-1))
	plan.CachedCost = write + reads
	plan.ReadSaving = plan.UncachedCost - plan.CachedCost

	if plan.ReadSaving <= 0 {
		plan.Reason = "caching does not beat uncached at expected reuse"
		return plan
	}
	plan.Enabled = true
	plan.Reason = "break-even satisfied"
	return plan
}

// cacheDecision reports whether to request provider-native caching, delegating
// to the planner at the default reuse. Kept for backward compatibility.
func cacheDecision(provider, systemPrompt string, tools []types.GraycodeRouterTool) bool {
	return planCache(provider, systemPrompt, tools, cacheDefaultReuse).Enabled
}
