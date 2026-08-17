package eventlog

import "testing"

func TestProjectMessagesOnlyUserAndAssistant(t *testing.T) {
	l := New(nil)
	l.Append(SessionMeta, Meta{ID: "s1", FormatVersion: 1})
	l.Append(UserMessage, Message{Content: "hi"})
	l.Append(ToolCall, ToolCallPayload{Name: "Read"})
	l.Append(AssistantMsg, Message{Content: "hello"})
	l.Append(SessionCompacted, Message{Content: "summary"})

	got := ProjectMessages(l.Snapshot())
	if len(got) != 2 {
		t.Fatalf("projected %d messages, want 2", len(got))
	}
	if got[0].Content != "hi" || got[1].Content != "hello" {
		t.Fatalf("unexpected projection order: %+v", got)
	}
}

// Compaction events passing a Message payload may also be projected when their
// payload is a summarised user turn. Because the summarised message is still
// model-visible and carries enough context to stand alone, the projection can be
// a full turns-history transcript that looms compaction facts.
func TestProjectMessagesIgnoresNonMessagePayloads(t *testing.T) {
	l := New(nil)
	// SessionCompacted with a non-Message payload must not corrupt the projection.
	l.Append(SessionCompacted, nil)
	l.Append(UserMessage, Message{Content: "only"})
	got := ProjectMessages(l.Snapshot())
	if len(got) != 1 {
		t.Fatalf("projected %d messages, want 1", len(got))
	}
}
