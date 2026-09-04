package engine

import (
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestTrimIncompleteTurn_NoopOnCompleteTurn(t *testing.T) {
	t.Parallel()
	p := &PersistenceService{}
	p.AppendAssistantJournaled(types.GraycodeRouterMessage{
		Role:    "assistant",
		Content: " ",
		ToolUse: []types.ToolCall{{ID: "t1", Name: "Bash"}},
	})
	p.AppendUserJournaled(types.GraycodeRouterMessage{
		Role:        "user",
		Content:     "out",
		ToolResults: []types.ToolResult{{ToolUseID: "t1", Content: "out"}},
	})

	p.TrimIncompleteTurn()

	got := p.RawMessages()
	if len(got) != 2 {
		t.Fatalf("complete turn should be untouched, got %d messages", len(got))
	}
}

func TestTrimIncompleteTurn_DropsUnansweredAssistantToolUse(t *testing.T) {
	t.Parallel()
	p := &PersistenceService{}
	p.AddUser("do the thing")
	p.AppendAssistantJournaled(types.GraycodeRouterMessage{
		Role:    "assistant",
		Content: " ",
		ToolUse: []types.ToolCall{{ID: "t1", Name: "Bash"}},
	})
	// Cancelled before tool results were appended.

	p.TrimIncompleteTurn()

	got := p.RawMessages()
	if len(got) != 1 {
		t.Fatalf("expected dangling assistant dropped, got %d messages", len(got))
	}
	if got[0].Role != "user" || got[0].Content != "do the thing" {
		t.Errorf("remaining message = %+v, want original user turn", got[0])
	}
}

func TestTrimIncompleteTurn_DropsOrphanedToolResults(t *testing.T) {
	t.Parallel()
	p := &PersistenceService{}
	p.AddUser("do the thing")
	// Simulate a transcript where results landed but the owning assistant
	// tool_use is missing (e.g. partial restore).
	p.AppendUserJournaled(types.GraycodeRouterMessage{
		Role:        "user",
		Content:     "out",
		ToolResults: []types.ToolResult{{ToolUseID: "t1", Content: "out"}},
	})

	p.TrimIncompleteTurn()

	got := p.RawMessages()
	if len(got) != 1 {
		t.Fatalf("expected orphaned tool_result dropped, got %d messages", len(got))
	}
	if got[0].Role != "user" || got[0].Content != "do the thing" {
		t.Errorf("remaining message = %+v, want original user turn", got[0])
	}
}

func TestTrimIncompleteTurn_DropsMultipleDanglingLayers(t *testing.T) {
	t.Parallel()
	p := &PersistenceService{}
	p.AddUser("first")
	p.AppendAssistantJournaled(types.GraycodeRouterMessage{Role: "assistant", Content: "done"})
	// Then a new turn started and was cancelled mid-flight:
	p.AppendUserJournaled(types.GraycodeRouterMessage{
		Role:        "user",
		Content:     "orphan result",
		ToolResults: []types.ToolResult{{ToolUseID: "x9", Content: "r"}},
	})
	p.AppendAssistantJournaled(types.GraycodeRouterMessage{
		Role:    "assistant",
		Content: " ",
		ToolUse: []types.ToolCall{{ID: "x8", Name: "Read"}},
	})

	p.TrimIncompleteTurn()

	got := p.RawMessages()
	if len(got) != 2 {
		t.Fatalf("expected dangling layers trimmed back to complete turn, got %d messages", len(got))
	}
	if got[1].Role != "assistant" || got[1].Content != "done" {
		t.Errorf("tail = %+v, want completed assistant turn", got[1])
	}
}

func TestTrimIncompleteTurn_KeepsResultsWhenOwnerPresent(t *testing.T) {
	t.Parallel()
	p := &PersistenceService{}
	p.AppendAssistantJournaled(types.GraycodeRouterMessage{
		Role:    "assistant",
		Content: " ",
		ToolUse: []types.ToolCall{{ID: "t1", Name: "Bash"}, {ID: "t2", Name: "Read"}},
	})
	p.AppendUserJournaled(types.GraycodeRouterMessage{
		Role:        "user",
		Content:     "out1",
		ToolResults: []types.ToolResult{{ToolUseID: "t1", Content: "out1"}},
	})
	// t2's result never landed, so the whole incomplete turn is removed.

	p.TrimIncompleteTurn()

	if got := p.RawMessages(); len(got) != 0 {
		t.Fatalf("expected incomplete turn removed, got %d messages", len(got))
	}
}

func TestTrimIncompleteTurn_EmptyAndNil(t *testing.T) {
	t.Parallel()
	var nilP *PersistenceService
	nilP.TrimIncompleteTurn() // must not panic

	p := &PersistenceService{}
	p.TrimIncompleteTurn() // empty transcript — no-op
	if n := p.MessageCount(); n != 0 {
		t.Errorf("empty transcript changed to %d messages", n)
	}
}
