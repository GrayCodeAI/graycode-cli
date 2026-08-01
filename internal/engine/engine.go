package engine

import (
	"os"

	"github.com/GrayCodeAI/hawk/internal/types"
)

const (
	maxContextMessages = 100 // auto-compact threshold
	maxRecoveryRetries = 3   // max_tokens recovery attempts
)

// Compact reduces conversation history (boundary-aware truncation).
func (s *Session) Compact() { s.compact() }

// SmartCompact reduces conversation history using LLM-generated summaries.
func (s *Session) SmartCompact() { s.smartCompact() }

// compact removes older messages while preserving tool_use/tool_result pairing.
func (s *Session) compact() {
	keepEnd := 16
	if pinned := s.Persistence().PinnedMessages(); pinned > keepEnd {
		keepEnd = pinned
	}
	if len(s.Persistence().RawMessages()) <= keepEnd+4 {
		return
	}
	// Keep first 4 and last keepEnd, but ensure we don't break tool pairs.
	cutStart := 4
	cutEnd := len(s.Persistence().RawMessages()) - keepEnd

	// Ensure cutEnd doesn't land in the middle of a tool_use/tool_result pair.
	// A tool_result (user msg with ToolResult) must follow its tool_use (assistant msg with ToolUse).
	// Walk cutEnd forward until we're at a clean boundary.
	for cutEnd < len(s.Persistence().RawMessages()) {
		msg := s.Persistence().RawMessages()[cutEnd]
		if len(msg.ToolResults) > 0 {
			// This is a tool_result — we'd orphan it. Include it.
			cutEnd++
			continue
		}
		if msg.Role == "assistant" && len(msg.ToolUse) > 0 {
			// This is a tool_use — we need its results too. Skip past them.
			cutEnd++
			continue
		}
		break
	}

	// Also walk cutStart forward to not orphan pairs at the beginning
	for cutStart < cutEnd {
		msg := s.Persistence().RawMessages()[cutStart]
		if msg.Role == "assistant" && len(msg.ToolUse) > 0 {
			// Include the tool results that follow
			cutStart++
			for cutStart < cutEnd && len(s.Persistence().RawMessages()[cutStart].ToolResults) > 0 {
				cutStart++
			}
			continue
		}
		break
	}

	if cutStart >= cutEnd {
		return // nothing to compact
	}

	keep := make([]types.EyrieMessage, 0, len(s.Persistence().RawMessages())-(cutEnd-cutStart)+1)
	keep = append(keep, s.Persistence().RawMessages()[:cutStart]...)
	keep = append(keep, types.EyrieMessage{
		Role:    "user",
		Content: "[Earlier conversation compacted to save context.]",
	})
	keep = append(keep, s.Persistence().RawMessages()[cutEnd:]...)
	s.Persistence().SetRawMessages(keep)
}

// readFileContent reads a file from disk and returns its content as a string.
// Used by critic and sandbox to capture original file state.
func readFileContent(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return "", err
	}
	return string(data), nil
}
