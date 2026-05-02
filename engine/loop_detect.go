package engine

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// LoopDetector detects repeated identical tool call patterns using SHA-256 signatures.
type LoopDetector struct {
	windowSize int
	maxRepeats int
	signatures []string
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

// LoopWarning returns the message to inject when a loop is detected.
func (ld *LoopDetector) LoopWarning() string {
	return "You appear to be stuck in a loop — the same tool calls are producing the same results repeatedly. Try a different approach, ask the user for clarification, or break the problem into smaller steps."
}
