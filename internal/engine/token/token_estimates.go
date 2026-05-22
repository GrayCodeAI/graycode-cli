package token

import (
	"encoding/json"
	"fmt"

	"github.com/GrayCodeAI/hawk/internal/types"
)

func EstimateTokens(msgs []types.EyrieMessage) int {
	total := 0
	for _, m := range msgs {
		total += EstimateMessageTokens(m)
	}
	return total
}

func EstimateMessageTokens(m types.EyrieMessage) int {
	tokens := CountTokens(m.Content)
	for _, tc := range m.ToolUse {
		tokens += CountTokens(tc.Name)
		for _, v := range tc.Arguments {
			switch val := v.(type) {
			case string:
				tokens += CountTokens(val)
			default:
				if encoded, err := json.Marshal(v); err == nil {
					tokens += CountTokens(string(encoded))
				} else {
					tokens += CountTokens(fmt.Sprintf("%v", v))
				}
			}
		}
	}
	if m.ToolResult != nil {
		tokens += CountTokens(m.ToolResult.Content)
	}
	return tokens
}
