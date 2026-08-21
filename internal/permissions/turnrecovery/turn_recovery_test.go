package turnrecovery

import (
	"strings"
	"testing"
)

// exactCall is a convenience for building tool calls.
func exactCall(name, args string) ToolCall {
	return ToolCall{Name: name, ArgumentsJSON: args}
}

func TestActionIDDeterministicAndDistinct(t *testing.T) {
	a := ActionID(exactCall("bash", `{"command":"ls"}`))
	b := ActionID(exactCall("bash", `{"command":"ls"}`))
	if a != b {
		t.Fatalf("ActionID must be deterministic: %x != %x", a, b)
	}
	c := ActionID(exactCall("bash", `{"command":"ls -la"}`))
	if a == c {
		t.Fatalf("distinct calls must not share an ActionID")
	}
}

func TestRequestIDIsOpaqueSequenceDigest(t *testing.T) {
	r1 := New()
	_, ok1 := r1.RememberAutoDenial("/ws", exactCall("bash", `{"command":"rm -rf x"}`))
	if !ok1 {
		t.Fatal("first denial must register")
	}
	if r1.denied[0].requestID != requestID(1) {
		t.Fatalf("first request id must be seq 1 digest")
	}
}

// The core opaque-token invariant: two denials in one turn get non-guessable,
// distinct request ids, and no two denials share one.
func TestDenialsGetDistinctOpaqueIds(t *testing.T) {
	r := New()
	idA, _ := r.RememberAutoDenial("/ws", exactCall("bash", `{"command":"a"}`))
	idB, _ := r.RememberAutoDenial("/ws", exactCall("bash", `{"command":"b"}`))
	if idA == idB {
		t.Fatal("distinct denials must not share a request id")
	}
	// The id derives from the sequence, not from the call, so it does not
	// leak the action being gated.
	if strings.Contains(idA.Hash(), "command") {
		t.Fatal("request id must be opaque, not derived from call content")
	}
}

func TestDuplicateExactCallReusesRequestId(t *testing.T) {
	r := New()
	call := exactCall("bash", `{"command":"ls"}`)
	first, ok1 := r.RememberAutoDenial("/ws", call)
	second, ok2 := r.RememberAutoDenial("/ws", call)
	if !ok1 || !ok2 {
		t.Fatalf("both registrations must succeed: %v %v", ok1, ok2)
	}
	if first != second {
		t.Fatalf("duplicate exact call must reuse the request id")
	}
	if want := 1; len(r.denied) != want {
		t.Fatalf("duplicate must not add an entry: got %d want %d", len(r.denied), want)
	}
}

func TestDeniedCallLookupByOpaqueId(t *testing.T) {
	r := New()
	call := exactCall("bash", `{"command":"mv a b"}`)
	id, _ := r.RememberAutoDenial("/ws", call)
	got, ok := r.DeniedCall(id)
	if !ok || got != call {
		t.Fatalf("DeniedCall must return the exact stored call by opaque id: %+v ok=%v", got, ok)
	}
	// A fabricated / non-present id must not resolve.
	garbage := ID{0xff}
	if _, ok := r.DeniedCall(garbage); ok {
		t.Fatal("non-pending request id must not resolve to a denied call")
	}
}

func TestPreservedOutcomeRedeniesSemanticEquivalent(t *testing.T) {
	r := New()
	r.RememberAutoDenial("/ws", exactCall("bash", `{"command":"rm -rf ./tmp","cwd":"/ws"}`))
	// Same command issued through a shell wrapper is still a semantic match.
	if !r.PreservedOutcome("/ws", exactCall("bash", `{"command":"sh -c 'rm -rf ./tmp'","cwd":"/ws"}`)) {
		t.Fatal("wrapped command must be preserved as auto-denied")
	}
	// A genuinely different command escapes the gate.
	if r.PreservedOutcome("/ws", exactCall("bash", `{"command":"ls"}`)) {
		t.Fatal("different command must not be preserved")
	}
}

func TestPreservedOutcomeDoesNotRedenyApprovedAction(t *testing.T) {
	r := New()
	call := exactCall("bash", `{"command":"git push","cwd":"/ws"}`)
	id, _ := r.RememberAutoDenial("/ws", call)
	r.RememberApproval(id, Approval{Authority: "user", HumanApproval: true})
	if r.PreservedOutcome("/ws", call) {
		t.Fatal("a human-approved action must not be preserved as still denied")
	}
}

// Generic text can never authorize: the only authorization path is
// RememberApproval bound to the exact opaque request id, then single-use
// TakeApproval of the exact call.
func TestGenericTextCannotAuthorize(t *testing.T) {
	r := New()
	call := exactCall("bash", `{"command":"sudo rm -rf /"}`)
	id, _ := r.RememberAutoDenial("/ws", call)
	// An approval granted without the exact opaque id has no effect.
	if _, ok := r.TakeApproval(call); ok {
		t.Fatal("must not authorize before any approval")
	}
	// A "generic" approval that names the call but not the pending id must not
	// bind: RememberApproval requires a real human approval and exact id.
	if other := requestID(99); other != id {
		if r.RememberApproval(other, Approval{Authority: "user", HumanApproval: true}) {
			t.Fatal("approval for a non-pending id must not bind")
		}
	}
	wantCall, ok := r.DeniedCall(id)
	if !ok || wantCall != call {
		t.Fatalf("denied call must still be recoverable by id: %+v ok=%v", wantCall, ok)
	}
	// Execution revalidation binds the exact call.
	if _, ok := r.TakeApproval(exactCall("bash", `{"command":"sudo rm -rf /x"}`)); ok {
		t.Fatal("a distinct exact call must not consume the approval")
	}
}

func TestTakeApprovalIsSingleUse(t *testing.T) {
	r := New()
	call := exactCall("bash", `{"command":"kubectl delete po x"}`)
	id, _ := r.RememberAutoDenial("/ws", call)
	r.RememberApproval(id, Approval{Authority: "user", HumanApproval: true})

	approval, ok := r.TakeApproval(call)
	if !ok || !approval.HumanApproval || approval.Authority != "user" {
		t.Fatalf("first TakeApproval must yield the approval credentials: %+v ok=%v", approval, ok)
	}
	if _, ok := r.TakeApproval(call); ok {
		t.Fatal("the same approval must be consumed after one live revalidation")
	}
}

func TestRememberApprovalRequiresHumanApproval(t *testing.T) {
	r := New()
	call := exactCall("bash", `{"command":"git reset --hard"}`)
	id, _ := r.RememberAutoDenial("/ws", call)
	if r.RememberApproval(id, Approval{Authority: "auto", HumanApproval: false}) {
		t.Fatal("a non-human approval must not bind")
	}
	if _, ok := r.TakeApproval(call); ok {
		t.Fatal("no approval may be consumable without a real human approval")
	}
}

func TestTurnBudgetCapsDenials(t *testing.T) {
	r := New()
	for i := 0; i < maxTurnDenials; i++ {
		if _, ok := r.RememberAutoDenial("/ws", exactCall("bash", `{"command":"cmd"}`+strings.Repeat("x", i))); !ok {
			t.Fatalf("denial %d must register within budget", i)
		}
	}
	// Exact duplicates still re-register within budget (reuse), so push a new
	// distinct call past the cap.
	if _, ok := r.RememberAutoDenial("/ws", exactCall("bash", `{"command":"overflow"}`)); ok {
		t.Fatal("denial beyond the per-turn budget must be refused")
	}
}
