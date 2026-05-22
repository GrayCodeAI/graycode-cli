package session

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SQLite-based session storage replaces the fragile JSONL + WAL approach.
// This uses database/sql with the "sqlite" driver name. Consumers must import
// a compatible driver, e.g.:
//
//	import _ "modernc.org/sqlite"
//
// This is a pure-Go SQLite implementation (no CGO required).

// schema defines the initial database schema (version 1).
const schema = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    project_dir TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    parent_id TEXT,
    status TEXT DEFAULT 'active',
    title TEXT,
    total_tokens INTEGER DEFAULT 0,
    total_cost_usd REAL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES sessions(id),
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    tool_use_id TEXT,
    tool_name TEXT,
    is_error BOOLEAN DEFAULT FALSE,
    tokens INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_dir);
CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at DESC);
`

// migrations is an ordered list of schema migrations. Each entry is applied
// exactly once, tracked by the schema_version table.
var migrations = []string{
	// Version 1: initial schema
	schema,
	// Version 2: add FTS for content search
	`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
		session_id,
		content,
		content='messages',
		content_rowid='id'
	);

	CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
		INSERT INTO messages_fts(rowid, session_id, content)
		VALUES (new.id, new.session_id, new.content);
	END;

	CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
		INSERT INTO messages_fts(messages_fts, rowid, session_id, content)
		VALUES ('delete', old.id, old.session_id, old.content);
	END;`,
}

// SessionRecord represents a persisted session in the SQLite store.
type SessionRecord struct {
	ID           string
	ProjectDir   string
	Provider     string
	Model        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ParentID     string
	Status       string
	Title        string
	TotalTokens  int
	TotalCostUSD float64
}

// MessageRecord represents a single message within a session.
type MessageRecord struct {
	ID        int64
	SessionID string
	Role      string
	Content   string
	ToolUseID string
	ToolName  string
	IsError   bool
	Tokens    int
	CreatedAt time.Time
}

// SessionStats contains aggregated statistics for a session.
type SessionStats struct {
	MessageCount int
	TotalTokens  int
	TotalCostUSD float64
	Duration     time.Duration
	ToolCalls    int
}

// SQLiteStore provides SQLite-backed session persistence.
type SQLiteStore struct {
	db     *sql.DB
	dbPath string
	mu     sync.RWMutex
}

// NewSQLiteStore opens (or creates) the SQLite database at dbPath and runs
// any pending migrations. The driver must already be registered with
// database/sql (e.g., via import _ "modernc.org/sqlite").
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.ExecContext(context.Background(), "PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Enable foreign keys.
	if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	s := &SQLiteStore{db: db, dbPath: dbPath}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// migrate applies any pending schema migrations.
func (s *SQLiteStore) migrate() error {
	// Ensure the schema_version table exists.
	_, err := s.db.ExecContext(context.Background(), `CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY
	)`)
	if err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	// Determine current version.
	var current int
	row := s.db.QueryRowContext(context.Background(), "SELECT COALESCE(MAX(version), 0) FROM schema_version")
	if err := row.Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	// Apply pending migrations.
	for i := current; i < len(migrations); i++ {
		tx, err := s.db.BeginTx(context.Background(), nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", i+1, err)
		}

		// Execute all statements in this migration.
		// Split on semicolons for multi-statement migrations.
		stmts := splitStatements(migrations[i])
		for _, stmt := range stmts {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := tx.ExecContext(context.Background(), stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d failed: %w\nstatement: %s", i+1, err, stmt)
			}
		}

		// Record the new version.
		if _, err := tx.ExecContext(context.Background(), "INSERT INTO schema_version (version) VALUES (?)", i+1); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", i+1, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", i+1, err)
		}
	}

	return nil
}

// splitStatements splits a SQL string on semicolons, being careful not to
// split inside string literals or BEGIN...END blocks (triggers, etc.).
func splitStatements(sql string) []string {
	var stmts []string
	var current strings.Builder
	inString := false
	beginDepth := 0

	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		if ch == '\'' {
			inString = !inString
			current.WriteByte(ch)
		} else if !inString {
			// Track BEGIN...END nesting for triggers
			upper := strings.ToUpper(sql[i:])
			if strings.HasPrefix(upper, "BEGIN") && (i+5 >= len(sql) || !isIdentChar(sql[i+5])) {
				beginDepth++
				current.WriteString(sql[i : i+5])
				i += 4
			} else if strings.HasPrefix(upper, "END") && (i+3 >= len(sql) || !isIdentChar(sql[i+3])) && beginDepth > 0 {
				beginDepth--
				current.WriteString(sql[i : i+3])
				i += 2
			} else if ch == ';' && beginDepth == 0 {
				s := strings.TrimSpace(current.String())
				if s != "" {
					stmts = append(stmts, s)
				}
				current.Reset()
			} else {
				current.WriteByte(ch)
			}
		} else {
			current.WriteByte(ch)
		}
	}

	s := strings.TrimSpace(current.String())
	if s != "" {
		stmts = append(stmts, s)
	}

	return stmts
}

func isIdentChar(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

// CreateSession inserts a new session record.
func (s *SQLiteStore) CreateSession(sess *SessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now()
	}
	if sess.UpdatedAt.IsZero() {
		sess.UpdatedAt = time.Now()
	}
	if sess.Status == "" {
		sess.Status = "active"
	}

	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO sessions (id, project_dir, provider, model, created_at, updated_at, parent_id, status, title, total_tokens, total_cost_usd)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.ProjectDir, sess.Provider, sess.Model,
		sess.CreatedAt, sess.UpdatedAt, sess.ParentID, sess.Status,
		sess.Title, sess.TotalTokens, sess.TotalCostUSD,
	)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// GetSession retrieves a session by ID.
func (s *SQLiteStore) GetSession(id string) (*SessionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRowContext(context.Background(), `SELECT id, project_dir, provider, model, created_at, updated_at,
		COALESCE(parent_id, ''), status, COALESCE(title, ''), total_tokens, total_cost_usd
		FROM sessions WHERE id = ?`, id)

	var sess SessionRecord
	err := row.Scan(&sess.ID, &sess.ProjectDir, &sess.Provider, &sess.Model,
		&sess.CreatedAt, &sess.UpdatedAt, &sess.ParentID, &sess.Status,
		&sess.Title, &sess.TotalTokens, &sess.TotalCostUSD)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &sess, nil
}

// ListSessions returns sessions for a project directory, ordered by most
// recently updated. If projectDir is empty, all sessions are returned.
// limit <= 0 means no limit.
func (s *SQLiteStore) ListSessions(projectDir string, limit int) ([]*SessionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var rows *sql.Rows
	var err error

	if projectDir == "" {
		if limit > 0 {
			rows, err = s.db.QueryContext(context.Background(), `SELECT id, project_dir, provider, model, created_at, updated_at,
				COALESCE(parent_id, ''), status, COALESCE(title, ''), total_tokens, total_cost_usd
				FROM sessions ORDER BY updated_at DESC LIMIT ?`, limit)
		} else {
			rows, err = s.db.QueryContext(context.Background(), `SELECT id, project_dir, provider, model, created_at, updated_at,
				COALESCE(parent_id, ''), status, COALESCE(title, ''), total_tokens, total_cost_usd
				FROM sessions ORDER BY updated_at DESC`)
		}
	} else {
		if limit > 0 {
			rows, err = s.db.QueryContext(context.Background(), `SELECT id, project_dir, provider, model, created_at, updated_at,
				COALESCE(parent_id, ''), status, COALESCE(title, ''), total_tokens, total_cost_usd
				FROM sessions WHERE project_dir = ? ORDER BY updated_at DESC LIMIT ?`, projectDir, limit)
		} else {
			rows, err = s.db.QueryContext(context.Background(), `SELECT id, project_dir, provider, model, created_at, updated_at,
				COALESCE(parent_id, ''), status, COALESCE(title, ''), total_tokens, total_cost_usd
				FROM sessions WHERE project_dir = ? ORDER BY updated_at DESC`, projectDir)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sessions []*SessionRecord
	for rows.Next() {
		var sess SessionRecord
		if err := rows.Scan(&sess.ID, &sess.ProjectDir, &sess.Provider, &sess.Model,
			&sess.CreatedAt, &sess.UpdatedAt, &sess.ParentID, &sess.Status,
			&sess.Title, &sess.TotalTokens, &sess.TotalCostUSD); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, &sess)
	}
	return sessions, rows.Err()
}

// AppendMessage adds a message to a session and updates the session's
// updated_at timestamp and token totals.
func (s *SQLiteStore) AppendMessage(sessionID string, msg *MessageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	result, err := tx.ExecContext(context.Background(), `INSERT INTO messages (session_id, role, content, tool_use_id, tool_name, is_error, tokens, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, msg.Role, msg.Content, msg.ToolUseID, msg.ToolName,
		msg.IsError, msg.Tokens, msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}

	id, _ := result.LastInsertId()
	msg.ID = id
	msg.SessionID = sessionID

	// Update session metadata.
	_, err = tx.ExecContext(context.Background(), `UPDATE sessions SET updated_at = ?, total_tokens = total_tokens + ?
		WHERE id = ?`, time.Now(), msg.Tokens, sessionID)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}

	return tx.Commit()
}

