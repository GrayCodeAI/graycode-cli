package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const graphLedgerSchema = `
CREATE TABLE IF NOT EXISTS graph_syncs (
	sync_id        TEXT PRIMARY KEY,
	project_id     TEXT NOT NULL DEFAULT '',
	session_id     TEXT NOT NULL DEFAULT '',
	schema_version TEXT NOT NULL,
	graph_digest   TEXT NOT NULL,
	facts          INTEGER NOT NULL,
	graph_json     TEXT NOT NULL,
	received_at    INTEGER NOT NULL
);`

// sqliteGraphLedger persists accepted graph syncs to a local SQLite database,
// giving durable retention and idempotency across daemon restarts.
type sqliteGraphLedger struct {
	db *sql.DB
}

// OpenGraphLedger opens or creates a durable graph-sync ledger at dbPath.
// The caller is responsible for closing it when done.
func OpenGraphLedger(dbPath string) (GraphLedger, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open graph ledger: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), "PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("graph ledger WAL: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), graphLedgerSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("graph ledger schema: %w", err)
	}
	return &sqliteGraphLedger{db: db}, nil
}

func (l *sqliteGraphLedger) InsertIfAbsent(ctx context.Context, rec GraphSyncRecord) (bool, error) {
	res, err := l.db.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO graph_syncs
			(sync_id, project_id, session_id, schema_version, graph_digest, facts, graph_json, received_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.SyncID, rec.ProjectID, rec.SessionID, rec.SchemaVersion,
		rec.Digest, rec.Facts, rec.GraphJSON, rec.ReceivedAt.UnixNano(),
	)
	if err != nil {
		return false, fmt.Errorf("graph ledger insert: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("graph ledger rows affected: %w", err)
	}
	return n == 1, nil
}

func (l *sqliteGraphLedger) Get(ctx context.Context, syncID string) (GraphSyncRecord, bool, error) {
	var rec GraphSyncRecord
	var receivedAt int64
	err := l.db.QueryRowContext(
		ctx,
		`SELECT sync_id, project_id, session_id, schema_version, graph_digest, facts, graph_json, received_at
		 FROM graph_syncs WHERE sync_id = ?`, syncID,
	).Scan(&rec.SyncID, &rec.ProjectID, &rec.SessionID, &rec.SchemaVersion,
		&rec.Digest, &rec.Facts, &rec.GraphJSON, &receivedAt)
	if err == sql.ErrNoRows {
		return GraphSyncRecord{}, false, nil
	}
	if err != nil {
		return GraphSyncRecord{}, false, fmt.Errorf("graph ledger get: %w", err)
	}
	rec.ReceivedAt = time.Unix(0, receivedAt).UTC()
	return rec, true, nil
}

func (l *sqliteGraphLedger) List(ctx context.Context, projectID, sessionID string) ([]GraphSyncRecord, error) {
	query := `SELECT sync_id, project_id, session_id, schema_version, graph_digest, facts, graph_json, received_at
		 FROM graph_syncs WHERE project_id = ?`
	args := []any{projectID}
	if sessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, sessionID)
	}
	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("graph ledger list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []GraphSyncRecord
	for rows.Next() {
		var rec GraphSyncRecord
		var receivedAt int64
		if err := rows.Scan(&rec.SyncID, &rec.ProjectID, &rec.SessionID, &rec.SchemaVersion,
			&rec.Digest, &rec.Facts, &rec.GraphJSON, &receivedAt); err != nil {
			return nil, fmt.Errorf("graph ledger list scan: %w", err)
		}
		rec.ReceivedAt = time.Unix(0, receivedAt).UTC()
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("graph ledger list: %w", err)
	}
	return out, nil
}

func (l *sqliteGraphLedger) Close() error { return l.db.Close() }
