package engine

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/sandbox"
)

func TestSandboxPolicy_FoldLastSwitchWins(t *testing.T) {
	sess := newTestSession()
	sess.EnsureSandboxPolicyStatement()

	// Initial default should be workspace
	if got := sess.PermSvc().SandboxMode(); got != sandbox.ModeWorkspace {
		t.Fatalf("initial SandboxMode = %q, want workspace", got)
	}

	// Change to strict
	sess.PermSvc().SetSandboxMode(sandbox.ModeStrict)
	if got := sess.PermSvc().SandboxMode(); got != sandbox.ModeStrict {
		t.Fatalf("after SetSandboxMode(strict) = %q, want strict", got)
	}

	// Change to off
	sess.PermSvc().SetSandboxMode(sandbox.ModeOff)
	if got := sess.PermSvc().SandboxMode(); got != sandbox.ModeOff {
		t.Fatalf("after SetSandboxMode(off) = %q, want off", got)
	}
}

func TestSandboxPolicy_OverridePersistenceAcrossReload(t *testing.T) {
	sess := newTestSession()
	sess.PermSvc().SetSandboxMode(sandbox.ModeStrict)

	wire := sess.JournalWire()
	if len(wire) == 0 {
		t.Fatal("expected non-empty journal wire")
	}

	// Create a new session and rehydrate the journal
	restored := newTestSession()
	if err := restored.ReplayJournal(wire); err != nil {
		t.Fatalf("ReplayJournal failed: %v", err)
	}

	if got := restored.PermSvc().SandboxMode(); got != sandbox.ModeStrict {
		t.Fatalf("restored SandboxMode = %q, want strict", got)
	}
}

func TestSandboxPolicy_ContextStatementEmittedOnlyOnFirstRequestAndChange(t *testing.T) {
	sess := newTestSession()

	// 1. First request emits statement
	stmt1 := sess.EnsureSandboxPolicyStatement()
	if stmt1 == "" {
		t.Fatal("expected non-empty statement on first call")
	}
	if !strings.HasPrefix(stmt1, "Sandbox policy: workspace") {
		t.Fatalf("expected workspace policy statement, got %q", stmt1)
	}
	msgCount1 := len(sess.Persistence().RawMessages())
	if msgCount1 != 1 {
		t.Fatalf("expected 1 message in persistence, got %d", msgCount1)
	}

	// 2. Second request without policy change emits NOTHING
	stmt2 := sess.EnsureSandboxPolicyStatement()
	if stmt2 != "" {
		t.Fatalf("expected empty statement on unchanged policy, got %q", stmt2)
	}
	msgCount2 := len(sess.Persistence().RawMessages())
	if msgCount2 != 1 {
		t.Fatalf("expected still 1 message in persistence, got %d", msgCount2)
	}

	// 3. Mode switch emits NEW statement
	sess.PermSvc().SetSandboxMode(sandbox.ModeStrict)
	stmt3 := sess.EnsureSandboxPolicyStatement()
	if stmt3 == "" {
		t.Fatal("expected non-empty statement after mode switch")
	}
	if !strings.HasPrefix(stmt3, "Sandbox policy: strict") {
		t.Fatalf("expected strict policy statement, got %q", stmt3)
	}
	msgCount3 := len(sess.Persistence().RawMessages())
	if msgCount3 != 2 {
		t.Fatalf("expected 2 messages in persistence, got %d", msgCount3)
	}

	// 4. Fourth request without policy change emits NOTHING
	stmt4 := sess.EnsureSandboxPolicyStatement()
	if stmt4 != "" {
		t.Fatalf("expected empty statement on unchanged policy, got %q", stmt4)
	}
	msgCount4 := len(sess.Persistence().RawMessages())
	if msgCount4 != 2 {
		t.Fatalf("expected still 2 messages in persistence, got %d", msgCount4)
	}
}

func TestSandboxPolicy_KVCacheStability(t *testing.T) {
	sess := newTestSession()
	baseSystem := "You are an AI programming assistant."
	sess.Persistence().SetSystem(baseSystem)

	// First ensure policy statement
	sess.EnsureSandboxPolicyStatement()
	if sys := sess.Persistence().System(); sys != baseSystem {
		t.Fatalf("system prompt changed after first statement: got %q, want %q", sys, baseSystem)
	}

	// Switch mode to strict
	sess.PermSvc().SetSandboxMode(sandbox.ModeStrict)
	sess.EnsureSandboxPolicyStatement()
	if sys := sess.Persistence().System(); sys != baseSystem {
		t.Fatalf("system prompt changed after switch to strict: got %q, want %q", sys, baseSystem)
	}

	// Switch mode to off
	sess.PermSvc().SetSandboxMode(sandbox.ModeOff)
	sess.EnsureSandboxPolicyStatement()
	if sys := sess.Persistence().System(); sys != baseSystem {
		t.Fatalf("system prompt changed after switch to off: got %q, want %q", sys, baseSystem)
	}
}
