package planning

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// GoalStatus represents the current state of a goal.
type GoalStatus string

const (
	GoalPending    GoalStatus = "pending"
	GoalInProgress GoalStatus = "in_progress"
	GoalCompleted  GoalStatus = "completed"
	GoalBlocked    GoalStatus = "blocked"
	GoalFailed     GoalStatus = "failed"
)

// Goal represents a tracked objective within a session.
type Goal struct {
	ID           string
	Description  string
	Status       GoalStatus
	SubGoals     []Goal
	TokenBudget  int
	TokensUsed   int
	CreatedAt    time.Time
	CompletedAt  *time.Time
	Priority     int // 1=highest, 5=lowest
	Dependencies []string
	Tags         []string
	Progress     float64 // 0.0 - 1.0
	ParentID     string  // empty if top-level
}

// GoalEvent records a state change in the goal lifecycle.
type GoalEvent struct {
	GoalID    string
	EventType string // "created", "started", "completed", "failed", "blocked", "progress"
	Message   string
	Timestamp time.Time
}

// GoalTracker manages the lifecycle and scheduling of goals.
type GoalTracker struct {
	Goals      map[string]*Goal
	ActiveGoal *Goal
	mu         sync.RWMutex
	History    []GoalEvent
}

// GoalOption is a functional option for configuring a new goal.
type GoalOption func(*Goal)

// WithPriority sets the goal's priority (1=highest, 5=lowest).
func WithPriority(p int) GoalOption {
	return func(g *Goal) {
		g.Priority = p
	}
}

// WithBudget sets the maximum token budget for the goal.
func WithBudget(tokens int) GoalOption {
	return func(g *Goal) {
		g.TokenBudget = tokens
	}
}

// WithDependencies sets the IDs of goals that must complete before this one.
func WithDependencies(deps ...string) GoalOption {
	return func(g *Goal) {
		g.Dependencies = deps
	}
}

// WithTags assigns tags to the goal for filtering/categorization.
func WithTags(tags ...string) GoalOption {
	return func(g *Goal) {
		g.Tags = tags
	}
}

// NewGoalTracker creates and returns an initialized GoalTracker.
func NewGoalTracker() *GoalTracker {
	return &GoalTracker{
		Goals:   make(map[string]*Goal),
		History: make([]GoalEvent, 0),
	}
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "goal-" + hex.EncodeToString(b)
}

// AddGoal creates a new goal with an auto-generated ID and applies options.
func (gt *GoalTracker) AddGoal(description string, opts ...GoalOption) *Goal {
	gt.mu.Lock()
	defer gt.mu.Unlock()

	g := &Goal{
		ID:          generateID(),
		Description: description,
		Status:      GoalPending,
		Priority:    3, // default medium priority
		CreatedAt:   time.Now(),
		SubGoals:    make([]Goal, 0),
		Tags:        make([]string, 0),
	}

	for _, opt := range opts {
		opt(g)
	}

	gt.Goals[g.ID] = g
	gt.History = append(gt.History, GoalEvent{
		GoalID:    g.ID,
		EventType: "created",
		Message:   fmt.Sprintf("Goal created: %s", description),
		Timestamp: time.Now(),
	})

	return g
}

// StartGoal transitions a goal to in_progress and sets it as the active goal.
func (gt *GoalTracker) StartGoal(id string) error {
	gt.mu.Lock()
	defer gt.mu.Unlock()

	g, ok := gt.Goals[id]
	if !ok {
		return fmt.Errorf("goal not found: %s", id)
	}

	if g.Status == GoalCompleted {
		return fmt.Errorf("cannot start a completed goal: %s", id)
	}
	if g.Status == GoalFailed {
		return fmt.Errorf("cannot start a failed goal: %s", id)
	}

	// Check dependencies
	for _, depID := range g.Dependencies {
		dep, exists := gt.Goals[depID]
		if exists && dep.Status != GoalCompleted {
			return fmt.Errorf("dependency not met: %s is not completed", depID)
		}
	}

	g.Status = GoalInProgress
	gt.ActiveGoal = g
	gt.History = append(gt.History, GoalEvent{
		GoalID:    g.ID,
		EventType: "started",
		Message:   fmt.Sprintf("Goal started: %s", g.Description),
		Timestamp: time.Now(),
	})

	return nil
}

