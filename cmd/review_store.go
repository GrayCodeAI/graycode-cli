package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sightLib "github.com/GrayCodeAI/sight"
)

// ReviewStatus represents the state of a review.
type ReviewStatus string

const (
	ReviewStatusPending  ReviewStatus = "pending"
	ReviewStatusRunning  ReviewStatus = "running"
	ReviewStatusOpen     ReviewStatus = "open"
	ReviewStatusPassed   ReviewStatus = "passed"
	ReviewStatusFixed    ReviewStatus = "fixed"
	ReviewStatusClosed   ReviewStatus = "closed"
	ReviewStatusFailed   ReviewStatus = "failed" // review process itself failed
)

// ReviewRecord represents a persisted review in the SQLite store.
type ReviewRecord struct {
	ID          int64
	SHA         string
	Status      ReviewStatus
	Findings    []sightLib.Finding
	Report      string
	MaxSeverity string
	TokensUsed  int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const reviewSchema = `
CREATE TABLE IF NOT EXISTS reviews (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sha TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    findings_json TEXT DEFAULT '[]',
    report TEXT DEFAULT '',
    max_severity TEXT DEFAULT 'info',
    tokens_used INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_reviews_sha ON reviews(sha);
CREATE INDEX IF NOT EXISTS idx_reviews_status ON reviews(status);
CREATE INDEX IF NOT EXISTS idx_reviews_created ON reviews(created_at DESC);
`

// ReviewStore provides SQLite-backed review persistence.
type ReviewStore struct {
	db *sql.DB
	mu sync.RWMutex
}

// OpenReviewStore opens or creates the review database.
func OpenReviewStore(projectDir string) (*ReviewStore, error) {
	dbPath := filepath.Join(projectDir, ".hawk", "reviews.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open review db: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, err
	}

	s := &ReviewStore{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *ReviewStore) migrate() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS review_schema_version (version INTEGER PRIMARY KEY)`)
	if err != nil {
		return err
	}
	var current int
	_ = s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM review_schema_version").Scan(&current)

	migrations := []string{reviewSchema}
	for i := current; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		for _, stmt := range splitReviewStatements(migrations[i]) {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := tx.Exec(stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d: %w", i+1, err)
			}
		}
		if _, err := tx.Exec("INSERT INTO review_schema_version (version) VALUES (?)", i+1); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Create inserts a new review record and returns its ID.
func (s *ReviewStore) Create(sha string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(
		"INSERT INTO reviews (sha, status) VALUES (?, ?)",
		sha, ReviewStatusPending,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update sets the review result after completion.
func (s *ReviewStore) Update(id int64, status ReviewStatus, result *sightLib.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	findingsJSON, _ := json.Marshal(result.Findings)
	maxSev := result.MaxSeverity().String()

	_, err := s.db.Exec(
		`UPDATE reviews SET status=?, findings_json=?, report=?, max_severity=?, tokens_used=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		status, string(findingsJSON), result.Report, maxSev, result.Stats.TokensUsed, id,
	)
	return err
}

// SetStatus updates only the status field.
func (s *ReviewStore) SetStatus(id int64, status ReviewStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("UPDATE reviews SET status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?", status, id)
	return err
}

// Get retrieves a single review by ID.
func (s *ReviewStore) Get(id int64) (*ReviewRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scanOne(s.db.QueryRow(
		"SELECT id, sha, status, findings_json, report, max_severity, tokens_used, created_at, updated_at FROM reviews WHERE id=?", id,
	))
}

// GetBySHA retrieves the latest review for a commit.
func (s *ReviewStore) GetBySHA(sha string) (*ReviewRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scanOne(s.db.QueryRow(
		"SELECT id, sha, status, findings_json, report, max_severity, tokens_used, created_at, updated_at FROM reviews WHERE sha=? ORDER BY created_at DESC LIMIT 1", sha,
	))
}

// ListOpen returns all reviews with open/pending/running status.
func (s *ReviewStore) ListOpen() ([]*ReviewRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query("SELECT id, sha, status, findings_json, report, max_severity, tokens_used, created_at, updated_at FROM reviews WHERE status IN ('pending','running','open') ORDER BY created_at DESC")
}

// ListAll returns all reviews ordered by creation time.
func (s *ReviewStore) ListAll(limit int) ([]*ReviewRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query(fmt.Sprintf("SELECT id, sha, status, findings_json, report, max_severity, tokens_used, created_at, updated_at FROM reviews ORDER BY created_at DESC LIMIT %d", limit))
}

// Summary returns counts by status.
func (s *ReviewStore) Summary() (map[ReviewStatus]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT status, COUNT(*) FROM reviews GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[ReviewStatus]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		m[ReviewStatus(status)] = count
	}
	return m, rows.Err()
}

// Close closes the database.
func (s *ReviewStore) Close() error {
	return s.db.Close()
}

func (s *ReviewStore) scanOne(row *sql.Row) (*ReviewRecord, error) {
	r := &ReviewRecord{}
	var findingsJSON, status string
	err := row.Scan(&r.ID, &r.SHA, &status, &findingsJSON, &r.Report, &r.MaxSeverity, &r.TokensUsed, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	r.Status = ReviewStatus(status)
	_ = json.Unmarshal([]byte(findingsJSON), &r.Findings)
	return r, nil
}

func (s *ReviewStore) query(q string) ([]*ReviewRecord, error) {
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*ReviewRecord
	for rows.Next() {
		r := &ReviewRecord{}
		var findingsJSON, status string
		if err := rows.Scan(&r.ID, &r.SHA, &status, &findingsJSON, &r.Report, &r.MaxSeverity, &r.TokensUsed, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Status = ReviewStatus(status)
		_ = json.Unmarshal([]byte(findingsJSON), &r.Findings)
		results = append(results, r)
	}
	return results, rows.Err()
}

func splitReviewStatements(s string) []string {
	return strings.Split(s, ";")
}
