package permissions

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// ApprovalRequest represents a request for approval of a high-risk operation.
type ApprovalRequest struct {
	ID          string
	Tool        string
	Args        map[string]interface{}
	Risk        string
	Description string
	CreatedAt   time.Time
	Status      string // "pending", "approved", "denied", "expired"
	ExpiresAt   time.Time
	Reason      string
	// DecisionAt and PauseDuration record how long the human deliberated before
	// deciding, for approval-latency observability (adopted from herm).
	DecisionAt    time.Time
	PauseDuration time.Duration
}

// ApprovalPolicy defines rules for how approval requests are handled.
type ApprovalPolicy struct {
	Name          string
	Tools         []string
	RiskLevel     string
	AutoApprove   bool
	RequireReason bool
	Timeout       time.Duration
	MaxPending    int
}

// ApprovalWorkflow manages the approval process for destructive or high-risk operations.
type ApprovalWorkflow struct {
	Policies []ApprovalPolicy
	Pending  []*ApprovalRequest
	History  []*ApprovalRequest
	PromptFn func(*ApprovalRequest) (bool, string)
	mu       sync.Mutex
	idSeq    int
}

// NewApprovalWorkflow creates an ApprovalWorkflow with default policies and the given prompt function.
func NewApprovalWorkflow(promptFn func(*ApprovalRequest) (bool, string)) *ApprovalWorkflow {
	wf := &ApprovalWorkflow{
		PromptFn: promptFn,
		Pending:  make([]*ApprovalRequest, 0),
		History:  make([]*ApprovalRequest, 0),
	}

	// Default policies
	wf.Policies = []ApprovalPolicy{
		{
			Name:        "read-auto-approve",
			Tools:       []string{"Read", "Search", "Grep", "List"},
			RiskLevel:   "LOW",
			AutoApprove: true,
			Timeout:     5 * time.Minute,
			MaxPending:  100,
		},
		{
			Name:        "edit-write-in-project",
			Tools:       []string{"Edit", "Write"},
			RiskLevel:   "MEDIUM",
			AutoApprove: true,
			Timeout:     5 * time.Minute,
			MaxPending:  50,
		},
		{
			Name:          "bash-unknown",
			Tools:         []string{"Bash"},
			RiskLevel:     "MEDIUM",
			AutoApprove:   false,
			RequireReason: false,
			Timeout:       2 * time.Minute,
			MaxPending:    10,
		},
		{
			Name:          "delete-always-ask",
			Tools:         []string{"Delete", "Remove"},
			RiskLevel:     "HIGH",
			AutoApprove:   false,
			RequireReason: true,
			Timeout:       2 * time.Minute,
			MaxPending:    5,
		},
		{
			Name:          "git-push-always-ask",
			Tools:         []string{"GitPush"},
			RiskLevel:     "HIGH",
			AutoApprove:   false,
			RequireReason: true,
			Timeout:       2 * time.Minute,
			MaxPending:    5,
		},
	}

	return wf
}

// RequestApproval creates an approval request for the given tool invocation.
// If the matching policy auto-approves, the request is approved immediately.
// Otherwise, the PromptFn is called to ask the user.
func (wf *ApprovalWorkflow) RequestApproval(tool string, args map[string]interface{}, risk string) (*ApprovalRequest, error) {
	wf.mu.Lock()
	defer wf.mu.Unlock()

	policy := wf.checkPolicyLocked(tool, risk)

	wf.idSeq++
	id := fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), wf.idSeq)

	timeout := 2 * time.Minute
	if policy != nil {
		timeout = policy.Timeout
	}

	description := buildDescription(tool, args)

	req := &ApprovalRequest{
		ID:          id,
		Tool:        tool,
		Args:        args,
		Risk:        risk,
		Description: description,
		CreatedAt:   time.Now(),
		Status:      "pending",
		ExpiresAt:   time.Now().Add(timeout),
	}

	// Check if auto-approve applies
	if policy != nil && policy.AutoApprove {
		req.Status = "approved"
		req.Reason = "auto-approved by policy: " + policy.Name
		wf.History = append(wf.History, req)
		return req, nil
	}

	// Check max pending
	if policy != nil && policy.MaxPending > 0 {
		pendingCount := 0
		for _, p := range wf.Pending {
			if p.Tool == tool && p.Status == "pending" {
				pendingCount++
			}
		}
		if pendingCount >= policy.MaxPending {
			return nil, fmt.Errorf("max pending requests (%d) reached for tool %s", policy.MaxPending, tool)
		}
	}

	// Call the prompt function if provided
	if wf.PromptFn != nil {
		approved, reason := wf.PromptFn(req)
		if approved {
			req.Status = "approved"
			req.Reason = reason
			wf.History = append(wf.History, req)
		} else {
			req.Status = "denied"
			req.Reason = reason
			wf.History = append(wf.History, req)
		}
		return req, nil
	}

	// No prompt function; leave as pending
	wf.Pending = append(wf.Pending, req)
	return req, nil
}

