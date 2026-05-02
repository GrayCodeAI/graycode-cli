// Package cmdhistory implements a structured command history store using SQLite.
// Inspired by atuin, it records every Bash tool call with rich context
// (exit code, duration, working directory, git branch, session) and provides
// powerful FTS5-backed search and recall.
package cmdhistory

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Entry represents a single command execution with context.
type Entry struct {
	ID        string
	Command   string
	ExitCode  int
	Duration  time.Duration
	CWD       string
	GitBranch string
	SessionID string
	CreatedAt time.Time
}

// SearchOpts controls filtering for the Search method.
type SearchOpts struct {
	Limit     int
	ExitCode  *int      // filter by exit code (nil = any)
	CWD       string    // filter by working directory
	SessionID string    // filter by session
	Since     time.Time // only entries after this time
}

// CommandCount pairs a command string with its execution count.
type CommandCount struct {
	Command string
	Count   int
}

// DirCount pairs a directory path with its execution count.
type DirCount struct {
	Dir   string
	Count int
}

// HistoryStats holds aggregate usage statistics.
type HistoryStats struct {
	TotalCommands  int
	UniqueCommands int
	SuccessRate    float64
	TopCommands    []CommandCount
	TopDirectories []DirCount
}

// Store provides access to the command history database.
type Store struct {
	db *sql.DB
}

// New opens or creates the command history database at the given path.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := createSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Record saves a command execution with its context.
func (s *Store) Record(entry Entry) error {
	if entry.ID == "" {
		entry.ID = generateID()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.Exec(
		`INSERT INTO entries (id, command, exit_code, duration_ms, cwd, git_branch, session_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID,
		entry.Command,
		entry.ExitCode,
		entry.Duration.Milliseconds(),
		entry.CWD,
		entry.GitBranch,
		entry.SessionID,
		entry.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert entry: %w", err)
	}
	return nil
}

// Search finds commands matching the query using FTS5 full-text search.
// The query is passed through to the FTS5 MATCH operator, so callers can use
// FTS5 query syntax (e.g. prefix queries with *, boolean AND/OR/NOT).
// For simple substring searches, each word is automatically turned into a prefix token.
func (s *Store) Search(query string, opts SearchOpts) ([]Entry, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	// Build FTS5 match expression: split on whitespace and add * for prefix matching.
	words := strings.Fields(query)
	if len(words) == 0 {
		return nil, nil
	}
	for i, w := range words {
		// Only add wildcard if the word doesn't already contain FTS5 operators.
		if !strings.ContainsAny(w, "*\"{}") {
			words[i] = w + "*"
		}
	}
	ftsQuery := strings.Join(words, " ")

	var clauses []string
	var args []interface{}

	clauses = append(clauses, "entries_fts MATCH ?")
	args = append(args, ftsQuery)

	if opts.ExitCode != nil {
		clauses = append(clauses, "e.exit_code = ?")
		args = append(args, *opts.ExitCode)
	}
	if opts.CWD != "" {
		clauses = append(clauses, "e.cwd = ?")
		args = append(args, opts.CWD)
	}
	if opts.SessionID != "" {
		clauses = append(clauses, "e.session_id = ?")
		args = append(args, opts.SessionID)
	}
	if !opts.Since.IsZero() {
		clauses = append(clauses, "e.created_at >= ?")
		args = append(args, opts.Since.Format(time.RFC3339Nano))
	}

	where := strings.Join(clauses, " AND ")
	args = append(args, opts.Limit)

	q := fmt.Sprintf(
		`SELECT e.id, e.command, e.exit_code, e.duration_ms, e.cwd, e.git_branch, e.session_id, e.created_at
		 FROM entries e
		 JOIN entries_fts ON entries_fts.rowid = e.rowid
		 WHERE %s
		 ORDER BY e.created_at DESC
		 LIMIT ?`, where,
	)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	return scanEntries(rows)
}

// Recent returns the N most recent commands, ordered newest-first.
func (s *Store) Recent(n int) ([]Entry, error) {
	if n <= 0 {
		n = 20
	}

	rows, err := s.db.Query(
		`SELECT id, command, exit_code, duration_ms, cwd, git_branch, session_id, created_at
		 FROM entries
		 ORDER BY created_at DESC
		 LIMIT ?`, n,
	)
	if err != nil {
		return nil, fmt.Errorf("recent query: %w", err)
	}
	defer rows.Close()

	return scanEntries(rows)
}

// SearchByDir returns commands executed in a specific directory, newest-first.
func (s *Store) SearchByDir(dir string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(
		`SELECT id, command, exit_code, duration_ms, cwd, git_branch, session_id, created_at
		 FROM entries
		 WHERE cwd = ?
		 ORDER BY created_at DESC
		 LIMIT ?`, dir, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search by dir: %w", err)
	}
	defer rows.Close()

	return scanEntries(rows)
}

// Stats returns aggregate usage statistics computed in SQL for efficiency.
func (s *Store) Stats() (*HistoryStats, error) {
	stats := &HistoryStats{}

	// Total and unique command counts, plus success rate.
	err := s.db.QueryRow(`
		SELECT
			COUNT(*) AS total,
			COUNT(DISTINCT command) AS uniq,
			COALESCE(
				CAST(SUM(CASE WHEN exit_code = 0 THEN 1 ELSE 0 END) AS REAL) / NULLIF(COUNT(*), 0),
				0
			) AS success_rate
		FROM entries
	`).Scan(&stats.TotalCommands, &stats.UniqueCommands, &stats.SuccessRate)
	if err != nil {
		return nil, fmt.Errorf("stats totals: %w", err)
	}

	// Top 10 commands by frequency.
	cmdRows, err := s.db.Query(`
		SELECT command, COUNT(*) AS cnt
		FROM entries
		GROUP BY command
		ORDER BY cnt DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, fmt.Errorf("stats top commands: %w", err)
	}
	defer cmdRows.Close()

	for cmdRows.Next() {
		var cc CommandCount
		if err := cmdRows.Scan(&cc.Command, &cc.Count); err != nil {
			return nil, fmt.Errorf("scan command count: %w", err)
		}
		stats.TopCommands = append(stats.TopCommands, cc)
	}
	if err := cmdRows.Err(); err != nil {
		return nil, err
	}

	// Top 10 directories by frequency.
	dirRows, err := s.db.Query(`
		SELECT cwd, COUNT(*) AS cnt
		FROM entries
		WHERE cwd != ''
		GROUP BY cwd
		ORDER BY cnt DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, fmt.Errorf("stats top dirs: %w", err)
	}
	defer dirRows.Close()

	for dirRows.Next() {
		var dc DirCount
		if err := dirRows.Scan(&dc.Dir, &dc.Count); err != nil {
			return nil, fmt.Errorf("scan dir count: %w", err)
		}
		stats.TopDirectories = append(stats.TopDirectories, dc)
	}
	if err := dirRows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// --- internal helpers ---

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var entries []Entry
	for rows.Next() {
		var (
			e          Entry
			durMS      int64
			createdStr string
		)
		err := rows.Scan(
			&e.ID, &e.Command, &e.ExitCode, &durMS,
			&e.CWD, &e.GitBranch, &e.SessionID, &createdStr,
		)
		if err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		e.Duration = time.Duration(durMS) * time.Millisecond
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// generateID produces a short 8-character hex ID from crypto/rand.
func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
