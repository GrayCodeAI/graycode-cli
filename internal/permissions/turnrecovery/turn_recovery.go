// Package turnrecovery ports fx's per-agent-turn permission recovery
// (vercel-labs/fx, src/core/agent/runtime/tool_admission.zig).
//
// The invariant it preserves is the strongest part of fx's security model:
// a tool call denied by the auto/permission classifier can only re-enter the
// human permission screen through the exact, non-guessable opaque request id
// issued for that particular denial in this agent turn. Generic model or
// user text ("please approve", "allow it") can never authorize the action.
//
// The flow:
//   - RememberAutoDenial registers a denied call and returns its opaque
//     request id. The id is the only handle the model may present later.
//   - PreservedOutcome re-denies a later call that is semantically identical
//     to a still-unapproved denial, so the model cannot defeat the gate by
//     re-issuing the same command with cosmetic differences.
//   - RememberApproval binds a human approval to the request id.
//   - TakeApproval is the live, single-use revalidation performed immediately
//     before execution: it matches the exact original call, returns the
//     approval credentials exactly once, and marks them consumed.
//
// IDs are domain-prefixed SHA-256 digests, matching fx's derivation so the
// semantics (not the bytes) are portable.
package turnrecovery

import (
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"strings"
)

// ID is a 256-bit opaque permission action/request id.
type ID [32]byte

// maxTurnDenials caps how many auto-denied calls a single agent turn tracks,
// matching fx's bound and preventing the registry from unbounded growth.
const maxTurnDenials = 64

// ToolCall is the minimal identity of a tool invocation.
type ToolCall struct {
	Name          string
	ArgumentsJSON string
}

// Hash returns the lowercase 64-character hex rendering of the id.
func (id ID) Hash() string {
	const hexDigits = "0123456789abcdef"
	var buf [64]byte
	for i, b := range id {
		buf[i*2] = hexDigits[b>>4]
		buf[i*2+1] = hexDigits[b&0x0f]
	}
	return string(buf[:])
}

func digest(domain string, parts ...string) ID {
	h := sha256.New()
	h.Write([]byte(domain))
	for _, p := range parts {
		h.Write([]byte(p))
	}
	var out ID
	copy(out[:], h.Sum(nil))
	return out
}

// ActionID returns the deterministic digest of the exact tool call.
func ActionID(call ToolCall) ID {
	return digest("fx.permission-action.v1\x00", call.Name, "\x00", call.ArgumentsJSON)
}

func requestID(sequence uint64) ID {
	return digest("fx.permission-request.v1:" + strconv.FormatUint(sequence, 10))
}

// SemanticActionID returns an id stable under cosmetic changes that carry no
// semantic weight for a command run (shell-wrapper prefixes, " 2>&1"
// suffix). For non-command tools it falls back to the exact ActionID.
func SemanticActionID(workspaceRoot string, call ToolCall) ID {
	if call.Name != "" {
		var args map[string]any
		if err := json.Unmarshal([]byte(call.ArgumentsJSON), &args); err == nil {
			if command, ok := args["command"].(string); ok {
				cwd, ok := args["cwd"].(string)
				if !ok || cwd == "." {
					cwd = workspaceRoot
				}
				return digest(
					"",
					"command\x00", normalizeCommand(command),
					"\x00cwd\x00", cwd,
				)
			}
		}
	}
	return ActionID(call)
}

// normalizeCommand strips shell-wrapper prefixes and stderr redirection so a
// command re-issued through a different wrapper still shares a semantic id.
func normalizeCommand(command string) string {
	normalized := strings.Trim(command, " \t\r\n")
	const redirect = " 2>&1"
	if strings.HasSuffix(normalized, redirect) {
		normalized = strings.TrimRight(normalized[:len(normalized)-len(redirect)], " \t")
	}
	for _, prefix := range []string{
		"sh -c '", "bash -c '", "zsh -c '",
		"/bin/sh -c '", "/bin/bash -c '", "/bin/zsh -c '",
	} {
		if strings.HasPrefix(normalized, prefix) &&
			strings.HasSuffix(normalized, "'") &&
			len(normalized) > len(prefix) {
			return strings.Trim(normalized[len(prefix):len(normalized)-1], " \t\r\n")
		}
	}
	return normalized
}

// Approval carries the authority credentials granted by a human approval.
type Approval struct {
	Authority     string
	HumanApproval bool
}

// approvedAction is the single-use approval bound to a denied entry.
type approvedAction struct {
	approval Approval
	consumed bool
}

type deniedEntry struct {
	requestID  ID
	exactID    ID
	semanticID ID
	call       ToolCall
	approval   *approvedAction
}

// Recovery is the per-agent-turn registry of auto-denied tool calls.
type Recovery struct {
	denied         []deniedEntry
	nextRequestSeq uint64
}

func New() *Recovery {
	return &Recovery{nextRequestSeq: 1}
}

// RememberAutoDenial registers an auto-denied call and returns its opaque
// request id, or ok=false when the call is not auto-denied or the turn budget
// is exhausted. A duplicate exact call returns the already-issued request id
// rather than registering a second entry.
func (r *Recovery) RememberAutoDenial(workspaceRoot string, call ToolCall) (ID, bool) {
	exactID := ActionID(call)
	for i := range r.denied {
		if r.denied[i].exactID == exactID {
			return r.denied[i].requestID, true
		}
	}
	if len(r.denied) >= maxTurnDenials {
		return ID{}, false
	}
	id := requestID(r.nextRequestSeq)
	r.nextRequestSeq++
	r.denied = append(r.denied, deniedEntry{
		requestID:  id,
		exactID:    exactID,
		semanticID: SemanticActionID(workspaceRoot, call),
		call:       call,
	})
	return id, true
}

// DeniedCall returns the exact call bound to the given opaque request id.
// ok is false when no such denial is pending in this turn.
func (r *Recovery) DeniedCall(id ID) (ToolCall, bool) {
	for i := range r.denied {
		if r.denied[i].requestID == id {
			return r.denied[i].call, true
		}
	}
	return ToolCall{}, false
}

// PreservedOutcome reports whether the call is semantically identical to a
// still-pending auto-denial, in which case it must be denied again.
func (r *Recovery) PreservedOutcome(workspaceRoot string, call ToolCall) bool {
	semantic := SemanticActionID(workspaceRoot, call)
	for i := range r.denied {
		if r.denied[i].semanticID == semantic && r.denied[i].approval == nil {
			return true
		}
	}
	return false
}

// RememberApproval binds a human approval to the request id. It returns false
// when there is no matching pending denial or no real human approval.
func (r *Recovery) RememberApproval(id ID, approval Approval) bool {
	if !approval.HumanApproval {
		return false
	}
	for i := range r.denied {
		if r.denied[i].requestID == id {
			a := approvedAction{approval: approval}
			r.denied[i].approval = &a
			return true
		}
	}
	return false
}

// TakeApproval is the live, single-use revalidation performed immediately
// before execution. It matches the exact original call, returns the approval
// credentials exactly once, and marks them consumed.
func (r *Recovery) TakeApproval(call ToolCall) (Approval, bool) {
	exactID := ActionID(call)
	for i := range r.denied {
		e := &r.denied[i]
		if e.exactID != exactID || e.approval == nil {
			continue
		}
		if e.approval.consumed {
			return Approval{}, false
		}
		e.approval.consumed = true
		return e.approval.approval, true
	}
	return Approval{}, false
}
