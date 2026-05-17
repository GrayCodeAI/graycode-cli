package hooks

import (
	"encoding/json"
	"testing"
)

func TestRegisterAndExecuteDecisionHook(t *testing.T) {
	ResetDecisionHooks()
	defer ResetDecisionHooks()

	RegisterDecisionHook(func(event string, data map[string]interface{}) *HookDecision {
		if event == "file_write" {
			return &HookDecision{Action: "deny", Reason: "read-only mode"}
		}
		return nil
	})

	// Should deny file_write
	decision := ExecuteDecisionHooks("file_write", nil)
	if decision == nil {
		t.Fatal("expected a decision, got nil")
	}
	if decision.Action != "deny" {
		t.Fatalf("expected deny, got %s", decision.Action)
	}
	if decision.Reason != "read-only mode" {
		t.Fatalf("expected 'read-only mode', got %s", decision.Reason)
	}

	// Should return nil for unmatched event
	decision = ExecuteDecisionHooks("file_read", nil)
	if decision != nil {
		t.Fatalf("expected nil for unmatched event, got %+v", decision)
	}
}

func TestDecisionHookFirstWins(t *testing.T) {
	ResetDecisionHooks()
	defer ResetDecisionHooks()

	RegisterDecisionHook(func(event string, data map[string]interface{}) *HookDecision {
		return &HookDecision{Action: "allow", Reason: "first"}
	})
	RegisterDecisionHook(func(event string, data map[string]interface{}) *HookDecision {
		return &HookDecision{Action: "deny", Reason: "second"}
	})

	decision := ExecuteDecisionHooks("any_event", nil)
	if decision == nil {
		t.Fatal("expected a decision")
	}
	if decision.Reason != "first" {
		t.Fatalf("expected first hook to win, got %s", decision.Reason)
	}
}

func TestDecisionHookModify(t *testing.T) {
	ResetDecisionHooks()
	defer ResetDecisionHooks()

	modified := json.RawMessage(`{"command":"echo safe"}`)
	RegisterDecisionHook(func(event string, data map[string]interface{}) *HookDecision {
		if event == "bash_execute" {
			return &HookDecision{
				Action:        "modify",
				Reason:        "sanitized",
				ModifiedInput: modified,
			}
		}
		return nil
	})

	decision := ExecuteDecisionHooks("bash_execute", map[string]interface{}{"command": "rm -rf /"})
	if decision == nil {
		t.Fatal("expected a decision")
	}
	if decision.Action != "modify" {
		t.Fatalf("expected modify, got %s", decision.Action)
	}
	if string(decision.ModifiedInput) != `{"command":"echo safe"}` {
		t.Fatalf("unexpected modified input: %s", string(decision.ModifiedInput))
	}
}

func TestDecisionHookNilWhenEmpty(t *testing.T) {
	ResetDecisionHooks()
	defer ResetDecisionHooks()

	decision := ExecuteDecisionHooks("any_event", nil)
	if decision != nil {
		t.Fatalf("expected nil when no hooks registered, got %+v", decision)
	}
}
