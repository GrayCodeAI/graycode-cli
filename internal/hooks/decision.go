package hooks

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// DecisionAction constants for HookDecision.Action.
const (
	ActionAllow    = "allow"
	ActionDeny     = "deny"
	ActionModify   = "modify"
	ActionInstruct = "instruct"
)

// HookDecision represents the outcome of a decision hook.
type HookDecision struct {
	Action        string          `json:"action"` // "allow", "deny", "modify", "instruct"
	Reason        string          `json:"reason,omitempty"`
	Message       string          `json:"message,omitempty"` // User-facing message (for deny/instruct)
	ModifiedInput json.RawMessage `json:"modified_input,omitempty"`
}

// DecisionMatcher specifies which events and tools a decision hook applies to.
// Empty arrays mean "match all".
type DecisionMatcher struct {
	Events    []string // Event types to match (e.g., "pre_tool", "post_tool")
	ToolNames []string // Tool names to match (e.g., "Bash", "Write", "Edit")
}

// Match returns true if the matcher applies to the given event and tool name.
// Event names are compared after CanonicalEvent so PreToolUse matches pre_tool.
func (m *DecisionMatcher) Match(event string, toolName string) bool {
	if len(m.Events) > 0 {
		found := false
		for _, e := range m.Events {
			if EventsMatch(e, event) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(m.ToolNames) > 0 {
		found := false
		for _, t := range m.ToolNames {
			if t == toolName {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// DecisionHookConfig configures a decision hook with matching and priority.
type DecisionHookConfig struct {
	Name     string
	Matcher  DecisionMatcher
	Priority int // Lower = earlier evaluation
}

// DecisionHookFn is a function that inspects an event and optionally returns a decision.
// Returning nil means "no opinion" (proceed normally).
type DecisionHookFn func(event string, data map[string]interface{}) *HookDecision

// WrappedDecisionHook pairs a config with its evaluation function.
type WrappedDecisionHook struct {
	Config DecisionHookConfig
	Fn     DecisionHookFn
}

var (
	decisionMu    sync.RWMutex
	decisionHooks []WrappedDecisionHook
)

// RegisterDecisionHook adds a decision hook to the global list.
func RegisterDecisionHook(fn DecisionHookFn) {
	decisionMu.Lock()
	defer decisionMu.Unlock()
	decisionHooks = append(decisionHooks, WrappedDecisionHook{
		Config: DecisionHookConfig{
			Name:     "anonymous",
			Matcher:  DecisionMatcher{},
			Priority: 100,
		},
		Fn: fn,
	})
}

// RegisterDecisionHookWithConfig adds a decision hook with matching and priority.
func RegisterDecisionHookWithConfig(config DecisionHookConfig, fn DecisionHookFn) {
	decisionMu.Lock()
	defer decisionMu.Unlock()
	decisionHooks = append(decisionHooks, WrappedDecisionHook{
		Config: config,
		Fn:     fn,
	})
	// Sort by priority
	for i := 0; i < len(decisionHooks); i++ {
		for j := i + 1; j < len(decisionHooks); j++ {
			if decisionHooks[j].Config.Priority < decisionHooks[i].Config.Priority {
				decisionHooks[i], decisionHooks[j] = decisionHooks[j], decisionHooks[i]
			}
		}
	}
}

// ExecuteDecisionHooks runs all registered decision hooks for the given event.
// It respects matchers (event type + tool name) and returns the first non-nil decision.
// If all hooks return nil, the result is nil (meaning no opinion, proceed normally).
// Fail-open: hook errors are logged but do not block the operation.
func ExecuteDecisionHooks(event string, data map[string]interface{}) *HookDecision {
	decisionMu.RLock()
	hooks := make([]WrappedDecisionHook, len(decisionHooks))
	copy(hooks, decisionHooks)
	decisionMu.RUnlock()

	toolName, _ := data["tool"].(string)

	for _, h := range hooks {
		// Check if this hook applies to this event/tool
		if !h.Config.Matcher.Match(event, toolName) {
			continue
		}

		// Execute exactly once with fail-open panic recovery. The previous
		// implementation invoked h.Fn twice (once in a discard closure, once
		// to capture the result), double-firing side-effecting hooks.
		decision := safeExecuteHook(h, event, data, toolName)
		if decision != nil {
			return decision
		}
	}
	return nil
}

// ExecuteDecisionHooksSafe runs all registered decision hooks with full fail-open semantics.
// Returns the first non-nil decision, or nil if all hooks return nil or panic.
func ExecuteDecisionHooksSafe(event string, data map[string]interface{}) *HookDecision {
	decisionMu.RLock()
	hooks := make([]WrappedDecisionHook, len(decisionHooks))
	copy(hooks, decisionHooks)
	decisionMu.RUnlock()

	toolName, _ := data["tool"].(string)

	for _, h := range hooks {
		if !h.Config.Matcher.Match(event, toolName) {
			continue
		}

		decision := safeExecuteHook(h, event, data, toolName)
		if decision != nil {
			return decision
		}
	}
	return nil
}

func safeExecuteHook(h WrappedDecisionHook, event string, data map[string]interface{}, toolName string) *HookDecision {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("decision hook panicked (fail-open)",
				"hook", h.Config.Name,
				"event", event,
				"tool", toolName,
				"panic", r)
		}
	}()
	return h.Fn(event, data)
}

// ResetDecisionHooks clears all registered decision hooks. Intended for testing.
func ResetDecisionHooks() {
	decisionMu.Lock()
	defer decisionMu.Unlock()
	decisionHooks = nil
}

// Allow creates an allow decision.
func Allow() *HookDecision {
	return &HookDecision{Action: ActionAllow}
}

// Deny creates a deny decision with a reason.
func Deny(reason string) *HookDecision {
	return &HookDecision{Action: ActionDeny, Reason: reason, Message: reason}
}

// Instruct creates an instruct decision that injects context without blocking.
// The agent receives the message as guidance for its next action.
func Instruct(message string) *HookDecision {
	return &HookDecision{Action: ActionInstruct, Message: message}
}

// Modify creates a modify decision with modified input.
func Modify(reason string, modifiedInput json.RawMessage) *HookDecision {
	return &HookDecision{Action: ActionModify, Reason: reason, ModifiedInput: modifiedInput}
}
