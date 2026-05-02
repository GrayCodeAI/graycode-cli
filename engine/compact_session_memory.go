package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/eyrie/client"
)

// SessionMemoryStrategy uses the session memory file as a compaction summary
// instead of making an LLM call.
type SessionMemoryStrategy struct{}

func (s *SessionMemoryStrategy) Name() string { return "session_memory" }

func (s *SessionMemoryStrategy) ShouldTrigger(msgs []client.EyrieMessage, tokenCount, threshold int) bool {
	if tokenCount < threshold {
		return false
	}
	memFile := sessionMemoryPath("")
	info, err := os.Stat(memFile)
	if err != nil || info.Size() < 100 {
		return false
	}
	return true
}

func (s *SessionMemoryStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	memContent, err := readSessionMemory("")
	if err != nil {
		return nil, fmt.Errorf("reading session memory: %w", err)
	}
	if strings.TrimSpace(memContent) == "" {
		return nil, fmt.Errorf("session memory is empty")
	}

	tokensBefore := EstimateTokens(sess.messages)

	cfg := DefaultSessionMemoryConfig()
	keepIdx := calculateMessagesToKeepIndex(sess.messages, cfg)
	keepIdx = adjustIndexToPreserveAPIInvariants(sess.messages, keepIdx)

	if keepIdx >= len(sess.messages)-2 {
		return nil, fmt.Errorf("not enough messages to compact")
	}

	kept := sess.messages[keepIdx:]
	kept = filterCompactBoundaries(kept)

	result := make([]client.EyrieMessage, 0, len(kept)+2)
	result = append(result, client.EyrieMessage{
		Role:    "user",
		Content: "[Session memory summary]\n" + memContent + "\n\n[Continue from the recent messages below.]",
	})
	result = append(result, client.EyrieMessage{
		Role:    "assistant",
		Content: "Understood. I have the context from the session memory above. Continuing with the recent conversation.",
	})
	result = append(result, kept...)

	tokensAfter := EstimateTokens(result)

	return &CompactResult{
		Messages:     result,
		Summary:      memContent,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Strategy:     "session_memory",
	}, nil
}

// SessionMemoryConfig controls session memory compaction thresholds.
type SessionMemoryConfig struct {
	MinTokens            int
	MinTextBlockMessages int
	MaxTokens            int
}

// DefaultSessionMemoryConfig returns defaults matching the archive.
func DefaultSessionMemoryConfig() SessionMemoryConfig {
	return SessionMemoryConfig{
		MinTokens:            10000,
		MinTextBlockMessages: 5,
		MaxTokens:            40000,
	}
}

// calculateMessagesToKeepIndex walks backward from the end of messages
// until we have enough tokens and text-block messages to keep.
func calculateMessagesToKeepIndex(msgs []client.EyrieMessage, cfg SessionMemoryConfig) int {
	if len(msgs) == 0 {
		return 0
	}

	tokenCount := 0
	textBlocks := 0
	idx := len(msgs) - 1

	for idx >= 0 {
		tokenCount += estimateMessageTokens(msgs[idx])
		if hasTextContent(msgs[idx]) {
			textBlocks++
		}

		if tokenCount >= cfg.MinTokens && textBlocks >= cfg.MinTextBlockMessages {
			break
		}
		if tokenCount >= cfg.MaxTokens {
			break
		}
		idx--
	}

	if idx < 0 {
		idx = 0
	}
	return idx
}

// filterCompactBoundaries removes old compact boundary messages from kept messages.
func filterCompactBoundaries(msgs []client.EyrieMessage) []client.EyrieMessage {
	result := make([]client.EyrieMessage, 0, len(msgs))
	for _, m := range msgs {
		if isCompactBoundary(m) {
			continue
		}
		result = append(result, m)
	}
	return result
}

func isCompactBoundary(m client.EyrieMessage) bool {
	if m.Role != "user" {
		return false
	}
	return strings.HasPrefix(m.Content, "[Session memory summary]") ||
		strings.HasPrefix(m.Content, "[Conversation summary]") ||
		strings.HasPrefix(m.Content, "[Earlier conversation compacted")
}

func sessionMemoryPath(sessionID string) string {
	home, _ := os.UserHomeDir()
	if sessionID != "" {
		return filepath.Join(home, ".hawk", "sessions", sessionID, "memory.md")
	}
	return filepath.Join(home, ".hawk", "memory.md")
}

func readSessionMemory(sessionID string) (string, error) {
	path := sessionMemoryPath(sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
