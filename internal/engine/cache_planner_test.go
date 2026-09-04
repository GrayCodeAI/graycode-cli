package engine

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func bigSys(n int) string { return strings.Repeat("s", n) }

func TestPlanCacheSegmentsAndBreakpoints(t *testing.T) {
	sys := bigSys(cacheMinPrefixBytes + 2048)
	tools := []types.GraycodeRouterTool{{Name: "t", Description: strings.Repeat("d", 4096), Parameters: map[string]interface{}{"type": "object"}}}
	p := planCache("anthropic", sys, tools, 2)
	if !p.Enabled {
		t.Fatalf("expected enabled, reason=%q", p.Reason)
	}
	if len(p.Segments) != 2 || p.Segments[0].Label != "system" || p.Segments[1].Label != "tools" {
		t.Fatalf("segments = %+v", p.Segments)
	}
	if p.Breakpoints != 2 {
		t.Fatalf("breakpoints = %d, want 2", p.Breakpoints)
	}
	if p.ReadSaving <= 0 {
		t.Fatalf("expected positive saving, got %d", p.ReadSaving)
	}
	// At reuse=2: uncached=2t, cached=1.25t+0.1t=1.35t, saving=0.65t>0.
	if p.CachedCost >= p.UncachedCost {
		t.Fatalf("cached %d should be < uncached %d", p.CachedCost, p.UncachedCost)
	}
}

func TestPlanCacheDisabledAtReuseOne(t *testing.T) {
	sys := bigSys(cacheMinPrefixBytes + 2048)
	p := planCache("anthropic", sys, nil, 1)
	if p.Enabled {
		t.Fatalf("single reuse must not enable caching, reason=%q", p.Reason)
	}
	if p.Reason != "caching does not beat uncached at expected reuse" {
		t.Fatalf("reason = %q", p.Reason)
	}
}

func TestPlanCacheNonAnthropicOff(t *testing.T) {
	sys := bigSys(cacheMinPrefixBytes * 3)
	for _, prov := range []string{"openai", "gemini", ""} {
		if planCache(prov, sys, nil, 2).Enabled {
			t.Fatalf("provider %q must not enable", prov)
		}
	}
}

func TestPlanCacheBelowMinSizeOff(t *testing.T) {
	p := planCache("anthropic", "tiny", nil, 2)
	if p.Enabled || p.Reason != "prefix below break-even size" {
		t.Fatalf("tiny prefix should be off: enabled=%v reason=%q", p.Enabled, p.Reason)
	}
}

func TestPlanCacheEmptyNoSegments(t *testing.T) {
	p := planCache("anthropic", "", nil, 2)
	if p.Enabled || p.Reason != "no stable prefix to cache" {
		t.Fatalf("empty prefix: %+v", p)
	}
}

func TestCacheDecisionDelegatesToPlanner(t *testing.T) {
	sys := bigSys(cacheMinPrefixBytes + 1)
	if !cacheDecision("anthropic", sys, nil) {
		t.Fatal("cacheDecision should enable at default reuse")
	}
	if cacheDecision("openai", sys, nil) {
		t.Fatal("non-anthropic off")
	}
}
