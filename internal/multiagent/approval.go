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
	"sync"
)

// RequestResponse is the operator's decision for an approval request.
type RequestResponse int

const (
	// ResponseApprove allows the tool call once.
	ResponseApprove RequestResponse = iota
	// ResponseApproveForSession auto-approves subsequent calls to the same tool.
	ResponseApproveForSession
	// ResponseApproveForN auto-approves the next N calls to the same tool.
	ResponseApproveForN
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
	// N is the number of approvals granted when the human responds
	// ResponseApproveForN. Defaults to 5 when unset (0).
	N int

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

	// mu guards sessionApproved and nApproved; Check may run from many worker
	// goroutines.
	mu              sync.Mutex
	sessionApproved map[string]bool
	nApproved       map[string]int
}

// NewMissionApprovalGate creates a gate with the given OnRequest handler.
// Pass nil to create a no-op gate.
func NewMissionApprovalGate(onRequest func(req *ApprovalRequest)) *MissionApprovalGate {
	return &MissionApprovalGate{
		OnRequest:       onRequest,
		sessionApproved: make(map[string]bool),
	}
}

// DenyAllGate returns an approval gate that deterministically rejects any
// tool call matching an approval requirement without waiting on interactive input.
func DenyAllGate() *MissionApprovalGate {
	return NewMissionApprovalGate(func(req *ApprovalRequest) {
		_ = req.Respond(ResponseReject)
	})
}

// Check consults the gate for a pending tool call. If the gate is enabled and
// the tool matches a risky category, it calls OnRequest and blocks until the
// operator responds. Returns an error if the call is rejected or the context
// expires; nil means proceed.
func (g *MissionApprovalGate) Check(ctx context.Context, toolName, summary string) error {
	if g == nil || g.OnRequest == nil {
		return nil
	}

	cat, risky := classifyMissionAction(toolName, summary)
	if !risky {
		return nil
	}

	// Session-level auto-approval (ResponseApproveForSession was used before).
	g.mu.Lock()
	approved := g.sessionApproved[toolName]
	nRemaining := g.nApproved[toolName]
	g.mu.Unlock()
	if approved {
		return nil
	}
	// N-count auto-approval (ResponseApproveForN was used before). Decrement
	// under lock so concurrent workers don't double-spend.
	if nRemaining > 0 {
		g.mu.Lock()
		if g.nApproved[toolName] > 0 {
			g.nApproved[toolName]--
		}
		g.mu.Unlock()
		return nil
	}

	req := &ApprovalRequest{
		ToolName: toolName,
		Summary:  missionApprovalSummary(toolName, summary),
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
		g.mu.Lock()
		g.sessionApproved[toolName] = true
		g.mu.Unlock()
		return nil
	case ResponseApproveForN:
		g.mu.Lock()
		// Default N=5 when the response carries no count.
		g.nApproved[toolName] += req.N
		g.mu.Unlock()
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
// summary carries the human-readable command/args text from the permission
// request; it is empty when no detail is available.
func classifyMissionAction(toolName, summary string) (string, bool) {
	canon := missionCanonicalTool(toolName)

	switch canon {
	case "WebFetch", "WebSearch":
		return "network", true
	case "Bash":
		if missionIsDestructiveDelete(summary) {
			return "file_deletion", true
		}
		if missionIsNetworkCommand(summary) {
			return "network", true
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

func missionApprovalSummary(toolName, summary string) string {
	if summary != "" {
		return missionCanonicalTool(toolName) + ": " + summary
	}
	return missionCanonicalTool(toolName)
}
