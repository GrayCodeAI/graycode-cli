package engine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// TestSession_SetConversationGraph guards graph ownership by PersistenceService.
func TestSession_SetConversationGraph_DualWrite(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)

	if s.Persistence().Graph() != nil {
		t.Fatal("graph should be nil before SetConversationGraph")
	}

	// Build a real Hawk graph backed by a temporary JSON store and attach it.
	dir := t.TempDir()
	graph, err := session.OpenConversationGraph(filepath.Join(dir, "conversation.json"), "test-session")
	if err != nil {
		t.Fatalf("OpenConversationGraph: %v", err)
	}
	t.Cleanup(func() { _ = graph.Close() })
	s.SetConversationGraph(graph)

	if s.Persistence().Graph() == nil {
		t.Error("s.Persistence().Graph() is nil after SetConversationGraph")
	}
}

func TestSession_NewSessionWithClient_WiresMemoryAndPersistenceServices(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)

	// The services start empty; verify their canonical ownership directly.
	if s.persist == nil {
		t.Fatal("s.persist is nil; NewSessionWithClient must wire the persistence service")
	}
	if s.memory == nil {
		t.Fatal("s.memory is nil; NewSessionWithClient must wire the memory service")
	}
	if !s.memory.IsZero() {
		t.Error("memory service should start unconfigured")
	}
}

// TestPersistenceService_AddUserWithImage_DataURL is a regression
// guard for the H4 fix: PersistenceService.AddUserWithImage must
// compose a data URL prefix ("data:<imageType>;base64,<base64>")
// and store it in the message Images slice. The previous
// implementation dropped the prefix (storing raw base64) and
// would have deadlocked on its first call (it took s.mu, then
// called s.SetRawMessages which re-takes the same lock).
func TestPersistenceService_AddUserWithImage_DataURL(t *testing.T) {
	t.Parallel()
	ps := NewPersistenceService(nil)

	const (
		imgType = "image/png"
		base64_ = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNgAAIAAAUAAen63NgAAAAASUVORK5CYII="
		wantURL = "data:image/png;base64," + base64_
	)
	ps.AddUserWithImage("look at this", base64_, imgType)

	msgs := ps.RawMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if len(msgs[0].Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(msgs[0].Images))
	}
	if msgs[0].Images[0] != wantURL {
		t.Errorf("image = %q, want %q (data URL prefix missing)", msgs[0].Images[0], wantURL)
	}
	if !strings.HasPrefix(msgs[0].Images[0], "data:image/png;base64,") {
		t.Errorf("image = %q, must start with the data: URL prefix", msgs[0].Images[0])
	}
	if msgs[0].Role != "user" {
		t.Errorf("role = %q, want 'user'", msgs[0].Role)
	}
	if msgs[0].Content != "look at this" {
		t.Errorf("content = %q, want 'look at this'", msgs[0].Content)
	}
}

func TestPersistenceService_RemoveLastExchangeAndCount(t *testing.T) {
	t.Parallel()

	ps := NewPersistenceService(nil)
	ps.LoadMessages([]types.EyrieMessage{
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a2"},
	})

	if got := ps.MessageCount(); got != 4 {
		t.Fatalf("MessageCount() = %d, want 4", got)
	}

	ps.RemoveLastExchange()
	msgs := ps.Messages()
	if len(msgs) != 2 {
		t.Fatalf("len(Messages()) = %d, want 2", len(msgs))
	}
	if msgs[0].Content != "u1" || msgs[1].Content != "a1" {
		t.Fatalf("unexpected remaining messages: %#v", msgs)
	}
	if got := ps.MessageCount(); got != 2 {
		t.Fatalf("MessageCount() after remove = %d, want 2", got)
	}
}
