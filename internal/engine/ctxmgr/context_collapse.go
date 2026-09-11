package ctxmgr

import (
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// CollapseRepeatedMessages finds and collapses similar consecutive messages to
// save context tokens. It collapses:
//   - 3+ consecutive tool_results with similar content into a summary
//   - Repeated error messages into a count
func CollapseRepeatedMessages(msgs []types.EyrieMessage) []types.EyrieMessage {
	if len(msgs) < 3 {
		return msgs
	}

	result := make([]types.EyrieMessage, 0, len(msgs))

	i := 0
	for i < len(msgs) {
		// Try to collapse consecutive identical error messages first (more
		// aggressive: collapses N into 1, whereas tool_result collapses N into 3).
		if isErrorMessage(msgs[i]) {
			j := i + 1
			for j < len(msgs) && isErrorMessage(msgs[j]) && extractErrorText(msgs[i]) == extractErrorText(msgs[j]) {
				j++
			}
			runLen := j - i
			if runLen >= 2 {
				errText := extractErrorText(msgs[i])
				result = append(result, types.EyrieMessage{
					Role:        msgs[i].Role,
					Content:     fmt.Sprintf("[Error repeated %d times: %s]", runLen, errText),
					ToolResults: msgs[i].ToolResults,
				})
				i = j
				continue
			}
		}

		// Try to collapse consecutive tool_results with similar content
		if len(msgs[i].ToolResults) > 0 {
			j := i + 1
			for j < len(msgs) && len(msgs[j].ToolResults) > 0 && isSimilarToolResult(msgs[i], msgs[j]) {
				j++
			}
			runLen := j - i
			if runLen >= 3 {
				// Keep first and last, collapse middle
				result = append(result, msgs[i])
				toolName := toolResultSource(msgs[i])
				collapsed := runLen - 2
				result = append(result, types.EyrieMessage{
					Role:    "user",
					Content: fmt.Sprintf("[Similar output from %s — %d results collapsed]", toolName, collapsed),
					ToolResults: []types.ToolResult{{
						ToolUseID: "collapsed",
						Content:   fmt.Sprintf("[Similar output from %s — %d results collapsed]", toolName, collapsed),
					}},
				})
				result = append(result, msgs[j-1])
				i = j
				continue
			}
		}

		result = append(result, msgs[i])
		i++
	}

	return result
}

// isSimilarToolResult checks whether two tool_result messages are similar
// enough to collapse. Two results are similar if they come from the same tool
// and their content shares the same first line or prefix (up to 100 chars).
func isSimilarToolResult(a, b types.EyrieMessage) bool {
	if len(a.ToolResults) == 0 || len(b.ToolResults) == 0 {
		return false
	}

	prefixA := contentPrefix(a.ToolResults[0].Content, 100)
	prefixB := contentPrefix(b.ToolResults[0].Content, 100)

	return prefixA == prefixB
}

// toolResultSource extracts the tool name from a tool_result message.
func toolResultSource(msg types.EyrieMessage) string {
	if len(msg.ToolResults) > 0 && msg.ToolResults[0].ToolUseID != "" {
		return msg.ToolResults[0].ToolUseID
	}
	return "tool"
}

// contentPrefix returns the first n characters of a string, or the first line,
// whichever is shorter.
func contentPrefix(s string, n int) string {
	if idx := strings.Index(s, "\n"); idx >= 0 && idx < n {
		return s[:idx]
	}
	if len(s) > n {
		return s[:n]
	}
	return s
}

// isErrorMessage returns true if a message appears to be an error.
func isErrorMessage(msg types.EyrieMessage) bool {
	if len(msg.ToolResults) > 0 && msg.ToolResults[0].IsError {
		return true
	}
	content := strings.ToLower(msg.Content)
	return strings.HasPrefix(content, "error:") || strings.HasPrefix(content, "error ")
}

// extractErrorText extracts the error text from an error message.
func extractErrorText(msg types.EyrieMessage) string {
	if len(msg.ToolResults) > 0 && msg.ToolResults[0].IsError {
		return msg.ToolResults[0].Content
	}
	return msg.Content
}
