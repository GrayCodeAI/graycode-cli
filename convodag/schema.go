package convodag

import "database/sql"

const schemaSQL = `
CREATE TABLE IF NOT EXISTS nodes (
	id         TEXT PRIMARY KEY,
	parent_id  TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL,
	role       TEXT NOT NULL,
	content    TEXT NOT NULL DEFAULT '',
	model      TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	metadata   TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(parent_id);
CREATE INDEX IF NOT EXISTS idx_nodes_session ON nodes(session_id);

CREATE TABLE IF NOT EXISTS heads (
	session_id TEXT PRIMARY KEY,
	node_id    TEXT NOT NULL,
	FOREIGN KEY (node_id) REFERENCES nodes(id)
);
`

// createSchema initializes the SQLite schema and enables WAL mode.
func createSchema(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return err
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return err
	}
	_, err := db.Exec(schemaSQL)
	return err
}
