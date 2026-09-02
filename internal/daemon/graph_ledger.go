package daemon

import (
	"context"
	"sync"
	"time"
)

// GraphSyncRecord is a durably retained, accepted graph sync.
type GraphSyncRecord struct {
	SyncID        string
	ProjectID     string
	SessionID     string
	SchemaVersion string
	Digest        string
	Facts         int
	GraphJSON     string
	ReceivedAt    time.Time
}

// GraphLedger durably retains accepted graph syncs so ingested facts and the
// idempotency contract survive daemon restarts. A nil GraphLedger on the
// server falls back to an in-memory ledger (matching the daemon's session
// architecture for tests and unconfigured runs).
type GraphLedger interface {
	// InsertIfAbsent records rec unless syncID is already present. It returns
	// true when inserted, false when a record already exists.
	InsertIfAbsent(ctx context.Context, rec GraphSyncRecord) (bool, error)
	// Get returns the stored record for syncID and whether it exists.
	Get(ctx context.Context, syncID string) (GraphSyncRecord, bool, error)
	// List returns every stored record for projectID, optionally narrowed to a
	// single session when sessionID is non-empty.
	List(ctx context.Context, projectID, sessionID string) ([]GraphSyncRecord, error)
	Close() error
}

// memoryGraphLedger is the default in-memory ledger. It does not survive
// daemon restarts; use OpenGraphLedger for durable retention.
type memoryGraphLedger struct {
	mu sync.Mutex
	m  map[string]GraphSyncRecord
}

func newMemoryGraphLedger() *memoryGraphLedger {
	return &memoryGraphLedger{m: make(map[string]GraphSyncRecord)}
}

func (l *memoryGraphLedger) InsertIfAbsent(ctx context.Context, rec GraphSyncRecord) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.m[rec.SyncID]; ok {
		return false, nil
	}
	l.m[rec.SyncID] = rec
	return true, nil
}

func (l *memoryGraphLedger) Get(ctx context.Context, syncID string) (GraphSyncRecord, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec, ok := l.m[syncID]
	return rec, ok, nil
}

func (l *memoryGraphLedger) List(ctx context.Context, projectID, sessionID string) ([]GraphSyncRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []GraphSyncRecord
	for _, rec := range l.m {
		if rec.ProjectID == projectID && (sessionID == "" || rec.SessionID == sessionID) {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (l *memoryGraphLedger) Close() error { return nil }
