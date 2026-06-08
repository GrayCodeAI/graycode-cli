package mission

import (
	"context"
	"testing"
	"time"
)

// TestApprovalGate_ApprovePath verifies that ResponseApprove allows the tool call.
func TestApprovalGate_ApprovePath(t *testing.T) {
	gate := NewMissionApprovalGate(func(req *ApprovalRequest) {
		// Operator approves asynchronously.
		go func() {
			if err := req.Respond(ResponseApprove); err != nil {
				t.Errorf("Respond returned error: %v", err)
			}
		}()
	})

	err := gate.Check(context.Background(), "Bash", map[string]interface{}{
		"command": "rm -rf /tmp/test",
	})
	if err != nil {
		t.Fatalf("expected nil error on approve, got: %v", err)
	}
}

// TestApprovalGate_RejectPath verifies that ResponseReject returns ErrToolRejected.
func TestApprovalGate_RejectPath(t *testing.T) {
	gate := NewMissionApprovalGate(func(req *ApprovalRequest) {
		go func() { _ = req.Respond(ResponseReject) }()
	})

	err := gate.Check(context.Background(), "Bash", map[string]interface{}{
		"command": "curl http://example.com",
	})
	if err == nil {
		t.Fatal("expected ErrToolRejected, got nil")
	}
	if err != ErrToolRejected {
		t.Fatalf("expected ErrToolRejected, got: %v", err)
	}
}

// TestApprovalGate_SessionApprovePath verifies that ResponseApproveForSession
// auto-approves subsequent calls to the same tool without calling OnRequest again.
func TestApprovalGate_SessionApprovePath(t *testing.T) {
	callCount := 0
	gate := NewMissionApprovalGate(func(req *ApprovalRequest) {
		callCount++
		go func() { _ = req.Respond(ResponseApproveForSession) }()
	})

	// First call: triggers OnRequest, gets session-approved.
	err := gate.Check(context.Background(), "WebFetch", map[string]interface{}{})
	if err != nil {
		t.Fatalf("first call: expected nil, got: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected OnRequest called once, got %d", callCount)
	}

	// Second call: should skip OnRequest because the tool is session-approved.
	err = gate.Check(context.Background(), "WebFetch", map[string]interface{}{})
	if err != nil {
		t.Fatalf("second call: expected nil (session approved), got: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("OnRequest should not be called for session-approved tool, called %d times", callCount)
	}
}

// TestApprovalGate_ContextCancellation verifies that a cancelled context returns
// ErrApprovalTimeout when the operator never responds.
func TestApprovalGate_ContextCancellation(t *testing.T) {
	gate := NewMissionApprovalGate(func(req *ApprovalRequest) {
		// Intentionally do NOT call Respond — simulates a slow operator.
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := gate.Check(ctx, "Bash", map[string]interface{}{
		"command": "wget http://example.com",
	})
	if err == nil {
		t.Fatal("expected ErrApprovalTimeout when context expires, got nil")
	}
	if err != ErrApprovalTimeout {
		t.Fatalf("expected ErrApprovalTimeout, got: %v", err)
	}
}

// TestApprovalGate_NilGate verifies that a nil gate is a no-op.
func TestApprovalGate_NilGate(t *testing.T) {
	var gate *MissionApprovalGate
	err := gate.Check(context.Background(), "Bash", map[string]interface{}{
		"command": "rm -rf /tmp/x",
	})
	if err != nil {
		t.Fatalf("nil gate should be a no-op, got: %v", err)
	}
}

// TestApprovalGate_NilOnRequest verifies that a gate with no OnRequest is a no-op.
func TestApprovalGate_NilOnRequest(t *testing.T) {
	gate := NewMissionApprovalGate(nil)
	err := gate.Check(context.Background(), "Bash", map[string]interface{}{
		"command": "rm -rf /tmp/x",
	})
	if err != nil {
		t.Fatalf("gate with nil OnRequest should be a no-op, got: %v", err)
	}
}

// TestApprovalGate_NonRiskyToolNotGated verifies that safe tools bypass the gate.
func TestApprovalGate_NonRiskyToolNotGated(t *testing.T) {
	called := false
	gate := NewMissionApprovalGate(func(req *ApprovalRequest) {
		called = true
	})

	err := gate.Check(context.Background(), "Read", map[string]interface{}{
		"file_path": "/etc/hosts",
	})
	if err != nil {
		t.Fatalf("non-risky tool should not be gated, got: %v", err)
	}
	if called {
		t.Fatal("OnRequest should not be called for non-risky tools")
	}
}

// TestApprovalRequest_MultipleRespondCalls verifies that only the first Respond
// call takes effect (subsequent calls are no-ops).
func TestApprovalRequest_MultipleRespondCalls(t *testing.T) {
	req := &ApprovalRequest{
		ToolName: "Bash",
		respond:  make(chan RequestResponse, 1),
	}

	// First call sends the value.
	if err := req.Respond(ResponseApprove); err != nil {
		t.Fatalf("first Respond failed: %v", err)
	}
	// Second call should not block or panic.
	if err := req.Respond(ResponseReject); err != nil {
		t.Fatalf("second Respond failed: %v", err)
	}

	// The channel should only contain the first response.
	resp := <-req.respond
	if resp != ResponseApprove {
		t.Fatalf("expected ResponseApprove from first call, got %v", resp)
	}
}

// TestApprovalGate_MissionField verifies the ApprovalGate field is present on Mission.
func TestApprovalGate_MissionField(t *testing.T) {
	m := New("test prompt", Config{})
	if m.ApprovalGate != nil {
		t.Fatal("ApprovalGate should default to nil")
	}

	gate := NewMissionApprovalGate(func(req *ApprovalRequest) {})
	m.ApprovalGate = gate
	if m.ApprovalGate == nil {
		t.Fatal("ApprovalGate should be assignable on Mission")
	}
}
