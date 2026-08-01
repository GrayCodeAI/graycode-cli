package safety

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/sandbox"
)

// TestCheckTool_SpecStageBlocksEvenYOLO verifies the core guarantee documented
// in permission_engine.go: the spec-stage gate is checked before autonomy, so
// no autonomy level (including YOLO) can bypass it while a spec workflow is
// mid-flight.
func TestCheckTool_SpecStageBlocksEvenYOLO(t *testing.T) {
	for _, stage := range []SpecStage{SpecStageSpecify, SpecStagePlan, SpecStageTasks} {
		pe := NewPermissionEngine()
		pe.Stage = stage
		pe.Autonomy = AutonomyYOLO

		allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "Write"})
		if allowed {
			t.Errorf("stage %v: YOLO autonomy bypassed spec gate for Write, want denied", stage)
		}
		if reason == "" {
			t.Errorf("stage %v: expected a deny reason", stage)
		}

		allowed, _ = pe.CheckTool(context.Background(), ToolCallInfo{Name: "Bash", Args: map[string]interface{}{"command": "rm -rf /"}})
		if allowed {
			t.Errorf("stage %v: YOLO autonomy bypassed spec gate for Bash, want denied", stage)
		}
	}
}

// TestCheckTool_SpecStageAllowsWorkflowAndReadTools verifies that while a
// spec workflow is active, the workflow's own tools and read-only tools are
// still allowed through without a user prompt.
func TestCheckTool_SpecStageAllowsWorkflowAndReadTools(t *testing.T) {
	pe := NewPermissionEngine()
	pe.Stage = SpecStageSpecify
	pe.Autonomy = AutonomySupervised

	for _, name := range []string{"Specify"} {
		allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: name})
		if !allowed {
			t.Errorf("tool %q: expected allowed during spec stage, got denied: %q", name, reason)
		}
	}
	if allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "Plan"}); allowed || reason == "" {
		t.Fatalf("Plan should wait for Specify, allowed=%v reason=%q", allowed, reason)
	}
	pe.SpecSlug = "test-spec"
	if allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "Plan"}); !allowed || reason != "" {
		t.Fatalf("Plan should be allowed after Specify, allowed=%v reason=%q", allowed, reason)
	}
	if allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "Tasks"}); allowed || reason == "" {
		t.Fatalf("Tasks should wait for Plan, allowed=%v reason=%q", allowed, reason)
	}

	allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "Read"})
	if !allowed {
		t.Errorf("Read: expected allowed during spec stage (read-only), got denied: %q", reason)
	}
}

// TestCheckTool_ApproveImplementationAlwaysPrompts verifies that
// ApproveImplementation is never auto-allowed by autonomy tier, bypass-kill,
// or auto-mode — it always calls PromptFn, even at YOLO.
func TestCheckTool_ApproveImplementationAlwaysPrompts(t *testing.T) {
	pe := NewPermissionEngine()
	pe.Stage = SpecStageTasks
	pe.Autonomy = AutonomyYOLO
	pe.BypassKill.Enable()

	promptCalled := false
	pe.PromptFn = func(req PermissionRequest) {
		promptCalled = true
		req.Response <- true
	}

	allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "ApproveImplementation"})
	if !promptCalled {
		t.Fatal("expected PromptFn to be called for ApproveImplementation despite YOLO autonomy and bypass-kill")
	}
	if !allowed {
		t.Errorf("expected approval after user said yes, got denied: %q", reason)
	}
}

// TestCheckTool_ApproveImplementationDeniedByUser verifies a user rejecting
// the ApproveImplementation prompt keeps the spec gate closed.
func TestCheckTool_ApproveImplementationDeniedByUser(t *testing.T) {
	pe := NewPermissionEngine()
	pe.Stage = SpecStagePlan
	pe.Autonomy = AutonomyYOLO
	pe.PromptFn = func(req PermissionRequest) {
		req.Response <- false
	}

	allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "ApproveImplementation"})
	if allowed {
		t.Error("expected ApproveImplementation to be denied when user says no")
	}
	if reason == "" {
		t.Error("expected a deny reason")
	}
}

