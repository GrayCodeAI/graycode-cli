package hooks

import (
	"encoding/json"
	"testing"
)

func TestAllow(t *testing.T) {
	decision := Allow()
	if decision == nil {
		t.Fatal("expected non-nil decision")
	}
	if decision.Action != ActionAllow {
		t.Errorf("Action = %q, want %q", decision.Action, ActionAllow)
	}
}

func TestDeny(t *testing.T) {
	decision := Deny("test reason")
	if decision == nil {
		t.Fatal("expected non-nil decision")
	}
	if decision.Action != ActionDeny {
		t.Errorf("Action = %q, want %q", decision.Action, ActionDeny)
	}
	if decision.Reason != "test reason" {
		t.Errorf("Reason = %q, want %q", decision.Reason, "test reason")
	}
	if decision.Message != "test reason" {
		t.Errorf("Message = %q, want %q", decision.Message, "test reason")
	}
}

func TestInstruct(t *testing.T) {
	decision := Instruct("be careful")
	if decision == nil {
		t.Fatal("expected non-nil decision")
	}
	if decision.Action != ActionInstruct {
		t.Errorf("Action = %q, want %q", decision.Action, ActionInstruct)
	}
	if decision.Message != "be careful" {
		t.Errorf("Message = %q, want %q", decision.Message, "be careful")
	}
}

func TestModify(t *testing.T) {
	modified := json.RawMessage(`{"command":"echo safe"}`)
	decision := Modify("sanitized", modified)
	if decision == nil {
		t.Fatal("expected non-nil decision")
	}
	if decision.Action != ActionModify {
		t.Errorf("Action = %q, want %q", decision.Action, ActionModify)
	}
	if decision.Reason != "sanitized" {
		t.Errorf("Reason = %q, want %q", decision.Reason, "sanitized")
	}
	if string(decision.ModifiedInput) != string(modified) {
		t.Errorf("ModifiedInput = %q, want %q", decision.ModifiedInput, modified)
	}
}

func TestExecuteDecisionHooksSafe_NoHooks(t *testing.T) {
	ResetDecisionHooks()
	defer ResetDecisionHooks()

	decision := ExecuteDecisionHooksSafe("bash_execute", map[string]interface{}{
		"tool": "shell",
	})
	if decision != nil {
		t.Errorf("expected nil decision with no hooks, got %v", decision)
	}
}

func TestExecuteDecisionHooksSafe_WithHook(t *testing.T) {
	ResetDecisionHooks()
	defer ResetDecisionHooks()

	RegisterDecisionHook(func(event string, data map[string]interface{}) *HookDecision {
		if event == "bash_execute" {
			return Allow()
		}
		return nil
	})

	decision := ExecuteDecisionHooksSafe("bash_execute", map[string]interface{}{
		"tool": "shell",
	})
	if decision == nil {
		t.Fatal("expected non-nil decision from registered hook")
	}
	if decision.Action != ActionAllow {
		t.Errorf("Action = %q, want %q", decision.Action, ActionAllow)
	}
}

func TestExecuteDecisionHooksSafe_HookNotMatching(t *testing.T) {
	ResetDecisionHooks()
	defer ResetDecisionHooks()

	RegisterDecisionHook(func(event string, data map[string]interface{}) *HookDecision {
		if event == "other_event" {
			return Allow()
		}
		return nil
	})

	decision := ExecuteDecisionHooksSafe("bash_execute", map[string]interface{}{
		"tool": "shell",
	})
	if decision != nil {
		t.Errorf("expected nil decision for non-matching hook, got %v", decision)
	}
}

func TestLoadDiscoveredHooks_EmptyDir(t *testing.T) {
	// Test with a temp directory that has no hooks
	count := LoadDiscoveredHooks(t.TempDir())
	if count != 0 {
		t.Errorf("expected 0 hooks for empty dir, got %d", count)
	}
}

func TestLoadDiscoveredHooks_EmptyPath(t *testing.T) {
	// Empty path should use cwd
	count := LoadDiscoveredHooks("")
	// Should not panic
	_ = count
}
