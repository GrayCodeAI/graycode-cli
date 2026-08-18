package eventlog

import (
	"testing"
	"time"
)

func mkEvent(t Type, seq uint64, data any) Event {
	return Event{Type: t, Seq: seq, At: time.Time{}, Data: data}
}

func TestValidateRelationsTurnStepNesting(t *testing.T) {
	// Valid turn/step nesting with assistant chunk and tool call/result.
	events := []Event{
		mkEvent(TurnStart, 1, BoundaryFact{Turn: 1}),
		mkEvent(StepStart, 2, BoundaryFact{Turn: 1, Step: 1}),
		mkEvent(AssistantChunk, 3, ChunkFact{Turn: 1, Step: 1, Chunk: "hi"}),
		mkEvent(AssistantMsg, 4, Message{Role: "assistant", Content: "hi", Turn: 1, Step: 1}),
		mkEvent(ToolCall, 5, ToolCallPayload{Turn: 1, Step: 1, ID: "call1", Name: "Bash"}),
		mkEvent(ToolResult, 6, ToolResultPayload{Turn: 1, Step: 1, ToolUseID: "call1", Content: "ok"}),
		mkEvent(StepEnd, 7, BoundaryFact{Turn: 1, Step: 1}),
		mkEvent(TurnEnd, 8, TurnEndFact{Turn: 1, Reason: "completed"}),
	}
	if err := ValidateRelations(events); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateRelationsToolResultWithoutCall(t *testing.T) {
	events := []Event{
		mkEvent(TurnStart, 1, BoundaryFact{Turn: 1}),
		mkEvent(StepStart, 2, BoundaryFact{Turn: 1, Step: 1}),
		mkEvent(ToolResult, 3, ToolResultPayload{Turn: 1, Step: 1, ToolUseID: "call1", Content: "ok"}),
	}
	if err := ValidateRelations(events); err == nil {
		t.Fatal("expected error for tool/result without prior tool/call")
	}
}

func TestValidateRelationsUnclosedTurnFails(t *testing.T) {
	events := []Event{
		mkEvent(TurnStart, 1, BoundaryFact{Turn: 1}),
		mkEvent(TurnStart, 2, BoundaryFact{Turn: 2}), // second turn/start without turn/end
	}
	if err := ValidateRelations(events); err == nil {
		t.Fatal("expected error for unclosed turn")
	}
}

func TestValidateRelationsCoreEventsNeedTurn(t *testing.T) {
	// todo/write outside any turn should fail.
	events := []Event{
		mkEvent(TodoWrite, 1, TodoWriteFact{}),
	}
	if err := ValidateRelations(events); err == nil {
		t.Fatal("expected error for core event outside turn")
	}
}

func TestValidateRelationsSurfaceReplacementExempt(t *testing.T) {
	// tool/result with surface replace should be exempt from pairing check.
	events := []Event{
		mkEvent(TurnStart, 1, BoundaryFact{Turn: 1}),
		mkEvent(StepStart, 2, BoundaryFact{Turn: 1, Step: 1}),
		mkEvent(ToolResult, 3, ToolResultPayload{Turn: 1, Step: 1, ToolUseID: "call1", Content: "ok"}),
	}
	events[2].SurfaceOp = &SurfaceOp{Op: "replace", Start: 0, End: 2}
	if err := ValidateRelations(events); err != nil {
		t.Fatalf("expected no error for surface-replace tool/result, got %v", err)
	}
}

func TestValidateRelationsSeqStrictlyIncreases(t *testing.T) {
	events := []Event{
		mkEvent(TurnStart, 1, BoundaryFact{Turn: 1}),
		mkEvent(TurnEnd, 1, TurnEndFact{Turn: 1}), // same seq — should fail
	}
	if err := ValidateRelations(events); err == nil {
		t.Fatal("expected error for non-increasing seq")
	}
}

func TestValidateRelationsEmpty(t *testing.T) {
	if err := ValidateRelations(nil); err != nil {
		t.Fatalf("expected no error for empty events, got %v", err)
	}
}