// Approve approves the pending request with the given ID.
func (wf *ApprovalWorkflow) Approve(id, reason string) error {
	wf.mu.Lock()
	defer wf.mu.Unlock()

	for i, req := range wf.Pending {
		if req.ID == id {
			if req.Status != "pending" {
				return fmt.Errorf("request %s is not pending (status: %s)", id, req.Status)
			}
			if time.Now().After(req.ExpiresAt) {
				req.Status = "expired"
				wf.Pending = append(wf.Pending[:i], wf.Pending[i+1:]...)
				wf.History = append(wf.History, req)
				return fmt.Errorf("request %s has expired", id)
			}
			req.Status = "approved"
			req.Reason = reason
			req.DecisionAt = time.Now()
			req.PauseDuration = req.DecisionAt.Sub(req.CreatedAt)
			wf.Pending = append(wf.Pending[:i], wf.Pending[i+1:]...)
			wf.History = append(wf.History, req)
			return nil
		}
	}
	return fmt.Errorf("request %s not found", id)
}

// Deny denies the pending request with the given ID.
func (wf *ApprovalWorkflow) Deny(id, reason string) error {
	wf.mu.Lock()
	defer wf.mu.Unlock()

	for i, req := range wf.Pending {
		if req.ID == id {
			if req.Status != "pending" {
				return fmt.Errorf("request %s is not pending (status: %s)", id, req.Status)
			}
			req.Status = "denied"
			req.Reason = reason
			req.DecisionAt = time.Now()
			req.PauseDuration = req.DecisionAt.Sub(req.CreatedAt)
			wf.Pending = append(wf.Pending[:i], wf.Pending[i+1:]...)
			wf.History = append(wf.History, req)
			return nil
		}
	}
	return fmt.Errorf("request %s not found", id)
}

// IsApproved returns true if the request with the given ID has been approved.
func (wf *ApprovalWorkflow) IsApproved(id string) bool {
	wf.mu.Lock()
	defer wf.mu.Unlock()

	for _, req := range wf.History {
		if req.ID == id && req.Status == "approved" {
			return true
		}
	}
	for _, req := range wf.Pending {
		if req.ID == id && req.Status == "approved" {
			return true
		}
	}
	return false
}

// GetPending returns all currently pending approval requests.
func (wf *ApprovalWorkflow) GetPending() []*ApprovalRequest {
	wf.mu.Lock()
	defer wf.mu.Unlock()

	result := make([]*ApprovalRequest, 0, len(wf.Pending))
	for _, req := range wf.Pending {
		if req.Status == "pending" {
			result = append(result, req)
		}
	}
	return result
}

// AddPolicy adds a new approval policy to the workflow.
func (wf *ApprovalWorkflow) AddPolicy(policy ApprovalPolicy) {
	wf.mu.Lock()
	defer wf.mu.Unlock()

	wf.Policies = append(wf.Policies, policy)
}

// CheckPolicy finds the matching policy for a tool and risk level.
func (wf *ApprovalWorkflow) CheckPolicy(tool string, risk string) *ApprovalPolicy {
	wf.mu.Lock()
	defer wf.mu.Unlock()

	return wf.checkPolicyLocked(tool, risk)
}

