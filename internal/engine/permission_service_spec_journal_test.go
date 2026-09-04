package engine

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
)

func TestPermissionServiceAdvanceSpecStageJournalsFact(t *testing.T) {
	s := NewPermissionService(nil)
	log := eventlog.New(nil)
	s.SetJournal(log)

	s.SetSpecStage(SpecStageProposal)
	s.AdvanceSpecStage("Specify")

	events := log.OfType(eventlog.SpecState)
	if len(events) != 1 {
		t.Fatalf("spec facts = %d, want 1", len(events))
	}
	fact, ok := events[0].Data.(eventlog.SpecFact)
	if !ok {
		t.Fatalf("data type = %T, want eventlog.SpecFact", events[0].Data)
	}
	if fact.Stage != "specify" {
		t.Fatalf("stage = %q, want specify", fact.Stage)
	}
}

func TestPermissionServiceNoJournalDoesNotPanic(t *testing.T) {
	s := NewPermissionService(nil)
	s.AdvanceSpecStage("Specify")
	// No panic: journal is optional.
}

func TestPermissionServiceCheckApprovalJournalsPermissionFact(t *testing.T) {
	s := NewPermissionService(nil)
	log := eventlog.New(nil)
	s.SetJournal(log)

	// Nil approval gate is an allow no-op, but the decision is still durable.
	allowed, msg := s.CheckApproval(context.Background(), "Bash", map[string]interface{}{"command": "rm -rf /tmp/x"})
	if !allowed || msg != "" {
		t.Fatalf("nil gate should allow, allowed=%v msg=%q", allowed, msg)
	}

	facts := log.OfType(eventlog.PermissionChange)
	if len(facts) != 1 {
		t.Fatalf("permission facts = %d, want 1", len(facts))
	}
	fact, ok := facts[0].Data.(eventlog.PermissionFact)
	if !ok {
		t.Fatalf("data type = %T, want eventlog.PermissionFact", facts[0].Data)
	}
	if !fact.Allowed || fact.Tool == "" {
		t.Fatalf("expected durable allow fact, got %#v", fact)
	}
}
