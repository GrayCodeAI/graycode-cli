package hooks

import (
	"encoding/json"
	"sync"
)

// HookDecision represents the outcome of a decision hook.
type HookDecision struct {
	Action        string          `json:"action"` // "allow", "deny", "modify"
	Reason        string          `json:"reason,omitempty"`
	ModifiedInput json.RawMessage `json:"modified_input,omitempty"`
}

// DecisionHookFn is a function that inspects an event and optionally returns a decision.
// Returning nil means "no opinion" (proceed normally).
type DecisionHookFn func(event string, data map[string]interface{}) *HookDecision

var (
	decisionMu    sync.RWMutex
	decisionHooks []DecisionHookFn
)

// RegisterDecisionHook adds a decision hook to the global list.
func RegisterDecisionHook(fn DecisionHookFn) {
	decisionMu.Lock()
	defer decisionMu.Unlock()
	decisionHooks = append(decisionHooks, fn)
}

// ExecuteDecisionHooks runs all registered decision hooks for the given event.
// It returns the first non-nil decision. If all hooks return nil, the result is nil
// (meaning no opinion, proceed normally).
func ExecuteDecisionHooks(event string, data map[string]interface{}) *HookDecision {
	decisionMu.RLock()
	hooks := make([]DecisionHookFn, len(decisionHooks))
	copy(hooks, decisionHooks)
	decisionMu.RUnlock()

	for _, fn := range hooks {
		if decision := fn(event, data); decision != nil {
			return decision
		}
	}
	return nil
}

// ResetDecisionHooks clears all registered decision hooks. Intended for testing.
func ResetDecisionHooks() {
	decisionMu.Lock()
	defer decisionMu.Unlock()
	decisionHooks = nil
}
