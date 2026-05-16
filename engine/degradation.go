package engine

import (
	"sync"
	"time"
)

// DegradationSignal is a type of quality drop indicator.
type DegradationSignal int

const (
	SignalLooping       DegradationSignal = iota // repeated identical tool calls
	SignalErrorSpike                             // 3+ consecutive failures
	SignalContextDrift                           // referencing non-existent files
	SignalNoProgress                             // many turns without meaningful change
)

// DegradationDetector monitors agent quality over turns and triggers recovery.
type DegradationDetector struct {
	mu              sync.Mutex
	turnCount       int
	errorCount      int
	consecutiveErrs int
	lastToolCalls   []string
	maxTurns        int
	detected        bool
	signal          DegradationSignal
}

// NewDegradationDetector creates a detector with default thresholds.
func NewDegradationDetector(maxTurns int) *DegradationDetector {
	if maxTurns <= 0 {
		maxTurns = 25
	}
	return &DegradationDetector{maxTurns: maxTurns}
}

// RecordTurn logs a turn and checks for degradation.
func (dd *DegradationDetector) RecordTurn(toolName string, success bool) {
	dd.mu.Lock()
	defer dd.mu.Unlock()
	dd.turnCount++

	if !success {
		dd.errorCount++
		dd.consecutiveErrs++
	} else {
		dd.consecutiveErrs = 0
	}

	// Track last 5 tool calls for loop detection
	dd.lastToolCalls = append(dd.lastToolCalls, toolName)
	if len(dd.lastToolCalls) > 5 {
		dd.lastToolCalls = dd.lastToolCalls[len(dd.lastToolCalls)-5:]
	}

	dd.checkDegradation()
}

// IsDegraded returns whether quality has dropped.
func (dd *DegradationDetector) IsDegraded() bool {
	dd.mu.Lock()
	defer dd.mu.Unlock()
	return dd.detected
}

// Signal returns the type of degradation detected.
func (dd *DegradationDetector) Signal() DegradationSignal {
	dd.mu.Lock()
	defer dd.mu.Unlock()
	return dd.signal
}

// RecoveryAction returns what to do when degradation is detected.
func (dd *DegradationDetector) RecoveryAction() string {
	dd.mu.Lock()
	defer dd.mu.Unlock()
	switch dd.signal {
	case SignalLooping:
		return "Agent is looping. Compacting context and trying a different approach."
	case SignalErrorSpike:
		return "Multiple consecutive errors. Pausing to reassess the approach."
	case SignalNoProgress:
		return "No progress after many turns. Breaking task into smaller subtasks."
	default:
		return "Quality degradation detected. Refreshing context."
	}
}

// Reset clears the detector state (e.g., after recovery or new task).
func (dd *DegradationDetector) Reset() {
	dd.mu.Lock()
	defer dd.mu.Unlock()
	dd.turnCount = 0
	dd.errorCount = 0
	dd.consecutiveErrs = 0
	dd.lastToolCalls = nil
	dd.detected = false
}

// Stats returns current metrics.
func (dd *DegradationDetector) Stats() (turns, errors, consecutive int) {
	dd.mu.Lock()
	defer dd.mu.Unlock()
	return dd.turnCount, dd.errorCount, dd.consecutiveErrs
}

func (dd *DegradationDetector) checkDegradation() {
	// Signal: looping (same tool called 4+ times in last 5)
	if len(dd.lastToolCalls) >= 4 {
		counts := make(map[string]int)
		for _, tc := range dd.lastToolCalls {
			counts[tc]++
		}
		for _, c := range counts {
			if c >= 4 {
				dd.detected = true
				dd.signal = SignalLooping
				return
			}
		}
	}

	// Signal: error spike (3+ consecutive)
	if dd.consecutiveErrs >= 3 {
		dd.detected = true
		dd.signal = SignalErrorSpike
		return
	}

	// Signal: no progress (exceeded max turns)
	if dd.turnCount >= dd.maxTurns {
		dd.detected = true
		dd.signal = SignalNoProgress
		return
	}
}

// DegradationTimeout returns a suggested timeout based on turn count.
func DegradationTimeout(turnCount int) time.Duration {
	// Increase timeout as turns increase (agent may need more time for complex tasks)
	base := 30 * time.Second
	if turnCount > 10 {
		base = 60 * time.Second
	}
	if turnCount > 20 {
		base = 90 * time.Second
	}
	return base
}
