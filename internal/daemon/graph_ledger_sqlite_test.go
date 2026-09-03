package daemon

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/testutil"
)

func TestSQLiteGraphLedger_PersistsAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "graph-syncs.db")

	rec := GraphSyncRecord{
		SyncID:        "sync-1",
		ProjectID:     "proj",
		SessionID:     "sess",
		SchemaVersion: "graycode.graph/v1",
		Digest:        "abc123",
		Facts:         4,
		GraphJSON:     `{"schema_version":"graycode.graph/v1","nodes":[]}`,
		ReceivedAt:    time.Now().UTC(),
	}

	ledger, err := OpenGraphLedger(dbPath)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	inserted, err := ledger.InsertIfAbsent(context.Background(), rec)
	if err != nil || !inserted {
		t.Fatalf("first insert: inserted=%v err=%v", inserted, err)
	}
	// Re-insert of the same sync ID must not overwrite.
	inserted, err = ledger.InsertIfAbsent(context.Background(), rec)
	if err != nil || inserted {
		t.Fatalf("duplicate insert: inserted=%v err=%v", inserted, err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen the same file, simulating a daemon restart.
	ledger2, err := OpenGraphLedger(dbPath)
	if err != nil {
		t.Fatalf("reopen ledger: %v", err)
	}
	defer ledger2.Close()

	got, ok, err := ledger2.Get(context.Background(), "sync-1")
	if err != nil || !ok {
		t.Fatalf("get after reopen: ok=%v err=%v", ok, err)
	}
	if got.Digest != "abc123" || got.Facts != 4 || got.GraphJSON != rec.GraphJSON {
		t.Fatalf("retained record mismatch: %+v", got)
	}
	if got.ProjectID != "proj" || got.SessionID != "sess" || got.SchemaVersion != "graycode.graph/v1" {
		t.Fatalf("retained metadata mismatch: %+v", got)
	}
	if got.ReceivedAt.IsZero() {
		t.Fatalf("received_at not retained")
	}
	if _, ok, err := ledger2.Get(context.Background(), "missing"); err != nil || ok {
		t.Fatalf("missing key: ok=%v err=%v", ok, err)
	}
}

// TestDaemon_GraphSync_DurableLedger proves the handler persists accepted
// facts through the injected ledger, and that a reopened ledger still sees
// them (surviving a daemon restart).
func TestDaemon_GraphSync_DurableLedger(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "graph-syncs.db")
	ledger, err := OpenGraphLedger(dbPath)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}

	srv := New(Config{Port: 0, Host: testutil.LoopbackHost, GraphLedger: ledger}, nil)
	addr := startTestDaemon(t, srv)
	t.Cleanup(func() { srv.Stop(context.Background()) })

	body := graphSyncBody(t, "durable-1", validTestExport())
	resp := postGraphSync(t, addr, body)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Reopen the ledger as a fresh server would and confirm the fact was
	// durably retained.
	if err := ledger.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}
	ledger2, err := OpenGraphLedger(dbPath)
	if err != nil {
		t.Fatalf("reopen ledger: %v", err)
	}
	defer ledger2.Close()

	rec, ok, err := ledger2.Get(context.Background(), "durable-1")
	if err != nil || !ok {
		t.Fatalf("get retained sync: ok=%v err=%v", ok, err)
	}
	if rec.Facts != 4 || rec.Digest == "" || rec.GraphJSON == "" {
		t.Fatalf("retained record incomplete: %+v", rec)
	}
}
