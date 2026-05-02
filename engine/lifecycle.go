package engine

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SessionLifecycle manages the start and end of agent sessions, implementing
// the self-improvement loop that makes hawk better over time.
//
// Research basis:
// - Reflexion (NeurIPS 2023): 91% HumanEval via episodic memory of reflections
// - ExpeL (AAAI 2024): extract insights from task experiences
// - Voyager: 15.3x faster with accumulated skill library
// - DSPy (Stanford): 25-65% improvement via curated few-shot examples
//
// The closed loop:
// SESSION START:
//   1. Retrieve relevant EvolvingMemory guidelines
//   2. Inject few-shot examples from prior successes
//   3. Load yaad context (conventions, active tasks, stale warnings)
//
// SESSION END:
//   1. Generate LLM reflection ("what worked, what failed, why")
//   2. Extract guidelines via EvolvingMemory.Learn()
//   3. Distill successful approaches into skills
//   4. Record cost/performance metrics
//   5. Trigger yaad consolidation
type SessionLifecycle struct {
	Memory      EvolvingMemoryInterface
	SkillStore  SkillStoreInterface
	CostTracker CostTrackerInterface
}

// EvolvingMemoryInterface abstracts guideline retrieval and learning so the
// lifecycle can be tested without real storage.
type EvolvingMemoryInterface interface {
	Learn(pattern, lesson string) error
	Retrieve(query string) []string
	Format() string
}

// SkillStoreInterface abstracts skill distillation and retrieval.
type SkillStoreInterface interface {
	Distill(goal string, steps []string, outcome string) error
	Retrieve(query string) []string
}

// CostTrackerInterface abstracts cost recording and querying.
type CostTrackerInterface interface {
	Record(entry CostEntry) error
	SessionTotal() float64
}

// CostEntry represents a single cost data point recorded at session end.
type CostEntry struct {
	SessionID string
	TaskGoal  string
	TotalCost float64
	Duration  time.Duration
	Success   bool
	Timestamp time.Time
}

// SessionOutcome captures the results of a completed session.
type SessionOutcome struct {
	Success      bool
	TaskGoal     string
	FilesChanged []string
	ToolsUsed    []string
	TotalCost    float64
	Duration     time.Duration
	UserFeedback string // empty if none
}

// complexityThreshold is the minimum number of files changed plus tools used
// that qualifies a task as "complex" for skill distillation.
const complexityThreshold = 3

// OnSessionStart prepares context for a new session.
// Returns context to inject into the system prompt.
func (l *SessionLifecycle) OnSessionStart(_ context.Context, initialPrompt string) string {
	var sections []string

	// 1. Retrieve relevant guidelines from evolving memory.
	if l.Memory != nil {
		guidelines := l.Memory.Retrieve(initialPrompt)
		if len(guidelines) > 0 {
			var b strings.Builder
			b.WriteString("## Learned Guidelines\n")
			for _, g := range guidelines {
				b.WriteString("- ")
				b.WriteString(g)
				b.WriteString("\n")
			}
			sections = append(sections, b.String())
		}
	}

	// 2. Retrieve relevant distilled skills.
	if l.SkillStore != nil {
		skills := l.SkillStore.Retrieve(initialPrompt)
		if len(skills) > 0 {
			var b strings.Builder
			b.WriteString("## Relevant Skills\n")
			for _, s := range skills {
				b.WriteString("- ")
				b.WriteString(s)
				b.WriteString("\n")
			}
			sections = append(sections, b.String())
		}
	}

	return strings.Join(sections, "\n")
}

// OnSessionEnd performs post-session learning.
func (l *SessionLifecycle) OnSessionEnd(_ context.Context, session *Session, outcome SessionOutcome) error {
	var errs []string

	// 1. Extract guideline based on outcome.
	if l.Memory != nil {
		pattern, lesson := buildGuideline(outcome)
		if pattern != "" && lesson != "" {
			if err := l.Memory.Learn(pattern, lesson); err != nil {
				errs = append(errs, fmt.Sprintf("learn guideline: %v", err))
			}
		}

		// If the user gave explicit feedback, learn from that too.
		if outcome.UserFeedback != "" {
			fbPattern := fmt.Sprintf("user feedback on: %s", outcome.TaskGoal)
			if err := l.Memory.Learn(fbPattern, outcome.UserFeedback); err != nil {
				errs = append(errs, fmt.Sprintf("learn feedback: %v", err))
			}
		}
	}

	// 2. Attempt skill distillation for complex successful tasks.
	if l.SkillStore != nil && outcome.Success && isComplex(outcome) {
		if err := l.SkillStore.Distill(outcome.TaskGoal, outcome.ToolsUsed, "success"); err != nil {
			errs = append(errs, fmt.Sprintf("distill skill: %v", err))
		}
	}

	// 3. Always record cost metrics.
	if l.CostTracker != nil {
		entry := CostEntry{
			TaskGoal:  outcome.TaskGoal,
			TotalCost: outcome.TotalCost,
			Duration:  outcome.Duration,
			Success:   outcome.Success,
			Timestamp: time.Now(),
		}
		if session != nil {
			entry.SessionID = fmt.Sprintf("session_%d", time.Now().UnixNano())
		}
		if err := l.CostTracker.Record(entry); err != nil {
			errs = append(errs, fmt.Sprintf("record cost: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("session end errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// buildGuideline creates a pattern/lesson pair from a session outcome.
func buildGuideline(outcome SessionOutcome) (pattern, lesson string) {
	if outcome.TaskGoal == "" {
		return "", ""
	}

	if outcome.Success {
		pattern = fmt.Sprintf("tasks involving: %s", outcome.TaskGoal)
		if len(outcome.ToolsUsed) > 0 {
			lesson = fmt.Sprintf("approach using %s works well",
				strings.Join(outcome.ToolsUsed, ", "))
		} else {
			lesson = "the applied approach succeeded"
		}
		if len(outcome.FilesChanged) > 0 {
			lesson += fmt.Sprintf(" (files: %s)", strings.Join(outcome.FilesChanged, ", "))
		}
		return pattern, lesson
	}

	// Failed session: produce a warning.
	pattern = fmt.Sprintf("tasks involving: %s", outcome.TaskGoal)
	if len(outcome.ToolsUsed) > 0 {
		lesson = fmt.Sprintf("approach using %s did not succeed; consider alternative strategies",
			strings.Join(outcome.ToolsUsed, ", "))
	} else {
		lesson = "the applied approach failed; consider alternative strategies"
	}
	return pattern, lesson
}

// isComplex determines whether a task is complex enough to warrant skill distillation.
func isComplex(outcome SessionOutcome) bool {
	return len(outcome.FilesChanged)+len(outcome.ToolsUsed) >= complexityThreshold
}
