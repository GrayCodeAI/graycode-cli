package cmd

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ProgressStep represents a single step in a multi-step task.
type ProgressStep struct {
	Name     string
	Status   string // "pending", "active", "done", "failed", "skipped"
	StartTime *time.Time
	EndTime   *time.Time
	Duration  time.Duration
	Substeps  []string
	Progress  float64
	failReason string
}

// ProgressTracker tracks the progress of a multi-step operation and renders
// it for display in the TUI. Inspired by cline's focus-chain pattern.
type ProgressTracker struct {
	Steps       []ProgressStep
	CurrentStep int
	StartTime   time.Time
	Title       string
	mu          sync.Mutex
}

// NewProgressTracker creates a new ProgressTracker with the given title.
func NewProgressTracker(title string) *ProgressTracker {
	return &ProgressTracker{
		Title:       title,
		StartTime:   time.Now(),
		CurrentStep: -1,
	}
}

// AddStep appends a new step with the given name in "pending" status.
func (pt *ProgressTracker) AddStep(name string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.Steps = append(pt.Steps, ProgressStep{
		Name:   name,
		Status: "pending",
	})
}

// StartStep marks the step at the given index as "active" and records its start time.
func (pt *ProgressTracker) StartStep(index int) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if index < 0 || index >= len(pt.Steps) {
		return
	}
	now := time.Now()
	pt.Steps[index].Status = "active"
	pt.Steps[index].StartTime = &now
	pt.CurrentStep = index
}

// CompleteStep marks the step at the given index as "done" and records its end time and duration.
func (pt *ProgressTracker) CompleteStep(index int) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if index < 0 || index >= len(pt.Steps) {
		return
	}
	now := time.Now()
	pt.Steps[index].Status = "done"
	pt.Steps[index].EndTime = &now
	pt.Steps[index].Progress = 1.0
	if pt.Steps[index].StartTime != nil {
		pt.Steps[index].Duration = now.Sub(*pt.Steps[index].StartTime)
	}
}

// FailStep marks the step at the given index as "failed" with the provided reason.
func (pt *ProgressTracker) FailStep(index int, reason string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if index < 0 || index >= len(pt.Steps) {
		return
	}
	now := time.Now()
	pt.Steps[index].Status = "failed"
	pt.Steps[index].EndTime = &now
	pt.Steps[index].failReason = reason
	if pt.Steps[index].StartTime != nil {
		pt.Steps[index].Duration = now.Sub(*pt.Steps[index].StartTime)
	}
}

// SkipStep marks the step at the given index as "skipped".
func (pt *ProgressTracker) SkipStep(index int) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if index < 0 || index >= len(pt.Steps) {
		return
	}
	pt.Steps[index].Status = "skipped"
}

