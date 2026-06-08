package mission

// Typed human-in-the-loop approval gate for mission-mode tool calls.
// Blocks execution of a tool call until the operator calls Respond().
//
// Pattern: kimi-agent-sdk go/wire/message.go + go/session.go (Apache-2.0)
// A caller receives an *ApprovalRequest via OnRequest, inspects it, and calls
// Respond() from any goroutine to unblock Await(). The gate is channel-based
// so it composes naturally with context cancellation.

import (
	"context"
	"errors"
	"strings"
)

// RequestResponse is the operator's decision for an approval request.
type RequestResponse int

const (
	// ResponseApprove allows the tool call once.
	ResponseApprove RequestResponse = iota
	// ResponseApproveForSession auto-approves subsequent calls to the same tool.
	ResponseApproveForSession
	// ResponseReject denies the tool call and causes an error event.
	ResponseReject
)

// Responder is implemented by anything that can resolve an ApprovalRequest.
type Responder interface {
	Respond(RequestResponse) error
}

// ApprovalRequest describes a tool call that is pending human approval.
// The caller receives this via MissionApprovalGate.OnRequest and must call
// Respond() to unblock the waiting mission worker.
type ApprovalRequest struct {
	// ToolName is the canonical name of the tool being called.
	ToolName string
	// Args are the tool call arguments as a map.
	Args map[string]interface{}
	// Summary is a one-line human-readable description of the action.
	Summary string
	// Category is the risk category matched by the gate classifier.
	Category string

	respond chan RequestResponse
}

// Respond sends a decision to the waiting Await() call. It is safe to call
// from any goroutine. Subsequent Respond() calls after the first are no-ops.
func (r *ApprovalRequest) Respond(resp RequestResponse) error {
	select {
	case r.respond <- resp:
	default:
	}
	return nil
}

// Await blocks until Respond() is called or ctx is cancelled.
// Returns ErrApprovalTimeout when ctx expires before the operator responds.
func (r *ApprovalRequest) Await(ctx context.Context) (RequestResponse, error) {
	select {
	case resp := <-r.respond:
		return resp, nil
	case <-ctx.Done():
		return ResponseReject, ErrApprovalTimeout
	}
}

// ErrApprovalTimeout is returned when the context expires before the operator
// calls Respond().
var ErrApprovalTimeout = errors.New("approval request timed out: context cancelled")

// ErrToolRejected is returned from the gate check when the operator rejects a
// tool call.
var ErrToolRejected = errors.New("tool call rejected by human approval gate")

// MissionApprovalGate wraps a mission's tool-call dispatch with the typed
// channel-based gate. When OnRequest is non-nil and a tool call matches a
// flagged category, OnRequest is invoked with an *ApprovalRequest; the worker
// goroutine blocks on Await() until the operator calls Respond(). If OnRequest
// is nil the gate is a no-op (auto-approve everything).
//
// SessionApproved tracks tools approved for the entire session via
// ResponseApproveForSession; subsequent calls to those tools skip the gate.
type MissionApprovalGate struct {
	// OnRequest is called synchronously in the worker goroutine before the tool
	// runs. The implementation must not call Await() itself — it should hand the
	// *ApprovalRequest to an operator UI and return immediately.
	OnRequest func(req *ApprovalRequest)

	// sessionApproved is the set of tool names auto-approved for this session.
	sessionApproved map[string]bool
}

// NewMissionApprovalGate creates a gate with the given OnRequest handler.
// Pass nil to create a no-op gate.
func NewMissionApprovalGate(onRequest func(req *ApprovalRequest)) *MissionApprovalGate {
	return &MissionApprovalGate{
		OnRequest:       onRequest,
		sessionApproved: make(map[string]bool),
	}
}

// Check consults the gate for a pending tool call. If the gate is enabled and
// the tool matches a risky category, it calls OnRequest and blocks until the
// operator responds. Returns an error if the call is rejected or the context
// expires; nil means proceed.
func (g *MissionApprovalGate) Check(ctx context.Context, toolName string, args map[string]interface{}) error {
	if g == nil || g.OnRequest == nil {
		return nil
	}

	cat, risky := classifyMissionAction(toolName, args)
	if !risky {
		return nil
	}

	// Session-level auto-approval (ResponseApproveForSession was used before).
	if g.sessionApproved[toolName] {
		return nil
	}

	req := &ApprovalRequest{
		ToolName: toolName,
		Args:     args,
		Summary:  missionApprovalSummary(toolName, args),
		Category: cat,
		respond:  make(chan RequestResponse, 1),
	}

	g.OnRequest(req)

	resp, err := req.Await(ctx)
	if err != nil {
		return err
	}

	switch resp {
	case ResponseApprove:
		return nil
	case ResponseApproveForSession:
		g.sessionApproved[toolName] = true
		return nil
	case ResponseReject:
		return ErrToolRejected
	default:
		return ErrToolRejected
	}
}

// classifyMissionAction mirrors the category heuristics from
// engine/approval_gate.go so the multiagent gate reuses the same risk model
// without importing the engine package (which would create a cycle).
func classifyMissionAction(toolName string, args map[string]interface{}) (string, bool) {
	canon := missionCanonicalTool(toolName)

	switch canon {
	case "WebFetch", "WebSearch":
		return "network", true
	case "Bash":
		if cmd, ok := args["command"].(string); ok {
			if missionIsDestructiveDelete(cmd) {
				return "file_deletion", true
			}
			if missionIsNetworkCommand(cmd) {
				return "network", true
			}
		}
	}
	return "", false
}

func missionCanonicalTool(name string) string {
	// Strip any namespace prefix (e.g. "mcp__hawk__Bash" -> "Bash").
	if idx := strings.LastIndex(name, "__"); idx >= 0 {
		return name[idx+2:]
	}
	return name
}

func missionIsDestructiveDelete(cmd string) bool {
	c := strings.ToLower(cmd)
	for _, pat := range []string{"rm -rf", "rm -r ", "rm -f", "rm ", "rmdir ", "unlink ", "shred "} {
		if strings.Contains(c, pat) {
			return true
		}
	}
	return false
}

func missionIsNetworkCommand(cmd string) bool {
	c := strings.ToLower(cmd)
	for _, pat := range []string{
		"curl ", "wget ", "nc ", "ssh ", "scp ", "ftp ",
		"git push", "git pull", "git fetch",
		"npm install", "pip install", "go get ",
	} {
		if strings.Contains(c, pat) {
			return true
		}
	}
	return false
}

func missionApprovalSummary(toolName string, args map[string]interface{}) string {
	if cmd, ok := args["command"].(string); ok && cmd != "" {
		return missionCanonicalTool(toolName) + ": " + cmd
	}
	return missionCanonicalTool(toolName)
}
