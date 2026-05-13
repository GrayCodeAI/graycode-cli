package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ReplayStep represents a single step in a session replay.
type ReplayStep struct {
	Index            int           `json:"index"`
	Role             string        `json:"role"`
	Content          string        `json:"content"`
	ToolName         string        `json:"tool_name,omitempty"`
	ToolArgs         string        `json:"tool_args,omitempty"`
	OriginalDuration time.Duration `json:"original_duration"`
	Timestamp        time.Time     `json:"timestamp"`
}

// ReplayResult summarizes the outcome of a full replay execution.
type ReplayResult struct {
	OriginalSteps int           `json:"original_steps"`
	ReplayedSteps int           `json:"replayed_steps"`
	Divergences   []Divergence  `json:"divergences,omitempty"`
	Duration      time.Duration `json:"duration"`
}

// Divergence records a mismatch between expected and actual output at a step.
type Divergence struct {
	StepIndex int    `json:"step_index"`
	Expected  string `json:"expected"`
	Got       string `json:"got"`
	Type      string `json:"type"` // "content_mismatch", "tool_mismatch", "error"
}

// Replay manages the re-execution of a previous session's prompts.
type Replay struct {
	SessionID   string       `json:"session_id"`
	Steps       []ReplayStep `json:"steps"`
	Speed       float64      `json:"speed"` // 1.0=realtime, 0=instant
	Breakpoints []int        `json:"breakpoints,omitempty"`
	Status      string       `json:"status"` // "idle", "playing", "paused", "stopped", "done"
	CurrentStep int          `json:"current_step"`
	mu          sync.Mutex
}

// NewReplay creates a new Replay instance for the given session ID.
func NewReplay(sessionID string) *Replay {
	return &Replay{
		SessionID:   sessionID,
		Steps:       make([]ReplayStep, 0),
		Speed:       1.0,
		Breakpoints: make([]int, 0),
		Status:      "idle",
		CurrentStep: 0,
	}
}

// LoadFromExport parses a JSONL replay format string into a Replay instance.
// Each line must be a JSON object with fields matching the replayEntry structure
// used by ExportReplay (seq, role, content, tool_name, delta_ms, timestamp).
func LoadFromExport(data string) (*Replay, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil, fmt.Errorf("empty replay data")
	}

	lines := strings.Split(data, "\n")
	replay := &Replay{
		Steps:       make([]ReplayStep, 0, len(lines)),
		Speed:       1.0,
		Breakpoints: make([]int, 0),
		Status:      "idle",
		CurrentStep: 0,
	}

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry struct {
			Seq        int       `json:"seq"`
			Timestamp  time.Time `json:"timestamp"`
			DeltaMs    int64     `json:"delta_ms"`
			Role       string    `json:"role"`
			Content    string    `json:"content"`
			ToolName   string    `json:"tool_name,omitempty"`
			ToolArgs   string    `json:"tool_args,omitempty"`
			ToolResult string    `json:"tool_result,omitempty"`
		}

		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("failed to parse replay entry at line %d: %w", i+1, err)
		}

		step := ReplayStep{
			Index:            len(replay.Steps),
			Role:             entry.Role,
			Content:          entry.Content,
			ToolName:         entry.ToolName,
			ToolArgs:         entry.ToolArgs,
			OriginalDuration: time.Duration(entry.DeltaMs) * time.Millisecond,
			Timestamp:        entry.Timestamp,
		}

		// If tool_result is present but ToolArgs is empty, store tool_result in ToolArgs
		// for compatibility with the export format.
		if entry.ToolResult != "" && step.ToolArgs == "" {
			step.ToolArgs = entry.ToolResult
		}

		replay.Steps = append(replay.Steps, step)
	}

	if len(replay.Steps) == 0 {
		return nil, fmt.Errorf("no valid replay steps found")
	}

	return replay, nil
}

