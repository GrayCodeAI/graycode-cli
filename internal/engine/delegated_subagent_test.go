package engine

import (
	"fmt"
	"testing"

	contracts "github.com/GrayCodeAI/hawk-core-contracts/policy"
	"github.com/GrayCodeAI/hawk/internal/eventlog"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

func TestSubAgentDelegatedPolicy_InheritsParentExplicitMode(t *testing.T) {
	registry := tool.NewRegistry()
	parent := NewSession("", "", "You are parent assistant.", registry)
	// Parent switches to strict mode
	if err := sandbox.SetSandboxMode(parent, sandbox.ModeStrict); err != nil {
		t.Fatalf("SetSandboxMode failed: %v", err)
	}

	sub := parent.SubSession("", "", registry)
	sandbox.InheritDelegatedPolicy(parent, sub)

	// Verify child journal has the delegated fact
	childJournal := sub.Persistence().Journal()
	if childJournal == nil {
		t.Fatal("child journal is nil")
	}
	sn := childJournal.Snapshot()
	var found bool
	for _, ev := range sn {
		if ev.Type == eventlog.SandboxMode {
			if f, ok := ev.Data.(eventlog.SandboxModeFact); ok {
				if f.Mode == string(sandbox.ModeStrict) && f.Source == eventlog.SandboxModeSourceDelegation {
					found = true
					break
				}
			}
		}
	}
	if !found {
		t.Fatalf("child journal missing delegated sandbox fact: %#v", sn)
	}

	// Verify child resolves to strict
	res := sandbox.ResolvePolicy(sub, sandbox.ModeWorkspace)
	if res.Mode != sandbox.ModeStrict {
		t.Fatalf("child resolved mode = %s, want %s", res.Mode, sandbox.ModeStrict)
	}
}

func TestSubAgentDelegatedPolicy_UnswitchedParentDynamicDefault(t *testing.T) {
	registry := tool.NewRegistry()
	parent := NewSession("", "", "You are parent assistant.", registry)
	// Parent has no explicit sandbox override

	sub := parent.SubSession("", "", registry)
	sandbox.InheritDelegatedPolicy(parent, sub)

	// Child journal should have no sandbox.mode event
	childJournal := sub.Persistence().Journal()
	if childJournal != nil {
		for _, ev := range childJournal.Snapshot() {
			if ev.Type == eventlog.SandboxMode {
				t.Fatalf("child journal must not contain sandbox.mode when parent is unswitched: %#v", ev)
			}
		}
	}

	// Child dynamically resolves deployment default
	res := sandbox.ResolvePolicy(sub, sandbox.ModeWorkspace)
	if res.Mode != sandbox.ModeWorkspace {
		t.Fatalf("child resolved mode = %s, want %s", res.Mode, sandbox.ModeWorkspace)
	}
	if res.Source != sandbox.SourceDefault {
		t.Fatalf("child resolved source = %s, want %s", res.Source, sandbox.SourceDefault)
	}
}

func TestSubAgentDelegatedPolicy_InteractiveAsksDeniedDeterministically(t *testing.T) {
	registry := tool.NewRegistry()
	parent := NewSession("", "", "You are parent assistant.", registry)

	sub := parent.SubSession("", "", registry)
	// Wire delegated approval pinning
	sub.PermSvc().SetAskUserFn(func(q string) (string, error) {
		t.Errorf("unexpected call to ask user: %s", q)
		return "", fmt.Errorf("unexpected call to ask user: %s", q)
	})
	sub.SetPermissionFn(func(req PermissionRequest) {
		if req.Response != nil {
			req.Response <- false
		}
	})

	// If a permission request is raised, it must be denied (false)
	respCh := make(chan bool, 1)
	req := PermissionRequest{
		PermissionRequest: contracts.PermissionRequest{
			ToolName: "Write",
			Summary:  "write file",
		},
		Response: respCh,
	}
	sub.PermSvc().PermissionFn()(req)
	select {
	case granted := <-respCh:
		if granted {
			t.Fatal("interactive permission granted for delegated subagent, want false (denied)")
		}
	default:
		t.Fatal("permission request blocked on channel, want immediate deterministic denial")
	}
}
