package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
)

func TestFoldLastSwitchWins(t *testing.T) {
	log := eventlog.New(nil)

	// Initially no sandbox mode
	if mode, ok := OverrideOf(log); ok || mode != "" {
		t.Fatalf("expected empty override on clean log, got %q (ok=%v)", mode, ok)
	}

	// Append strict
	if err := SetSandboxMode(log, ModeStrict); err != nil {
		t.Fatalf("SetSandboxMode failed: %v", err)
	}
	if mode, ok := OverrideOf(log); !ok || mode != ModeStrict {
		t.Fatalf("expected ModeStrict, got %q (ok=%v)", mode, ok)
	}

	// Append workspace
	if err := SetSandboxMode(log, ModeWorkspace); err != nil {
		t.Fatalf("SetSandboxMode failed: %v", err)
	}
	if mode, ok := OverrideOf(log); !ok || mode != ModeWorkspace {
		t.Fatalf("expected ModeWorkspace, got %q (ok=%v)", mode, ok)
	}

	// Append off
	if err := SetSandboxMode(log, ModeOff); err != nil {
		t.Fatalf("SetSandboxMode failed: %v", err)
	}
	if mode, ok := OverrideOf(log); !ok || mode != ModeOff {
		t.Fatalf("expected ModeOff, got %q (ok=%v)", mode, ok)
	}

	// Verify total events in log: exactly 3
	events := log.Snapshot()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
}

func TestSetSandboxMode_RejectsInvalid(t *testing.T) {
	log := eventlog.New(nil)

	invalidModes := []Mode{"", "read-only", "workspace-write", "danger-full-access", "admin", "custom"}
	for _, m := range invalidModes {
		if err := SetSandboxMode(log, m); err == nil {
			t.Errorf("expected SetSandboxMode(%q) to return error, got nil", m)
		}
	}

	// Ensure no events were appended for invalid modes
	if len(log.Snapshot()) != 0 {
		t.Fatalf("expected 0 events, got %d", len(log.Snapshot()))
	}
}

type testSessionWithExplicit struct {
	explicit Mode
	journal  *eventlog.Log
	cwd      string
}

func (s *testSessionWithExplicit) ExplicitSandboxMode() Mode { return s.explicit }
func (s *testSessionWithExplicit) Journal() *eventlog.Log    { return s.journal }
func (s *testSessionWithExplicit) Cwd() string               { return s.cwd }

func TestResolvePrecedence(t *testing.T) {
	log := eventlog.New(nil)
	sess := &testSessionWithExplicit{
		journal: log,
		cwd:     "/tmp/test-workspace",
	}

	// 1. Default fallback when no events and no explicit override
	res := ResolvePolicy(sess, ModeStrict)
	if res.Mode != ModeStrict || res.Source != SourceDefault {
		t.Errorf("expected default ModeStrict, got %+v", res)
	}

	// Default fallback without explicit defaultMode defaults to ModeWorkspace
	res = ResolvePolicy(sess, "")
	if res.Mode != ModeWorkspace || res.Source != SourceDefault {
		t.Errorf("expected default ModeWorkspace, got %+v", res)
	}

	// 2. Fold events from journal
	if err := SetSandboxMode(log, ModeOff); err != nil {
		t.Fatalf("SetSandboxMode failed: %v", err)
	}
	res = ResolvePolicy(sess, ModeStrict)
	if res.Mode != ModeOff || res.Source != SourceLog {
		t.Errorf("expected log ModeOff, got %+v", res)
	}

	// 3. Explicit override wins over log
	sess.explicit = ModeStrict
	res = ResolvePolicy(sess, ModeWorkspace)
	if res.Mode != ModeStrict || res.Source != SourceOverride {
		t.Errorf("expected override ModeStrict, got %+v", res)
	}

	// Method on SandboxPolicy
	policy := &SandboxPolicy{}
	res = policy.Resolve(sess, ModeWorkspace)
	if res.Mode != ModeStrict || res.Source != SourceOverride {
		t.Errorf("expected SandboxPolicy.Resolve override ModeStrict, got %+v", res)
	}
}

func TestCanonicalizeWorkspaceRoot_SymlinkParity(t *testing.T) {
	tmpDir := t.TempDir()

	// Create structure:
	// tmpDir/real/target/sub
	// tmpDir/link -> tmpDir/real/target/sub
	realTarget := filepath.Join(tmpDir, "real", "target", "sub")
	if err := os.MkdirAll(realTarget, 0o755); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(tmpDir, "link")
	if err := os.Symlink(realTarget, linkPath); err != nil {
		t.Fatal(err)
	}

	// Canonicalize linkPath
	canonLink := CanonicalizeWorkspaceRoot(linkPath)
	canonRealTarget := CanonicalizeWorkspaceRoot(realTarget)
	if canonLink != canonRealTarget {
		t.Errorf("CanonicalizeWorkspaceRoot(link) = %q, want %q", canonLink, canonRealTarget)
	}

	// Canonicalize linkPath/..
	// Process cwd resolution: if process is at linkPath (which is realTarget), cd .. goes to real/target
	canonParent := CanonicalizeWorkspaceRoot(linkPath + "/..")
	expectedParent := CanonicalizeWorkspaceRoot(filepath.Join(realTarget, ".."))
	if canonParent != expectedParent {
		t.Errorf("CanonicalizeWorkspaceRoot(link/..) = %q, want %q", canonParent, expectedParent)
	}
}

func TestFormatPolicyStatement(t *testing.T) {
	// Strict
	strictStmt := FormatPolicyStatement(ModeStrict, "/workspace/root")
	if strictStmt != "Sandbox policy: strict (read-only). Do not refuse a required modification from this policy alone; try the tool and follow denial/escalation guidance." {
		t.Errorf("unexpected strict statement: %q", strictStmt)
	}

	// Workspace
	wsStmt := FormatPolicyStatement(ModeWorkspace, "/workspace/root")
	if wsStmt != "Sandbox policy: workspace. You may modify files under /workspace/root. Some platform temporary areas are writable." {
		t.Errorf("unexpected workspace statement: %q", wsStmt)
	}

	// Off
	offStmt := FormatPolicyStatement(ModeOff, "/workspace/root")
	if offStmt != "Sandbox policy: off. File sandbox does not restrict modifications." {
		t.Errorf("unexpected off statement: %q", offStmt)
	}
}
