package session_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/session"
)

func newTestStore(t *testing.T) *session.SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	store, err := session.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestGainTracker_EnsureSchema(t *testing.T) {
	store := newTestStore(t)
	g := session.NewGainTracker(store)
	ctx := context.Background()
	if err := g.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	// Calling again should be a no-op
	if err := g.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema second call: %v", err)
	}
}

func TestGainTracker_Record(t *testing.T) {
	store := newTestStore(t)
	g := session.NewGainTracker(store)
	if err := g.EnsureSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ev := session.GainEvent{
		SessionID:        "sess-1",
		Command:          "tok npm test",
		OriginalBytes:    1000,
		CompressedBytes:  200,
		OriginalTokens:   250,
		CompressedTokens: 50,
		Mode:             "aggressive",
		Tier:             "code",
		Model:            "gpt-4o",
	}
	if err := g.Record(ctx, ev); err != nil {
		t.Fatalf("Record: %v", err)
	}
}

func TestGainTracker_RecordRequiresSessionID(t *testing.T) {
	store := newTestStore(t)
	g := session.NewGainTracker(store)
	_ = g.EnsureSchema(context.Background())
	err := g.Record(context.Background(), session.GainEvent{Command: "x"})
	if err == nil {
		t.Error("expected error for empty SessionID")
	}
}

func TestGainTracker_AggregateForSession(t *testing.T) {
	store := newTestStore(t)
	g := session.NewGainTracker(store)
	_ = g.EnsureSchema(context.Background())
	ctx := context.Background()
	events := []session.GainEvent{
		{SessionID: "s1", Command: "a", OriginalBytes: 1000, CompressedBytes: 200, OriginalTokens: 250, CompressedTokens: 50, Mode: "aggressive"},
		{SessionID: "s1", Command: "b", OriginalBytes: 2000, CompressedBytes: 800, OriginalTokens: 500, CompressedTokens: 200, Mode: "aggressive"},
		{SessionID: "s1", Command: "c", OriginalBytes: 500, CompressedBytes: 250, OriginalTokens: 125, CompressedTokens: 60, Mode: "minimal"},
		// Different session — should not be included
		{SessionID: "s2", Command: "d", OriginalBytes: 100, CompressedBytes: 50, OriginalTokens: 25, CompressedTokens: 12, Mode: "minimal"},
	}
	for _, ev := range events {
		if err := g.Record(ctx, ev); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	agg, err := g.AggregateForSession(ctx, "s1", 30)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if agg.EventCount != 3 {
		t.Errorf("expected 3 events for s1, got %d", agg.EventCount)
	}
	// Bytes saved: 800 + 1200 + 250 = 2250
	if agg.TotalBytesSaved != 2250 {
		t.Errorf("expected 2250 bytes saved, got %d", agg.TotalBytesSaved)
	}
	// Tokens saved: 200 + 300 + 65 = 565
	if agg.TotalTokensSaved != 565 {
		t.Errorf("expected 565 tokens saved, got %d", agg.TotalTokensSaved)
	}
}

func TestGainTracker_AggregateForSession_Empty(t *testing.T) {
	store := newTestStore(t)
	g := session.NewGainTracker(store)
	_ = g.EnsureSchema(context.Background())
	agg, err := g.AggregateForSession(context.Background(), "nonexistent", 30)
	if err != nil {
		t.Fatalf("Aggregate on empty: %v", err)
	}
	if agg.EventCount != 0 {
		t.Errorf("expected 0 events, got %d", agg.EventCount)
	}
}

func TestGainTracker_ListForSession(t *testing.T) {
	store := newTestStore(t)
	g := session.NewGainTracker(store)
	_ = g.EnsureSchema(context.Background())
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = g.Record(ctx, session.GainEvent{
			SessionID: "s1", Command: "x",
			OriginalBytes: 100, CompressedBytes: 50,
		})
		time.Sleep(2 * time.Millisecond)
	}
	// Different session to verify isolation
	_ = g.Record(ctx, session.GainEvent{SessionID: "s2", Command: "y", OriginalBytes: 100, CompressedBytes: 50})

	out, err := g.ListForSession(ctx, "s1", 10)
	if err != nil {
		t.Fatalf("ListForSession: %v", err)
	}
	if len(out) != 5 {
		t.Errorf("expected 5 events for s1, got %d", len(out))
	}
	// Newest first
	if out[0].ID <= out[1].ID {
		t.Error("expected descending ID order")
	}
}

func TestGainTracker_PruneForSession(t *testing.T) {
	store := newTestStore(t)
	g := session.NewGainTracker(store)
	_ = g.EnsureSchema(context.Background())
	ctx := context.Background()
	// Old event
	_ = g.Record(ctx, session.GainEvent{
		SessionID: "s1", Command: "old",
		OriginalBytes: 100, CompressedBytes: 50,
		Timestamp: time.Now().Add(-100 * 24 * time.Hour),
	})
	// Recent event
	_ = g.Record(ctx, session.GainEvent{
		SessionID: "s1", Command: "new",
		OriginalBytes: 100, CompressedBytes: 50,
	})
	deleted, err := g.PruneForSession(ctx, "s1", 30*24*time.Hour)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 pruned, got %d", deleted)
	}
	agg, _ := g.AggregateForSession(ctx, "s1", 0)
	if agg.EventCount != 1 {
		t.Errorf("expected 1 remaining, got %d", agg.EventCount)
	}
}

func TestGainTracker_PrunePreservesOtherSessions(t *testing.T) {
	store := newTestStore(t)
	g := session.NewGainTracker(store)
	_ = g.EnsureSchema(context.Background())
	ctx := context.Background()
	// Old event in s1
	_ = g.Record(ctx, session.GainEvent{
		SessionID: "s1", Command: "old",
		OriginalBytes: 100, CompressedBytes: 50,
		Timestamp: time.Now().Add(-100 * 24 * time.Hour),
	})
	// Old event in s2
	_ = g.Record(ctx, session.GainEvent{
		SessionID: "s2", Command: "old",
		OriginalBytes: 100, CompressedBytes: 50,
		Timestamp: time.Now().Add(-100 * 24 * time.Hour),
	})
	// Prune s1 only
	_, err := g.PruneForSession(ctx, "s1", 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// s2 still has its event
	agg, _ := g.AggregateForSession(ctx, "s2", 0)
	if agg.EventCount != 1 {
		t.Errorf("expected s2 to keep its 1 event, got %d", agg.EventCount)
	}
}
