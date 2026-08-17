package engine

import (
	"testing"

	"github.com/GrayCodeAI/hawk/internal/eventlog"
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
