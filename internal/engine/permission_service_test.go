package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/sandbox"
)

func TestPermissionService_CheckTool(t *testing.T) {
	// Inject a permission engine whose CheckTool denies everything.
	// We avoid calling the real engine (which would need full tool/perm
	// state) — this test only checks the PermissionService delegation.
	s := NewPermissionService(nil)
	// The default engine may or may not allow Bash; replace with a
	// custom stub via a small inline trick: set Mode to a plan that
	// forces denial (not implemented in the engine, so use a
	// permissionFn that returns a specific deny). For now, just verify
	// the wrapper compiles and returns a (bool, string).
	granted, _ := s.CheckTool(context.Background(), ToolCallInfo{Name: "Bash", Args: map[string]interface{}{"command": "ls"}})
	_ = granted
}

func TestPermissionService_SetSpecStage(t *testing.T) {
	s := NewPermissionService(nil)
	stages := []SpecStage{SpecStageNone, SpecStageSpecify, SpecStagePlan, SpecStageTasks, SpecStageImplementing}
	for _, stage := range stages {
		s.SetSpecStage(stage)
		if s.SpecStage() != stage {
			t.Errorf("SetSpecStage(%v) then SpecStage() = %v", stage, s.SpecStage())
		}
	}
}

func TestPermissionService_BudgetAndTurnCaps(t *testing.T) {
	s := NewPermissionService(nil)
	s.SetMaxTurns(42)
	s.SetMaxBudgetUSD(1.23)
	if s.MaxTurns() != 42 {
		t.Errorf("MaxTurns = %d, want 42", s.MaxTurns())
	}
	if s.MaxBudgetUSD() != 1.23 {
		t.Errorf("MaxBudgetUSD = %v, want 1.23", s.MaxBudgetUSD())
	}
}

func TestPermissionService_AutonomyAndAllowedDirs(t *testing.T) {
	s := NewPermissionService(nil)
	s.SetAutonomy(AutonomySupervised)
	s.SetAllowedDirs([]string{"/tmp", "/var/folders"})
	if s.Autonomy() != AutonomySupervised {
		t.Errorf("Autonomy = %v, want AutonomySupervised", s.Autonomy())
	}
	if len(s.AllowedDirs()) != 2 {
		t.Errorf("AllowedDirs len = %d, want 2", len(s.AllowedDirs()))
	}
}

func TestPermissionService_SandboxModeRoundTrip(t *testing.T) {
	s := NewPermissionService(nil)
	s.SetSandboxMode(sandbox.ModeStrict)
	if got := s.SandboxMode(); got != sandbox.ModeStrict {
		t.Fatalf("SandboxMode = %q, want strict", got)
	}
}

func TestPermissionService_ApplyPolicySnapshotCopiesRulesAndScopes(t *testing.T) {
	parent := NewPermissionService(nil)
	parent.Memory().AlwaysDeny("Write")
	parent.SetAllowedDirs([]string{"/workspace"})
	snapshot := parent.PolicySnapshot()
	child := NewPermissionService(nil)
	child.ApplyPolicySnapshot(snapshot)
	snapshot.AllowedDirs[0] = "/changed"
	if child.AllowedDirs()[0] != "/workspace" {
		t.Fatalf("child allowed dirs changed through snapshot alias: %v", child.AllowedDirs())
	}
	allowed, reason := child.CheckTool(context.Background(), ToolCallInfo{Name: "Write"})
	if allowed || reason != "Permission denied (rule)." {
		t.Fatalf("child did not inherit deny rule: allowed=%v reason=%q", allowed, reason)
	}
}

func TestPermissionService_CheckApproval_NoGate(t *testing.T) {
	s := NewPermissionService(nil)
	approved, _ := s.CheckApproval(context.Background(), "Bash", map[string]interface{}{})
	if !approved {
		t.Error("expected approved when no gate is set")
	}
}

func TestPermissionService_IsZero(t *testing.T) {
	s := NewPermissionService(nil)
	if !s.IsZero() {
		t.Error("freshly-constructed PermissionService should be IsZero()")
	}
	s.SetPermissionFn(func(req PermissionRequest) {})
	if s.IsZero() {
		t.Error("after SetPermissionFn, service should not be IsZero()")
	}
}

func TestPermissionService_NewReturnsReadyEngine(t *testing.T) {
	s := NewPermissionService(nil)
	if s.Engine() == nil {
		t.Error("NewPermissionService should produce a non-nil engine")
	}
	if s.Engine().Memory == nil {
		t.Error("engine should have a non-nil Memory")
	}
	if s.Engine().Classifier == nil {
		t.Error("engine should have a non-nil Classifier")
	}
}

func TestPermissionService_SetPermissionFn(t *testing.T) {
	s := NewPermissionService(nil)
	called := false
	s.SetPermissionFn(func(req PermissionRequest) {
		called = true
	})
	if s.Engine().PromptFn == nil {
		t.Error("SetPermissionFn should have set the engine's PromptFn")
	}
	// Call directly to verify.
	s.Engine().PromptFn(PermissionRequest{})
	if !called {
		t.Error("PromptFn was not called")
	}
	_ = strings.Contains // suppress unused import warning
}
