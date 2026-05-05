package engine

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// DoomLoopThreshold is the number of repeated patterns before escalation (matches OpenCode).
const DoomLoopThreshold = 3

// LoopDetector detects repeated identical tool call patterns using SHA-256 signatures.
// Supports two severity levels: warning (soft) and doom loop (hard escalation).
type LoopDetector struct {
	windowSize    int
	maxRepeats    int
	signatures    []string
	escalated     bool
	escalateCount int
}

// NewLoopDetector creates a detector with a sliding window.
func NewLoopDetector(windowSize, maxRepeats int) *LoopDetector {
	return &LoopDetector{windowSize: windowSize, maxRepeats: maxRepeats}
}

// RecordStep hashes the tool calls and results from a single agent step.
func (ld *LoopDetector) RecordStep(toolNames []string, inputs []string, outputs []string) {
	var b strings.Builder
	for i := range toolNames {
		b.WriteString(toolNames[i])
		b.WriteByte(0)
		if i < len(inputs) {
			b.WriteString(inputs[i])
		}
		b.WriteByte(0)
		if i < len(outputs) {
			b.WriteString(outputs[i])
		}
		b.WriteByte(0)
	}
	sig := fmt.Sprintf("%x", sha256.Sum256([]byte(b.String())))
	ld.signatures = append(ld.signatures, sig)
	if len(ld.signatures) > ld.windowSize {
		ld.signatures = ld.signatures[len(ld.signatures)-ld.windowSize:]
	}
}

// IsLooping returns true if any signature appears more than maxRepeats times in the window.
func (ld *LoopDetector) IsLooping() bool {
	counts := make(map[string]int, len(ld.signatures))
	for _, sig := range ld.signatures {
		counts[sig]++
		if counts[sig] >= ld.maxRepeats {
			return true
		}
	}
	return false
}

// IsDoomLoop returns true when the agent has been stuck for DoomLoopThreshold
// consecutive escalation attempts — it should hard-stop and ask the user.
func (ld *LoopDetector) IsDoomLoop() bool {
	if !ld.IsLooping() {
		ld.escalateCount = 0
		return false
	}
	ld.escalateCount++
	return ld.escalateCount >= DoomLoopThreshold
}

// Escalated returns whether the detector has already fired a warning.
func (ld *LoopDetector) Escalated() bool { return ld.escalated }

// MarkEscalated records that a warning was shown.
func (ld *LoopDetector) MarkEscalated() { ld.escalated = true }

// Reset clears the escalation state (e.g., after user provides new direction).
func (ld *LoopDetector) Reset() {
	ld.signatures = nil
	ld.escalated = false
	ld.escalateCount = 0
}

// LoopWarning returns the soft warning message (first detection).
func (ld *LoopDetector) LoopWarning() string {
	return "You appear to be stuck in a loop — the same tool calls are producing the same results repeatedly. Try a different approach, ask the user for clarification, or break the problem into smaller steps."
}

// DoomLoopWarning returns the hard escalation message (repeated loops).
func (ld *LoopDetector) DoomLoopWarning() string {
	return "DOOM LOOP DETECTED: You have been stuck repeating the same actions " +
		fmt.Sprintf("%d times", ld.escalateCount) +
		". STOP and ask the user for help. Do not attempt the same approach again. " +
		"Explain what you tried, why it failed, and ask for direction."
}
