package engine

import "time"

// CompactionTrigger monitors token usage and triggers compaction proactively.
type CompactionTrigger struct {
	Threshold    float64 // trigger at this % of context window (e.g. 0.8 = 80%)
	WindowSize   int     // total context window tokens
	LastCompact  time.Time
	MinInterval  time.Duration // don't compact more often than this
}

// NewCompactionTrigger creates a trigger with sensible defaults for solo dev use.
func NewCompactionTrigger(windowSize int) *CompactionTrigger {
	return &CompactionTrigger{
		Threshold:   0.75, // compact at 75% full
		WindowSize:  windowSize,
		MinInterval: 30 * time.Second,
	}
}

// ShouldCompact returns true if current token usage warrants compaction.
func (ct *CompactionTrigger) ShouldCompact(currentTokens int) bool {
	if ct.WindowSize <= 0 {
		return false
	}
	if time.Since(ct.LastCompact) < ct.MinInterval {
		return false
	}
	usage := float64(currentTokens) / float64(ct.WindowSize)
	return usage >= ct.Threshold
}

// MarkCompacted records that compaction just happened.
func (ct *CompactionTrigger) MarkCompacted() {
	ct.LastCompact = time.Now()
}
