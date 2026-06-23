package engine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/storage"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// TestSession_SetConvoDAG_DualWrite is a regression guard for the
// H3 fix: SetConvoDAG must propagate the DAG to the persistence
// service, not just the legacy field. Otherwise new code reading
// s.Persistence().DAG() would see nil even after the field was set.
func TestSession_SetConvoDAG_DualWrite(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)

	if s.Persistence().DAG() != nil {
		t.Fatal("DAG should be nil before SetConvoDAG")
	}

	// Build a real DAG backed by a temp SQLite store and attach it.
	dir := t.TempDir()
	dag, err := storage.NewDAG(filepath.Join(dir, "convo.db"), "test-session")
	if err != nil {
		t.Fatalf("storage.NewDAG: %v", err)
	}
	t.Cleanup(func() { _ = dag.Close() })
	s.SetConvoDAG(dag)

	if s.ConvoDAG == nil {
		t.Error("s.ConvoDAG is nil after SetConvoDAG")
	}
	if s.Persistence().DAG() == nil {
		t.Error("s.Persistence().DAG() is nil after SetConvoDAG — H3 regression: SetConvoDAG must dual-write to the persistence service")
	}
	if s.Persistence().DAG() != s.ConvoDAG {
		t.Error("s.Persistence().DAG() != s.ConvoDAG — both views should be the same instance")
	}
}

// TestSession_NewSessionWithClient_AliasesMemoryFields is a
// regression guard for the H3 fix: NewSessionWithClient should
// alias the 5 fields read by AddUser/AddAssistant/AddUserWithImage/
// ForkConversation/SwitchBranch from the sub-service getters, so
// legacy direct-field reads return the sub-service state.
func TestSession_NewSessionWithClient_AliasesMemoryFields(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)

	// All five fields must be wired to the sub-service. The sub-
	// services start empty, so we just assert the aliasing didn't
	// return nil pointers.
	if s.persist == nil {
		t.Fatal("s.persist is nil; NewSessionWithClient must wire the persistence service")
	}
	if got := s.persist.DAG(); got != s.ConvoDAG {
		t.Errorf("s.persist.DAG() = %v, want same as s.ConvoDAG = %v", got, s.ConvoDAG)
	}
	if got := s.persist.Steering(); got != s.Steering {
		t.Errorf("s.persist.Steering() = %v, want same as s.Steering = %v", got, s.Steering)
	}
	if s.memory == nil {
		t.Fatal("s.memory is nil; NewSessionWithClient must wire the memory service")
	}
	// Memory/Yaad/Enhanced default to nil (no backend installed);
	// the aliasing is what we care about: when SetMemory is called,
	// the legacy field should pick up the new value through the
	// constructor's aliasing pass.
	if got := s.memory.Memory(); got != s.Memory {
		t.Errorf("s.memory.Memory() = %v, want same as s.Memory = %v", got, s.Memory)
	}
	if got := s.memory.Yaad(); got != s.YaadBridge {
		t.Errorf("s.memory.Yaad() = %v, want same as s.YaadBridge = %v", got, s.YaadBridge)
	}
	if got := s.memory.Enhanced(); got != s.EnhancedMemory {
		t.Errorf("s.memory.Enhanced() = %v, want same as s.EnhancedMemory = %v", got, s.EnhancedMemory)
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
