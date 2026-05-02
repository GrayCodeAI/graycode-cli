package session

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
)

// CheckpointTrigger classifies events that may trigger a checkpoint.
type CheckpointTrigger int

const (
	TriggerFileWrite    CheckpointTrigger = iota // file was modified
	TriggerToolError                              // tool execution failed
	TriggerUserFeedback                           // user gave correction
	TriggerPlanChange                             // plan/subtask status changed
	TriggerContextShift                           // topic changed significantly
)

// String returns a human-readable name for the trigger.
func (ct CheckpointTrigger) String() string {
	switch ct {
	case TriggerFileWrite:
		return "file_write"
	case TriggerToolError:
		return "tool_error"
	case TriggerUserFeedback:
		return "user_feedback"
	case TriggerPlanChange:
		return "plan_change"
	case TriggerContextShift:
		return "context_shift"
	default:
		return "unknown"
	}
}

// SmartCheckpointer takes snapshots only when meaningful state changes occur,
// filtering out redundant checkpoints.
type SmartCheckpointer struct {
	mu          sync.Mutex
	store       *SnapshotStore
	lastContent string // hash of last checkpointed state
	triggers    map[CheckpointTrigger]bool

	// stats
	eventsSeen         int
	checkpointsTaken   int
	eventsFiltered     int
}

// NewSmartCheckpointer creates a checkpointer that wraps a SnapshotStore.
// All trigger types are enabled by default.
func NewSmartCheckpointer(store *SnapshotStore) *SmartCheckpointer {
	return &SmartCheckpointer{
		store: store,
		triggers: map[CheckpointTrigger]bool{
			TriggerFileWrite:    true,
			TriggerToolError:    true,
			TriggerUserFeedback: true,
			TriggerPlanChange:   true,
			TriggerContextShift: true,
		},
	}
}

// SetTrigger enables or disables a specific trigger type.
func (sc *SmartCheckpointer) SetTrigger(trigger CheckpointTrigger, enabled bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.triggers[trigger] = enabled
}

// ShouldCheckpoint returns true only if the state has meaningfully changed
// since the last checkpoint.
func (sc *SmartCheckpointer) ShouldCheckpoint(event CheckpointTrigger, session *Session) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Check if trigger type is enabled.
	if !sc.triggers[event] {
		return false
	}

	// Compute content hash of the current session state.
	currentHash := sessionContentHash(session)

	// If the hash matches the last checkpoint, state hasn't changed.
	if currentHash == sc.lastContent {
		return false
	}

	return true
}

// OnEvent processes a checkpoint event. If meaningful state change is detected,
// it takes a snapshot with the provided action label.
func (sc *SmartCheckpointer) OnEvent(event CheckpointTrigger, session *Session, action string) {
	sc.mu.Lock()
	sc.eventsSeen++

	// Check if trigger type is enabled.
	if !sc.triggers[event] {
		sc.eventsFiltered++
		sc.mu.Unlock()
		return
	}

	// Compute content hash.
	currentHash := sessionContentHash(session)
	if currentHash == sc.lastContent {
		sc.eventsFiltered++
		sc.mu.Unlock()
		return
	}

	sc.lastContent = currentHash
	sc.checkpointsTaken++
	store := sc.store
	sc.mu.Unlock()

	// Take the snapshot outside the lock (it does its own I/O).
	label := fmt.Sprintf("[%s] %s", event, action)
	if store != nil {
		store.Take(label, session)
	}
}

// Stats returns a human-readable summary of checkpoint activity.
func (sc *SmartCheckpointer) Stats() string {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.eventsSeen == 0 {
		return "0 events seen, 0 checkpoints taken"
	}
	filteredPct := float64(sc.eventsSeen-sc.checkpointsTaken) / float64(sc.eventsSeen) * 100
	return fmt.Sprintf("%d events seen, %d checkpoints taken (%.0f%% filtered)",
		sc.eventsSeen, sc.checkpointsTaken, filteredPct)
}

// EventsSeen returns the total number of events processed.
func (sc *SmartCheckpointer) EventsSeen() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.eventsSeen
}

// CheckpointsTaken returns the number of checkpoints actually created.
func (sc *SmartCheckpointer) CheckpointsTaken() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.checkpointsTaken
}

// sessionContentHash computes a deterministic hash of the session's message
// content to detect meaningful state changes.
func sessionContentHash(session *Session) string {
	if session == nil {
		return ""
	}
	h := sha256.New()
	for _, msg := range session.Messages {
		h.Write([]byte(msg.Role))
		h.Write([]byte(msg.Content))
		for _, tc := range msg.ToolUse {
			h.Write([]byte(tc.Name))
			h.Write([]byte(tc.ID))
		}
		if msg.ToolResult != nil {
			h.Write([]byte(msg.ToolResult.Content))
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// FormatTriggers returns a summary of which triggers are enabled.
func (sc *SmartCheckpointer) FormatTriggers() string {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	var enabled []string
	var disabled []string
	allTriggers := []CheckpointTrigger{
		TriggerFileWrite, TriggerToolError, TriggerUserFeedback,
		TriggerPlanChange, TriggerContextShift,
	}
	for _, t := range allTriggers {
		if sc.triggers[t] {
			enabled = append(enabled, t.String())
		} else {
			disabled = append(disabled, t.String())
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Enabled: %s", strings.Join(enabled, ", ")))
	if len(disabled) > 0 {
		b.WriteString(fmt.Sprintf("\nDisabled: %s", strings.Join(disabled, ", ")))
	}
	return b.String()
}
