package engine

import (
	"context"
	"strings"

	"github.com/hawk/eyrie/client"
)

// CompactStrategy defines a conversation compaction approach.
type CompactStrategy interface {
	Name() string
	ShouldTrigger(msgs []client.EyrieMessage, tokenCount, threshold int) bool
	Compact(ctx context.Context, s *Session) (*CompactResult, error)
}

// CompactResult holds the outcome of a compaction operation.
type CompactResult struct {
	Messages     []client.EyrieMessage
	Summary      string
	TokensBefore int
	TokensAfter  int
	Strategy     string
}

// CompactConfig controls auto-compaction behavior.
type CompactConfig struct {
	AutoEnabled       bool
	ContextWindowSize int
	AutoCompactBuffer int
	MaxOutputTokens   int
	MaxFailures       int
}

// DefaultCompactConfig returns sensible defaults matching the archive behavior.
func DefaultCompactConfig() CompactConfig {
	return CompactConfig{
		AutoEnabled:       true,
		ContextWindowSize: 200000,
		AutoCompactBuffer: 13000,
		MaxOutputTokens:   20000,
		MaxFailures:       3,
	}
}

// StrategyRegistry manages compaction strategies in priority order.
type StrategyRegistry struct {
	strategies []CompactStrategy
	config     CompactConfig
}

// NewStrategyRegistry creates a registry with default strategies.
func NewStrategyRegistry(config CompactConfig) *StrategyRegistry {
	r := &StrategyRegistry{config: config}
	r.strategies = []CompactStrategy{
		&MicroCompactStrategy{},
		&SessionMemoryStrategy{},
		&SmartCompactStrategy{},
		&TruncateStrategy{},
	}
	return r
}

// SelectStrategy picks the highest-priority strategy whose trigger fires.
func (r *StrategyRegistry) SelectStrategy(msgs []client.EyrieMessage, tokenCount int) CompactStrategy {
	threshold := r.config.ContextWindowSize - r.config.AutoCompactBuffer - r.config.MaxOutputTokens
	for _, s := range r.strategies {
		if s.ShouldTrigger(msgs, tokenCount, threshold) {
			return s
		}
	}
	return &TruncateStrategy{}
}

// EstimateTokens provides a rough token estimate for messages.
func EstimateTokens(msgs []client.EyrieMessage) int {
	total := 0
	for _, m := range msgs {
		total += estimateMessageTokens(m)
	}
	return total
}

func estimateMessageTokens(m client.EyrieMessage) int {
	tokens := CountTokensFast(m.Content)
	for _, tc := range m.ToolUse {
		tokens += CountTokensFast(tc.Name)
		for _, v := range tc.Arguments {
			switch val := v.(type) {
			case string:
				tokens += CountTokensFast(val)
			default:
				tokens += 10
			}
		}
	}
	if m.ToolResult != nil {
		tokens += CountTokensFast(m.ToolResult.Content)
	}
	return tokens
}

// compactableTools are tools whose old results can be safely cleared.
var compactableTools = map[string]bool{
	"Bash":      true,
	"Read":      true,
	"Grep":      true,
	"Glob":      true,
	"WebFetch":  true,
	"WebSearch":  true,
	"Edit":      true,
	"Write":     true,
	"LS":        true,
	"ToolSearch": true,
}

// isCompactableTool returns true if the tool's results can be cleared during micro-compaction.
func isCompactableTool(name string) bool {
	return compactableTools[name]
}

// adjustIndexToPreserveAPIInvariants walks backward from startIdx to ensure
// tool_use/tool_result pairs are never split.
func adjustIndexToPreserveAPIInvariants(msgs []client.EyrieMessage, startIdx int) int {
	if startIdx <= 0 {
		return 0
	}
	if startIdx >= len(msgs) {
		return len(msgs)
	}

	idx := startIdx
	for idx > 0 {
		msg := msgs[idx]
		if msg.ToolResult != nil {
			idx--
			continue
		}
		if msg.Role == "assistant" && len(msg.ToolUse) > 0 {
			resultCount := len(msg.ToolUse)
			needed := 0
			for j := idx + 1; j < len(msgs) && needed < resultCount; j++ {
				if msgs[j].ToolResult != nil {
					needed++
				} else {
					break
				}
			}
			if needed < resultCount {
				idx--
				continue
			}
		}
		break
	}
	return idx
}

// hasTextContent returns true if the message contains meaningful text (not just tool results).
func hasTextContent(m client.EyrieMessage) bool {
	if m.ToolResult != nil {
		return false
	}
	return strings.TrimSpace(m.Content) != ""
}
