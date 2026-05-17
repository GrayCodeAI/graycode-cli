package permissions

import (
	"strings"
	"testing"
	"time"
)

func TestNewApprovalWorkflow(t *testing.T) {
	wf := NewApprovalWorkflow(nil)
	if wf == nil {
		t.Fatal("expected non-nil workflow")
	}
	if len(wf.Policies) == 0 {
		t.Fatal("expected default policies")
	}
	if len(wf.Pending) != 0 {
		t.Fatal("expected empty pending list")
	}
	if len(wf.History) != 0 {
		t.Fatal("expected empty history list")
	}
}

func TestAutoApproveReadTools(t *testing.T) {
	wf := NewApprovalWorkflow(nil)
	req, err := wf.RequestApproval("Read", map[string]interface{}{"file_path": "/tmp/test.txt"}, "LOW")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Status != "approved" {
		t.Errorf("expected approved, got %s", req.Status)
	}
	if !strings.Contains(req.Reason, "auto-approved") {
		t.Errorf("expected auto-approved reason, got %s", req.Reason)
	}
}

func TestAutoApproveEditWrite(t *testing.T) {
	wf := NewApprovalWorkflow(nil)
	req, err := wf.RequestApproval("Edit", map[string]interface{}{"file_path": "/project/main.go"}, "MEDIUM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Status != "approved" {
		t.Errorf("expected approved, got %s", req.Status)
	}
}

func TestBashRequiresApproval(t *testing.T) {
	promptCalled := false
	promptFn := func(req *ApprovalRequest) (bool, string) {
		promptCalled = true
		return true, "user approved"
	}

	wf := NewApprovalWorkflow(promptFn)
	req, err := wf.RequestApproval("Bash", map[string]interface{}{"command": "ls -la"}, "MEDIUM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !promptCalled {
		t.Error("expected prompt function to be called")
	}
	if req.Status != "approved" {
		t.Errorf("expected approved, got %s", req.Status)
	}
	if req.Reason != "user approved" {
		t.Errorf("expected 'user approved' reason, got %s", req.Reason)
	}
}

func TestBashDenied(t *testing.T) {
	promptFn := func(req *ApprovalRequest) (bool, string) {
		return false, "too dangerous"
	}

	wf := NewApprovalWorkflow(promptFn)
	req, err := wf.RequestApproval("Bash", map[string]interface{}{"command": "rm -rf /"}, "MEDIUM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Status != "denied" {
		t.Errorf("expected denied, got %s", req.Status)
	}
	if req.Reason != "too dangerous" {
		t.Errorf("expected 'too dangerous' reason, got %s", req.Reason)
	}
}

func TestDeleteAlwaysAsks(t *testing.T) {
	promptCalled := false
	promptFn := func(req *ApprovalRequest) (bool, string) {
		promptCalled = true
		return true, "confirmed delete"
	}

	wf := NewApprovalWorkflow(promptFn)
	req, err := wf.RequestApproval("Delete", map[string]interface{}{"path": "/tmp/file.txt"}, "HIGH")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !promptCalled {
		t.Error("expected prompt to be called for delete")
	}
	if req.Status != "approved" {
		t.Errorf("expected approved, got %s", req.Status)
	}
}

func TestGitPushAlwaysAsks(t *testing.T) {
	promptCalled := false
	promptFn := func(req *ApprovalRequest) (bool, string) {
		promptCalled = true
		return false, "not now"
	}

	wf := NewApprovalWorkflow(promptFn)
	req, err := wf.RequestApproval("GitPush", map[string]interface{}{"remote": "origin", "branch": "main"}, "HIGH")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !promptCalled {
		t.Error("expected prompt to be called for git push")
	}
	if req.Status != "denied" {
		t.Errorf("expected denied, got %s", req.Status)
	}
}

