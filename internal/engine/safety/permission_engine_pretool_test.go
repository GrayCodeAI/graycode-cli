package safety

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/hooks"
)

func TestCheckTool_PreToolUseDenyBeforeAutonomy(t *testing.T) {
	hooks.ResetDecisionHooks()
	t.Cleanup(hooks.ResetDecisionHooks)

	hooks.RegisterDecisionHookWithConfig(hooks.DecisionHookConfig{
		Name: "block-write",
		Matcher: hooks.DecisionMatcher{
			Events:    []string{"PreToolUse"}, // vendor alias
			ToolNames: []string{"Write"},
		},
		Priority: 1,
	}, func(event string, data map[string]interface{}) *hooks.HookDecision {
		return hooks.Deny("blocked by policy hook")
	})

	pe := NewPermissionEngine()
	pe.Autonomy = AutonomyYOLO // would otherwise allow everything

	granted, reason := pe.CheckTool(context.Background(), ToolCallInfo{
		Name: "Write",
		ID:   "1",
		Args: map[string]interface{}{"path": "x.txt"},
	})
	if granted {
		t.Fatal("expected PreToolUse deny even under YOLO")
	}
	if reason == "" || reason != "blocked by policy hook" {
		t.Fatalf("reason=%q", reason)
	}
}

func TestCheckTool_PreToolUseAllowContinues(t *testing.T) {
	hooks.ResetDecisionHooks()
	t.Cleanup(hooks.ResetDecisionHooks)

	hooks.RegisterDecisionHook(func(event string, data map[string]interface{}) *hooks.HookDecision {
		return hooks.Allow()
	})

	pe := NewPermissionEngine()
	pe.Autonomy = AutonomySupervised
	// No PromptFn → would deny at prompt stage for Write
	granted, _ := pe.CheckTool(context.Background(), ToolCallInfo{
		Name: "Write",
		ID:   "1",
		Args: map[string]interface{}{"path": "x.txt"},
	})
	// Allow from hook must NOT short-circuit to granted without user/autonomy
	if granted {
		t.Fatal("allow hook must not grant by itself under supervised")
	}
}

func TestCheckTool_DryRunBeforePreTool(t *testing.T) {
	hooks.ResetDecisionHooks()
	t.Cleanup(hooks.ResetDecisionHooks)

	called := false
	hooks.RegisterDecisionHook(func(event string, data map[string]interface{}) *hooks.HookDecision {
		called = true
		return hooks.Deny("should not run")
	})

	pe := NewPermissionEngine()
	pe.DryRun = true
	granted, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "Write"})
	if granted || reason == "" {
		t.Fatalf("dry-run should deny: granted=%v reason=%q", granted, reason)
	}
	if called {
		t.Fatal("PreTool hooks should not run when DryRun denies first")
	}
}
