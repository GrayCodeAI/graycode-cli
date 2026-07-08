package session

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Handover represents a session transfer between models, machines, or team members.
type Handover struct {
	SessionID string          `json:"session_id"`
	FromModel string          `json:"from_model"`
	ToModel   string          `json:"to_model"`
	Context   HandoverContext `json:"context"`
	CreatedAt time.Time       `json:"created_at"`
	Status    string          `json:"status"`
}

// HandoverContext captures the full state needed to continue work.
type HandoverContext struct {
	Goal                  string   `json:"goal"`
	Progress              string   `json:"progress"`
	FilesModified         []string `json:"files_modified"`
	PendingTasks          []string `json:"pending_tasks"`
	Warnings              []string `json:"warnings"`
	KeyDecisions          []string `json:"key_decisions"`
	CurrentState          string   `json:"current_state"`
	TokensBudgetRemaining int      `json:"tokens_budget_remaining"`
}

// HandoverManager coordinates handover creation and tracking.
type HandoverManager struct {
	Handovers []*Handover
	mu        sync.Mutex
}

// NewHandoverManager creates a new HandoverManager.
func NewHandoverManager() *HandoverManager {
	return &HandoverManager{
		Handovers: make([]*Handover, 0),
	}
}

// PrepareHandover creates a Handover from a session's messages and file list.
// It extracts the goal, summarizes progress, lists modified files,
// identifies pending work, and records key decisions.
func (m *HandoverManager) PrepareHandover(sessionID, fromModel string, messages []Message, files []string) *Handover {
	m.mu.Lock()
	defer m.mu.Unlock()

	h := &Handover{
		SessionID: sessionID,
		FromModel: fromModel,
		CreatedAt: time.Now(),
		Status:    "prepared",
		Context: HandoverContext{
			Goal:          ExtractGoal(messages),
			Progress:      extractProgress(messages),
			FilesModified: files,
			PendingTasks:  ExtractPendingTasks(messages),
			KeyDecisions:  ExtractDecisions(messages),
			CurrentState:  "in_progress",
		},
	}

	m.Handovers = append(m.Handovers, h)
	return h
}

// GenerateHandoverPrompt produces a structured markdown prompt that can be
// fed to a receiving model so it understands the work context.
func GenerateHandoverPrompt(handover *Handover) string {
	var sb strings.Builder

	sb.WriteString("## Session Handover\n\n")
	sb.WriteString(fmt.Sprintf("You are continuing work started by %s.\n\n", handover.FromModel))

	sb.WriteString("### Goal\n")
	sb.WriteString(handover.Context.Goal)
	sb.WriteString("\n\n")

	sb.WriteString("### Progress\n")
	if handover.Context.Progress != "" {
		sb.WriteString(handover.Context.Progress)
	} else {
		sb.WriteString("- No progress recorded yet\n")
	}
	sb.WriteString("\n\n")

	if len(handover.Context.KeyDecisions) > 0 {
		sb.WriteString("### Key Decisions\n")
		for _, d := range handover.Context.KeyDecisions {
			sb.WriteString(fmt.Sprintf("- %s\n", d))
		}
		sb.WriteString("\n")
	}

	if len(handover.Context.PendingTasks) > 0 {
		sb.WriteString("### Pending Tasks\n")
		for _, t := range handover.Context.PendingTasks {
			sb.WriteString(fmt.Sprintf("- %s\n", t))
		}
		sb.WriteString("\n")
	}

	if len(handover.Context.Warnings) > 0 {
		sb.WriteString("### Warnings\n")
		for _, w := range handover.Context.Warnings {
			sb.WriteString(fmt.Sprintf("- %s\n", w))
		}
		sb.WriteString("\n")
	}

	if len(handover.Context.FilesModified) > 0 {
		sb.WriteString("### Files Modified\n")
		sb.WriteString(strings.Join(handover.Context.FilesModified, ", "))
		sb.WriteString("\n")
	}

	return sb.String()
}

// AcceptHandover marks a handover as accepted by the receiving model.
func (m *HandoverManager) AcceptHandover(handover *Handover, toModel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if handover == nil {
		return fmt.Errorf("handover is nil")
	}
	if handover.Status == "accepted" {
		return fmt.Errorf("handover already accepted")
	}
	if handover.Status == "rejected" {
		return fmt.Errorf("handover was rejected")
	}

	handover.ToModel = toModel
	handover.Status = "accepted"
	return nil
}

// ExtractGoal returns the primary goal from the first user message.
func ExtractGoal(messages []Message) string {
	for _, msg := range messages {
		if msg.Role == "user" && msg.Content != "" {
			goal := msg.Content
			// Trim to first sentence or line if it's long
			if idx := strings.Index(goal, "\n"); idx > 0 && idx < 200 {
				goal = goal[:idx]
			} else if len(goal) > 200 {
				// Find a natural break point
				if idx := strings.LastIndex(goal[:200], ". "); idx > 0 {
					goal = goal[:idx+1]
				} else {
					goal = goal[:200]
				}
			}
			return strings.TrimSpace(goal)
		}
	}
	return "No goal identified"
}

