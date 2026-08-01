package engine

import (
	"context"
	"strings"
	"sync"
)

// approval_gate.go implements human-in-the-loop approval gates for high-risk
// tool actions (Task B).
//
// This is an ADDITIVE layer on top of the existing permission engine
// (PermissionEngine.CheckTool). It introduces a config-driven gate that, for
// explicitly flagged high-risk actions (file deletion, network calls, external
// API), pauses and requires an explicit human confirmation — even when the
// active autonomy level would otherwise auto-allow the action.
//
// Default behavior is unchanged: with a nil/empty ApprovalGate the gate is a
// no-op, so existing autonomy levels behave exactly as before.

// ApprovalCategory classifies a high-risk action for gating purposes.
type ApprovalCategory string

const (
	ApprovalFileDeletion ApprovalCategory = "file_deletion"
	ApprovalNetwork      ApprovalCategory = "network"
	ApprovalExternalAPI  ApprovalCategory = "external_api"
)

// ApprovalResponse is the typed result of a human approval gate decision.
// It extends the previous bool return with an ApproveForSession option that
// caches the decision for the session lifetime so the human is not prompted
// again for the same category.
type ApprovalResponse int

const (
	// ApprovalReject denies the action.
	ApprovalReject ApprovalResponse = iota
	// ApprovalApprove allows this one action.
	ApprovalApprove
	// ApprovalApproveForSession allows all future actions of the same category
	// within this session without prompting again.
	ApprovalApproveForSession
)

// ApprovalGate is a config-driven human-in-the-loop gate. It is consulted after
// the normal permission check passes, for tools/actions that match one of the
// configured high-risk categories. When the active autonomy level is at or
// below MaxAutoApprove the gate is skipped (the operator has opted into
// automation at that level); above it, the gate requires explicit confirmation.
type ApprovalGate struct {
	// Enabled turns the gate on. When false the gate is a no-op.
	Enabled bool
	// Categories is the set of high-risk categories that require approval. An
	// empty set with Enabled=true defaults to all known categories.
	Categories map[ApprovalCategory]bool
	// FlaggedTools maps canonical tool names to a category that always requires
	// approval regardless of argument heuristics (e.g. a custom "DeployTool").
	FlaggedTools map[string]ApprovalCategory
	// MaxAutoApprove is the highest autonomy level at which flagged actions are
	// auto-approved without prompting. Levels strictly above this still gate.
	// Defaults to the zero value (AutonomySupervised), meaning every level above
	// supervised gates flagged actions.
	MaxAutoApprove AutonomyLevel
	// ConfirmFn asks the human to confirm an action. The typed ApprovalResponse
	// lets callers return ApprovalApproveForSession to skip future prompts for
	// the same category. When nil the gate falls back to Session.AskUserFn.
	// If both are nil the action is denied (fail-closed).
	ConfirmFn func(req ApprovalRequest) ApprovalResponse

	// sessionApprovals caches categories the human approved for the full session.
	sessionMu       sync.Mutex
	sessionApproved map[ApprovalCategory]bool
}

// ApprovalRequest describes a gated action presented to the human.
type ApprovalRequest struct {
	ToolName string
	Category ApprovalCategory
	Summary  string
	Args     map[string]interface{}
}

// categoryEnabled reports whether the gate covers a given category.
func (g *ApprovalGate) categoryEnabled(c ApprovalCategory) bool {
	if len(g.Categories) == 0 {
		return true // Enabled with no explicit categories => all categories
	}
	return g.Categories[c]
}

// classifyAction determines whether a tool call is a high-risk action and, if
// so, which category it falls under. The second return is false when the action
// is not high-risk.
func (g *ApprovalGate) classifyAction(toolName string, args map[string]interface{}) (ApprovalCategory, bool) {
	canon := canonicalToolName(toolName)

	if g.FlaggedTools != nil {
		if cat, ok := g.FlaggedTools[canon]; ok {
			return cat, true
		}
	}

	switch canon {
	case "WebFetch", "WebSearch":
		return ApprovalNetwork, true
	case "Bash":
		if cmd, ok := args["command"].(string); ok {
			if isDestructiveDelete(cmd) {
				return ApprovalFileDeletion, true
			}
			if isNetworkCommand(cmd) {
				return ApprovalNetwork, true
			}
		}
	}
	return "", false
}

// isDestructiveDelete heuristically detects file-deletion commands.
func isDestructiveDelete(cmd string) bool {
	c := strings.ToLower(cmd)
	for _, pat := range []string{"rm -rf", "rm -r ", "rm -f", "rm ", "rmdir ", "unlink ", "shred "} {
		if strings.Contains(c, pat) {
			return true
		}
	}
	return false
}

// isNetworkCommand heuristically detects commands that make network calls.
func isNetworkCommand(cmd string) bool {
	c := strings.ToLower(cmd)
	for _, pat := range []string{"curl ", "wget ", "nc ", "ssh ", "scp ", "ftp ", "git push", "git pull", "git fetch", "npm install", "pip install", "go get "} {
		if strings.Contains(c, pat) {
			return true
		}
	}
	return false
}

// sessionApprove records a session-wide approval for a category.
func (g *ApprovalGate) sessionApprove(cat ApprovalCategory) {
	g.sessionMu.Lock()
	defer g.sessionMu.Unlock()
	if g.sessionApproved == nil {
		g.sessionApproved = make(map[ApprovalCategory]bool)
	}
	g.sessionApproved[cat] = true
}

// isSessionApproved returns true if the category was previously approved for
// the full session.
func (g *ApprovalGate) isSessionApproved(cat ApprovalCategory) bool {
	g.sessionMu.Lock()
	defer g.sessionMu.Unlock()
	return g.sessionApproved[cat]
}

// CheckApproval consults the approval gate for a tool call. It returns
// (allowed, denyMessage). When the gate is disabled, the action is not
// high-risk, or the autonomy level is within the auto-approve threshold, it
// returns (true, ""). Otherwise it prompts the human and honors their decision.
//
// This is invoked from the tool execution pipeline after the normal permission
// check succeeds; it never loosens an existing denial.
func (s *Session) CheckApproval(ctx context.Context, toolName string, args map[string]interface{}) (bool, string) {
	if s == nil || s.PermSvc() == nil {
		return false, "permission service is unavailable"
	}
	s.syncPermissionCompatibility()
	return s.PermSvc().CheckApproval(ctx, toolName, args)
}

func approvalSummary(toolName string, args map[string]interface{}) string {
	if cmd, ok := args["command"].(string); ok && cmd != "" {
		return canonicalToolName(toolName) + ": " + cmd
	}
	return canonicalToolName(toolName)
}

func isAffirmative(ans string) bool {
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "y", "yes", "approve", "ok", "allow", "true":
		return true
	}
	return false
}
