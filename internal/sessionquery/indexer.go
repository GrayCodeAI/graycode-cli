package sessionquery

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/storage"
)

// Indexer manages incremental full-text indexing of sessions.
type Indexer struct {
	db          *DB
	sessionsDir string
	mu          sync.Mutex
}

// NewIndexer creates a new session indexer.
func NewIndexer(db *DB, sessionsDir string) *Indexer {
	if sessionsDir == "" {
		sessionsDir = storage.SessionsDir()
	}
	return &Indexer{
		db:          db,
		sessionsDir: sessionsDir,
	}
}

// IndexSession indexes or updates a single session if modified.
func (idx *Indexer) IndexSession(ctx context.Context, sessionID string) (bool, error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	conn := idx.db.Conn()
	if conn == nil {
		return false, fmt.Errorf("database connection closed")
	}

	jsonlPath := filepath.Join(idx.sessionsDir, sessionID+".jsonl")
	legacyPath := filepath.Join(idx.sessionsDir, sessionID+".json")

	var targetPath string
	var info os.FileInfo
	var err error

	if info, err = os.Stat(jsonlPath); err == nil && !info.IsDir() {
		targetPath = jsonlPath
	} else if info, err = os.Stat(legacyPath); err == nil && !info.IsDir() {
		targetPath = legacyPath
	} else {
		// Session file does not exist, remove from index if present
		_ = idx.removeSessionLocked(ctx, conn, sessionID)
		return false, fmt.Errorf("session %s not found on disk: %w", sessionID, session.ErrNotFound)
	}

	modTimeNano := info.ModTime().UnixNano()

	// Check if already indexed with matching modTime
	var storedModTime int64
	err = conn.QueryRowContext(ctx, "SELECT file_mod_time FROM sessions_meta WHERE session_id = ?", sessionID).Scan(&storedModTime)
	if err == nil && storedModTime == modTimeNano {
		// Fresh, no update needed
		return false, nil
	}

	// Load session
	sess, err := session.Load(sessionID)
	if err != nil {
		return false, fmt.Errorf("failed to load session %s for indexing: %w", sessionID, err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete old records
	if _, err := tx.ExecContext(ctx, "DELETE FROM messages_fts WHERE session_id = ?", sessionID); err != nil {
		return false, fmt.Errorf("failed to clear old FTS entries for %s: %w", sessionID, err)
	}

	// Insert message records
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO messages_fts(session_id, role, msg_index, content) VALUES (?, ?, ?, ?)")
	if err != nil {
		return false, fmt.Errorf("failed to prepare FTS insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	msgCount := 0
	for i, msg := range sess.Messages {
		content := extractMessageContent(&msg)
		if strings.TrimSpace(content) == "" {
			continue
		}
		msgCount++
		if _, err := stmt.ExecContext(ctx, sessionID, msg.Role, i, content); err != nil {
			return false, fmt.Errorf("failed to insert FTS entry: %w", err)
		}
	}

	// Upsert sessions_meta
	workspace := sess.CWD
	if workspace == "" {
		workspace = "."
	}

	updatedAtUnix := sess.UpdatedAt.Unix()
	if updatedAtUnix <= 0 {
		updatedAtUnix = time.Now().Unix()
	}

	upsertMeta := `
INSERT INTO sessions_meta(session_id, workspace, model, provider, updated_at, message_count, file_mod_time)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id) DO UPDATE SET
    workspace = excluded.workspace,
    model = excluded.model,
    provider = excluded.provider,
    updated_at = excluded.updated_at,
    message_count = excluded.message_count,
    file_mod_time = excluded.file_mod_time;
`
	if _, err := tx.ExecContext(ctx, upsertMeta, sessionID, workspace, sess.Model, sess.Provider, updatedAtUnix, msgCount, modTimeNano); err != nil {
		return false, fmt.Errorf("failed to upsert session meta: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit indexing transaction: %w", err)
	}

	_ = targetPath // referenced
	return true, nil
}

func (idx *Indexer) removeSessionLocked(ctx context.Context, conn *sql.DB, sessionID string) error {
	_, _ = conn.ExecContext(ctx, "DELETE FROM messages_fts WHERE session_id = ?", sessionID)
	_, _ = conn.ExecContext(ctx, "DELETE FROM sessions_meta WHERE session_id = ?", sessionID)
	return nil
}

// SyncAll incrementally indexes all session files in sessionsDir and removes deleted ones.
func (idx *Indexer) SyncAll(ctx context.Context) (int, error) {
	entries, err := os.ReadDir(idx.sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	activeIDs := make(map[string]bool)
	indexedCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		var sessionID string
		if strings.HasSuffix(name, ".jsonl") {
			sessionID = strings.TrimSuffix(name, ".jsonl")
		} else if strings.HasSuffix(name, ".json") {
			sessionID = strings.TrimSuffix(name, ".json")
		} else {
			continue
		}

		activeIDs[sessionID] = true
		updated, err := idx.IndexSession(ctx, sessionID)
		if err != nil {
			continue
		}
		if updated {
			indexedCount++
		}
	}

	// Clean up stale sessions in database no longer present on disk
	conn := idx.db.Conn()
	if conn != nil {
		rows, err := conn.QueryContext(ctx, "SELECT session_id FROM sessions_meta")
		if err == nil {
			var staleIDs []string
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err == nil {
					if !activeIDs[id] {
						staleIDs = append(staleIDs, id)
					}
				}
			}
			_ = rows.Close()

			idx.mu.Lock()
			for _, id := range staleIDs {
				_ = idx.removeSessionLocked(ctx, conn, id)
			}
			idx.mu.Unlock()
		}
	}

	return indexedCount, nil
}

// RebuildIndex drops all existing index tables and re-indexes all sessions from scratch.
func (idx *Indexer) RebuildIndex(ctx context.Context) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	conn := idx.db.Conn()
	if conn == nil {
		return fmt.Errorf("database connection closed")
	}

	dropSQL := `
DROP TABLE IF EXISTS messages_fts;
DROP TABLE IF EXISTS sessions_meta;
`
	if _, err := conn.ExecContext(ctx, dropSQL); err != nil {
		return fmt.Errorf("failed to drop tables: %w", err)
	}

	if _, err := conn.ExecContext(ctx, schemaDDL); err != nil {
		return fmt.Errorf("failed to recreate schema: %w", err)
	}

	idx.mu.Unlock()
	_, err := idx.SyncAll(ctx)
	idx.mu.Lock()
	return err
}

func extractMessageContent(msg *session.Message) string {
	var sb strings.Builder
	if msg.Content != "" {
		sb.WriteString(msg.Content)
		sb.WriteString(" ")
	}
	for _, p := range msg.ContentParts {
		if p.Type == "text" && p.Text != "" {
			sb.WriteString(p.Text)
			sb.WriteString(" ")
		}
	}
	for _, tc := range msg.ToolUse {
		if tc.Name != "" {
			sb.WriteString(tc.Name)
			sb.WriteString(" ")
		}
	}
	for _, tr := range msg.ToolResults {
		if tr.Content != "" {
			sb.WriteString(tr.Content)
			sb.WriteString(" ")
		}
	}
	return strings.TrimSpace(sb.String())
}
