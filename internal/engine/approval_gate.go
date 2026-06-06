package engine

import (
	"context"
	"strings"
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
	// ConfirmFn asks the human to confirm an action. It returns true to allow.
	// When nil the gate falls back to Session.AskUserFn, and if that is also nil
	// the action is denied (fail-closed) so a misconfigured gate never silently
	// auto-approves a high-risk action.
	ConfirmFn func(req ApprovalRequest) bool
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

// CheckApproval consults the approval gate for a tool call. It returns
// (allowed, denyMessage). When the gate is disabled, the action is not
// high-risk, or the autonomy level is within the auto-approve threshold, it
// returns (true, ""). Otherwise it prompts the human and honors their decision.
//
// This is invoked from the tool execution pipeline after the normal permission
// check succeeds; it never loosens an existing denial.
func (s *Session) CheckApproval(_ context.Context, toolName string, args map[string]interface{}) (bool, string) {
	g := s.Approval
	if g == nil || !g.Enabled {
		return true, ""
	}

	cat, risky := g.classifyAction(toolName, args)
	if !risky || !g.categoryEnabled(cat) {
		return true, ""
	}

	// Within the auto-approve threshold the operator has opted into automation.
	if s.Autonomy <= g.MaxAutoApprove {
		return true, ""
	}

	req := ApprovalRequest{
		ToolName: canonicalToolName(toolName),
		Category: cat,
		Summary:  approvalSummary(toolName, args),
		Args:     args,
	}

	if g.ConfirmFn != nil {
		if g.ConfirmFn(req) {
			return true, ""
		}
		return false, "Action denied by human approval gate (" + string(cat) + ")."
	}

	// Fall back to the session's generic ask-user callback.
	if s.AskUserFn != nil {
		q := "Approve high-risk action [" + string(cat) + "]: " + req.Summary + "? (yes/no)"
		ans, err := s.AskUserFn(q)
		if err == nil && isAffirmative(ans) {
			return true, ""
		}
		return false, "Action denied by human approval gate (" + string(cat) + ")."
	}

	// No way to ask: fail closed.
	return false, "High-risk action requires approval but no confirmation handler is configured (" + string(cat) + ")."
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
