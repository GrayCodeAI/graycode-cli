package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// ConversationRecovery — detects interrupted sessions and offers to resume
// them with full context restoration.
// ─────────────────────────────────────────────────────────────────────────────

// InterruptionType classifies what was happening when the session stopped.
type InterruptionType string

const (
	InterruptionNone          InterruptionType = "none"
	InterruptionMidTool       InterruptionType = "mid_tool"       // tool was executing
	InterruptionMidResponse   InterruptionType = "mid_response"   // assistant was writing a response
	InterruptionAwaitingInput InterruptionType = "awaiting_input" // waiting for user input
	InterruptionToolError     InterruptionType = "tool_error"     // tool execution failed
	InterruptionPermissionAsk InterruptionType = "permission_ask" // waiting for permission approval
)

// RecoveryCandidate represents an interrupted session that can be resumed.
type RecoveryCandidate struct {
	SessionID       string
	CWD             string
	Model           string
	Provider        string
	Interruption    InterruptionType
	LastToolName    string
	LastToolID      string
	MessageCount    int
	LastActivity    time.Time
	UnresolvedTools []string // tool_use IDs without matching tool_result
	Age             time.Duration
}

// ScanForRecovery scans the sessions directory for sessions that may have been
// interrupted (have WAL files or unresolved tool calls).
func ScanForRecovery() []RecoveryCandidate {
	dir := sessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var candidates []RecoveryCandidate
	now := time.Now()

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		ext := filepath.Ext(e.Name())
		if ext != ".jsonl" && ext != ".json" && ext != ".wal" {
			continue
		}

		id := e.Name()[:len(e.Name())-len(ext)]
		// Skip if we already processed this ID
		alreadySeen := false
		for _, c := range candidates {
			if c.SessionID == id {
				alreadySeen = true
				break
			}
		}
		if alreadySeen {
			continue
		}

		candidate := analyzeSessionForRecovery(id, now)
		if candidate != nil {
			candidates = append(candidates, *candidate)
		}
	}

	// Sort by last activity, most recent first
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].LastActivity.After(candidates[j].LastActivity)
	})

	return candidates
}

// analyzeSessionForRecovery loads a session and checks for interruption signs.
func analyzeSessionForRecovery(id string, now time.Time) *RecoveryCandidate {
	// Try loading from JSONL first
	s, err := Load(id)
	if err != nil {
		// Try recovering from WAL
		s, err = RecoverFromWAL(id)
		if err != nil || s == nil {
			return nil
		}
	}

	if len(s.Messages) == 0 {
		return nil
	}

	candidate := &RecoveryCandidate{
		SessionID:    s.ID,
		CWD:          s.CWD,
		Model:        s.Model,
		Provider:     s.Provider,
		MessageCount: len(s.Messages),
		LastActivity: s.UpdatedAt,
		Age:          now.Sub(s.UpdatedAt),
	}

	// Find unresolved tool calls
	unresolved := findUnresolvedTools(s.Messages)
	candidate.UnresolvedTools = unresolved

	// Classify interruption
	candidate.Interruption, candidate.LastToolName, candidate.LastToolID = classifyInterruption(s.Messages)

	return candidate
}

// findUnresolvedTools returns tool_use IDs that have no matching tool_result.
func findUnresolvedTools(messages []Message) []string {
	toolUseIDs := make(map[string]string) // id -> tool name
	resolvedIDs := make(map[string]bool)

	for _, msg := range messages {
		if msg.Role == "assistant" {
			for _, tu := range msg.ToolUse {
				toolUseIDs[tu.ID] = tu.Name
			}
		}
		if msg.Role == "user" && msg.ToolResult != nil {
			resolvedIDs[msg.ToolResult.ToolUseID] = true
		}
	}

	var unresolved []string
	for id, name := range toolUseIDs {
		if !resolvedIDs[id] {
			unresolved = append(unresolved, name)
		}
	}
	return unresolved
}

// classifyInterruption determines what was happening when the session stopped.
func classifyInterruption(messages []Message) (InterruptionType, string, string) {
	if len(messages) == 0 {
		return InterruptionNone, "", ""
	}

	last := messages[len(messages)-1]

	// Check for unresolved tool calls first
	unresolved := findUnresolvedTools(messages)

	switch last.Role {
	case "assistant":
		if len(last.ToolUse) > 0 {
			// Assistant called tools — check if results are missing
			if len(unresolved) > 0 {
				lastTool := last.ToolUse[len(last.ToolUse)-1]
				return InterruptionMidTool, lastTool.Name, lastTool.ID
			}
			// All tools resolved but last message is assistant with tool calls
			// This means it was about to generate a response
			return InterruptionMidResponse, "", ""
		}
		if last.Content != "" {
			// Assistant was writing text response
			return InterruptionMidResponse, "", ""
		}
	case "user":
		if last.ToolResult != nil {
			// User message is a tool result — check if more tools are pending
			if len(unresolved) > 0 {
				return InterruptionToolError, last.ToolResult.ToolUseID, last.ToolResult.ToolUseID
			}
			// Tool result delivered, assistant should respond
			return InterruptionAwaitingInput, "", ""
		}
		// Regular user message — session was waiting for assistant
		return InterruptionAwaitingInput, "", ""
	}

	return InterruptionNone, "", ""
}

