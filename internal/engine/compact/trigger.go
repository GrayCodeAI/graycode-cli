package compact

import "time"

type CompactionTrigger struct {
	Threshold   float64
	WindowSize  int
	LastCompact time.Time
	MinInterval time.Duration
}

func NewCompactionTrigger(windowSize int) *CompactionTrigger {
	return &CompactionTrigger{
		Threshold:   0.75,
		WindowSize:  windowSize,
		MinInterval: 30 * time.Second,
	}
}

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

func (ct *CompactionTrigger) MarkCompacted() {
	ct.LastCompact = time.Now()
}
