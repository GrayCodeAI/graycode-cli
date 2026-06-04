// session_gain.go: per-session gain tracking for compression events.
//
// Each call to tok.Compress(), filter.SmartTruncate, or any other
// compression step that happens during a hawk session can be recorded
// here for later aggregation. The tracker stores events in a separate
// SQLite table in the same database as the session store, so per-
// session and per-day aggregations are available without joining
// against an external DB.
//
// Per-session gain tracking for compression stats
// and tok's internal/tracking, but session-scoped.
//
// Usage:
//
//	tracker := session.NewGainTracker(sess.SQLiteStore())
//	tracker.Record(ctx, session.GainEvent{
//	    SessionID: sess.ID,
//	    Command:   "tok npm test",
//	    OriginalBytes:   12000,
//	    CompressedBytes: 2400,
//	    OriginalTokens:  3000,
//	    CompressedTokens: 600,
//	    Mode:             "aggressive",
//	    Tier:             "code",
//	})
package session

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GainEvent is a single compression event recorded against a session.
type GainEvent struct {
	ID               int64
	SessionID        string
	Timestamp        time.Time
	Command          string
	OriginalBytes    int
	CompressedBytes  int
	OriginalTokens   int
	CompressedTokens int
	Mode             string
	Tier             string
	Model            string
}

// GainAggregate is the result of an aggregate query scoped to a
// session (or set of sessions).
type GainAggregate struct {
	EventCount       int
	TotalBytesSaved  int
	TotalTokensSaved int
	AvgSavingsPct    float64
	PeriodStart      time.Time
	PeriodEnd        time.Time
}

// GainTracker records per-session gain events. The zero value is
// not usable; construct via NewGainTracker with a non-nil
// SQLiteStore.
type GainTracker struct {
	store *SQLiteStore
}

// NewGainTracker returns a tracker that writes gain events into
// the given SQLiteStore's database. The store must already be
// open (Close will close it; GainTracker does not).
func NewGainTracker(store *SQLiteStore) *GainTracker {
	return &GainTracker{store: store}
}

// schema is the gains table definition. Appended to the
// session store's schema on first use.
const gainsSchema = `
CREATE TABLE IF NOT EXISTS gains (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id        TEXT    NOT NULL,
    ts                INTEGER NOT NULL,
    command           TEXT    NOT NULL DEFAULT '',
    original_bytes    INTEGER NOT NULL,
    compressed_bytes  INTEGER NOT NULL,
    original_tokens   INTEGER NOT NULL,
    compressed_tokens INTEGER NOT NULL,
    mode              TEXT    NOT NULL DEFAULT '',
    tier              TEXT    NOT NULL DEFAULT '',
    model             TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_gains_session_ts ON gains(session_id, ts);
`

// EnsureSchema creates the gains table and indexes if they do not
// already exist. Safe to call multiple times.
func (g *GainTracker) EnsureSchema(ctx context.Context) error {
	if g == nil || g.store == nil || g.store.db == nil {
		return fmt.Errorf("session: GainTracker has no store")
	}
	_, err := g.store.db.ExecContext(ctx, gainsSchema)
	if err != nil {
		return fmt.Errorf("session: gains schema: %w", err)
	}
	return nil
}

// Record adds a new gain event. Timestamp defaults to now if zero.
// SessionID is required.
func (g *GainTracker) Record(ctx context.Context, ev GainEvent) error {
	if g == nil || g.store == nil || g.store.db == nil {
		return fmt.Errorf("session: GainTracker has no store")
	}
	if ev.SessionID == "" {
		return fmt.Errorf("session: GainEvent.SessionID is required")
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	_, err := g.store.db.ExecContext(ctx, `
		INSERT INTO gains
		  (session_id, ts, command, original_bytes, compressed_bytes,
		   original_tokens, compressed_tokens, mode, tier, model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		ev.SessionID, ev.Timestamp.Unix(), ev.Command,
		ev.OriginalBytes, ev.CompressedBytes,
		ev.OriginalTokens, ev.CompressedTokens,
		ev.Mode, ev.Tier, ev.Model,
	)
	if err != nil {
		return fmt.Errorf("session: record gain: %w", err)
	}
	return nil
}

// AggregateForSession returns aggregate stats for one session over
// the last `days` days (or all-time if days <= 0).
func (g *GainTracker) AggregateForSession(ctx context.Context, sessionID string, days int) (GainAggregate, error) {
	if g == nil || g.store == nil || g.store.db == nil {
		return GainAggregate{}, fmt.Errorf("session: GainTracker has no store")
	}
	if sessionID == "" {
		return GainAggregate{}, fmt.Errorf("session: sessionID required")
	}
	cutoff := int64(0)
	if days > 0 {
		cutoff = time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	}
	row := g.store.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(original_bytes - compressed_bytes), 0),
			COALESCE(SUM(original_tokens - compressed_tokens), 0),
			COALESCE(AVG(
				CASE WHEN original_bytes = 0 THEN 0
				     ELSE 100.0 * (original_bytes - compressed_bytes) / original_bytes
				END
			), 0)
		FROM gains
		WHERE session_id = ? AND ts >= ?
	`, sessionID, cutoff)
	var agg GainAggregate
	if err := row.Scan(&agg.EventCount, &agg.TotalBytesSaved, &agg.TotalTokensSaved, &agg.AvgSavingsPct); err != nil {
		if err == sql.ErrNoRows {
			return agg, nil
		}
		return agg, fmt.Errorf("session: aggregate: %w", err)
	}
	agg.PeriodEnd = time.Now()
	if cutoff > 0 {
		agg.PeriodStart = time.Unix(cutoff, 0)
	}
	return agg, nil
}

// ListForSession returns the most recent n gain events for one
// session, newest first.
func (g *GainTracker) ListForSession(ctx context.Context, sessionID string, n int) ([]GainEvent, error) {
	if g == nil || g.store == nil || g.store.db == nil {
		return nil, fmt.Errorf("session: GainTracker has no store")
	}
	if n <= 0 {
		n = 50
	}
	rows, err := g.store.db.QueryContext(ctx, `
		SELECT id, session_id, ts, command,
		       original_bytes, compressed_bytes,
		       original_tokens, compressed_tokens,
		       mode, tier, model
		FROM gains
		WHERE session_id = ?
		ORDER BY id DESC
		LIMIT ?
	`, sessionID, n)
	if err != nil {
		return nil, fmt.Errorf("session: list gains: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []GainEvent
	for rows.Next() {
		var ev GainEvent
		var ts int64
		if err := rows.Scan(&ev.ID, &ev.SessionID, &ts, &ev.Command,
			&ev.OriginalBytes, &ev.CompressedBytes,
			&ev.OriginalTokens, &ev.CompressedTokens,
			&ev.Mode, &ev.Tier, &ev.Model); err != nil {
			return nil, err
		}
		ev.Timestamp = time.Unix(ts, 0)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// PruneForSession deletes gain events for one session older than
// `maxAge`. Returns the number of rows deleted.
func (g *GainTracker) PruneForSession(ctx context.Context, sessionID string, maxAge time.Duration) (int64, error) {
	if g == nil || g.store == nil || g.store.db == nil {
		return 0, fmt.Errorf("session: GainTracker has no store")
	}
	cutoff := time.Now().Add(-maxAge).Unix()
	res, err := g.store.db.ExecContext(ctx,
		`DELETE FROM gains WHERE session_id = ? AND ts < ?`,
		sessionID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("session: prune: %w", err)
	}
	return res.RowsAffected()
}
