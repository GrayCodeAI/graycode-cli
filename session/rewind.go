package session

import (
	"fmt"
	"time"
)

// Checkpoint represents a restorable point in the conversation.
type Checkpoint struct {
	Index       int       `json:"index"`
	MessageID   string    `json:"message_id,omitempty"`
	Role        string    `json:"role"`
	Preview     string    `json:"preview"`
	ToolName    string    `json:"tool_name,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	TokenCount  int       `json:"token_count,omitempty"`
}

// ListCheckpoints extracts interactive restore points from a session.
// Returns user/assistant message boundaries where rewind is safe.
func ListCheckpoints(sess *Session) []Checkpoint {
	if sess == nil {
		return nil
	}

	var checkpoints []Checkpoint
	for i, msg := range sess.Messages {
		// Only checkpoint at user messages and assistant text responses
		if msg.Role == "user" && msg.ToolResult == nil {
			preview := truncatePreview(msg.Content, 80)
			checkpoints = append(checkpoints, Checkpoint{
				Index:   i,
				Role:    "user",
				Preview: preview,
			})
		} else if msg.Role == "assistant" && len(msg.ToolUse) == 0 && msg.Content != "" {
			preview := truncatePreview(msg.Content, 80)
			checkpoints = append(checkpoints, Checkpoint{
				Index:   i,
				Role:    "assistant",
				Preview: preview,
			})
		}
	}
	return checkpoints
}

// RewindTo truncates the session to the given checkpoint index.
// All messages after the checkpoint are removed.
func RewindTo(sess *Session, checkpointIndex int) error {
	if sess == nil {
		return fmt.Errorf("nil session")
	}
	if checkpointIndex < 0 || checkpointIndex >= len(sess.Messages) {
		return fmt.Errorf("invalid checkpoint index %d (session has %d messages)", checkpointIndex, len(sess.Messages))
	}

	// Include the checkpoint message itself
	sess.Messages = sess.Messages[:checkpointIndex+1]
	sess.UpdatedAt = time.Now()
	return nil
}

// RewindLastExchange removes the most recent user+assistant exchange.
func RewindLastExchange(sess *Session) error {
	if sess == nil || len(sess.Messages) < 2 {
		return fmt.Errorf("not enough messages to rewind")
	}

	// Walk backward removing messages until we've removed one complete exchange
	removed := 0
	i := len(sess.Messages) - 1
	for i >= 0 && removed < 2 {
		msg := sess.Messages[i]
		if msg.Role == "user" && msg.ToolResult == nil {
			removed++
		} else if msg.Role == "assistant" && len(msg.ToolUse) == 0 {
			removed++
		}
		i--
	}

	if i < 0 {
		i = 0
	}
	sess.Messages = sess.Messages[:i+1]
	sess.UpdatedAt = time.Now()
	return nil
}

// FormatCheckpointList produces a human-readable list of checkpoints for selection.
func FormatCheckpointList(checkpoints []Checkpoint) string {
	if len(checkpoints) == 0 {
		return "No checkpoints available."
	}

	var result string
	for i, cp := range checkpoints {
		icon := ">"
		if cp.Role == "assistant" {
			icon = "<"
		}
		result += fmt.Sprintf("  %s [%d] %s: %s\n", icon, i, cp.Role, cp.Preview)
	}
	return result
}

func truncatePreview(s string, maxLen int) string {
	// Remove newlines
	clean := ""
	for _, r := range s {
		if r == '\n' || r == '\r' {
			clean += " "
		} else {
			clean += string(r)
		}
	}
	if len(clean) > maxLen {
		return clean[:maxLen-3] + "..."
	}
	return clean
}
