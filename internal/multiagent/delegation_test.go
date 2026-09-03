package mission

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
	"github.com/GrayCodeAI/graycode-cli/internal/sandbox"
)

func TestDelegatedPolicy_ChildLogReconstructsPolicy(t *testing.T) {
	parentLog := eventlog.New(nil)
	parentLog.AppendSandboxModeWithSource(string(sandbox.ModeStrict), eventlog.SandboxModeSourceUser)

	childLog := eventlog.New(nil)
	override, ok := sandbox.InheritDelegatedPolicy(parentLog, childLog)
	if !ok || override != sandbox.ModeStrict {
		t.Fatalf("InheritDelegatedPolicy() = (%v, %v), want (%v, true)", override, ok, sandbox.ModeStrict)
	}

	// Verify child log has exactly one sandbox.mode event with source=delegation
	sn := childLog.Snapshot()
	if len(sn) != 1 {
		t.Fatalf("child log events count = %d, want 1", len(sn))
	}
	if sn[0].Type != eventlog.SandboxMode {
		t.Fatalf("child log event[0] type = %s, want %s", sn[0].Type, eventlog.SandboxMode)
	}
	fact, ok := sn[0].Data.(eventlog.SandboxModeFact)
	if !ok || fact.Mode != string(sandbox.ModeStrict) || fact.Source != eventlog.SandboxModeSourceDelegation {
		t.Fatalf("child fact = %#v, want mode=%s source=%s", fact, sandbox.ModeStrict, eventlog.SandboxModeSourceDelegation)
	}

	// Reconstruct effective policy from child log alone
	resolved := sandbox.ResolvePolicy(childLog, sandbox.ModeWorkspace)
	if resolved.Mode != sandbox.ModeStrict {
		t.Fatalf("resolved child mode = %s, want %s", resolved.Mode, sandbox.ModeStrict)
	}
	if resolved.Source != sandbox.SourceLog {
		t.Fatalf("resolved child source = %s, want %s", resolved.Source, sandbox.SourceLog)
	}
}

func TestDelegatedPolicy_UnswitchedParentDynamicDefault(t *testing.T) {
	parentLog := eventlog.New(nil) // No sandbox.mode facts

	childLog := eventlog.New(nil)
	override, ok := sandbox.InheritDelegatedPolicy(parentLog, childLog)
	if ok || override != "" {
		t.Fatalf("unswitched parent yielded override = (%v, %v), want ('', false)", override, ok)
	}

	// Child log must be untouched
	if len(childLog.Snapshot()) != 0 {
		t.Fatalf("child log must have 0 events for unswitched parent, got %d", len(childLog.Snapshot()))
	}

	// Child dynamically resolves deployment default
	resolved := sandbox.ResolvePolicy(childLog, sandbox.ModeWorkspace)
	if resolved.Mode != sandbox.ModeWorkspace {
		t.Fatalf("resolved mode = %s, want %s (deployment default)", resolved.Mode, sandbox.ModeWorkspace)
	}
	if resolved.Source != sandbox.SourceDefault {
		t.Fatalf("resolved source = %s, want %s", resolved.Source, sandbox.SourceDefault)
	}
}

func TestDelegatedPolicy_ParentSwitchAfterSpawnHasNoEffect(t *testing.T) {
	parentLog := eventlog.New(nil)
	parentLog.AppendSandboxModeWithSource(string(sandbox.ModeStrict), eventlog.SandboxModeSourceUser)

	childLog := eventlog.New(nil)
	sandbox.InheritDelegatedPolicy(parentLog, childLog)

	// Parent switches to "off" after child was spawned
	parentLog.AppendSandboxModeWithSource(string(sandbox.ModeOff), eventlog.SandboxModeSourceUser)

	// Child remains strict (fixed at spawn)
	resolvedChild := sandbox.ResolvePolicy(childLog, sandbox.ModeWorkspace)
	if resolvedChild.Mode != sandbox.ModeStrict {
		t.Fatalf("child mode after parent switch = %s, want %s", resolvedChild.Mode, sandbox.ModeStrict)
	}

	resolvedParent := sandbox.ResolvePolicy(parentLog, sandbox.ModeWorkspace)
	if resolvedParent.Mode != sandbox.ModeOff {
		t.Fatalf("parent mode = %s, want %s", resolvedParent.Mode, sandbox.ModeOff)
	}
}

func TestDelegatedPolicy_DenyAllGateDeterministicRejection(t *testing.T) {
	gate := DenyAllGate()
	if gate == nil {
		t.Fatal("DenyAllGate() returned nil")
	}

	ctx := context.Background()
	err := gate.Check(ctx, "Bash", "destructive command rm -rf /")
	if err == nil {
		t.Fatal("DenyAllGate.Check() succeeded, want deterministic rejection")
	}
	if !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("error = %v, want tool rejected error", err)
	}
}

func TestDelegatedPolicy_FormatDelegationStatement(t *testing.T) {
	stmt := sandbox.FormatDelegationStatement()
	if stmt == "" {
		t.Fatal("FormatDelegationStatement() returned empty string")
	}
	if !strings.Contains(stmt, "Delegation policy") {
		t.Fatalf("stmt %q missing 'Delegation policy' prefix", stmt)
	}
	if !strings.Contains(stmt, "report the limitation") {
		t.Fatalf("stmt %q missing guidance on reporting limitation", stmt)
	}
}