// Play re-executes all user prompts in the replay using the provided execute function.
// It compares assistant responses against the original to detect divergences.
// The executeFn takes a user prompt and returns the model's response.
func (r *Replay) Play(ctx context.Context, executeFn func(string) (string, error)) (*ReplayResult, error) {
	r.mu.Lock()
	r.Status = "playing"
	r.CurrentStep = 0
	r.mu.Unlock()

	start := time.Now()
	result := &ReplayResult{
		OriginalSteps: len(r.Steps),
		Divergences:   make([]Divergence, 0),
	}

	for i := 0; i < len(r.Steps); i++ {
		// Check context cancellation.
		select {
		case <-ctx.Done():
			r.mu.Lock()
			r.Status = "stopped"
			r.mu.Unlock()
			result.Duration = time.Since(start)
			return result, ctx.Err()
		default:
		}

		// Check if stopped.
		r.mu.Lock()
		status := r.Status
		r.mu.Unlock()

		if status == "stopped" {
			result.Duration = time.Since(start)
			return result, nil
		}

		// Handle pause.
		for {
			r.mu.Lock()
			s := r.Status
			r.mu.Unlock()
			if s != "paused" {
				break
			}
			// Wait briefly before checking again.
			select {
			case <-ctx.Done():
				r.mu.Lock()
				r.Status = "stopped"
				r.mu.Unlock()
				result.Duration = time.Since(start)
				return result, ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}

		// Check breakpoints.
		r.mu.Lock()
		for _, bp := range r.Breakpoints {
			if bp == i {
				r.Status = "paused"
				break
			}
		}
		if r.Status == "paused" {
			r.mu.Unlock()
			// Wait for resume or stop.
			for {
				r.mu.Lock()
				s := r.Status
				r.mu.Unlock()
				if s != "paused" {
					break
				}
				select {
				case <-ctx.Done():
					r.mu.Lock()
					r.Status = "stopped"
					r.mu.Unlock()
					result.Duration = time.Since(start)
					return result, ctx.Err()
				case <-time.After(50 * time.Millisecond):
				}
			}
			// Re-check if stopped after breakpoint resume.
			r.mu.Lock()
			if r.Status == "stopped" {
				r.mu.Unlock()
				result.Duration = time.Since(start)
				return result, nil
			}
			r.mu.Unlock()
		} else {
			r.mu.Unlock()
		}

		step := r.Steps[i]

		r.mu.Lock()
		r.CurrentStep = i
		r.mu.Unlock()

		// Only re-execute user prompts.
		if step.Role != "user" {
			continue
		}

		// Apply speed delay.
		if r.Speed > 0 && step.OriginalDuration > 0 {
			delay := time.Duration(float64(step.OriginalDuration) / r.Speed)
			select {
			case <-ctx.Done():
				r.mu.Lock()
				r.Status = "stopped"
				r.mu.Unlock()
				result.Duration = time.Since(start)
				return result, ctx.Err()
			case <-time.After(delay):
			}
		}

		// Execute the prompt.
		actual, err := executeFn(step.Content)
		result.ReplayedSteps++

		if err != nil {
			div := Divergence{
				StepIndex: i,
				Expected:  findExpectedResponse(r.Steps, i),
				Got:       err.Error(),
				Type:      "error",
			}
			result.Divergences = append(result.Divergences, div)
			continue
		}

		// Compare with expected response (the next assistant message).
		expected := findExpectedResponse(r.Steps, i)
		if expected != "" {
			if div := DetectDivergence(expected, actual); div != nil {
				div.StepIndex = i
				result.Divergences = append(result.Divergences, *div)
			}
		}
	}

	r.mu.Lock()
	r.Status = "done"
	r.mu.Unlock()

	result.Duration = time.Since(start)
	return result, nil
}

// PlayStep re-executes a single step at the given index.
func (r *Replay) PlayStep(ctx context.Context, step int, executeFn func(string) (string, error)) (*ReplayStep, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if step < 0 || step >= len(r.Steps) {
		return nil, fmt.Errorf("step index %d out of range [0, %d)", step, len(r.Steps))
	}

	s := r.Steps[step]
	if s.Role != "user" {
		return &s, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	r.CurrentStep = step
	r.Status = "playing"

	// Unlock during execution to avoid holding the lock.
	r.mu.Unlock()
	actual, err := executeFn(s.Content)
	r.mu.Lock()

	if err != nil {
		r.Status = "idle"
		return nil, fmt.Errorf("execution failed at step %d: %w", step, err)
	}

	result := &ReplayStep{
		Index:     step,
		Role:      "assistant",
		Content:   actual,
		Timestamp: time.Now(),
	}

	r.Status = "idle"
	return result, nil
}

// SetBreakpoint adds a breakpoint at the given step index.
func (r *Replay) SetBreakpoint(stepIndex int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Don't add duplicates.
	for _, bp := range r.Breakpoints {
		if bp == stepIndex {
			return
		}
	}
	r.Breakpoints = append(r.Breakpoints, stepIndex)
}

// RemoveBreakpoint removes a breakpoint at the given step index.
func (r *Replay) RemoveBreakpoint(stepIndex int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, bp := range r.Breakpoints {
		if bp == stepIndex {
			r.Breakpoints = append(r.Breakpoints[:i], r.Breakpoints[i+1:]...)
			return
		}
	}
}

// Pause pauses the replay execution.
func (r *Replay) Pause() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Status == "playing" {
		r.Status = "paused"
	}
}

// Resume resumes a paused replay.
func (r *Replay) Resume() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Status == "paused" {
		r.Status = "playing"
	}
}

