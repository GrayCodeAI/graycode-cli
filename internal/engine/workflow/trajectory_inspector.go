package workflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

// TrajectoryEvent represents a single event in an agent's trajectory.
type TrajectoryEvent struct {
	Index     int               `json:"index"`
	Type      string            `json:"type"` // "thought", "action", "observation", "error", "decision"
	Content   string            `json:"content"`
	ToolName  string            `json:"tool_name,omitempty"`
	Duration  time.Duration     `json:"duration"`
	Tokens    int               `json:"tokens"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// TrajectoryInspector records and visualizes agent action sequences for
// debugging and analysis. Inspired by SWE-agent's trajectory inspector.
type TrajectoryInspector struct {
	Events    []TrajectoryEvent `json:"events"`
	SessionID string            `json:"session_id"`
	StartTime time.Time         `json:"start_time"`
	mu        sync.RWMutex
}

// NewTrajectoryInspector creates a new inspector for the given session.
func NewTrajectoryInspector(sessionID string) *TrajectoryInspector {
	return &TrajectoryInspector{
		Events:    []TrajectoryEvent{},
		SessionID: sessionID,
		StartTime: time.Now(),
	}
}

// Record adds a new event to the trajectory.
func (ti *TrajectoryInspector) Record(eventType, content string, toolName string, duration time.Duration, tokens int) {
	ti.mu.Lock()
	defer ti.mu.Unlock()

	event := TrajectoryEvent{
		Index:     len(ti.Events),
		Type:      eventType,
		Content:   content,
		ToolName:  toolName,
		Duration:  duration,
		Tokens:    tokens,
		Timestamp: time.Now(),
		Metadata:  make(map[string]string),
	}
	ti.Events = append(ti.Events, event)
}

// RenderTimeline returns a formatted timeline of all events in the trajectory.
func (ti *TrajectoryInspector) RenderTimeline() string {
	ti.mu.RLock()
	defer ti.mu.RUnlock()

	if len(ti.Events) == 0 {
		return fmt.Sprintf("Trajectory: %s (empty)\n", ti.SessionID)
	}

	totalDuration := ti.totalDuration()
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Trajectory: %s (%s)\n", ti.SessionID, inspectorFormatDuration(totalDuration)))
	b.WriteString("═══════════════════════════════════════\n")

	for _, event := range ti.Events {
		elapsed := event.Timestamp.Sub(ti.StartTime).Seconds()
		icon := eventIcon(event.Type)
		label := strings.ToUpper(event.Type)

		if event.ToolName != "" {
			b.WriteString(fmt.Sprintf("[%.1fs] %s %s: %s(%s)\n", elapsed, icon, label, event.ToolName, event.Content))
		} else {
			b.WriteString(fmt.Sprintf("[%.1fs] %s %s: %s\n", elapsed, icon, label, event.Content))
		}
	}

	return b.String()
}

// RenderStats returns a formatted summary of trajectory statistics.
func (ti *TrajectoryInspector) RenderStats() string {
	ti.mu.RLock()
	defer ti.mu.RUnlock()

	totalDuration := ti.totalDuration()
	totalTokens := 0
	typeCounts := make(map[string]int)
	var actionDurations []time.Duration

	for _, event := range ti.Events {
		totalTokens += event.Tokens
		typeCounts[event.Type]++
		if event.Type == "action" {
			actionDurations = append(actionDurations, event.Duration)
		}
	}

	avgAction := time.Duration(0)
	if len(actionDurations) > 0 {
		var total time.Duration
		for _, d := range actionDurations {
			total += d
		}
		avgAction = total / time.Duration(len(actionDurations))
	}

	// Find most used tool.
	toolUsage := ti.toolUsage()
	mostUsedTool := ""
	mostUsedCount := 0
	for tool, count := range toolUsage {
		if count > mostUsedCount {
			mostUsedTool = tool
			mostUsedCount = count
		}
	}

	var b strings.Builder
	b.WriteString("Trajectory Stats:\n")
	b.WriteString(fmt.Sprintf("Events: %d | Duration: %s | Tokens: %s\n",
		len(ti.Events), inspectorFormatDuration(totalDuration), inspectorFormatTokens(totalTokens)))
	b.WriteString(fmt.Sprintf("Thoughts: %d | Actions: %d | Observations: %d | Errors: %d\n",
		typeCounts["thought"], typeCounts["action"], typeCounts["observation"], typeCounts["error"]))

	mostUsedStr := "N/A"
	if mostUsedTool != "" {
		mostUsedStr = fmt.Sprintf("%s (%dx)", mostUsedTool, mostUsedCount)
	}
	b.WriteString(fmt.Sprintf("Avg action time: %.1fs | Most used tool: %s\n",
		avgAction.Seconds(), mostUsedStr))

	return b.String()
}

// FindPatterns analyzes the trajectory for common patterns and returns descriptions.
func (ti *TrajectoryInspector) FindPatterns() []string {
	ti.mu.RLock()
	defer ti.mu.RUnlock()

	var patterns []string

	// Check for read-before-edit pattern.
	readBeforeEdit := ti.checkReadBeforeEdit()
	if readBeforeEdit {
		patterns = append(patterns, "Read-before-edit pattern used consistently")
	}

	// Check for errors.
	errorCount := 0
	for _, event := range ti.Events {
		if event.Type == "error" {
			errorCount++
		}
	}
	if errorCount == 0 {
		patterns = append(patterns, "No errors encountered")
	} else {
		patterns = append(patterns, fmt.Sprintf("%d errors encountered", errorCount))
	}

	// Check for retries on Bash commands.
	bashRetries := ti.countBashRetries()
	if bashRetries > 0 {
		patterns = append(patterns, fmt.Sprintf("%d retries on Bash commands", bashRetries))
	}

	// Check for rapid action sequences.
	rapidActions := ti.countRapidActions()
	if rapidActions > 3 {
		patterns = append(patterns, fmt.Sprintf("%d rapid consecutive actions (< 0.5s apart)", rapidActions))
	}

	// Check for long-running operations.
	longOps := 0
	for _, event := range ti.Events {
		if event.Duration > 5*time.Second {
			longOps++
		}
	}
	if longOps > 0 {
		patterns = append(patterns, fmt.Sprintf("%d long-running operations (> 5s)", longOps))
	}

	if len(patterns) == 0 {
		patterns = append(patterns, "No notable patterns detected")
	}

	return patterns
}

// GetByType returns all events matching the given type.
func (ti *TrajectoryInspector) GetByType(eventType string) []TrajectoryEvent {
	ti.mu.RLock()
	defer ti.mu.RUnlock()

	var result []TrajectoryEvent
	for _, event := range ti.Events {
		if event.Type == eventType {
			result = append(result, event)
		}
	}
	return result
}

// GetToolUsage returns a map of tool names to their usage counts.
func (ti *TrajectoryInspector) GetToolUsage() map[string]int {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	return ti.toolUsage()
}

// ExportJSON serializes the trajectory inspector state to JSON.
func (ti *TrajectoryInspector) ExportJSON() (string, error) {
	ti.mu.RLock()
	defer ti.mu.RUnlock()

	data, err := json.MarshalIndent(ti, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to export trajectory: %w", err)
	}
	return string(data), nil
}

// Replay returns a channel that emits events at the given speed multiplier.
// A speed of 1.0 replays in real-time, 2.0 at double speed, etc.
func (ti *TrajectoryInspector) Replay(speed float64) <-chan TrajectoryEvent {
	ti.mu.RLock()
	events := make([]TrajectoryEvent, len(ti.Events))
	copy(events, ti.Events)
	ti.mu.RUnlock()

	ch := make(chan TrajectoryEvent)

	if speed <= 0 {
		speed = 1.0
	}

	go func() {
		defer close(ch)

		for i, event := range events {
			if i > 0 {
				gap := events[i].Timestamp.Sub(events[i-1].Timestamp)
				scaledGap := time.Duration(float64(gap) / speed)
				if scaledGap > 0 {
					time.Sleep(scaledGap)
				}
			}
			ch <- event
		}
	}()

	return ch
}

// Summarize returns a one-paragraph summary of what happened in the trajectory.
func (ti *TrajectoryInspector) Summarize() string {
	ti.mu.RLock()
	defer ti.mu.RUnlock()

	if len(ti.Events) == 0 {
		return "Empty trajectory with no recorded events."
	}

	totalDuration := ti.totalDuration()
	typeCounts := make(map[string]int)
	toolUsage := ti.toolUsage()
	var toolNames []string

	for _, event := range ti.Events {
		typeCounts[event.Type]++
	}

	for tool := range toolUsage {
		toolNames = append(toolNames, tool)
	}
	sort.Strings(toolNames)

	totalTokens := 0
	for _, event := range ti.Events {
		totalTokens += event.Tokens
	}

	errorCount := typeCounts["error"]

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Session %s completed in %s with %d events. ",
		ti.SessionID, inspectorFormatDuration(totalDuration), len(ti.Events)))

	if typeCounts["thought"] > 0 {
		b.WriteString(fmt.Sprintf("The agent engaged in %d thought steps, ", typeCounts["thought"]))
	}
	if typeCounts["action"] > 0 {
		b.WriteString(fmt.Sprintf("performed %d actions", typeCounts["action"]))
		if len(toolNames) > 0 {
			b.WriteString(fmt.Sprintf(" using tools: %s", strings.Join(toolNames, ", ")))
		}
		b.WriteString(", ")
	}
	if typeCounts["observation"] > 0 {
		b.WriteString(fmt.Sprintf("and processed %d observations. ", typeCounts["observation"]))
	}
	if errorCount > 0 {
		b.WriteString(fmt.Sprintf("Encountered %d errors during execution. ", errorCount))
	} else {
		b.WriteString("No errors were encountered. ")
	}
	if typeCounts["decision"] > 0 {
		b.WriteString(fmt.Sprintf("Reached %d decision points. ", typeCounts["decision"]))
	}
	if totalTokens > 0 {
		b.WriteString(fmt.Sprintf("Total token usage: %s.", inspectorFormatTokens(totalTokens)))
	}

	return strings.TrimSpace(b.String())
}

// --- Internal helpers ---

// totalDuration returns the time elapsed from the first to the last event.
func (ti *TrajectoryInspector) totalDuration() time.Duration {
	if len(ti.Events) == 0 {
		return 0
	}
	last := ti.Events[len(ti.Events)-1]
	return last.Timestamp.Sub(ti.StartTime)
}

// toolUsage returns a map of tool name to count (internal, no lock).
func (ti *TrajectoryInspector) toolUsage() map[string]int {
	usage := make(map[string]int)
	for _, event := range ti.Events {
		if event.ToolName != "" {
			usage[event.ToolName]++
		}
	}
	return usage
}

// checkReadBeforeEdit checks if Read is consistently used before Edit.
func (ti *TrajectoryInspector) checkReadBeforeEdit() bool {
	editCount := 0
	readBeforeEditCount := 0

	for i, event := range ti.Events {
		if event.ToolName == "Edit" || event.ToolName == "FileWrite" {
			editCount++
			// Look back for a preceding Read.
			for j := i - 1; j >= 0 && j >= i-3; j-- {
				if ti.Events[j].ToolName == "Read" || ti.Events[j].ToolName == "FileRead" {
					readBeforeEditCount++
					break
				}
			}
		}
	}

	return editCount > 0 && readBeforeEditCount == editCount
}

// countBashRetries counts how many times a Bash command was retried
// (same tool used consecutively).
func (ti *TrajectoryInspector) countBashRetries() int {
	retries := 0
	for i := 1; i < len(ti.Events); i++ {
		if ti.Events[i].ToolName == "Bash" && ti.Events[i-1].ToolName == "Bash" {
			retries++
		}
	}
	return retries
}

// countRapidActions counts actions that happen less than 0.5s apart.
func (ti *TrajectoryInspector) countRapidActions() int {
	rapid := 0
	for i := 1; i < len(ti.Events); i++ {
		gap := ti.Events[i].Timestamp.Sub(ti.Events[i-1].Timestamp)
		if gap < 500*time.Millisecond && ti.Events[i].Type == "action" {
			rapid++
		}
	}
	return rapid
}

// eventIcon returns the emoji icon for a given event type.
func eventIcon(eventType string) string {
	switch eventType {
	case "thought":
		return "NOTE:" // 💭
	case "action":
		return "FIX:" // 🔧
	case "observation":
		return "VIEW:" // 👁
	case "error":
		return icons.CloseThick() // ❌
	case "decision":
		return icons.CheckBold() // ✅
	default:
		return "•" // •
	}
}

// inspectorFormatDuration formats a duration in a human-friendly way.
func inspectorFormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// inspectorFormatTokens formats a token count with comma separators.
func inspectorFormatTokens(tokens int) string {
	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	}
	return fmt.Sprintf("%d,%03d", tokens/1000, tokens%1000)
}