// GetMessages retrieves all messages for a session, ordered by creation time.
func (s *SQLiteStore) GetMessages(sessionID string) ([]*MessageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(context.Background(), `SELECT id, session_id, role, content,
		COALESCE(tool_use_id, ''), COALESCE(tool_name, ''), is_error, tokens, created_at
		FROM messages WHERE session_id = ? ORDER BY id ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var messages []*MessageRecord
	for rows.Next() {
		var msg MessageRecord
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content,
			&msg.ToolUseID, &msg.ToolName, &msg.IsError, &msg.Tokens, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, &msg)
	}
	return messages, rows.Err()
}

// UpdateSession updates specific fields of a session. Supported keys:
// status, title, model, provider, total_tokens, total_cost_usd.
func (s *SQLiteStore) UpdateSession(id string, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(updates) == 0 {
		return nil
	}

	// Whitelist of allowed fields.
	allowed := map[string]bool{
		"status":         true,
		"title":          true,
		"model":          true,
		"provider":       true,
		"total_tokens":   true,
		"total_cost_usd": true,
		"parent_id":      true,
	}

	var setClauses []string
	var args []interface{}

	for key, val := range updates {
		if !allowed[key] {
			return fmt.Errorf("disallowed update field: %s", key)
		}
		setClauses = append(setClauses, key+" = ?")
		args = append(args, val)
	}

	// Always update updated_at.
	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, time.Now())
	args = append(args, id)

	query := fmt.Sprintf("UPDATE sessions SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	result, err := s.db.ExecContext(context.Background(), query, args...)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session %s not found", id)
	}
	return nil
}

// DeleteSession removes a session and all its messages.
func (s *SQLiteStore) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete messages first (FK constraint).
	if _, err := tx.ExecContext(context.Background(), "DELETE FROM messages WHERE session_id = ?", id); err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}

	result, err := tx.ExecContext(context.Background(), "DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session %s not found", id)
	}

	return tx.Commit()
}

// ForkSession creates a copy of a session with a new ID, duplicating all
// messages. The new session's parent_id points to the original.
func (s *SQLiteStore) ForkSession(originalID, newID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Copy the session record.
	now := time.Now()
	_, err = tx.ExecContext(context.Background(), `INSERT INTO sessions (id, project_dir, provider, model, created_at, updated_at, parent_id, status, title, total_tokens, total_cost_usd)
		SELECT ?, project_dir, provider, model, ?, ?, ?, status, title, total_tokens, total_cost_usd
		FROM sessions WHERE id = ?`,
		newID, now, now, originalID, originalID)
	if err != nil {
		return fmt.Errorf("copy session: %w", err)
	}

	// Copy all messages.
	_, err = tx.ExecContext(context.Background(), `INSERT INTO messages (session_id, role, content, tool_use_id, tool_name, is_error, tokens, created_at)
		SELECT ?, role, content, tool_use_id, tool_name, is_error, tokens, created_at
		FROM messages WHERE session_id = ? ORDER BY id ASC`,
		newID, originalID)
	if err != nil {
		return fmt.Errorf("copy messages: %w", err)
	}

	return tx.Commit()
}

// SearchSessions performs a full-text search across message content and returns
// sessions that contain matching messages. Requires the FTS migration to have
// been applied (migration 2).
func (s *SQLiteStore) SearchSessions(query string) ([]*SessionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Use FTS5 match syntax.
	rows, err := s.db.QueryContext(context.Background(), `SELECT DISTINCT s.id, s.project_dir, s.provider, s.model,
		s.created_at, s.updated_at, COALESCE(s.parent_id, ''), s.status,
		COALESCE(s.title, ''), s.total_tokens, s.total_cost_usd
		FROM sessions s
		INNER JOIN messages_fts fts ON fts.session_id = s.id
		WHERE messages_fts MATCH ?
		ORDER BY s.updated_at DESC`, query)
	if err != nil {
		// Fall back to LIKE search if FTS is not available.
		return s.searchFallback(query)
	}
	defer func() { _ = rows.Close() }()

	var sessions []*SessionRecord
	for rows.Next() {
		var sess SessionRecord
		if err := rows.Scan(&sess.ID, &sess.ProjectDir, &sess.Provider, &sess.Model,
			&sess.CreatedAt, &sess.UpdatedAt, &sess.ParentID, &sess.Status,
			&sess.Title, &sess.TotalTokens, &sess.TotalCostUSD); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, &sess)
	}
	return sessions, rows.Err()
}

// searchFallback uses LIKE when FTS is not available.
func (s *SQLiteStore) searchFallback(query string) ([]*SessionRecord, error) {
	pattern := "%" + query + "%"
	rows, err := s.db.QueryContext(context.Background(), `SELECT DISTINCT s.id, s.project_dir, s.provider, s.model,
		s.created_at, s.updated_at, COALESCE(s.parent_id, ''), s.status,
		COALESCE(s.title, ''), s.total_tokens, s.total_cost_usd
		FROM sessions s
		INNER JOIN messages m ON m.session_id = s.id
		WHERE m.content LIKE ?
		ORDER BY s.updated_at DESC`, pattern)
	if err != nil {
		return nil, fmt.Errorf("search sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sessions []*SessionRecord
	for rows.Next() {
		var sess SessionRecord
		if err := rows.Scan(&sess.ID, &sess.ProjectDir, &sess.Provider, &sess.Model,
			&sess.CreatedAt, &sess.UpdatedAt, &sess.ParentID, &sess.Status,
			&sess.Title, &sess.TotalTokens, &sess.TotalCostUSD); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, &sess)
	}
	return sessions, rows.Err()
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

// Compact removes old messages from a session, keeping only the last keepLast
// messages. This is useful for long-running sessions where older context is
// no longer needed.
func (s *SQLiteStore) Compact(sessionID string, keepLast int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if keepLast <= 0 {
		return fmt.Errorf("keepLast must be positive, got %d", keepLast)
	}

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Find the cutoff: delete all messages except the last N.
	_, err = tx.ExecContext(context.Background(), `DELETE FROM messages
		WHERE session_id = ? AND id NOT IN (
			SELECT id FROM messages WHERE session_id = ? ORDER BY id DESC LIMIT ?
		)`, sessionID, sessionID, keepLast)
	if err != nil {
		return fmt.Errorf("compact messages: %w", err)
	}

	// Recalculate total tokens.
	var totalTokens int
	row := tx.QueryRowContext(context.Background(), "SELECT COALESCE(SUM(tokens), 0) FROM messages WHERE session_id = ?", sessionID)
	if err := row.Scan(&totalTokens); err != nil {
		return fmt.Errorf("sum tokens: %w", err)
	}

	_, err = tx.ExecContext(context.Background(), "UPDATE sessions SET total_tokens = ?, updated_at = ? WHERE id = ?",
		totalTokens, time.Now(), sessionID)
	if err != nil {
		return fmt.Errorf("update token total: %w", err)
	}

	return tx.Commit()
}

// GetSessionStats returns aggregate statistics for a session.
func (s *SQLiteStore) GetSessionStats(id string) (*SessionStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stats SessionStats
	var createdAt, updatedAt time.Time

	// Get session-level stats.
	row := s.db.QueryRowContext(context.Background(), `SELECT total_tokens, total_cost_usd, created_at, updated_at
		FROM sessions WHERE id = ?`, id)
	if err := row.Scan(&stats.TotalTokens, &stats.TotalCostUSD, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session %s not found", id)
		}
		return nil, fmt.Errorf("get session stats: %w", err)
	}
	stats.Duration = updatedAt.Sub(createdAt)

	// Count messages and tool calls.
	row = s.db.QueryRowContext(context.Background(), `SELECT COUNT(*), COALESCE(SUM(CASE WHEN tool_name != '' AND tool_name IS NOT NULL THEN 1 ELSE 0 END), 0)
		FROM messages WHERE session_id = ?`, id)
	if err := row.Scan(&stats.MessageCount, &stats.ToolCalls); err != nil {
		return nil, fmt.Errorf("count messages: %w", err)
	}

	return &stats, nil
}
