package cmdhistory

import "database/sql"

const schemaSQL = `
CREATE TABLE IF NOT EXISTS entries (
	id         TEXT PRIMARY KEY,
	command    TEXT NOT NULL,
	exit_code  INTEGER NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	cwd        TEXT NOT NULL DEFAULT '',
	git_branch TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_entries_cwd ON entries(cwd);
CREATE INDEX IF NOT EXISTS idx_entries_session ON entries(session_id);
CREATE INDEX IF NOT EXISTS idx_entries_created ON entries(created_at);

CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(
	command,
	content='entries',
	content_rowid='rowid'
);

-- Triggers to keep FTS index in sync with entries table.
CREATE TRIGGER IF NOT EXISTS entries_ai AFTER INSERT ON entries BEGIN
	INSERT INTO entries_fts(rowid, command) VALUES (new.rowid, new.command);
END;

CREATE TRIGGER IF NOT EXISTS entries_ad AFTER DELETE ON entries BEGIN
	INSERT INTO entries_fts(entries_fts, rowid, command) VALUES ('delete', old.rowid, old.command);
END;

CREATE TRIGGER IF NOT EXISTS entries_au AFTER UPDATE ON entries BEGIN
	INSERT INTO entries_fts(entries_fts, rowid, command) VALUES ('delete', old.rowid, old.command);
	INSERT INTO entries_fts(rowid, command) VALUES (new.rowid, new.command);
END;
`

// createSchema initializes the SQLite schema and enables WAL mode.
func createSchema(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return err
	}
	_, err := db.Exec(schemaSQL)
	return err
}
