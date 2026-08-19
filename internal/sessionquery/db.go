package sessionquery

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/storage"
	_ "modernc.org/sqlite"
)

const schemaDDL = `
CREATE TABLE IF NOT EXISTS sessions_meta (
    session_id TEXT PRIMARY KEY,
    workspace TEXT NOT NULL,
    model TEXT NOT NULL,
    provider TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    message_count INTEGER NOT NULL,
    file_mod_time INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_workspace ON sessions_meta(workspace);

CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    session_id UNINDEXED,
    role UNINDEXED,
    msg_index UNINDEXED,
    content,
    tokenize='porter unicode61'
);
`

// DB wraps the SQLite database connection with connection synchronization.
type DB struct {
	mu     sync.RWMutex
	conn   *sql.DB
	dbPath string
}

// OpenDB opens (or creates) the SQLite database at dbPath and initializes the schema.
func OpenDB(dbPath string) (*DB, error) {
	if dbPath == "" {
		dbPath = filepath.Join(storage.CacheDir(), "session_query.db")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o750); err != nil {
		return nil, fmt.Errorf("failed to create directory for session query database: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("failed to open session query database: %w", err)
	}

	// Single writer connection pool
	conn.SetMaxOpenConns(1)

	if _, err := conn.Exec(schemaDDL); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to initialize session query schema: %w", err)
	}

	return &DB{
		conn:   conn,
		dbPath: dbPath,
	}, nil
}

// Close closes the underlying SQLite connection.
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn != nil {
		err := d.conn.Close()
		d.conn = nil
		return err
	}
	return nil
}

// Conn returns the raw SQL DB connection.
func (d *DB) Conn() *sql.DB {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.conn
}
