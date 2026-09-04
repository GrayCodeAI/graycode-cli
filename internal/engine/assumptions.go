package engine

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

// AssumptionStatus tracks whether an assumption has been verified.
type AssumptionStatus int

const (
	AssumptionUnverified AssumptionStatus = iota
	AssumptionConfirmed
	AssumptionFailed
)

// Assumption is a single assumption the agent is making.
type Assumption struct {
	Text   string
	Status AssumptionStatus
	Proof  string // evidence for/against
}

// AssumptionTracker logs and verifies agent assumptions.
type AssumptionTracker struct {
	mu          sync.Mutex
	Assumptions []Assumption
}

// NewAssumptionTracker creates a tracker.
func NewAssumptionTracker() *AssumptionTracker {
	return &AssumptionTracker{}
}

// Add logs a new assumption.
func (at *AssumptionTracker) Add(text string) {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.Assumptions = append(at.Assumptions, Assumption{Text: text, Status: AssumptionUnverified})
}

// VerifyFileExists checks if a file assumption is correct.
func (at *AssumptionTracker) VerifyFileExists(text, path string) {
	at.mu.Lock()
	defer at.mu.Unlock()
	a := Assumption{Text: text}
	if _, err := os.Stat(path); err == nil {
		a.Status = AssumptionConfirmed
		a.Proof = path + " exists"
	} else {
		a.Status = AssumptionFailed
		a.Proof = path + " NOT found"
	}
	at.Assumptions = append(at.Assumptions, a)
}

// VerifyCommandSucceeds was removed: it ran caller-supplied strings through
// `sh -c`, bypassing the permission/safety stack, and had no callers.

// Failed returns all assumptions that were proven wrong.
func (at *AssumptionTracker) Failed() []Assumption {
	at.mu.Lock()
	defer at.mu.Unlock()
	var failed []Assumption
	for _, a := range at.Assumptions {
		if a.Status == AssumptionFailed {
			failed = append(failed, a)
		}
	}
	return failed
}

// Summary returns a formatted summary of all assumptions.
func (at *AssumptionTracker) Summary() string {
	at.mu.Lock()
	defer at.mu.Unlock()
	if len(at.Assumptions) == 0 {
		return "No assumptions tracked."
	}
	var sb strings.Builder
	for _, a := range at.Assumptions {
		icon := icons.Question()
		switch a.Status {
		case AssumptionConfirmed:
			icon = icons.CheckBold()
		case AssumptionFailed:
			icon = icons.CloseThick()
		}
		sb.WriteString(fmt.Sprintf("  %s %s", icon, a.Text))
		if a.Proof != "" {
			sb.WriteString(" — " + a.Proof)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// Reset clears all assumptions (e.g., for new task).
func (at *AssumptionTracker) Reset() {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.Assumptions = nil
}
