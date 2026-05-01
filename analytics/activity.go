package analytics

import (
	"fmt"
	"sync"
	"time"
)

// ActivityTracker tracks user typing vs CLI/agent execution time to provide a
// ratio of how much time the user spends vs how much the agent works.
type ActivityTracker struct {
	mu sync.Mutex

	userTotal time.Duration
	execTotal time.Duration

	userStart *time.Time
	execStart *time.Time
}

// NewActivityTracker creates a new ActivityTracker.
func NewActivityTracker() *ActivityTracker {
	return &ActivityTracker{}
}

// StartUserInput marks the beginning of user input time.
func (a *ActivityTracker) StartUserInput() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// End any active execution period
	if a.execStart != nil {
		a.execTotal += time.Since(*a.execStart)
		a.execStart = nil
	}

	if a.userStart == nil {
		now := time.Now()
		a.userStart = &now
	}
}

// EndUserInput marks the end of user input time.
func (a *ActivityTracker) EndUserInput() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.userStart != nil {
		a.userTotal += time.Since(*a.userStart)
		a.userStart = nil
	}
}

// StartExecution marks the beginning of an agent/tool execution period.
func (a *ActivityTracker) StartExecution() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// End any active user input period
	if a.userStart != nil {
		a.userTotal += time.Since(*a.userStart)
		a.userStart = nil
	}

	if a.execStart == nil {
		now := time.Now()
		a.execStart = &now
	}
}

// EndExecution marks the end of an agent/tool execution period.
func (a *ActivityTracker) EndExecution() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.execStart != nil {
		a.execTotal += time.Since(*a.execStart)
		a.execStart = nil
	}
}

// UserTime returns the accumulated user input duration.
func (a *ActivityTracker) UserTime() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()

	total := a.userTotal
	if a.userStart != nil {
		total += time.Since(*a.userStart)
	}
	return total
}

// ExecTime returns the accumulated execution duration.
func (a *ActivityTracker) ExecTime() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()

	total := a.execTotal
	if a.execStart != nil {
		total += time.Since(*a.execStart)
	}
	return total
}

// Summary returns a formatted summary: "User: 5m30s | Agent: 12m15s | Ratio: 2.2x"
func (a *ActivityTracker) Summary() string {
	userTime := a.UserTime()
	execTime := a.ExecTime()

	ratio := 0.0
	if userTime > 0 {
		ratio = float64(execTime) / float64(userTime)
	}

	return fmt.Sprintf("User: %s | Agent: %s | Ratio: %.1fx",
		formatDurationCompact(userTime),
		formatDurationCompact(execTime),
		ratio,
	)
}

// formatDurationCompact formats a duration in a compact human-friendly form.
func formatDurationCompact(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) - m*60
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) - h*60
	return fmt.Sprintf("%dh%dm", h, m)
}
