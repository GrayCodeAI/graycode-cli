package engine

import (
	"reflect"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/eventlog"
	"github.com/GrayCodeAI/hawk/internal/types"
)

func TestJournaledAppendKeepsTranscriptAndProjectionInSync(t *testing.T) {
	ps := NewPersistenceService(nil)

	// No journal yet: appends do not panic and behave like plain transcript writes.
	ps.AppendUserJournaled(types.EyrieMessage{Role: "user", Content: "pre-journal"})
	if got := ps.MessageCount(); got != 1 {
		t.Fatalf("messages = %d, want 1", got)
	}
	if got := ps.JournalProjection(); got != nil {
		t.Fatalf("projection = %v, want nil when no journal attached", got)
	}

	ps.SetJournal(eventlog.New(nil))
	want := []types.EyrieMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	ps.SetRawMessages(nil) // reset transcript; journal still holds prior facts
	ps.AppendUserJournaled(want[0])
	ps.AppendAssistantJournaled(want[1])

	got := ps.Messages()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Messages() = %+v, want %+v", got, want)
	}
	proj := ps.JournalProjection()
	if !reflect.DeepEqual(proj, want) {
		t.Fatalf("JournalProjection() = %+v, want %+v", proj, want)
	}
}

func TestJournaledAppendPreservesToolFactoryRoundTrip(t *testing.T) {
	ps := NewPersistenceService(nil)
	ps.SetJournal(eventlog.New(nil))
	in := types.EyrieMessage{
		Role:    "assistant",
		Content: "did tool call",
		ToolUse: []types.ToolCall{
			{ID: "tc1", Name: "Read", Arguments: map[string]interface{}{"path": "/x"}},
		},
	}
	ps.AppendAssistantJournaled(in)
	ps.AppendUserJournaled(types.EyrieMessage{
		Role:    "user",
		Content: "tool result",
		ToolResults: []types.ToolResult{
			{ToolUseID: "tc1", Content: "content!", IsError: false},
		},
	})

	proj := ps.JournalProjection()
	if len(proj) != 2 {
		t.Fatalf("projected %d messages, want 2", len(proj))
	}
	if len(proj[0].ToolUse) != 1 || proj[0].ToolUse[0].ID != "tc1" {
		t.Fatalf("tool call not reconstructed: %+v", proj[0].ToolUse)
	}
	if len(proj[1].ToolResults) != 1 || proj[1].ToolResults[0].Content != "content!" {
		t.Fatalf("tool result not reconstructed: %+v", proj[1].ToolResults)
	}
	// The live transcript must equal the projection for the same appends.
	if !reflect.DeepEqual(ps.Messages(), proj) {
		t.Fatalf("transcript != projection: %+v vs %+v", ps.Messages(), proj)
	}
}

func TestAddUserAddAssistantRoutesThroughJournal(t *testing.T) {
	ps := NewPersistenceService(nil)
	ps.SetJournal(eventlog.New(nil))
	ps.AddUser("hello")
	ps.AddAssistant("hi")
	want := []types.EyrieMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	if got := ps.Messages(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Messages() = %+v, want %+v", got, want)
	}
	if got := ps.JournalProjection(); !reflect.DeepEqual(got, want) {
		t.Fatalf("JournalProjection() = %+v, want %+v", got, want)
	}
}

func TestReconstructiblePassesForJournaledTranscript(t *testing.T) {
	ps := NewPersistenceService(nil)
	ps.SetJournal(eventlog.New(nil))
	ps.AddUser("hello")
	ps.AddAssistant("hi")
	if err := ps.Reconstructible(); err != nil {
		t.Fatalf("Reconstructible() = %v, want nil", err)
	}
}

func TestReconstructibleFailsAfterTranscriptEdit(t *testing.T) {
	ps := NewPersistenceService(nil)
	ps.SetJournal(eventlog.New(nil))
	ps.AddUser("hello")
	ps.AddAssistant("hi")
	// SetRawMessages rewrites the transcript without a journal event, so the
	// projection no longer matches. The invariant must report that.
	ps.SetRawMessages(nil)
	if err := ps.Reconstructible(); err == nil {
		t.Fatal("Reconstructible() = nil, want mismatch after SetRawMessages")
	}
}

func TestJournaledAppendContentPartsRoundTrip(t *testing.T) {
	ps := NewPersistenceService(nil)
	ps.SetJournal(eventlog.New(nil))
	img := &types.ImageURLPart{URL: "data:image/png;base64,abc", Detail: "high"}
	in := types.EyrieMessage{Role: "user", Content: "see image", ContentParts: []types.ContentPart{{Type: "image_url", ImageURL: img}}}
	ps.AppendUserJournaled(in)
	got := ps.JournalProjection()[0]
	if len(got.ContentParts) != 1 {
		t.Fatalf("content parts = %d, want 1", len(got.ContentParts))
	}
	if got.ContentParts[0].ImageURL == nil || got.ContentParts[0].ImageURL.URL != img.URL {
		t.Fatalf("image part not reconstructed: %+v", got.ContentParts)
	}
}
