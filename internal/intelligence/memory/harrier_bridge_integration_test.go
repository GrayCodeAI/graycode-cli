package memory

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	graphcontracts "github.com/GrayCodeAI/eagle/graph"
	"github.com/GrayCodeAI/graycode-cli/internal/graphjournal"
)

func newTestBridge(t *testing.T) *HarrierBridge {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.harrier/data", 0o755)

	b := NewHarrierBridge()
	if b == nil {
		t.Fatal("NewHarrierBridge returned nil")
	}
	return b
}

func TestHarrierBridge_Init(t *testing.T) {
	b := newTestBridge(t)
	if !b.ready {
		// FIXME: test skipped in TestHarrierBridge_Init
		// FIXME: harrier bridge requires the harrier dependency to be available at runtime
		t.Skip("harrier bridge could not initialize (missing harrier dependency)")
	}
}

func TestHarrierBridge_Remember(t *testing.T) {
	// FIXME: test skipped in TestHarrierBridge_Remember
	b := newTestBridge(t)
	if !b.ready {
		// FIXME: harrier dependency must be available to test remember functionality
		t.Skip("harrier not available")
	}
	err := b.Remember("test content to remember", "explicit")
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
}

// FIXME: test skipped in TestHarrierBridge_Remember

func TestHarrierBridge_Recall(t *testing.T) {
	b := newTestBridge(t)
	if !b.ready {
		// FIXME: harrier dependency must be available to test recall functionality
		t.Skip("harrier not available")
	}
	_ = b.Remember("golang error handling patterns", "convention")

	result, err := b.Recall("error handling", 500)
	if err != nil {
		t.Fatalf("Recall: %v", err)
		// FIXME: test skipped in TestHarrierBridge_Recall
	}
	_ = result
}

func TestHarrierBridgeRecallRecordsPortableContextGraph(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())
	b := newTestBridge(t)
	if !b.ready {
		// TODO: track hermetic harrier availability so this test runs without skipping.
		t.Skip("harrier not available")
	}
	defer b.Close()

	if err := b.Remember("private graph context about error handling", "decision"); err != nil {
		t.Fatalf("Remember() error = %v", err)
	}
	b.ConfigureGraphObservation(
		"session-context",
		graphcontracts.Scope{RepositoryID: "graycode"},
	)
	if _, err := b.Recall("private graph context", 500); err != nil {
		t.Fatalf("Recall() error = %v", err)
	}

	entries, err := graphjournal.Load("session-context")
	if err != nil {
		t.Fatalf("graphjournal.Load() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Context == nil {
		t.Fatalf("context entries = %#v, want one", entries)
	}
	if len(entries[0].Context.Nodes) == 0 {
		t.Fatal("context graph has no knowledge nodes")
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, secret := range []string{"private graph context about error handling", "private graph context"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("context graph journal leaked %q", secret)
		}
	}
	if entries[0].OccurredAt.After(time.Now().Add(time.Second)) {
		t.Fatal("context observation timestamp is in the future")
	}
}

func TestHarrierBridgeCodeSearchRecordsPortableContextGraph(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())
	b := newTestBridge(t)
	if !b.ready {
		// TODO: track hermetic harrier availability so this test runs without skipping.
		t.Skip("harrier not available")
	}
	defer b.Close()

	if err := b.InitCodeIndex(); err != nil {
		t.Fatalf("InitCodeIndex() error = %v", err)
	}
	if err := b.IndexCodeChunk(
		"/private/project/auth.go",
		"func privateAuthenticationImplementation() {}",
		"privateAuthenticationImplementation",
		"go",
		10,
		12,
		8,
		"file-hash",
	); err != nil {
		t.Fatalf("IndexCodeChunk() error = %v", err)
	}
	b.ConfigureGraphObservation(
		"session-code-context",
		graphcontracts.Scope{RepositoryID: "graycode"},
	)
	if _, err := b.SearchCode("privateAuthenticationImplementation", 5); err != nil {
		t.Fatalf("SearchCode() error = %v", err)
	}

	entries, err := graphjournal.Load("session-code-context")
	if err != nil {
		t.Fatalf("graphjournal.Load() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Context == nil || len(entries[0].Context.Nodes) != 1 {
		t.Fatalf("code context entries = %#v, want one node", entries)
	}
	if got := entries[0].Context.Nodes[0].Attributes["entity_type"]; got != "code_chunk" {
		t.Fatalf("entity_type = %q, want code_chunk", got)
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, secret := range []string{
		"/private/project/auth.go",
		"func privateAuthenticationImplementation() {}",
		"privateAuthenticationImplementation",
	} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("code context graph journal leaked %q", secret)
		}
	}
}