// CompleteGoal marks a goal as completed and records the completion time.
// If the goal is a sub-goal, it checks whether the parent can advance.
func (gt *GoalTracker) CompleteGoal(id string) error {
	gt.mu.Lock()
	defer gt.mu.Unlock()

	g, ok := gt.Goals[id]
	if !ok {
		return fmt.Errorf("goal not found: %s", id)
	}

	now := time.Now()
	g.Status = GoalCompleted
	g.CompletedAt = &now
	g.Progress = 1.0

	if gt.ActiveGoal != nil && gt.ActiveGoal.ID == id {
		gt.ActiveGoal = nil
	}

	gt.History = append(gt.History, GoalEvent{
		GoalID:    g.ID,
		EventType: "completed",
		Message:   fmt.Sprintf("Goal completed: %s", g.Description),
		Timestamp: now,
	})

	// Check if this goal is a sub-goal and update parent progress
	if g.ParentID != "" {
		gt.updateParentProgress(g.ParentID)
	}

	return nil
}

// updateParentProgress recalculates a parent goal's progress based on sub-goals.
// Must be called with lock held.
func (gt *GoalTracker) updateParentProgress(parentID string) {
	parent, ok := gt.Goals[parentID]
	if !ok {
		return
	}

	if len(parent.SubGoals) == 0 {
		return
	}

	completedCount := 0
	for i := range parent.SubGoals {
		sub, exists := gt.Goals[parent.SubGoals[i].ID]
		if exists && sub.Status == GoalCompleted {
			completedCount++
		}
	}

	parent.Progress = float64(completedCount) / float64(len(parent.SubGoals))

	// If all sub-goals are complete, mark parent as completed
	if completedCount == len(parent.SubGoals) {
		now := time.Now()
		parent.Status = GoalCompleted
		parent.CompletedAt = &now
		gt.History = append(gt.History, GoalEvent{
			GoalID:    parent.ID,
			EventType: "completed",
			Message:   fmt.Sprintf("Goal completed (all sub-goals done): %s", parent.Description),
			Timestamp: now,
		})
	}
}

// FailGoal marks a goal as failed with a reason.
func (gt *GoalTracker) FailGoal(id string, reason string) error {
	gt.mu.Lock()
	defer gt.mu.Unlock()

	g, ok := gt.Goals[id]
	if !ok {
		return fmt.Errorf("goal not found: %s", id)
	}

	g.Status = GoalFailed

	if gt.ActiveGoal != nil && gt.ActiveGoal.ID == id {
		gt.ActiveGoal = nil
	}

	gt.History = append(gt.History, GoalEvent{
		GoalID:    g.ID,
		EventType: "failed",
		Message:   fmt.Sprintf("Goal failed: %s — %s", g.Description, reason),
		Timestamp: time.Now(),
	})

	return nil
}

// UpdateProgress sets the progress for a goal (clamped to 0.0-1.0).
func (gt *GoalTracker) UpdateProgress(id string, progress float64) {
	gt.mu.Lock()
	defer gt.mu.Unlock()

	g, ok := gt.Goals[id]
	if !ok {
		return
	}

	if progress < 0 {
		progress = 0
	}
	if progress > 1.0 {
		progress = 1.0
	}

	g.Progress = progress
	gt.History = append(gt.History, GoalEvent{
		GoalID:    g.ID,
		EventType: "progress",
		Message:   fmt.Sprintf("Progress updated to %.0f%%", progress*100),
		Timestamp: time.Now(),
	})
}

// RecordTokens adds token usage to a goal's running total.
func (gt *GoalTracker) RecordTokens(id string, tokens int) {
	gt.mu.Lock()
	defer gt.mu.Unlock()

	g, ok := gt.Goals[id]
	if !ok {
		return
	}

	g.TokensUsed += tokens
}

// IsBudgetExceeded returns true if the goal has exceeded its token budget.
func (gt *GoalTracker) IsBudgetExceeded(id string) bool {
	gt.mu.RLock()
	defer gt.mu.RUnlock()

	g, ok := gt.Goals[id]
	if !ok {
		return false
	}

	if g.TokenBudget <= 0 {
		return false // no budget set means unlimited
	}

	return g.TokensUsed >= g.TokenBudget
}

// GetNextGoal returns the highest-priority unblocked goal.
// It prefers goals already in_progress, then pending goals ordered by priority.
func (gt *GoalTracker) GetNextGoal() *Goal {
	gt.mu.RLock()
	defer gt.mu.RUnlock()

	// First check if there's an active in_progress goal
	var bestInProgress *Goal
	var bestPending *Goal

	for _, g := range gt.Goals {
		if g.Status == GoalCompleted || g.Status == GoalFailed {
			continue
		}

		// Check if dependencies are met
		if gt.isBlocked(g) {
			continue
		}

		// Check if budget is exceeded
		if g.TokenBudget > 0 && g.TokensUsed >= g.TokenBudget {
			continue
		}

		if g.Status == GoalInProgress {
			if bestInProgress == nil || g.Priority < bestInProgress.Priority {
				bestInProgress = g
			}
		} else if g.Status == GoalPending {
			if bestPending == nil || g.Priority < bestPending.Priority {
				bestPending = g
			}
		}
	}

	if bestInProgress != nil {
		return bestInProgress
	}
	return bestPending
}