// SetProgress sets the progress percentage (0.0 to 1.0) for the step at the given index.
func (pt *ProgressTracker) SetProgress(index int, pct float64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if index < 0 || index >= len(pt.Steps) {
		return
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 1.0 {
		pct = 1.0
	}
	pt.Steps[index].Progress = pct
}

// AddSubstep adds a substep description to the step at the given index.
func (pt *ProgressTracker) AddSubstep(stepIndex int, substep string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if stepIndex < 0 || stepIndex >= len(pt.Steps) {
		return
	}
	pt.Steps[stepIndex].Substeps = append(pt.Steps[stepIndex].Substeps, substep)
}

// Render produces a full multi-line progress display suitable for the TUI.
func (pt *ProgressTracker) Render() string {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	var b strings.Builder

	elapsed := time.Since(pt.StartTime)
	b.WriteString(fmt.Sprintf("%s (%s)\n", pt.Title, formatDurationShort(elapsed)))

	separator := strings.Repeat("─", 40)
	b.WriteString(separator + "\n")

	for i, step := range pt.Steps {
		icon := pt.stepIcon(step.Status)
		line := fmt.Sprintf("%s %s", icon, step.Name)

		// Show duration for completed/failed steps
		if step.Status == "done" || step.Status == "failed" {
			line += fmt.Sprintf(" (%s)", formatDurationShort(step.Duration))
		}
		// Show elapsed time and progress for active steps
		if step.Status == "active" && step.StartTime != nil {
			stepElapsed := time.Since(*step.StartTime)
			line += fmt.Sprintf(" (%s)", formatDurationShort(stepElapsed))
		}
		b.WriteString(line + "\n")

		// Render substeps for the active step
		if step.Status == "active" && len(step.Substeps) > 0 {
			for j, sub := range step.Substeps {
				if j < len(step.Substeps)-1 {
					b.WriteString(fmt.Sprintf("  ├─ %s\n", sub))
				} else {
					// Last substep: show progress if set
					if step.Progress > 0 && step.Progress < 1.0 {
						b.WriteString(fmt.Sprintf("  └─ %s... (%d%%)\n", sub, int(step.Progress*100)))
					} else {
						b.WriteString(fmt.Sprintf("  └─ %s\n", sub))
					}
				}
			}
		}

		// Show fail reason
		if step.Status == "failed" && step.failReason != "" {
			b.WriteString(fmt.Sprintf("     └─ %s\n", step.failReason))
		}

		_ = i
	}

	b.WriteString(separator + "\n")

	// Overall progress bar
	overallPct := pt.overallProgress()
	barWidth := 20
	filled := int(overallPct * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	b.WriteString(fmt.Sprintf("Progress: [%s] %d%%\n", bar, int(overallPct*100)))

	// ETA
	remaining := pt.estimateRemainingLocked()
	if remaining > 0 {
		b.WriteString(fmt.Sprintf("ETA: ~%s remaining\n", formatDurationShort(remaining)))
	}

	return b.String()
}

// RenderCompact produces a single-line progress summary.
func (pt *ProgressTracker) RenderCompact() string {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	total := len(pt.Steps)
	done := 0
	for _, s := range pt.Steps {
		if s.Status == "done" || s.Status == "failed" || s.Status == "skipped" {
			done++
		}
	}

	var currentName string
	var currentPct float64
	if pt.CurrentStep >= 0 && pt.CurrentStep < total {
		currentName = pt.Steps[pt.CurrentStep].Name
		currentPct = pt.Steps[pt.CurrentStep].Progress
	}

	remaining := pt.estimateRemainingLocked()
	pctStr := ""
	if currentPct > 0 {
		pctStr = fmt.Sprintf("%d%%", int(currentPct*100))
	}

	etaStr := ""
	if remaining > 0 {
		etaStr = fmt.Sprintf("ETA %s", formatDurationShort(remaining))
	}

	details := ""
	if pctStr != "" && etaStr != "" {
		details = fmt.Sprintf(" (%s, %s)", pctStr, etaStr)
	} else if pctStr != "" {
		details = fmt.Sprintf(" (%s)", pctStr)
	} else if etaStr != "" {
		details = fmt.Sprintf(" (%s)", etaStr)
	}

	return fmt.Sprintf("[%d/%d] %s...%s", done, total, currentName, details)
}

// RenderDone produces a completion summary.
func (pt *ProgressTracker) RenderDone() string {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	elapsed := time.Since(pt.StartTime)
	failures := 0
	for _, s := range pt.Steps {
		if s.Status == "failed" {
			failures++
		}
	}

	return fmt.Sprintf("✓ %s complete (%s)\n  %d steps, %d failures",
		pt.Title, formatDurationShort(elapsed), len(pt.Steps), failures)
}

// IsComplete returns true if all steps have finished (done, failed, or skipped).
func (pt *ProgressTracker) IsComplete() bool {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if len(pt.Steps) == 0 {
		return false
	}
	for _, s := range pt.Steps {
		if s.Status == "pending" || s.Status == "active" {
			return false
		}
	}
	return true
}

// GetElapsed returns the time elapsed since the tracker was created.
func (pt *ProgressTracker) GetElapsed() time.Duration {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return time.Since(pt.StartTime)
}

// EstimateRemaining estimates the remaining time based on average step duration.
func (pt *ProgressTracker) EstimateRemaining() time.Duration {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return pt.estimateRemainingLocked()
}

// estimateRemainingLocked calculates estimated remaining time. Must be called with mu held.
func (pt *ProgressTracker) estimateRemainingLocked() time.Duration {
	var completedDuration time.Duration
	completedCount := 0
	pendingCount := 0
	var activeRemaining time.Duration

	for _, s := range pt.Steps {
		switch s.Status {
		case "done", "failed":
			completedDuration += s.Duration
			completedCount++
		case "pending":
			pendingCount++
		case "active":
			// Estimate remaining time for active step based on progress
			if s.Progress > 0 && s.StartTime != nil {
				elapsed := time.Since(*s.StartTime)
				estimated := time.Duration(float64(elapsed) / s.Progress)
				activeRemaining = estimated - elapsed
				if activeRemaining < 0 {
					activeRemaining = 0
				}
			}
		}
	}

	if completedCount == 0 {
		return activeRemaining
	}

	avgDuration := completedDuration / time.Duration(completedCount)
	return activeRemaining + avgDuration*time.Duration(pendingCount)
}

// overallProgress calculates the overall progress as a fraction (0.0 to 1.0). Must be called with mu held.
func (pt *ProgressTracker) overallProgress() float64 {
	total := len(pt.Steps)
	if total == 0 {
		return 0
	}

	var progress float64
	for _, s := range pt.Steps {
		switch s.Status {
		case "done", "skipped":
			progress += 1.0
		case "failed":
			progress += 1.0
		case "active":
			progress += s.Progress
		}
	}
	return progress / float64(total)
}

// stepIcon returns the display icon for a step based on its status.
func (pt *ProgressTracker) stepIcon(status string) string {
	switch status {
	case "done":
		return "✓"
	case "active":
		return "●"
	case "failed":
		return "✗"
	case "skipped":
		return "—"
	default: // pending
		return "○"
	}
}

// formatDurationShort formats a duration in a human-friendly short form.
func formatDurationShort(d time.Duration) string {
	if d < time.Second {
		ms := d.Milliseconds()
		if ms == 0 {
			return "0s"
		}
		return fmt.Sprintf("%dms", ms)
	}
	if d < time.Minute {
		secs := d.Seconds()
		if secs == float64(int(secs)) {
			return fmt.Sprintf("%ds", int(secs))
		}
		return fmt.Sprintf("%.1fs", secs)
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		if secs == 0 {
			return fmt.Sprintf("%dm", mins)
		}
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}
