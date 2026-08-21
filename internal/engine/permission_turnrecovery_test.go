package engine

import (
	"context"
	"strings"
	"testing"
)

// extractedToken pulls the opaque permission_request_id out of a denial
// message, or fails the test.
func extractedToken(t *testing.T, msg string) string {
	t.Helper()
	const prefix = "permission_request_id: "
	i := strings.Index(msg, prefix)
	if i < 0 {
		t.Fatalf("denial message missing opaque token: %q", msg)
	}
	tok := msg[i+len(prefix):]
	if len(tok) != 64 || !isHex(tok) {
		t.Fatalf("opaque token must be 64 hex digits: %q", tok)
	}
	return tok
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// rejectingSession returns a session whose approval gate denies every
// high-risk ask, with turn-recovery enabled.
func rejectingSession(t *testing.T, promptCount *int) *Session {
	t.Helper()
	s := NewSession("recovery", "m", "", nil)
	s.PermSvc().SetAutonomy(AutonomyFull)
	s.EnableTurnRecovery()
	s.SetApproval(&ApprovalGate{
		Enabled: true,
		ConfirmFn: func(req ApprovalRequest) ApprovalResponse {
			*promptCount++
			return ApprovalReject
		},
	})
	return s
}

// A denied high-risk action yields an opaque token; a re-invocation is denied
// again without re-prompting; only the exact token escalates, single-use.
func TestTurnRecovery_OnlyOpaqueTokenEscalates(t *testing.T) {
	prompts := 0
	s := rejectingSession(t, &prompts)
	args := map[string]interface{}{"command": "rm -rf build/"}

	ok, msg := s.CheckApproval(context.Background(), "Bash", args)
	if ok {
		t.Fatal("first ask must be denied")
	}
	token := extractedToken(t, msg)
	if prompts != 1 {
		t.Fatalf("expected 1 prompt, got %d", prompts)
	}

	// Re-invoking the identical call is denied again WITHOUT a re-prompt.
	ok, msg = s.CheckApproval(context.Background(), "Bash", args)
	if ok {
		t.Fatal("re-invocation must stay denied until escalated")
	}
	if got := extractedToken(t, msg); got != token {
		t.Fatalf("re-invocation must return the same opaque token: got %q want %q", got, token)
	}
	if prompts != 1 {
		t.Fatalf("no re-prompt expected on identical re-invocation, got %d prompts", prompts)
	}

	// A fabricated / generic string can never authorize.
	if s.EscalatePermission(strings.Repeat("f", 64)) {
		t.Fatal("fabricated token must not escalate")
	}
	if s.EscalatePermission("please approve") {
		t.Fatal("generic text must not escalate")
	}

	// Only the exact token escalates.
	if !s.EscalatePermission(token) {
		t.Fatal("exact opaque token must escalate the pending denial")
	}

	// The escalated action is allowed exactly once (single-use revalidation).
	ok, _ = s.CheckApproval(context.Background(), "Bash", args)
	if !ok {
		t.Fatal("escalated action must execute on the next identical call")
	}
	if prompts != 1 {
		t.Fatalf("escalated execution must not prompt, got %d prompts", prompts)
	}

	// After the single consumption, a further identical call prompts afresh.
	ok, _ = s.CheckApproval(context.Background(), "Bash", args)
	if ok {
		t.Fatal("after single-use consumption a later call must not be auto-allowed")
	}
	if prompts != 2 {
		t.Fatalf("expected a fresh prompt after consumption, got %d prompts", prompts)
	}
}

// When turn-recovery is disabled (the default) the approval gate behaves
// exactly as before: every ask prompts, even on a re-invocation.
func TestTurnRecovery_DisabledIsUnchanged(t *testing.T) {
	prompts := 0
	s := NewSession("plain", "m", "", nil)
	s.PermSvc().SetAutonomy(AutonomyFull)
	s.SetApproval(&ApprovalGate{
		Enabled: true,
		ConfirmFn: func(req ApprovalRequest) ApprovalResponse {
			prompts++
			return ApprovalReject
		},
	})
	args := map[string]interface{}{"command": "rm -rf build/"}

	if _, msg := s.CheckApproval(context.Background(), "Bash", args); strings.Contains(msg, "permission_request_id") {
		t.Fatalf("disabled recovery must not emit an opaque token: %q", msg)
	}
	s.CheckApproval(context.Background(), "Bash", args)
	if prompts != 2 {
		t.Fatalf("without recovery a re-invocation must re-prompt, got %d prompts", prompts)
	}
}