func TestHarrierBridge_Close(t *testing.T) {
	b := newTestBridge(t)
	if !b.ready {
		// FIXME: harrier not available
		t.Skip("harrier not available")
	}
	_ = b.store.Close()
}

// TestHarrierBridge_EnsureBackups verifies the scheduler is started once per
// database directory, idempotent across repeated calls, and torn down by
// Close so a later bridge can restart snapshots.
func TestHarrierBridge_EnsureBackups(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.harrier/data", 0o755)

	b := NewHarrierBridge()
	if !b.ready {
		// TODO: track hermetic harrier availability so this test runs without skipping.
		t.Skip("harrier not available")
	}

	b.EnsureBackups()
	b.EnsureBackups() // must be a no-op, not a second scheduler

	b.mu.Lock()
	sched := b.backupSched
	b.mu.Unlock()
	if sched == nil {
		t.Fatal("expected backupSched to be set after EnsureBackups")
	}

	// Second bridge on the same dbDir reuses the existing claim.
	b2 := NewHarrierBridge()
	if !b2.ready {
		// TODO: track hermetic harrier availability so this test runs without skipping.
		t.Skip("harrier not available")
	}
	b2.EnsureBackups()
	b2.mu.Lock()
	dup := b2.backupSched
	b2.mu.Unlock()
	if dup != nil {
		t.Fatal("second bridge must reuse the existing scheduler")
	}
	// Close a non-owning bridge: no scheduler to stop, no claim freed.
	b2.Close()

	b.Close()
	b.mu.Lock()
	stopped := b.backupSched == nil
	b.mu.Unlock()
	if !stopped {
		t.Fatal("expected Close to clear the scheduler reference")
	}

	// After Close the directory claim is released: a fresh bridge can
	// re-register without error.
	b3 := NewHarrierBridge()
	if !b3.ready {
		// TODO: track hermetic harrier availability so this test runs without skipping.
		t.Skip("harrier not available")
	}
	b3.EnsureBackups()
	b3.mu.Lock()
	restarted := b3.backupSched != nil
	b3.mu.Unlock()
	if !restarted {
		t.Fatal("expected a fresh bridge to start its own scheduler after Close")
	}
	b3.Close()
}

func TestConfidenceTracker_WithBridge(t *testing.T) {
	b := newTestBridge(t)
	if !b.ready {
		// FIXME: test skipped
		t.Skip("harrier not available")
	}

	// FIXME: test skipped in TestConfidenceTracker_WithBridge
	ct := NewConfidenceTracker(b)
	ct.RecordAccess("node-1", "node-2")
	ct.OnSessionSuccess()
	ct.Reset()
}

func TestProactiveContext_WithBridge(t *testing.T) {
	b := newTestBridge(t)
	// FIXME: test skipped
	if !b.ready {
		// FIXME: test skipped
		t.Skip("harrier not available")
	}

	pc := NewProactiveContext(b)
	pc.TrackFile("main.go")
	pc.TrackFiles([]string{"config.go", "handler.go"})

	// FIXME: test skipped in TestProactiveContext_WithBridge

	ctx := pc.ContextForFile("main.go")
	_ = ctx

	pc.Reset()
}

func TestGraphAwareBudget_WithBridge(t *testing.T) {
	// FIXME: test skipped
	b := newTestBridge(t)
	// FIXME: test skipped
	if !b.ready {
		// FIXME: test skipped
		t.Skip("harrier not available")
	}

	pc := NewProactiveContext(b)
	gb := NewGraphAwareBudget(b, pc)

	budget := gb.ComputeBudget([]string{"main.go"}, 0.5)
	if budget <= 0 {
		t.Errorf("budget = %d, want > 0", budget)
	}

	injection := gb.BuildInjection("fix the bug", []string{"main.go"}, 500)
	_ = injection
}
