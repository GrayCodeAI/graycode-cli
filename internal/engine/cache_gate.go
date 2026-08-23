package engine

import (
	"encoding/json"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// Prompt-cache break-even gate, adopting caveman's cacheengine arithmetic in
// miniature: provider-native caching charges a write premium on cached input
// (Anthropic 5m: write=1.25x, read=0.1x) and pays off only when the stable
// prefix is reused. Below the break-even prefix size the premium costs more
// than one reuse saves, so caching stays OFF rather than burning the write.
//
// Full segment planning and key-sharding belong in eyrie; this is the
// client-side gate only.

// cacheMinPrefixBytes is the smallest stable prefix worth a cache write.
// ~8 KiB approximates 2k tokens: at Anthropic economics, two reuses of a
// 2k-token prefix already beat paying full price twice (2x1.0 > 1.25+0.1).
const cacheMinPrefixBytes = 8 * 1024

// cacheDecision reports whether to request provider-native prompt caching
// for this call. Deterministic and pure so it can be tested without a
// provider connection.
func cacheDecision(provider, systemPrompt string, tools []types.EyrieTool) bool {
	if !strings.EqualFold(provider, "anthropic") {
		return false // other providers: implicit caching; no explicit controls
	}
	stable := len(systemPrompt)
	if raw, err := json.Marshal(tools); err == nil {
		stable += len(raw)
	}
	return stable >= cacheMinPrefixBytes
}