func TestPendingWithoutPromptFn(t *testing.T) {
	wf := NewApprovalWorkflow(nil)
	req, err := wf.RequestApproval("Bash", map[string]interface{}{"command": "echo hello"}, "MEDIUM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Status != "pending" {
		t.Errorf("expected pending, got %s", req.Status)
	}

	pending := wf.GetPending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].ID != req.ID {
		t.Error("pending request ID mismatch")
	}
}

func TestApproveRequest(t *testing.T) {
	wf := NewApprovalWorkflow(nil)
	req, err := wf.RequestApproval("Bash", map[string]interface{}{"command": "make build"}, "MEDIUM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = wf.Approve(req.ID, "looks safe")
	if err != nil {
		t.Fatalf("unexpected error approving: %v", err)
	}

	if !wf.IsApproved(req.ID) {
		t.Error("expected request to be approved")
	}

	pending := wf.GetPending()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
}

func TestDenyRequest(t *testing.T) {
	wf := NewApprovalWorkflow(nil)
	req, err := wf.RequestApproval("Bash", map[string]interface{}{"command": "shutdown -h now"}, "MEDIUM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = wf.Deny(req.ID, "not allowed")
	if err != nil {
		t.Fatalf("unexpected error denying: %v", err)
	}

	if wf.IsApproved(req.ID) {
		t.Error("expected request to not be approved")
	}

	pending := wf.GetPending()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
}

func TestApproveNotFound(t *testing.T) {
	wf := NewApprovalWorkflow(nil)
	err := wf.Approve("nonexistent-id", "reason")
	if err == nil {
		t.Error("expected error for nonexistent ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestDenyNotFound(t *testing.T) {
	wf := NewApprovalWorkflow(nil)
	err := wf.Deny("nonexistent-id", "reason")
	if err == nil {
		t.Error("expected error for nonexistent ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestExpirePending(t *testing.T) {
	wf := NewApprovalWorkflow(nil)

	// Manually create a request that is already expired
	req := &ApprovalRequest{
		ID:        "expired-req-1",
		Tool:      "Bash",
		Args:      map[string]interface{}{"command": "test"},
		Risk:      "MEDIUM",
		CreatedAt: time.Now().Add(-10 * time.Minute),
		Status:    "pending",
		ExpiresAt: time.Now().Add(-5 * time.Minute), // Already expired
	}
	wf.Pending = append(wf.Pending, req)

	// Also add a non-expired request
	req2 := &ApprovalRequest{
		ID:        "valid-req-1",
		Tool:      "Bash",
		Args:      map[string]interface{}{"command": "test2"},
		Risk:      "MEDIUM",
		CreatedAt: time.Now(),
		Status:    "pending",
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	wf.Pending = append(wf.Pending, req2)

	wf.ExpirePending()

	pending := wf.GetPending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending after expire, got %d", len(pending))
	}
	if pending[0].ID != "valid-req-1" {
		t.Error("wrong request remained pending")
	}

	// Check that expired request moved to history
	found := false
	for _, h := range wf.History {
		if h.ID == "expired-req-1" && h.Status == "expired" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected expired request in history")
	}
}

func TestAddPolicy(t *testing.T) {
	wf := NewApprovalWorkflow(nil)
	initialCount := len(wf.Policies)

	wf.AddPolicy(ApprovalPolicy{
		Name:        "custom-policy",
		Tools:       []string{"CustomTool"},
		RiskLevel:   "CRITICAL",
		AutoApprove: false,
		Timeout:     1 * time.Minute,
		MaxPending:  3,
	})

	if len(wf.Policies) != initialCount+1 {
		t.Errorf("expected %d policies, got %d", initialCount+1, len(wf.Policies))
	}
}

func TestCheckPolicy(t *testing.T) {
	wf := NewApprovalWorkflow(nil)

	// Match by tool name
	policy := wf.CheckPolicy("Read", "LOW")
	if policy == nil {
		t.Fatal("expected policy for Read")
	}
	if policy.Name != "read-auto-approve" {
		t.Errorf("expected read-auto-approve, got %s", policy.Name)
	}

	// Match Bash
	policy = wf.CheckPolicy("Bash", "MEDIUM")
	if policy == nil {
		t.Fatal("expected policy for Bash")
	}
	if policy.Name != "bash-unknown" {
		t.Errorf("expected bash-unknown, got %s", policy.Name)
	}

	// Match Delete
	policy = wf.CheckPolicy("Delete", "HIGH")
	if policy == nil {
		t.Fatal("expected policy for Delete")
	}
	if policy.Name != "delete-always-ask" {
		t.Errorf("expected delete-always-ask, got %s", policy.Name)
	}

	// Unknown tool falls back to risk level
	policy = wf.CheckPolicy("UnknownTool", "HIGH")
	if policy == nil {
		t.Fatal("expected policy for unknown tool with HIGH risk")
	}
}

func TestCheckPolicyNoMatch(t *testing.T) {
	wf := NewApprovalWorkflow(nil)
	policy := wf.CheckPolicy("UnknownTool", "UNKNOWN_LEVEL")
	if policy != nil {
		t.Error("expected nil policy for completely unknown tool and risk")
	}
}

func TestFormatRequest(t *testing.T) {
	req := &ApprovalRequest{
		ID:          "test-123",
		Tool:        "Bash",
		Args:        map[string]interface{}{"command": "rm -rf ./build/"},
		Risk:        "HIGH",
		Description: "Execute shell command: rm -rf ./build/",
		CreatedAt:   time.Now(),
		Status:      "pending",
		ExpiresAt:   time.Now().Add(2 * time.Minute),
	}

	output := FormatRequest(req)

	if !strings.Contains(output, "Approval Required") {
		t.Error("expected 'Approval Required' in output")
	}
	if !strings.Contains(output, "Bash") {
		t.Error("expected 'Bash' in output")
	}
	if !strings.Contains(output, "rm -rf ./build/") {
		t.Error("expected command in output")
	}
	if !strings.Contains(output, "HIGH") {
		t.Error("expected risk level in output")
	}
	if !strings.Contains(output, "[y]es / [n]o / [a]lways") {
		t.Error("expected approval prompt options in output")
	}
}

func TestFormatHistory(t *testing.T) {
	history := []*ApprovalRequest{
		{
			ID:        "h1",
			Tool:      "Bash",
			Risk:      "MEDIUM",
			Status:    "approved",
			Reason:    "user said yes",
			CreatedAt: time.Now(),
		},
		{
			ID:        "h2",
			Tool:      "Delete",
			Risk:      "HIGH",
			Status:    "denied",
			Reason:    "too risky",
			CreatedAt: time.Now(),
		},
		{
			ID:        "h3",
			Tool:      "Bash",
			Risk:      "MEDIUM",
			Status:    "expired",
			Reason:    "request timed out",
			CreatedAt: time.Now(),
		},
	}

	output := FormatHistory(history, 10)
	if !strings.Contains(output, "Approval History") {
		t.Error("expected 'Approval History' header")
	}
	if !strings.Contains(output, "✓") {
		t.Error("expected approved icon")
	}
	if !strings.Contains(output, "✗") {
		t.Error("expected denied icon")
	}
	if !strings.Contains(output, "⏰") {
		t.Error("expected expired icon")
	}
}

func TestFormatHistoryWithLimit(t *testing.T) {
	history := make([]*ApprovalRequest, 5)
	for i := range history {
		history[i] = &ApprovalRequest{
			ID:        strings.Repeat("x", i+1),
			Tool:      "Bash",
			Risk:      "MEDIUM",
			Status:    "approved",
			Reason:    "ok",
			CreatedAt: time.Now(),
		}
	}

	output := FormatHistory(history, 2)
	// Should only show last 2 entries
	lines := strings.Split(strings.TrimSpace(output), "\n")
	// Header + separator + 2 entries = 4 lines
	entryCount := 0
	for _, line := range lines {
		if strings.Contains(line, "✓") {
			entryCount++
		}
	}
	if entryCount != 2 {
		t.Errorf("expected 2 entries with limit, got %d", entryCount)
	}
}

func TestFormatHistoryEmpty(t *testing.T) {
	output := FormatHistory(nil, 10)
	if !strings.Contains(output, "no history") {
		t.Error("expected 'no history' for empty list")
	}
}

func TestIsApproved(t *testing.T) {
	wf := NewApprovalWorkflow(nil)

	// Not approved - doesn't exist
	if wf.IsApproved("nonexistent") {
		t.Error("expected false for nonexistent ID")
	}

	// Auto-approved request
	req, _ := wf.RequestApproval("Read", map[string]interface{}{}, "LOW")
	if !wf.IsApproved(req.ID) {
		t.Error("expected auto-approved request to be approved")
	}
}

func TestMaxPendingEnforced(t *testing.T) {
	wf := NewApprovalWorkflow(nil)

	// The bash policy has MaxPending of 10
	// Fill up pending
	for i := 0; i < 10; i++ {
		_, err := wf.RequestApproval("Bash", map[string]interface{}{"command": "test"}, "MEDIUM")
		if err != nil {
			t.Fatalf("unexpected error on request %d: %v", i, err)
		}
	}

	// The 11th should fail
	_, err := wf.RequestApproval("Bash", map[string]interface{}{"command": "test"}, "MEDIUM")
	if err == nil {
		t.Error("expected error when max pending exceeded")
	}
	if !strings.Contains(err.Error(), "max pending") {
		t.Errorf("expected 'max pending' error, got: %v", err)
	}
}

func TestBuildDescription(t *testing.T) {
	tests := []struct {
		tool     string
		args     map[string]interface{}
		contains string
	}{
		{"Bash", map[string]interface{}{"command": "ls"}, "Execute shell command: ls"},
		{"Delete", map[string]interface{}{"path": "/tmp/x"}, "Delete: /tmp/x"},
		{"Delete", map[string]interface{}{"file_path": "/tmp/y"}, "Delete: /tmp/y"},
		{"GitPush", map[string]interface{}{"remote": "origin", "branch": "main"}, "Git push to origin/main"},
		{"GitPush", map[string]interface{}{"remote": "origin"}, "Git push to origin"},
		{"GitPush", map[string]interface{}{}, "Git push"},
		{"Edit", map[string]interface{}{"file_path": "/a/b.go"}, "Edit file: /a/b.go"},
		{"Write", map[string]interface{}{"file_path": "/a/c.go"}, "Write file: /a/c.go"},
		{"Unknown", map[string]interface{}{}, "Unknown operation"},
	}

	for _, tt := range tests {
		desc := buildDescription(tt.tool, tt.args)
		if !strings.Contains(desc, tt.contains) {
			t.Errorf("buildDescription(%s, %v) = %q, want containing %q", tt.tool, tt.args, desc, tt.contains)
		}
	}
}

func TestApproveExpiredRequest(t *testing.T) {
	wf := NewApprovalWorkflow(nil)

	// Manually insert an expired pending request
	req := &ApprovalRequest{
		ID:        "soon-expired",
		Tool:      "Bash",
		Args:      map[string]interface{}{"command": "test"},
		Risk:      "MEDIUM",
		CreatedAt: time.Now().Add(-10 * time.Minute),
		Status:    "pending",
		ExpiresAt: time.Now().Add(-1 * time.Second), // Already expired
	}
	wf.Pending = append(wf.Pending, req)

	err := wf.Approve("soon-expired", "approved")
	if err == nil {
		t.Error("expected error when approving expired request")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected 'expired' error, got: %v", err)
	}
}

func TestApprovalConcurrentAccess(t *testing.T) {
	wf := NewApprovalWorkflow(func(req *ApprovalRequest) (bool, string) {
		return true, "ok"
	})

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = wf.RequestApproval("Bash", map[string]interface{}{"command": "test"}, "MEDIUM")
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// All should be in history since promptFn approves
	if len(wf.History) != 10 {
		t.Errorf("expected 10 history entries, got %d", len(wf.History))
	}
}
