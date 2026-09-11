package intelligence

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// CommandSuggestion represents a single proactive command suggestion.
type CommandSuggestion struct {
	Command     string
	Description string
	Confidence  float64
	Category    string
	Context     string
}

// SuggestionRule defines a conditional rule that produces a suggestion.
type SuggestionRule struct {
	Name      string
	Condition func(ctx map[string]string) bool
	Suggest   func(ctx map[string]string) *CommandSuggestion
	Priority  int
}

// SuggestionEngine evaluates rules against the current context to produce
// proactive command suggestions.
type SuggestionEngine struct {
	Rules     []SuggestionRule
	History   []string
	Context   map[string]string
	dismissed map[string]time.Time
	mu        sync.RWMutex
}

// NewSuggestionEngine creates a SuggestionEngine with built-in rules.
func NewSuggestionEngine() *SuggestionEngine {
	se := &SuggestionEngine{
		Rules:     make([]SuggestionRule, 0),
		History:   make([]string, 0),
		Context:   make(map[string]string),
		dismissed: make(map[string]time.Time),
	}

	// Rule: After editing files -> suggest "run tests"
	se.Rules = append(se.Rules, SuggestionRule{
		Name: "after_edit_run_tests",
		Condition: func(ctx map[string]string) bool {
			edited := ctx["files_edited"]
			return edited != "" && edited != "0"
		},
		Suggest: func(ctx map[string]string) *CommandSuggestion {
			count := ctx["files_edited"]
			return &CommandSuggestion{
				Command:     "run tests",
				Description: fmt.Sprintf("You've edited %s files — verify nothing broke", count),
				Confidence:  0.92,
				Category:    "validation",
				Context:     "files_edited",
			}
		},
		Priority: 1,
	})

	// Rule: After test failure -> suggest "fix the failing test"
	se.Rules = append(se.Rules, SuggestionRule{
		Name: "after_test_failure",
		Condition: func(ctx map[string]string) bool {
			return ctx["test_status"] == "failed"
		},
		Suggest: func(ctx map[string]string) *CommandSuggestion {
			testName := ctx["failed_test"]
			desc := "A test is failing — fix it before continuing"
			if testName != "" {
				desc = fmt.Sprintf("Test %s is failing — fix it before continuing", testName)
			}
			return &CommandSuggestion{
				Command:     "fix the failing test",
				Description: desc,
				Confidence:  0.95,
				Category:    "fix",
				Context:     "test_failure",
			}
		},
		Priority: 1,
	})

	// Rule: After many edits -> suggest "commit changes"
	se.Rules = append(se.Rules, SuggestionRule{
		Name: "many_edits_commit",
		Condition: func(ctx map[string]string) bool {
			edited := ctx["files_edited"]
			if edited == "" {
				return false
			}
			n, err := strconv.Atoi(edited)
			return err == nil && n >= 5
		},
		Suggest: func(ctx map[string]string) *CommandSuggestion {
			count := ctx["files_edited"]
			testInfo := ""
			if ctx["test_status"] == "passed" {
				testInfo = ", tests passing"
			}
			return &CommandSuggestion{
				Command:     "commit changes",
				Description: fmt.Sprintf("%s files modified%s", count, testInfo),
				Confidence:  0.78,
				Category:    "workflow",
				Context:     "many_edits",
			}
		},
		Priority: 2,
	})

	// Rule: On new session -> suggest "review pending tasks"
	se.Rules = append(se.Rules, SuggestionRule{
		Name: "new_session_review",
		Condition: func(ctx map[string]string) bool {
			return ctx["session_state"] == "new"
		},
		Suggest: func(ctx map[string]string) *CommandSuggestion {
			return &CommandSuggestion{
				Command:     "review pending tasks",
				Description: "New session — check what needs attention",
				Confidence:  0.70,
				Category:    "planning",
				Context:     "new_session",
			}
		},
		Priority: 3,
	})

	// Rule: After error -> suggest "try a different approach"
	se.Rules = append(se.Rules, SuggestionRule{
		Name: "after_error_retry",
		Condition: func(ctx map[string]string) bool {
			return ctx["last_error"] != ""
		},
		Suggest: func(ctx map[string]string) *CommandSuggestion {
			errMsg := ctx["last_error"]
			if len(errMsg) > 60 {
				errMsg = errMsg[:60] + "..."
			}
			return &CommandSuggestion{
				Command:     "try a different approach",
				Description: fmt.Sprintf("Previous attempt errored: %s", errMsg),
				Confidence:  0.80,
				Category:    "recovery",
				Context:     "error",
			}
		},
		Priority: 2,
	})

	// Rule: File created -> suggest "add tests for new code"
	se.Rules = append(se.Rules, SuggestionRule{
		Name: "file_created_add_tests",
		Condition: func(ctx map[string]string) bool {
			return ctx["file_created"] != ""
		},
		Suggest: func(ctx map[string]string) *CommandSuggestion {
			file := ctx["file_created"]
			return &CommandSuggestion{
				Command:     "add tests for new code",
				Description: fmt.Sprintf("New file %s created — add test coverage", file),
				Confidence:  0.85,
				Category:    "testing",
				Context:     "file_created",
			}
		},
		Priority: 2,
	})

	// Rule: Long silence -> suggest "shall I continue?"
	se.Rules = append(se.Rules, SuggestionRule{
		Name: "long_silence_continue",
		Condition: func(ctx map[string]string) bool {
			return ctx["idle"] == "true"
		},
		Suggest: func(ctx map[string]string) *CommandSuggestion {
			return &CommandSuggestion{
				Command:     "shall I continue?",
				Description: "No activity detected — ready to help when you are",
				Confidence:  0.60,
				Category:    "engagement",
				Context:     "idle",
			}
		},
		Priority: 4,
	})

	return se
}

