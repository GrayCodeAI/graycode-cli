package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// IntegrityCheck validates a session's structural integrity.
type IntegrityCheck struct {
	Valid    bool     `json:"valid"`
	Warnings []string `json:"warnings,omitempty"`
	Errors   []string `json:"errors,omitempty"`
	Stats    IntegrityStats `json:"stats"`
}

// IntegrityStats holds session statistics found during validation.
type IntegrityStats struct {
	MessageCount     int `json:"message_count"`
	UserMessages     int `json:"user_messages"`
	AssistantMessages int `json:"assistant_messages"`
	ToolUses         int `json:"tool_uses"`
	ToolResults      int `json:"tool_results"`
	OrphanedResults  int `json:"orphaned_results"`
	EmptyMessages    int `json:"empty_messages"`
}

// ValidateIntegrity checks a session for structural problems.
func ValidateIntegrity(sess *Session) *IntegrityCheck {
	check := &IntegrityCheck{Valid: true}
	if sess == nil {
		check.Valid = false
		check.Errors = append(check.Errors, "session is nil")
		return check
	}

	stats := &check.Stats
	stats.MessageCount = len(sess.Messages)

	if sess.ID == "" {
		check.Warnings = append(check.Warnings, "session has no ID")
	}
	if sess.CreatedAt.IsZero() {
		check.Warnings = append(check.Warnings, "session has no created_at timestamp")
	}

	// Track tool_use IDs to detect orphaned results
	toolUseIDs := make(map[string]bool)

	for i, msg := range sess.Messages {
		switch msg.Role {
		case "user":
			stats.UserMessages++
			if msg.ToolResult != nil {
				stats.ToolResults++
				if !toolUseIDs[msg.ToolResult.ToolUseID] {
					stats.OrphanedResults++
					check.Warnings = append(check.Warnings, fmt.Sprintf("message %d: tool_result references unknown tool_use_id %q", i, msg.ToolResult.ToolUseID))
				}
			}
		case "assistant":
			stats.AssistantMessages++
			for _, tc := range msg.ToolUse {
				stats.ToolUses++
				toolUseIDs[tc.ID] = true
			}
		default:
			check.Warnings = append(check.Warnings, fmt.Sprintf("message %d: unexpected role %q", i, msg.Role))
		}

		if msg.Content == "" && msg.ToolResult == nil && len(msg.ToolUse) == 0 {
			stats.EmptyMessages++
		}
	}

	// Check for tool_uses without corresponding results
	resultIDs := make(map[string]bool)
	for _, msg := range sess.Messages {
		if msg.ToolResult != nil {
			resultIDs[msg.ToolResult.ToolUseID] = true
		}
	}
	for id := range toolUseIDs {
		if !resultIDs[id] {
			check.Warnings = append(check.Warnings, fmt.Sprintf("tool_use %q has no corresponding tool_result", id))
		}
	}

	if stats.OrphanedResults > 0 {
		check.Warnings = append(check.Warnings, fmt.Sprintf("%d orphaned tool_results found", stats.OrphanedResults))
	}

	if stats.EmptyMessages > 3 {
		check.Warnings = append(check.Warnings, fmt.Sprintf("%d empty messages found", stats.EmptyMessages))
	}

	if len(check.Errors) > 0 {
		check.Valid = false
	}

	return check
}

// ComputeChecksum returns a SHA-256 hash of the session content.
func ComputeChecksum(sess *Session) string {
	data, _ := json.Marshal(sess.Messages)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// SessionStats computes lightweight stats without full validation.
func Stats(sess *Session) map[string]interface{} {
	if sess == nil {
		return nil
	}

	users, assistants, tools := 0, 0, 0
	totalChars := 0
	for _, m := range sess.Messages {
		switch m.Role {
		case "user":
			users++
		case "assistant":
			assistants++
		}
		totalChars += len(m.Content)
		tools += len(m.ToolUse)
	}

	duration := time.Duration(0)
	if !sess.CreatedAt.IsZero() && !sess.UpdatedAt.IsZero() {
		duration = sess.UpdatedAt.Sub(sess.CreatedAt)
	}

	return map[string]interface{}{
		"id":            sess.ID,
		"messages":      len(sess.Messages),
		"user_messages": users,
		"assistant_messages": assistants,
		"tool_calls":    tools,
		"total_chars":   totalChars,
		"est_tokens":    totalChars / 4,
		"duration":      duration.String(),
		"model":         sess.Model,
		"provider":      sess.Provider,
	}
}
