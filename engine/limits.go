package engine

import (
	"fmt"
	"strings"
	"sync"
)

// SafetyLimits caps what the agent can do in a single session.
type SafetyLimits struct {
	MaxToolCalls    int     // max total tool invocations (default: 200)
	MaxFileWrites   int     // max files created/modified (default: 50)
	MaxBashCommands int     // max bash executions (default: 100)
	MaxCostUSD      float64 // max spend (default: from MaxBudgetUSD)
	MaxTurns        int     // max LLM turns (default: from MaxTurns)
	MaxOutputTokens int     // max total output tokens (default: 500K)
}

// LimitTracker tracks usage against limits.
type LimitTracker struct {
	mu         sync.Mutex
	limits     SafetyLimits
	toolCalls  int
	fileWrites int
	bashCmds   int
	costUSD    float64
	turns      int
	outTokens  int
}

// NewLimitTracker creates a LimitTracker with the given limits.
func NewLimitTracker(limits SafetyLimits) *LimitTracker {
	return &LimitTracker{limits: limits}
}

// RecordToolCall records a tool invocation. Tools named "Bash" or "bash" also
// increment the bash command counter; "Write" or "Edit" increment file writes.
func (lt *LimitTracker) RecordToolCall(toolName string) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.toolCalls++
	switch toolName {
	case "Bash", "bash":
		lt.bashCmds++
	case "Write", "Edit", "file_write", "file_edit":
		lt.fileWrites++
	}
}

// RecordCost adds to the running cost total.
func (lt *LimitTracker) RecordCost(usd float64) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.costUSD += usd
}

// RecordTokens adds output tokens to the running total.
func (lt *LimitTracker) RecordTokens(n int) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.outTokens += n
}

// RecordTurn increments the turn counter.
func (lt *LimitTracker) RecordTurn() {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.turns++
}

// IsExceeded returns true and a human-readable reason when any limit is breached.
func (lt *LimitTracker) IsExceeded() (bool, string) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if lt.limits.MaxToolCalls > 0 && lt.toolCalls >= lt.limits.MaxToolCalls {
		return true, fmt.Sprintf("tool call limit reached (%d/%d)", lt.toolCalls, lt.limits.MaxToolCalls)
	}
	if lt.limits.MaxFileWrites > 0 && lt.fileWrites >= lt.limits.MaxFileWrites {
		return true, fmt.Sprintf("file write limit reached (%d/%d)", lt.fileWrites, lt.limits.MaxFileWrites)
	}
	if lt.limits.MaxBashCommands > 0 && lt.bashCmds >= lt.limits.MaxBashCommands {
		return true, fmt.Sprintf("bash command limit reached (%d/%d)", lt.bashCmds, lt.limits.MaxBashCommands)
	}
	if lt.limits.MaxCostUSD > 0 && lt.costUSD >= lt.limits.MaxCostUSD {
		return true, fmt.Sprintf("cost limit reached ($%.2f/$%.2f)", lt.costUSD, lt.limits.MaxCostUSD)
	}
	if lt.limits.MaxTurns > 0 && lt.turns >= lt.limits.MaxTurns {
		return true, fmt.Sprintf("turn limit reached (%d/%d)", lt.turns, lt.limits.MaxTurns)
	}
	if lt.limits.MaxOutputTokens > 0 && lt.outTokens >= lt.limits.MaxOutputTokens {
		return true, fmt.Sprintf("output token limit reached (%d/%d)", lt.outTokens, lt.limits.MaxOutputTokens)
	}
	return false, ""
}

// Summary returns a one-line summary of usage vs limits.
func (lt *LimitTracker) Summary() string {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	var parts []string
	if lt.limits.MaxToolCalls > 0 {
		parts = append(parts, fmt.Sprintf("Tools: %d/%d", lt.toolCalls, lt.limits.MaxToolCalls))
	}
	if lt.limits.MaxFileWrites > 0 {
		parts = append(parts, fmt.Sprintf("Files: %d/%d", lt.fileWrites, lt.limits.MaxFileWrites))
	}
	if lt.limits.MaxBashCommands > 0 {
		parts = append(parts, fmt.Sprintf("Bash: %d/%d", lt.bashCmds, lt.limits.MaxBashCommands))
	}
	if lt.limits.MaxCostUSD > 0 {
		parts = append(parts, fmt.Sprintf("Cost: $%.2f/$%.2f", lt.costUSD, lt.limits.MaxCostUSD))
	}
	if lt.limits.MaxTurns > 0 {
		parts = append(parts, fmt.Sprintf("Turns: %d/%d", lt.turns, lt.limits.MaxTurns))
	}
	if lt.limits.MaxOutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("Tokens: %d/%d", lt.outTokens, lt.limits.MaxOutputTokens))
	}
	if len(parts) == 0 {
		return "no limits configured"
	}
	return strings.Join(parts, " | ")
}

// DefaultLimits returns conservative safety limits for normal interactive use.
func DefaultLimits() SafetyLimits {
	return SafetyLimits{
		MaxToolCalls:    200,
		MaxFileWrites:   50,
		MaxBashCommands: 100,
		MaxCostUSD:      0, // inherit from MaxBudgetUSD
		MaxTurns:        0, // inherit from MaxTurns
		MaxOutputTokens: 500_000,
	}
}

// VibeLimits returns more permissive limits for autonomous/vibe mode.
func VibeLimits() SafetyLimits {
	return SafetyLimits{
		MaxToolCalls:    500,
		MaxFileWrites:   150,
		MaxBashCommands: 300,
		MaxCostUSD:      5.0,
		MaxTurns:        0,
		MaxOutputTokens: 1_000_000,
	}
}

// ResearchLimits returns strict per-iteration limits suitable for research tasks.
func ResearchLimits() SafetyLimits {
	return SafetyLimits{
		MaxToolCalls:    50,
		MaxFileWrites:   10,
		MaxBashCommands: 25,
		MaxCostUSD:      0.50,
		MaxTurns:        20,
		MaxOutputTokens: 100_000,
	}
}
