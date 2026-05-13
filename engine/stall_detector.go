package engine

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// StallEntry represents a single recorded tool invocation in the stall detection window.
type StallEntry struct {
	ToolName   string
	ArgsHash   string
	OutputHash string
	Timestamp  time.Time
}

// StallResult describes the outcome of a stall analysis.
type StallResult struct {
	IsStalled   bool
	Level       string // "none", "soft", "hard"
	RepeatCount int
	Pattern     string
	Suggestion  string
}

// StallDetector monitors recent tool calls for repetition patterns that indicate
// the agent is stuck. Inspired by goose's repetition inspector and cline's loop detection.
type StallDetector struct {
	Window        []StallEntry
	WindowSize    int
	SoftThreshold int // repeats before a warning
	HardThreshold int // repeats before escalation
	mu            sync.Mutex
}

// NewStallDetector creates a StallDetector with sensible defaults.
func NewStallDetector() *StallDetector {
	return &StallDetector{
		Window:        make([]StallEntry, 0, 10),
		WindowSize:    10,
		SoftThreshold: 3,
		HardThreshold: 5,
	}
}

// Record adds a tool invocation to the sliding window.
func (sd *StallDetector) Record(toolName string, args map[string]interface{}, output string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	argsHash := hashArgs(args)
	outputHash := hashString(output)

	entry := StallEntry{
		ToolName:   toolName,
		ArgsHash:   argsHash,
		OutputHash: outputHash,
		Timestamp:  time.Now(),
	}

	sd.Window = append(sd.Window, entry)
	if len(sd.Window) > sd.WindowSize {
		sd.Window = sd.Window[len(sd.Window)-sd.WindowSize:]
	}
}

// Check analyzes the window for stall patterns and returns the result.
func (sd *StallDetector) Check() *StallResult {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	result := &StallResult{Level: "none"}

	// Check for same tool + same args repetition.
	if count, pattern := sd.detectRepetition(); count > 0 {
		result.RepeatCount = count
		result.Pattern = pattern
		if count >= sd.HardThreshold {
			result.IsStalled = true
			result.Level = "hard"
			result.Suggestion = "Stop and try a completely different approach, or ask the user for help."
		} else if count >= sd.SoftThreshold {
			result.IsStalled = true
			result.Level = "soft"
			result.Suggestion = "Consider trying a different approach or reading the file to check current state."
		}
		return result
	}

	// Check for oscillation (A→B→A→B).
	if sd.detectOscillation() {
		result.IsStalled = true
		result.Level = "soft"
		result.RepeatCount = 2
		result.Pattern = "oscillation: alternating between two tool calls"
		result.Suggestion = "You are alternating between two actions without progress. Try a third approach."
		return result
	}

	// Check for error loop (same output repeated).
	if sd.detectErrorLoop() {
		result.IsStalled = true
		result.Level = "soft"
		result.RepeatCount = 3
		result.Pattern = "error loop: same output repeated 3+ times"
		result.Suggestion = "The same error keeps occurring. Read the relevant file or try a fundamentally different fix."
		return result
	}

	return result
}

// DetectRepetition finds the longest run of identical (tool+args) entries in the window.
// Returns the count and a human-readable pattern description.
func (sd *StallDetector) DetectRepetition() (int, string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	return sd.detectRepetition()
}

// detectRepetition is the internal unlocked version.
func (sd *StallDetector) detectRepetition() (int, string) {
	if len(sd.Window) < 2 {
		return 0, ""
	}

	// Count consecutive identical entries from the end of the window.
	last := sd.Window[len(sd.Window)-1]
	count := 1
	for i := len(sd.Window) - 2; i >= 0; i-- {
		entry := sd.Window[i]
		if entry.ToolName == last.ToolName && entry.ArgsHash == last.ArgsHash {
			count++
		} else {
			break
		}
	}

	if count < 2 {
		// Also check for non-consecutive repeats of the same signature.
		sig := last.ToolName + ":" + last.ArgsHash
		total := 0
		for _, e := range sd.Window {
			if e.ToolName+":"+e.ArgsHash == sig {
				total++
			}
		}
		if total >= sd.SoftThreshold {
			pattern := fmt.Sprintf("%s (same args) repeated %d times in window", last.ToolName, total)
			return total, pattern
		}
		return 0, ""
	}

	pattern := fmt.Sprintf("%s (same args) repeated %d times consecutively", last.ToolName, count)
	return count, pattern
}