// isBlocked checks whether a goal's dependencies are all completed.
// Must be called with at least a read lock held.
func (gt *GoalTracker) isBlocked(g *Goal) bool {
	for _, depID := range g.Dependencies {
		dep, exists := gt.Goals[depID]
		if !exists {
			// Unknown dependency — treat as blocking
			return true
		}
		if dep.Status != GoalCompleted {
			return true
		}
	}
	return false
}

// DecomposeGoal splits a goal into sub-goals. The parent goal completes
// automatically when all sub-goals are completed.
func (gt *GoalTracker) DecomposeGoal(id string, subDescriptions []string) error {
	gt.mu.Lock()
	defer gt.mu.Unlock()

	parent, ok := gt.Goals[id]
	if !ok {
		return fmt.Errorf("goal not found: %s", id)
	}

	if len(subDescriptions) == 0 {
		return fmt.Errorf("no sub-goal descriptions provided")
	}

	parent.SubGoals = make([]Goal, 0, len(subDescriptions))

	for _, desc := range subDescriptions {
		sub := &Goal{
			ID:          generateID(),
			Description: desc,
			Status:      GoalPending,
			Priority:    parent.Priority,
			CreatedAt:   time.Now(),
			ParentID:    parent.ID,
			SubGoals:    make([]Goal, 0),
			Tags:        make([]string, 0),
		}
		gt.Goals[sub.ID] = sub
		parent.SubGoals = append(parent.SubGoals, *sub)
		gt.History = append(gt.History, GoalEvent{
			GoalID:    sub.ID,
			EventType: "created",
			Message:   fmt.Sprintf("Sub-goal created: %s (parent: %s)", desc, parent.Description),
			Timestamp: time.Now(),
		})
	}

	return nil
}

// BuildGoalContext formats the current goal state for injection into the system prompt.
func (gt *GoalTracker) BuildGoalContext() string {
	gt.mu.RLock()
	defer gt.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("## Current Objectives\n\n")

	// Active goal
	if gt.ActiveGoal != nil {
		g := gt.ActiveGoal
		progressPct := int(g.Progress * 100)
		if g.TokenBudget > 0 {
			sb.WriteString(fmt.Sprintf("Active: %s (%d%% complete, %d/%d tokens)\n\n",
				g.Description, progressPct, g.TokensUsed, g.TokenBudget))
		} else {
			sb.WriteString(fmt.Sprintf("Active: %s (%d%% complete)\n\n",
				g.Description, progressPct))
		}
	}

	// Pending goals
	var pending []*Goal
	for _, g := range gt.Goals {
		if g.Status == GoalPending && (gt.ActiveGoal == nil || g.ID != gt.ActiveGoal.ID) {
			pending = append(pending, g)
		}
	}

	if len(pending) > 0 {
		sb.WriteString("Pending:\n")
		// Sort by priority (simple selection since we use stdlib only)
		sortGoalsByPriority(pending)
		for _, g := range pending {
			line := fmt.Sprintf("- %s (priority %d", g.Description, g.Priority)
			if len(g.Dependencies) > 0 {
				line += fmt.Sprintf(", depends on: %s", strings.Join(g.Dependencies, ", "))
			}
			line += ")\n"
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}

	// Summary
	total := 0
	completed := 0
	for _, g := range gt.Goals {
		// Only count top-level goals (no parent)
		if g.ParentID == "" {
			total++
			if g.Status == GoalCompleted {
				completed++
			}
		}
	}

	if total > 0 {
		sb.WriteString(fmt.Sprintf("Completed: %d/%d goals\n", completed, total))
	}

	return sb.String()
}

// ContinuationPrompt generates a prompt when goals remain incomplete.
func (gt *GoalTracker) ContinuationPrompt() string {
	next := gt.GetNextGoal()
	if next == nil {
		return ""
	}

	return fmt.Sprintf("You have remaining objectives. Next goal: %s\nContinue working on this.", next.Description)
}

// sortGoalsByPriority sorts goals by priority ascending (1 first).
func sortGoalsByPriority(goals []*Goal) {
	n := len(goals)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if goals[j].Priority > goals[j+1].Priority {
				goals[j], goals[j+1] = goals[j+1], goals[j]
			}
		}
	}
}