// ResumeSession loads a session for resumption, cleaning up any orphaned state.
// Returns the session, a summary of what was recovered, and any errors.
func ResumeSession(sessionID string) (*Session, string, error) {
	s, err := Load(sessionID)
	if err != nil {
		// Try WAL recovery
		s, err = RecoverFromWAL(sessionID)
		if err != nil || s == nil {
			return nil, "", fmt.Errorf("cannot load session %s: %w", sessionID, err)
		}
	}

	if len(s.Messages) == 0 {
		return nil, "", fmt.Errorf("session %s has no messages", sessionID)
	}

	var recoveryNotes []string

	// Detect and clean orphaned tool uses
	orphaned := findOrphanedToolUses(s.Messages)
	if len(orphaned) > 0 {
		recoveryNotes = append(recoveryNotes, fmt.Sprintf("Found %d orphaned tool call(s) without results", len(orphaned)))
	}

	// Detect interrupted turns
	interruption, toolName, _ := classifyInterruption(s.Messages)
	if interruption != InterruptionNone {
		switch interruption {
		case InterruptionMidTool:
			recoveryNotes = append(recoveryNotes, fmt.Sprintf("Session interrupted during %s execution", toolName))
		case InterruptionMidResponse:
			recoveryNotes = append(recoveryNotes, "Session interrupted while assistant was responding")
		case InterruptionToolError:
			recoveryNotes = append(recoveryNotes, "Session stopped with unresolved tool error")
		case InterruptionPermissionAsk:
			recoveryNotes = append(recoveryNotes, "Session was waiting for permission approval")
		}
	}

	// Clean up WAL if it exists (session file has everything)
	walPath := filepath.Join(sessionsDir(), sessionID+".wal")
	if _, err := os.Stat(walPath); err == nil {
		_ = os.Remove(walPath)
		recoveryNotes = append(recoveryNotes, "Cleaned up stale WAL file")
	}

	note := "Session recovered"
	if len(recoveryNotes) > 0 {
		note += ": " + strings.Join(recoveryNotes, "; ")
	}

	s.UpdatedAt = time.Now()
	return s, note, nil
}

// findOrphanedToolUses returns tool_use blocks that have no matching tool_result
// and are not part of the last assistant message (i.e., truly orphaned).
func findOrphanedToolUses(messages []Message) []string {
	toolUseIDs := make(map[string]string)
	resolvedIDs := make(map[string]bool)
	lastAssistantIdx := -1

	for i, msg := range messages {
		if msg.Role == "assistant" {
			lastAssistantIdx = i
			for _, tu := range msg.ToolUse {
				toolUseIDs[tu.ID] = tu.Name
			}
		}
		if msg.Role == "user" && msg.ToolResult != nil {
			resolvedIDs[msg.ToolResult.ToolUseID] = true
		}
	}

	var orphaned []string
	for id, name := range toolUseIDs {
		if !resolvedIDs[id] {
			// Check if this tool_use is in the last assistant message
			// (those are "pending" not "orphaned")
			if lastAssistantIdx >= 0 {
				isInLast := false
				for _, tu := range messages[lastAssistantIdx].ToolUse {
					if tu.ID == id {
						isInLast = true
						break
					}
				}
				if !isInLast {
					orphaned = append(orphaned, name)
				}
			} else {
				orphaned = append(orphaned, name)
			}
		}
	}
	return orphaned
}

// FormatRecoveryCandidates formats recovery candidates for display.
func FormatRecoveryCandidates(candidates []RecoveryCandidate) string {
	if len(candidates) == 0 {
		return "No interrupted sessions found."
	}

	var b strings.Builder
	b.WriteString("Interrupted sessions (hawk --recover <id> to resume):\n")
	b.WriteString(strings.Repeat("─", 60) + "\n")

	for i, c := range candidates {
		shortID := c.SessionID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		var status string
		switch c.Interruption {
		case InterruptionMidTool:
			status = fmt.Sprintf("⚡ interrupted: %s", c.LastToolName)
		case InterruptionMidResponse:
			status = "⚡ interrupted: mid-response"
		case InterruptionToolError:
			status = "⚡ interrupted: tool error"
		case InterruptionAwaitingInput:
			status = "⏸ awaiting input"
		default:
			status = "✓ complete"
		}

		age := formatAge(c.Age)
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, shortID, status))
		b.WriteString(fmt.Sprintf("   %s · %d msgs · %s\n\n", c.CWD, c.MessageCount, age))
	}

	return b.String()
}