// Stop terminates the replay execution.
func (r *Replay) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Status = "stopped"
}

// DetectDivergence compares expected and actual output and returns a Divergence
// if they differ. Returns nil if they match.
func DetectDivergence(expected, actual string) *Divergence {
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)

	if expected == actual {
		return nil
	}

	// Determine the type of divergence.
	divType := "content_mismatch"

	// Check if this looks like a tool mismatch (contains tool-like patterns).
	if looksLikeToolCall(expected) || looksLikeToolCall(actual) {
		if extractToolName(expected) != extractToolName(actual) {
			divType = "tool_mismatch"
		}
	}

	return &Divergence{
		Expected: expected,
		Got:      actual,
		Type:     divType,
	}
}

// FormatReplayStatus returns a human-readable progress display for the replay.
func (r *Replay) FormatReplayStatus() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	total := len(r.Steps)
	if total == 0 {
		return "[replay] no steps loaded"
	}

	progress := float64(r.CurrentStep+1) / float64(total) * 100
	barWidth := 20
	filled := int(float64(barWidth) * float64(r.CurrentStep+1) / float64(total))
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("=", filled) + strings.Repeat("-", barWidth-filled)

	speedStr := "instant"
	if r.Speed > 0 {
		speedStr = fmt.Sprintf("%.1fx", r.Speed)
	}

	bpStr := ""
	if len(r.Breakpoints) > 0 {
		bpStr = fmt.Sprintf(" | breakpoints: %v", r.Breakpoints)
	}

	return fmt.Sprintf("[replay] %s | step %d/%d [%s] %.0f%% | speed: %s%s",
		r.Status, r.CurrentStep+1, total, bar, progress, speedStr, bpStr)
}

// FormatDivergences renders a human-readable summary of divergences found during replay.
func FormatDivergences(divs []Divergence) string {
	if len(divs) == 0 {
		return "No divergences detected."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d divergence(s):\n\n", len(divs)))

	for i, d := range divs {
		b.WriteString(fmt.Sprintf("--- Divergence #%d (step %d, type: %s) ---\n", i+1, d.StepIndex, d.Type))
		b.WriteString(fmt.Sprintf("  Expected: %s\n", truncate(d.Expected, 120)))
		b.WriteString(fmt.Sprintf("  Got:      %s\n", truncate(d.Got, 120)))
		b.WriteByte('\n')
	}

	return b.String()
}

// ExtractUserPrompts returns only the user messages from a slice of replay steps.
func ExtractUserPrompts(steps []ReplayStep) []string {
	prompts := make([]string, 0)
	for _, s := range steps {
		if s.Role == "user" {
			prompts = append(prompts, s.Content)
		}
	}
	return prompts
}

// findExpectedResponse locates the next assistant response after a user step.
func findExpectedResponse(steps []ReplayStep, userStepIndex int) string {
	for i := userStepIndex + 1; i < len(steps); i++ {
		if steps[i].Role == "assistant" {
			return steps[i].Content
		}
		// If we hit another user message, stop looking.
		if steps[i].Role == "user" {
			break
		}
	}
	return ""
}

// looksLikeToolCall checks if a string appears to contain a tool invocation.
func looksLikeToolCall(s string) bool {
	toolIndicators := []string{"tool_use", "tool_call", "function_call", "\"name\":", "\"tool\":"}
	lower := strings.ToLower(s)
	for _, indicator := range toolIndicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	return false
}

// extractToolName attempts to extract a tool name from a string that looks like a tool call.
func extractToolName(s string) string {
	// Try to parse as JSON and find a "name" field.
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(s), &obj); err == nil {
		if name, ok := obj["name"].(string); ok {
			return name
		}
	}

	// Simple heuristic: look for "name": "something" pattern.
	idx := strings.Index(s, `"name"`)
	if idx >= 0 {
		rest := s[idx+6:]
		// Find the value after the colon.
		colonIdx := strings.Index(rest, ":")
		if colonIdx >= 0 {
			rest = rest[colonIdx+1:]
			rest = strings.TrimSpace(rest)
			if len(rest) > 0 && rest[0] == '"' {
				end := strings.Index(rest[1:], `"`)
				if end >= 0 {
					return rest[1 : end+1]
				}
			}
		}
	}

	return ""
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
