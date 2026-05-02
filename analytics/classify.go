package analytics

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// TurnCategory classifies what a conversation turn was about.
type TurnCategory string

const (
	CategoryCoding       TurnCategory = "coding"
	CategoryDebugging    TurnCategory = "debugging"
	CategoryTesting      TurnCategory = "testing"
	CategoryExploration  TurnCategory = "exploration"
	CategoryPlanning     TurnCategory = "planning"
	CategoryGitOps       TurnCategory = "git_ops"
	CategoryRefactoring  TurnCategory = "refactoring"
	CategoryConversation TurnCategory = "conversation"
)

// ClassifyTurn determines the category of a turn from tool names and user message.
// Deterministic — no LLM calls.
func ClassifyTurn(toolNames []string, userMessage string) TurnCategory {
	hasEdit := false
	hasRead := false
	hasTest := false
	hasGit := false
	hasPlan := false

	for _, t := range toolNames {
		switch t {
		case "Write", "Edit":
			hasEdit = true
		case "Read", "Grep", "Glob", "LS", "CodeSearch":
			hasRead = true
		case "Bash", "PowerShell":
			// handled below via message content
		case "EnterPlanMode", "ExitPlanMode", "TodoWrite", "TaskCreate":
			hasPlan = true
		}
	}

	lower := strings.ToLower(userMessage)

	// Check bash commands for test/git patterns
	for _, t := range toolNames {
		if t == "Bash" || t == "PowerShell" {
			if containsAny(lower, "test", "pytest", "jest", "vitest", "go test", "cargo test") {
				hasTest = true
			}
			if containsAny(lower, "git push", "git commit", "git merge", "git rebase") {
				hasGit = true
			}
		}
	}

	if hasGit {
		return CategoryGitOps
	}
	if hasPlan {
		return CategoryPlanning
	}
	if hasTest && !hasEdit {
		return CategoryTesting
	}
	if containsAny(lower, "refactor", "rename", "simplify", "clean up") && hasEdit {
		return CategoryRefactoring
	}
	if containsAny(lower, "fix", "bug", "error", "debug", "broken", "failing") && hasEdit {
		return CategoryDebugging
	}
	if hasEdit {
		return CategoryCoding
	}
	if hasRead && !hasEdit {
		return CategoryExploration
	}
	return CategoryConversation
}

// OneShotTracker tracks the percentage of edit turns that succeed without retries.
type OneShotTracker struct {
	mu            sync.Mutex
	totalEdits    int
	retriedEdits  int
	lastWasEdit   bool
}

// RecordTurn records a turn's tool usage for one-shot rate calculation.
// An edit turn immediately followed by another edit turn = retry.
func (t *OneShotTracker) RecordTurn(toolNames []string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	hasEdit := false
	for _, name := range toolNames {
		if name == "Write" || name == "Edit" {
			hasEdit = true
			break
		}
	}

	if hasEdit {
		t.totalEdits++
		if t.lastWasEdit {
			t.retriedEdits++ // consecutive edit = retry
		}
	}
	t.lastWasEdit = hasEdit
}

// Rate returns the one-shot success rate as a percentage (0-100).
func (t *OneShotTracker) Rate() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.totalEdits == 0 {
		return 100.0
	}
	return float64(t.totalEdits-t.retriedEdits) / float64(t.totalEdits) * 100.0
}

// Stats returns total edits and first-try successes.
func (t *OneShotTracker) Stats() (total, firstTry int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.totalEdits, t.totalEdits - t.retriedEdits
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// CommandContext captures metadata about a shell command execution.
type CommandContext struct {
	Command  string        `json:"command"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	CWD      string        `json:"cwd"`
}

// CommandTracker records shell command executions for analytics.
type CommandTracker struct {
	mu       sync.Mutex
	commands []CommandContext
}

// Record adds a command execution to the tracker.
func (t *CommandTracker) Record(cmd string, exitCode int, duration time.Duration, cwd string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.commands = append(t.commands, CommandContext{
		Command: cmd, ExitCode: exitCode, Duration: duration, CWD: cwd,
	})
}

// FailureRate returns the fraction of commands with non-zero exit codes.
func (t *CommandTracker) FailureRate() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.commands) == 0 {
		return 0
	}
	failures := 0
	for _, c := range t.commands {
		if c.ExitCode != 0 {
			failures++
		}
	}
	return float64(failures) / float64(len(t.commands))
}

// MostUsed returns the top n most frequently used commands.
func (t *CommandTracker) MostUsed(n int) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	counts := map[string]int{}
	for _, c := range t.commands {
		counts[c.Command]++
	}
	type kv struct {
		cmd   string
		count int
	}
	var sorted []kv
	for cmd, count := range counts {
		sorted = append(sorted, kv{cmd, count})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
	var out []string
	for i := 0; i < n && i < len(sorted); i++ {
		out = append(out, sorted[i].cmd)
	}
	return out
}

// AvgDuration returns the average duration across all recorded commands.
func (t *CommandTracker) AvgDuration() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.commands) == 0 {
		return 0
	}
	var total time.Duration
	for _, c := range t.commands {
		total += c.Duration
	}
	return total / time.Duration(len(t.commands))
}
