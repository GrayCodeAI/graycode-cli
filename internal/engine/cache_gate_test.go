package engine

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestCacheDecisionNonAnthropicOff(t *testing.T) {
	big := strings.Repeat("x", cacheMinPrefixBytes*3)
	for _, p := range []string{"openai", "gemini", "", "anthropic2"} {
		if cacheDecision(p, big, nil) {
			t.Fatalf("provider %q must not enable explicit caching", p)
		}
	}
}

func TestCacheDecisionSmallPrefixOff(t *testing.T) {
	small := "tiny system prompt"
	if cacheDecision("anthropic", small, nil) {
		t.Fatal("below break-even prefix must keep caching off")
	}
}

func TestCacheDecisionLargePrefixOn(t *testing.T) {
	sys := strings.Repeat("y", cacheMinPrefixBytes+1)
	if !cacheDecision("anthropic", sys, nil) {
		t.Fatal("above break-even prefix should enable caching")
	}
}

func TestCacheDecisionCountsToolCatalog(t *testing.T) {
	tools := []types.GraycodeRouterTool{
		{Name: "t1", Description: strings.Repeat("d", 9000), Parameters: map[string]interface{}{"type": "object"}},
	}
	sys := "small"
	if !cacheDecision("anthropic", sys, tools) {
		t.Fatal("tool catalog bytes count toward the stable prefix")
	}
}

func TestCacheDecisionCaseInsensitiveProvider(t *testing.T) {
	sys := strings.Repeat("z", cacheMinPrefixBytes+5)
	if !cacheDecision("Anthropic", sys, nil) {
		t.Fatal("provider match should be case-insensitive")
	}
}