// ExtractDecisions scans assistant messages for decision-like patterns.
func ExtractDecisions(messages []Message) []string {
	var decisions []string
	decisionMarkers := []string{
		"decided to ",
		"choosing ",
		"using ",
		"going with ",
		"selected ",
		"opting for ",
		"decision: ",
		"i'll use ",
		"we'll use ",
	}

	for _, msg := range messages {
		if msg.Role != "assistant" || msg.Content == "" {
			continue
		}

		lines := strings.Split(msg.Content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			lower := strings.ToLower(line)
			for _, marker := range decisionMarkers {
				if strings.Contains(lower, marker) {
					// Clean up the decision text
					decision := strings.TrimPrefix(line, "- ")
					decision = strings.TrimPrefix(decision, "* ")
					if len(decision) > 150 {
						decision = decision[:150]
					}
					decisions = append(decisions, decision)
					break
				}
			}
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, d := range decisions {
		if !seen[d] {
			seen[d] = true
			unique = append(unique, d)
		}
	}

	return unique
}

// ExtractPendingTasks identifies tasks that are mentioned but not yet completed.
func ExtractPendingTasks(messages []Message) []string {
	var pending []string
	todoMarkers := []string{
		"todo:",
		"TODO:",
		"still need to ",
		"remaining: ",
		"next step",
		"haven't yet ",
		"not yet ",
		"pending: ",
		"will need to ",
		"need to ",
	}

	for _, msg := range messages {
		if msg.Role != "assistant" || msg.Content == "" {
			continue
		}

		lines := strings.Split(msg.Content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			lower := strings.ToLower(line)
			for _, marker := range todoMarkers {
				if strings.Contains(lower, strings.ToLower(marker)) {
					task := strings.TrimPrefix(line, "- ")
					task = strings.TrimPrefix(task, "* ")
					if len(task) > 150 {
						task = task[:150]
					}
					pending = append(pending, task)
					break
				}
			}
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, t := range pending {
		if !seen[t] {
			seen[t] = true
			unique = append(unique, t)
		}
	}

	return unique
}

// extractProgress summarizes what has been accomplished by examining
// assistant messages for completion indicators.
func extractProgress(messages []Message) string {
	var items []string
	completionMarkers := []string{
		"created ",
		"added ",
		"implemented ",
		"fixed ",
		"updated ",
		"wrote ",
		"configured ",
		"set up ",
		"refactored ",
		"completed ",
		"done",
	}

	for _, msg := range messages {
		if msg.Role != "assistant" || msg.Content == "" {
			continue
		}

		lines := strings.Split(msg.Content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			lower := strings.ToLower(line)
			for _, marker := range completionMarkers {
				if strings.Contains(lower, marker) {
					item := strings.TrimPrefix(line, "- ")
					item = strings.TrimPrefix(item, "* ")
					if len(item) > 150 {
						item = item[:150]
					}
					items = append(items, fmt.Sprintf("- %s", item))
					break
				}
			}
		}
	}

	if len(items) == 0 {
		return "- No completed work recorded"
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			unique = append(unique, item)
		}
	}

	return strings.Join(unique, "\n")
}

// FormatHandover produces a human-readable string representation of a handover.
func FormatHandover(handover *Handover) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Handover [%s]\n", handover.SessionID))
	sb.WriteString(fmt.Sprintf("  From: %s\n", handover.FromModel))
	if handover.ToModel != "" {
		sb.WriteString(fmt.Sprintf("  To:   %s\n", handover.ToModel))
	}
	sb.WriteString(fmt.Sprintf("  Status: %s\n", handover.Status))
	sb.WriteString(fmt.Sprintf("  Created: %s\n", handover.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("  Goal: %s\n", handover.Context.Goal))

	if len(handover.Context.FilesModified) > 0 {
		sb.WriteString(fmt.Sprintf("  Files: %s\n", strings.Join(handover.Context.FilesModified, ", ")))
	}

	if len(handover.Context.PendingTasks) > 0 {
		sb.WriteString("  Pending:\n")
		for _, t := range handover.Context.PendingTasks {
			sb.WriteString(fmt.Sprintf("    - %s\n", t))
		}
	}

	return sb.String()
}

// SaveHandover persists a handover to disk as JSON.
func SaveHandover(handover *Handover, path string) error {
	data, err := json.MarshalIndent(handover, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal handover: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write handover: %w", err)
	}
	return nil
}

// LoadHandover reads a handover from disk.
func LoadHandover(path string) (*Handover, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path supplied by trusted internal caller (handover file location)
	if err != nil {
		return nil, fmt.Errorf("read handover: %w", err)
	}
	var h Handover
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("unmarshal handover: %w", err)
	}
	return &h, nil
}