func (wf *ApprovalWorkflow) checkPolicyLocked(tool string, risk string) *ApprovalPolicy {
	// First try to match by tool name
	for i := range wf.Policies {
		for _, t := range wf.Policies[i].Tools {
			if strings.EqualFold(t, tool) {
				return &wf.Policies[i]
			}
		}
	}

	// Then try to match by risk level
	for i := range wf.Policies {
		if strings.EqualFold(wf.Policies[i].RiskLevel, risk) {
			return &wf.Policies[i]
		}
	}

	return nil
}

// FormatRequest formats an approval request for display to the user.
func FormatRequest(req *ApprovalRequest) string {
	var sb strings.Builder
	sb.WriteString(icons.Alert() + " Approval Required:\n")
	sb.WriteString(fmt.Sprintf("  Tool: %s\n", req.Tool))

	// Show relevant args
	if cmd, ok := req.Args["command"]; ok {
		sb.WriteString(fmt.Sprintf("  Command: %v\n", cmd))
	} else if len(req.Args) > 0 {
		for k, v := range req.Args {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
			break // Show only the first arg in compact form
		}
	}

	if req.Description != "" {
		sb.WriteString(fmt.Sprintf("  Description: %s\n", req.Description))
	}

	sb.WriteString(fmt.Sprintf("  Risk: %s\n", req.Risk))
	sb.WriteString("\n  [y]es / [n]o / [a]lways allow this pattern?")

	return sb.String()
}

// FormatHistory formats the approval history for display.
func FormatHistory(history []*ApprovalRequest, limit int) string {
	var sb strings.Builder
	sb.WriteString("Approval History:\n")
	sb.WriteString(strings.Repeat("-", 60) + "\n")

	count := len(history)
	start := 0
	if limit > 0 && limit < count {
		start = count - limit
	}

	if count == 0 {
		sb.WriteString("  (no history)\n")
		return sb.String()
	}

	for i := start; i < count; i++ {
		req := history[i]
		statusIcon := " "
		switch req.Status {
		case "approved":
			statusIcon = icons.CheckBold()
		case "denied":
			statusIcon = icons.CloseThick()
		case "expired":
			statusIcon = "[time]"
		}

		sb.WriteString(fmt.Sprintf(
			"  [%s] %s | %s | %s | %s\n",
			statusIcon,
			req.CreatedAt.Format("15:04:05"),
			req.Tool,
			req.Risk,
			req.Reason,
		))
	}

	return sb.String()
}

// ExpirePending marks all expired pending requests.
func (wf *ApprovalWorkflow) ExpirePending() {
	wf.mu.Lock()
	defer wf.mu.Unlock()

	now := time.Now()
	remaining := make([]*ApprovalRequest, 0)

	for _, req := range wf.Pending {
		if req.Status == "pending" && now.After(req.ExpiresAt) {
			req.Status = "expired"
			req.Reason = "request timed out"
			wf.History = append(wf.History, req)
		} else {
			remaining = append(remaining, req)
		}
	}

	wf.Pending = remaining
}

// buildDescription creates a human-readable description from tool and args.
func buildDescription(tool string, args map[string]interface{}) string {
	switch strings.ToLower(tool) {
	case "bash":
		if cmd, ok := args["command"]; ok {
			return fmt.Sprintf("Execute shell command: %v", cmd)
		}
	case "delete", "remove":
		if path, ok := args["path"]; ok {
			return fmt.Sprintf("Delete: %v", path)
		}
		if path, ok := args["file_path"]; ok {
			return fmt.Sprintf("Delete: %v", path)
		}
	case "gitpush":
		if remote, ok := args["remote"]; ok {
			if branch, ok2 := args["branch"]; ok2 {
				return fmt.Sprintf("Git push to %v/%v", remote, branch)
			}
			return fmt.Sprintf("Git push to %v", remote)
		}
		return "Git push"
	case "edit":
		if path, ok := args["file_path"]; ok {
			return fmt.Sprintf("Edit file: %v", path)
		}
	case "write":
		if path, ok := args["file_path"]; ok {
			return fmt.Sprintf("Write file: %v", path)
		}
	}
	return fmt.Sprintf("%s operation", tool)
}
