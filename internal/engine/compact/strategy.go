package compact

import (
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

type CompactResult struct {
	Messages     []types.GraycodeRouterMessage
	Summary      string
	TokensBefore int
	TokensAfter  int
	Strategy     string
}

type CompactConfig struct {
	AutoEnabled       bool
	ContextWindowSize int
	AutoCompactBuffer int
	MaxOutputTokens   int
	MaxFailures       int
	Cooldown          time.Duration
}

func DefaultCompactConfig() CompactConfig {
	return CompactConfig{
		AutoEnabled:       true,
		ContextWindowSize: 200000,
		AutoCompactBuffer: 13000,
		MaxOutputTokens:   20000,
		MaxFailures:       3,
		Cooldown:          5 * time.Minute,
	}
}

var compactableTools = map[string]bool{
	"Bash":       true,
	"Read":       true,
	"Grep":       true,
	"Glob":       true,
	"WebFetch":   true,
	"WebSearch":  true,
	"Edit":       true,
	"Write":      true,
	"LS":         true,
	"ToolSearch": true,
}

func IsCompactableTool(name string) bool {
	return compactableTools[name]
}

func AdjustIndexToPreserveAPIInvariants(msgs []types.GraycodeRouterMessage, startIdx int) int {
	if startIdx <= 0 {
		return 0
	}
	if startIdx >= len(msgs) {
		return len(msgs)
	}

	idx := startIdx
	for idx > 0 {
		msg := msgs[idx]
		if len(msg.ToolResults) > 0 {
			idx--
			continue
		}
		if msg.Role == "assistant" && len(msg.ToolUse) > 0 {
			resultCount := len(msg.ToolUse)
			needed := 0
			for j := idx + 1; j < len(msgs) && needed < resultCount; j++ {
				if len(msgs[j].ToolResults) > 0 {
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

func HasTextContent(m types.GraycodeRouterMessage) bool {
	if len(m.ToolResults) > 0 {
		return false
	}
	return strings.TrimSpace(m.Content) != ""
}