// DetectOscillation returns true if the window shows an A→B→A→B pattern.
func (sd *StallDetector) DetectOscillation() bool {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	return sd.detectOscillation()
}

// detectOscillation is the internal unlocked version.
func (sd *StallDetector) detectOscillation() bool {
	if len(sd.Window) < 4 {
		return false
	}

	// Check the last 4 entries for A→B→A→B pattern.
	w := sd.Window[len(sd.Window)-4:]
	sigA1 := w[0].ToolName + ":" + w[0].ArgsHash
	sigB1 := w[1].ToolName + ":" + w[1].ArgsHash
	sigA2 := w[2].ToolName + ":" + w[2].ArgsHash
	sigB2 := w[3].ToolName + ":" + w[3].ArgsHash

	// A and B must be different, and pattern must repeat.
	if sigA1 == sigB1 {
		return false
	}
	return sigA1 == sigA2 && sigB1 == sigB2
}

// DetectErrorLoop returns true if the same output hash appears 3+ times in the window.
func (sd *StallDetector) DetectErrorLoop() bool {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	return sd.detectErrorLoop()
}

// detectErrorLoop is the internal unlocked version.
func (sd *StallDetector) detectErrorLoop() bool {
	if len(sd.Window) < 3 {
		return false
	}

	counts := make(map[string]int, len(sd.Window))
	for _, entry := range sd.Window {
		counts[entry.OutputHash]++
		if counts[entry.OutputHash] >= 3 {
			return true
		}
	}
	return false
}

// BuildEscalation formats a human-readable escalation message from a StallResult.
func (sd *StallDetector) BuildEscalation(result *StallResult) string {
	if result == nil || !result.IsStalled {
		return ""
	}

	var b strings.Builder

	switch result.Level {
	case "hard":
		b.WriteString("🚨 HARD STALL DETECTED")
	case "soft":
		b.WriteString("⚠ Stall detected")
	}

	if result.RepeatCount > 0 {
		b.WriteString(fmt.Sprintf(": same tool call repeated %d times", result.RepeatCount))
	}
	b.WriteByte('\n')

	if result.Pattern != "" {
		b.WriteString("Pattern: ")
		b.WriteString(result.Pattern)
		b.WriteByte('\n')
	}

	b.WriteString("\nSuggestions:\n")
	b.WriteString("- Try a completely different approach\n")
	b.WriteString("- Read the file again to check current state\n")
	b.WriteString("- Ask the user for clarification\n")

	if result.Suggestion != "" {
		b.WriteString("- ")
		b.WriteString(result.Suggestion)
		b.WriteByte('\n')
	}

	return b.String()
}

// Reset clears the window after successful unstalling.
func (sd *StallDetector) Reset() {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.Window = sd.Window[:0]
}

// FormatWindow returns a human-readable representation of the current window for debugging.
func (sd *StallDetector) FormatWindow() string {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	if len(sd.Window) == 0 {
		return "(empty window)"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Stall Window (%d/%d entries):\n", len(sd.Window), sd.WindowSize))
	for i, entry := range sd.Window {
		b.WriteString(fmt.Sprintf("  [%d] %s args=%s out=%s @%s\n",
			i,
			entry.ToolName,
			entry.ArgsHash[:8],
			entry.OutputHash[:8],
			entry.Timestamp.Format("15:04:05"),
		))
	}
	return b.String()
}

// hashArgs deterministically hashes a map of arguments.
func hashArgs(args map[string]interface{}) string {
	if args == nil {
		return hashString("")
	}
	// Use JSON marshaling for deterministic key ordering.
	data, err := json.Marshal(args)
	if err != nil {
		// Fallback: hash the fmt representation.
		return hashString(fmt.Sprintf("%v", args))
	}
	return hashString(string(data))
}

// hashString computes a hex-encoded SHA-256 hash of a string.
func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}
