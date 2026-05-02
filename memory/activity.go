package memory

import (
	"fmt"
	"sync"
	"time"
)

// ActivityTracker monitors memory save activity and nudges the agent
// to persist learnings when saves have been idle for too long.
type ActivityTracker struct {
	mu           sync.Mutex
	lastSaveTime time.Time
	saveCount    int
	nudgeAfter   time.Duration
}

// NewActivityTracker creates a tracker that nudges after the given duration of inactivity.
func NewActivityTracker(nudgeAfter time.Duration) *ActivityTracker {
	if nudgeAfter <= 0 {
		nudgeAfter = 10 * time.Minute
	}
	return &ActivityTracker{nudgeAfter: nudgeAfter}
}

// RecordSave records that a memory save just occurred.
func (a *ActivityTracker) RecordSave() {
	a.mu.Lock()
	a.lastSaveTime = time.Now()
	a.saveCount++
	a.mu.Unlock()
}

// NudgeMessage returns a nudge string if the agent hasn't saved memories recently,
// or "" if no nudge is needed.
func (a *ActivityTracker) NudgeMessage() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.lastSaveTime.IsZero() {
		// No saves yet — nudge after first nudgeAfter period
		return ""
	}
	if time.Since(a.lastSaveTime) < a.nudgeAfter {
		return ""
	}
	return fmt.Sprintf("[Memory nudge: %d memories saved this session, last save %s ago. Consider persisting new learnings.]",
		a.saveCount, time.Since(a.lastSaveTime).Truncate(time.Second))
}