// TestCheckTool_SpecStageImplementingUsesAutonomy verifies the gate opens
// once Stage transitions to Implementing: ordinary autonomy-tier logic
// governs tool calls again, independent of spec stage.
func TestCheckTool_SpecStageImplementingUsesAutonomy(t *testing.T) {
	pe := NewPermissionEngine()
	pe.Stage = SpecStageImplementing
	pe.Autonomy = AutonomyYOLO

	allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "Write"})
	if !allowed {
		t.Errorf("expected Write allowed at YOLO once Implementing, got denied: %q", reason)
	}
}

// TestCheckTool_SpecStageNoneIgnoresGate verifies that outside of any spec
// workflow (Stage == SpecStageNone), the spec gate does not apply at all and
// autonomy-tier logic governs directly.
func TestCheckTool_SpecStageNoneIgnoresGate(t *testing.T) {
	pe := NewPermissionEngine()
	pe.Stage = SpecStageNone
	pe.Autonomy = AutonomySupervised

	promptCalled := false
	pe.PromptFn = func(req PermissionRequest) {
		promptCalled = true
		req.Response <- true
	}

	allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "Write"})
	if !promptCalled {
		t.Fatal("expected supervised autonomy to prompt for Write when no spec workflow is active")
	}
	if !allowed {
		t.Errorf("expected allowed after user approval, got denied: %q", reason)
	}
}

// TestCheckTool_DryRunOverridesEverything verifies DryRun denies
// unconditionally, even during spec stages that would otherwise allow the
// tool through.
func TestCheckTool_DryRunOverridesEverything(t *testing.T) {
	pe := NewPermissionEngine()
	pe.Stage = SpecStageSpecify
	pe.DryRun = true

	allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "Specify"})
	if allowed {
		t.Error("expected DryRun to deny even a spec-stage-allowed tool")
	}
	if reason == "" {
		t.Error("expected a deny reason")
	}
}

func TestCheckTool_ExplicitDenyOverridesAutonomy(t *testing.T) {
	for _, tier := range []AutonomyLevel{AutonomyBasic, AutonomySemi, AutonomyFull, AutonomyYOLO} {
		pe := NewPermissionEngine()
		pe.Autonomy = tier
		pe.Memory.AlwaysDeny("Write")
		allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "Write", Args: map[string]interface{}{"file_path": "x.txt"}})
		if allowed || reason != "Permission denied (rule)." {
			t.Fatalf("tier %v: allowed=%v reason=%q", tier, allowed, reason)
		}
	}
	pe := NewPermissionEngine()
	pe.Autonomy = AutonomyYOLO
	pe.AutoMode.Record("Write", "x.txt", true)
	pe.Memory.AlwaysDenyPattern("Write:x.txt")
	if allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "Write", Args: map[string]interface{}{"file_path": "x.txt"}}); allowed || reason != "Permission denied (rule)." {
		t.Fatalf("explicit deny did not beat auto-allow: allowed=%v reason=%q", allowed, reason)
	}
}

func TestCheckTool_SpecWorkflowRequiresOrderButAllowsSupportTools(t *testing.T) {
	pe := NewPermissionEngine()
	pe.Stage = SpecStageSpecify
	for _, name := range []string{"AskUserQuestion", "SpecStatus", "SpecEdit", "SpecList", "SpecConfig", "Clarify"} {
		allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: name})
		if !allowed {
			t.Errorf("support tool %q denied during spec stage: %q", name, reason)
		}
	}
	if allowed, _ := pe.CheckTool(context.Background(), ToolCallInfo{Name: "Tasks"}); allowed {
		t.Fatal("Tasks must not skip the Plan stage")
	}
	if allowed, _ := pe.CheckTool(context.Background(), ToolCallInfo{Name: "ApproveImplementation"}); allowed {
		t.Fatal("ApproveImplementation must not skip the Tasks stage")
	}
}

func TestCheckTool_StrictSandboxIsIndependentOfAutonomy(t *testing.T) {
	pe := NewPermissionEngine()
	pe.Autonomy = AutonomyYOLO
	pe.SandboxMode = sandbox.ModeStrict
	if allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "Write"}); allowed || reason == "" {
		t.Fatalf("strict sandbox allowed Write: reason=%q", reason)
	}
	if allowed, reason := pe.CheckTool(context.Background(), ToolCallInfo{Name: "AskUserQuestion"}); !allowed || reason != "" {
		t.Fatalf("strict sandbox blocked user clarification: allowed=%v reason=%q", allowed, reason)
	}
}
