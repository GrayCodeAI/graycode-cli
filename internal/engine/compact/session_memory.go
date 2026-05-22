package compact

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/eyrie/client"

	"github.com/GrayCodeAI/hawk/internal/engine/token"
)

type SessionMemoryConfig struct {
	MinTokens            int
	MinTextBlockMessages int
	MaxTokens            int
}

func DefaultSessionMemoryConfig() SessionMemoryConfig {
	return SessionMemoryConfig{
		MinTokens:            10000,
		MinTextBlockMessages: 5,
		MaxTokens:            40000,
	}
}

func CalculateMessagesToKeepIndex(msgs []client.EyrieMessage, cfg SessionMemoryConfig) int {
	if len(msgs) == 0 {
		return 0
	}

	tokenCount := 0
	textBlocks := 0
	idx := len(msgs) - 1

	for idx >= 0 {
		tokenCount += token.EstimateMessageTokens(msgs[idx])
		if HasTextContent(msgs[idx]) {
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

func FilterCompactBoundaries(msgs []client.EyrieMessage) []client.EyrieMessage {
	result := make([]client.EyrieMessage, 0, len(msgs))
	for _, m := range msgs {
		if IsCompactBoundary(m) {
			continue
		}
		result = append(result, m)
	}
	return result
}

func IsCompactBoundary(m client.EyrieMessage) bool {
	if m.Role != "user" {
		return false
	}
	return strings.HasPrefix(m.Content, "[Session memory summary]") ||
		strings.HasPrefix(m.Content, "[Conversation summary]") ||
		strings.HasPrefix(m.Content, "[Earlier conversation compacted")
}

func SessionMemoryPath(sessionID string) string {
	home, _ := os.UserHomeDir()
	if sessionID != "" {
		return filepath.Join(home, ".hawk", "sessions", sessionID, "memory.md")
	}
	return filepath.Join(home, ".hawk", "memory.md")
}

func ReadSessionMemory(sessionID string) (string, error) {
	path := SessionMemoryPath(sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
