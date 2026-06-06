package engine

import (
	"context"
	"testing"
)

func TestApprovalGate_Disabled_NoOp(t *testing.T) {
	s := NewSession("test", "m", "", nil)
	s.Autonomy = AutonomyYOLO
	// No gate configured: high-risk action proceeds (default behavior unchanged).
	ok, _ := s.CheckApproval(context.Background(), "Bash", map[string]interface{}{"command": "rm -rf /tmp/x"})
	if !ok {
		t.Fatal("nil gate should be a no-op (allow)")
	}

	s.Approval = &ApprovalGate{Enabled: false}
	ok, _ = s.CheckApproval(context.Background(), "Bash", map[string]interface{}{"command": "rm -rf /tmp/x"})
	if !ok {
		t.Fatal("disabled gate should be a no-op (allow)")
	}
}

func TestApprovalGate_FlaggedDestructiveRequiresApproval(t *testing.T) {
	approvedCalls := 0
	s := NewSession("test", "m", "", nil)
	s.Autonomy = AutonomyFull // above MaxAutoApprove default (supervised)
	s.Approval = &ApprovalGate{
		Enabled: true,
		ConfirmFn: func(req ApprovalRequest) bool {
			approvedCalls++
			if req.Category != ApprovalFileDeletion {
				t.Errorf("expected file_deletion category, got %s", req.Category)
			}
			return false // human denies
		},
	}

	ok, msg := s.CheckApproval(context.Background(), "Bash", map[string]interface{}{"command": "rm -rf build/"})
	if ok {
		t.Fatal("destructive action should be blocked when human denies")
	}
	if msg == "" {
		t.Fatal("expected a denial message")
	}
	if approvedCalls != 1 {
		t.Fatalf("expected ConfirmFn called once, got %d", approvedCalls)
	}
}

func TestApprovalGate_HumanApproves(t *testing.T) {
	s := NewSession("test", "m", "", nil)
	s.Autonomy = AutonomyFull
	s.Approval = &ApprovalGate{
		Enabled:   true,
		ConfirmFn: func(req ApprovalRequest) bool { return true },
	}
	ok, _ := s.CheckApproval(context.Background(), "Bash", map[string]interface{}{"command": "curl http://example.com"})
	if !ok {
		t.Fatal("action should proceed when human approves")
	}
}

func TestApprovalGate_AutoApproveThreshold(t *testing.T) {
	called := false
	s := NewSession("test", "m", "", nil)
	s.Autonomy = AutonomyBasic // <= MaxAutoApprove
	s.Approval = &ApprovalGate{
		Enabled:        true,
		MaxAutoApprove: AutonomySemi,
		ConfirmFn:      func(req ApprovalRequest) bool { called = true; return false },
	}
	ok, _ := s.CheckApproval(context.Background(), "Bash", map[string]interface{}{"command": "rm -rf x"})
	if !ok {
		t.Fatal("within auto-approve threshold the action should proceed without prompting")
	}
	if called {
		t.Fatal("ConfirmFn should not be invoked within auto-approve threshold")
	}
}

func TestApprovalGate_NonRiskyActionNotGated(t *testing.T) {
	called := false
	s := NewSession("test", "m", "", nil)
	s.Autonomy = AutonomyYOLO
	s.Approval = &ApprovalGate{
		Enabled:   true,
		ConfirmFn: func(req ApprovalRequest) bool { called = true; return false },
	}
	// A plain read-style command is not high-risk.
	ok, _ := s.CheckApproval(context.Background(), "Bash", map[string]interface{}{"command": "ls -la"})
	if !ok {
		t.Fatal("non-risky action should not be gated")
	}
	if called {
		t.Fatal("ConfirmFn should not be invoked for non-risky actions")
	}
}

func TestApprovalGate_CategoryFilter(t *testing.T) {
	called := false
	s := NewSession("test", "m", "", nil)
	s.Autonomy = AutonomyFull
	s.Approval = &ApprovalGate{
		Enabled:    true,
		Categories: map[ApprovalCategory]bool{ApprovalNetwork: true}, // only network gated
		ConfirmFn:  func(req ApprovalRequest) bool { called = true; return false },
	}
	// File deletion is not in the enabled category set => allowed.
	ok, _ := s.CheckApproval(context.Background(), "Bash", map[string]interface{}{"command": "rm -rf x"})
	if !ok {
		t.Fatal("deletion should be allowed when only network category is enabled")
	}
	if called {
		t.Fatal("ConfirmFn should not be called for unfiltered category")
	}

	// Network IS gated.
	ok2, _ := s.CheckApproval(context.Background(), "WebFetch", map[string]interface{}{"url": "http://x"})
	if ok2 {
		t.Fatal("network action should be gated and denied")
	}
	if !called {
		t.Fatal("ConfirmFn should be called for network action")
	}
}

func TestApprovalGate_FlaggedTool(t *testing.T) {
	s := NewSession("test", "m", "", nil)
	s.Autonomy = AutonomyFull
	denied := false
	s.Approval = &ApprovalGate{
		Enabled:      true,
		FlaggedTools: map[string]ApprovalCategory{"Write": ApprovalExternalAPI},
		ConfirmFn:    func(req ApprovalRequest) bool { denied = true; return false },
	}
	ok, _ := s.CheckApproval(context.Background(), "Write", map[string]interface{}{"file_path": "/x"})
	if ok {
		t.Fatal("flagged tool should require approval")
	}
	if !denied {
		t.Fatal("ConfirmFn should be invoked for flagged tool")
	}
}

func TestApprovalGate_FailClosedNoHandler(t *testing.T) {
	s := NewSession("test", "m", "", nil)
	s.Autonomy = AutonomyFull
	s.AskUserFn = nil
	s.Approval = &ApprovalGate{Enabled: true} // no ConfirmFn, no AskUserFn
	ok, msg := s.CheckApproval(context.Background(), "Bash", map[string]interface{}{"command": "rm -rf x"})
	if ok {
		t.Fatal("with no confirmation handler the gate must fail closed (deny)")
	}
	if msg == "" {
		t.Fatal("expected fail-closed denial message")
	}
}

func TestApprovalGate_FallbackAskUserFn(t *testing.T) {
	s := NewSession("test", "m", "", nil)
	s.Autonomy = AutonomyFull
	s.AskUserFn = func(q string) (string, error) { return "yes", nil }
	s.Approval = &ApprovalGate{Enabled: true}
	ok, _ := s.CheckApproval(context.Background(), "Bash", map[string]interface{}{"command": "rm -rf x"})
	if !ok {
		t.Fatal("AskUserFn returning yes should approve")
	}
}
