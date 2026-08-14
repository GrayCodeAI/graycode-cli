package compression

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// TimelineEvent represents a single event in a session timeline.
type TimelineEvent struct {
	ID        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Type      string            `json:"type"` // "action", "decision", "milestone", "error", "user_input", "file_change"
	Content   string            `json:"content"`
	Duration  time.Duration     `json:"duration,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Timeline tracks a chronological sequence of events during a session.
type Timeline struct {
	Events    []TimelineEvent `json:"events"`
	SessionID string          `json:"session_id"`
	StartTime time.Time       `json:"start_time"`
	mu        sync.RWMutex
}

// NewTimeline creates a new Timeline for the given session ID.
func NewTimeline(sessionID string) *Timeline {
	return &Timeline{
		Events:    make([]TimelineEvent, 0),
		SessionID: sessionID,
		StartTime: time.Now(),
	}
}

// AddEvent records a generic event with the given type, content, and metadata.
func (t *Timeline) AddEvent(eventType, content string, metadata map[string]string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	event := TimelineEvent{
		ID:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Type:      eventType,
		Content:   content,
		Metadata:  metadata,
	}
	t.Events = append(t.Events, event)
}

// AddAction records a tool action with its target and duration.
func (t *Timeline) AddAction(tool, target string, duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	event := TimelineEvent{
		ID:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Type:      "action",
		Content:   fmt.Sprintf("%s %s", tool, target),
		Duration:  duration,
		Metadata: map[string]string{
			"tool":   tool,
			"target": target,
		},
	}
	t.Events = append(t.Events, event)
}

// AddDecision records a decision with its reasoning.
func (t *Timeline) AddDecision(decision, reason string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	event := TimelineEvent{
		ID:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Type:      "decision",
		Content:   decision,
		Metadata: map[string]string{
			"reason": reason,
		},
	}
	t.Events = append(t.Events, event)
}

// AddMilestone records a significant milestone in the session.
func (t *Timeline) AddMilestone(description string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	event := TimelineEvent{
		ID:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Type:      "milestone",
		Content:   description,
	}
	t.Events = append(t.Events, event)
}

// AddError records an error that occurred during the session.
func (t *Timeline) AddError(err string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	event := TimelineEvent{
		ID:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Type:      "error",
		Content:   err,
	}
	t.Events = append(t.Events, event)
}

// AddFileChange records a file system change (create, modify, or delete).
func (t *Timeline) AddFileChange(path, action string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	event := TimelineEvent{
		ID:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Type:      "file_change",
		Content:   fmt.Sprintf("%s %s", action, path),
		Metadata: map[string]string{
			"path":   path,
			"action": action,
		},
	}
	t.Events = append(t.Events, event)
}

// GetByType returns all events matching the given type.
func (t *Timeline) GetByType(eventType string) []TimelineEvent {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []TimelineEvent
	for _, e := range t.Events {
		if e.Type == eventType {
			result = append(result, e)
		}
	}
	return result
}

// GetBetween returns all events between start and end times (inclusive).
func (t *Timeline) GetBetween(start, end time.Time) []TimelineEvent {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []TimelineEvent
	for _, e := range t.Events {
		if (e.Timestamp.Equal(start) || e.Timestamp.After(start)) &&
			(e.Timestamp.Equal(end) || e.Timestamp.Before(end)) {
			result = append(result, e)
		}
	}
	return result
}

// GetMilestones returns all milestone events.
func (t *Timeline) GetMilestones() []TimelineEvent {
	return t.GetByType("milestone")
}

// Duration returns the total elapsed time since the timeline started.
func (t *Timeline) Duration() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.durationLocked()
}

// durationLocked computes duration without acquiring the lock (caller must hold it).
func (t *Timeline) durationLocked() time.Duration {
	if len(t.Events) == 0 {
		return time.Since(t.StartTime)
	}
	lastEvent := t.Events[len(t.Events)-1]
	return lastEvent.Timestamp.Sub(t.StartTime)
}

// RenderTimeline produces a human-readable chronological view of the session.
func (t *Timeline) RenderTimeline(maxWidth int) string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if maxWidth < 40 {
		maxWidth = 40
	}

	totalDuration := t.durationLocked()
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("Session Timeline (%s):\n", timelineFmtDuration(totalDuration)))
	sepLen := maxWidth - 1
	if sepLen > 39 {
		sepLen = 39
	}
	separator := strings.Repeat("═", sepLen)
	sb.WriteString(separator + "\n")

	// Event counts for summary
	actionCount := 0
	decisionCount := 0

	// Render each event
	for _, e := range t.Events {
		offset := e.Timestamp.Sub(t.StartTime)
		minutes := int(offset.Minutes())
		seconds := int(offset.Seconds()) % 60
		timeStr := fmt.Sprintf("%02d:%02d", minutes, seconds)

		icon := timelineEventIcon(e.Type)
		line := timelineFmtEventLine(e, icon)

		if e.Duration > 0 {
			line += fmt.Sprintf(" (%s)", timelineFmtShortDuration(e.Duration))
		}

		sb.WriteString(fmt.Sprintf("%s  %s\n", timeStr, line))

		switch e.Type {
		case "action":
			actionCount++
		case "decision":
			decisionCount++
		}
	}

	// Footer
	sb.WriteString(separator + "\n")
	sb.WriteString(fmt.Sprintf("Duration: %s | Actions: %d | Decisions: %d\n",
		timelineFmtDuration(totalDuration), actionCount, decisionCount))

	return sb.String()
}

// Summarize returns a one-paragraph summary of the session.
func (t *Timeline) Summarize() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.Events) == 0 {
		return "Empty session with no recorded events."
	}

	totalDuration := t.durationLocked()
	milestones := 0
	actions := 0
	decisions := 0
	errors := 0
	fileChanges := 0
	userInputs := 0

	for _, e := range t.Events {
		switch e.Type {
		case "milestone":
			milestones++
		case "action":
			actions++
		case "decision":
			decisions++
		case "error":
			errors++
		case "file_change":
			fileChanges++
		case "user_input":
			userInputs++
		}
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Session %s lasted %s with %d total events.",
		t.SessionID, timelineFmtDuration(totalDuration), len(t.Events)))

	if actions > 0 {
		parts = append(parts, fmt.Sprintf("%d actions were performed", actions))
	}
	if decisions > 0 {
		parts = append(parts, fmt.Sprintf("%d decisions were made", decisions))
	}
	if fileChanges > 0 {
		parts = append(parts, fmt.Sprintf("%d files were changed", fileChanges))
	}
	if milestones > 0 {
		parts = append(parts, fmt.Sprintf("%d milestones were reached", milestones))
	}
	if errors > 0 {
		parts = append(parts, fmt.Sprintf("%d errors occurred", errors))
	}

	// Include the last milestone as the session outcome if available.
	milestoneEvents := t.getMilestonesLocked()
	if len(milestoneEvents) > 0 {
		last := milestoneEvents[len(milestoneEvents)-1]
		parts = append(parts, fmt.Sprintf("Final milestone: %s.", last.Content))
	}

	return strings.Join(parts, " ")
}

// getMilestonesLocked returns milestones without acquiring the lock (caller must hold it).
func (t *Timeline) getMilestonesLocked() []TimelineEvent {
	var result []TimelineEvent
	for _, e := range t.Events {
		if e.Type == "milestone" {
			result = append(result, e)
		}
	}
	return result
}

// timelineEventIcon returns the emoji icon for a given event type.
func timelineEventIcon(eventType string) string {
	switch eventType {
	case "action":
		return icons.Magnify() // magnifying glass
	case "decision":
		return icons.Brain() // light bulb
	case "milestone":
		return icons.CheckBold() // check mark
	case "error":
		return icons.CloseThick() // cross mark
	case "user_input":
		return "EDIT:" // memo
	case "file_change":
		return "[edit]" // pencil
	default:
		return "•" // bullet
	}
}

// timelineFmtEventLine formats an event into a display line.
func timelineFmtEventLine(e TimelineEvent, icon string) string {
	switch e.Type {
	case "action":
		tool := e.Metadata["tool"]
		target := e.Metadata["target"]
		return fmt.Sprintf("%s %s: %s", icon, tool, target)
	case "decision":
		return fmt.Sprintf("%s Decision: %s", icon, e.Content)
	case "milestone":
		return fmt.Sprintf("%s Milestone: %s", icon, e.Content)
	case "error":
		return fmt.Sprintf("%s Error: %s", icon, e.Content)
	case "user_input":
		return fmt.Sprintf("%s User: \"%s\"", icon, e.Content)
	case "file_change":
		action := e.Metadata["action"]
		path := e.Metadata["path"]
		return fmt.Sprintf("%s %s %s", icon, timelineCapitalizeFirst(action), path)
	default:
		return fmt.Sprintf("%s %s", icon, e.Content)
	}
}

// timelineFmtDuration formats a duration like "12m 30s" or "5s".
func timelineFmtDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60

	if minutes > 0 && seconds > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%ds", seconds)
}

// timelineFmtShortDuration formats a duration for inline display like "0.1s" or "3.2s".
func timelineFmtShortDuration(d time.Duration) string {
	secs := d.Seconds()
	if secs < 0.1 {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", secs)
}

// timelineCapitalizeFirst returns the string with its first character uppercased.
func timelineCapitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