// Suggest evaluates all rules against the current context and returns
// top suggestions sorted by confidence (highest first).
func (se *SuggestionEngine) Suggest(ctx map[string]string) []*CommandSuggestion {
	se.mu.RLock()
	defer se.mu.RUnlock()

	var suggestions []*CommandSuggestion

	for _, rule := range se.Rules {
		if rule.Condition == nil || rule.Suggest == nil {
			continue
		}
		if !rule.Condition(ctx) {
			continue
		}
		suggestion := rule.Suggest(ctx)
		if suggestion == nil {
			continue
		}
		// Skip dismissed suggestions
		if se.isDismissed(suggestion.Command) {
			continue
		}
		suggestions = append(suggestions, suggestion)
	}

	// Sort by confidence descending
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})

	return suggestions
}

// UpdateContext updates a key-value pair in the engine's internal context state.
func (se *SuggestionEngine) UpdateContext(key, value string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.Context[key] = value
}

// RecordCommand records a command in the engine's history for context tracking.
func (se *SuggestionEngine) RecordCommand(cmd string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.History = append(se.History, cmd)
}

// GetTopSuggestion returns the single best suggestion based on the engine's
// current internal context, or nil if no rules match.
func (se *SuggestionEngine) GetTopSuggestion() *CommandSuggestion {
	se.mu.RLock()
	ctx := make(map[string]string, len(se.Context))
	for k, v := range se.Context {
		ctx[k] = v
	}
	se.mu.RUnlock()

	suggestions := se.Suggest(ctx)
	if len(suggestions) == 0 {
		return nil
	}
	return suggestions[0]
}

// FormatCommandSuggestions formats a list of command suggestions for display.
func FormatCommandSuggestions(suggestions []*CommandSuggestion) string {
	if len(suggestions) == 0 {
		return "No suggestions."
	}

	var sb strings.Builder
	sb.WriteString("Suggestions:\n")

	for i, s := range suggestions {
		sb.WriteString(fmt.Sprintf("%d. "+icons.Brain()+" %s (confidence: %.2f)\n", i+1, capitalizeSuggestion(s.Command), s.Confidence))
		sb.WriteString(fmt.Sprintf("   %s\n", s.Description))
	}

	return sb.String()
}

// AddRule adds a custom rule to the engine.
func (se *SuggestionEngine) AddRule(rule SuggestionRule) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.Rules = append(se.Rules, rule)
}

// Dismiss marks a command as dismissed so it won't be suggested again soon.
// The dismissal expires after 10 minutes.
func (se *SuggestionEngine) Dismiss(command string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.dismissed[command] = time.Now()
}

// isDismissed checks if a command has been recently dismissed.
// Must be called with at least a read lock held.
func (se *SuggestionEngine) isDismissed(command string) bool {
	dismissedAt, ok := se.dismissed[command]
	if !ok {
		return false
	}
	// Dismissals expire after 10 minutes
	if time.Since(dismissedAt) > 10*time.Minute {
		return false
	}
	return true
}

// capitalizeSuggestion returns the string with its first letter uppercased.
func capitalizeSuggestion(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
