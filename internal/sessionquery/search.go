package sessionquery

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/GrayCodeAI/hawk/internal/session"
)

// ErrUnauthorized indicates the caller is not authorized to access sessions in the requested workspace.
var ErrUnauthorized = errors.New("unauthorized: caller workspace does not have access to the target session")

// SearchParams specifies filters and parameters for full-text session queries.
type SearchParams struct {
	CallerWorkspace string   `json:"caller_workspace,omitempty"` // Enforces workspace authorization boundary
	Workspace       string   `json:"workspace,omitempty"`        // Filter by workspace path
	SessionID       string   `json:"session_id,omitempty"`       // Scope search to a specific session
	Query           string   `json:"query"`                      // Full-text search term
	Roles           []string `json:"roles,omitempty"`            // e.g. ["user", "assistant"]
	Limit           int      `json:"limit,omitempty"`            // Results page limit (default 10, max 50)
	Offset          int      `json:"offset,omitempty"`           // Pagination offset
	MaxBytes        int      `json:"max_bytes,omitempty"`        // Maximum total bytes of content to return (default 16 KiB)
}

// SearchMatch represents an individual matched message within a session.
type SearchMatch struct {
	SessionID string `json:"session_id"`
	Workspace string `json:"workspace"`
	Model     string `json:"model,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Role      string `json:"role"`
	MsgIndex  int    `json:"msg_index"`
	Snippet   string `json:"snippet"`
	Content   string `json:"content"`
}

// SearchResponse represents the paginated search result.
type SearchResponse struct {
	Matches    []SearchMatch `json:"matches"`
	TotalCount int           `json:"total_count"`
	HasMore    bool          `json:"has_more"`
	Offset     int           `json:"offset"`
	Limit      int           `json:"limit"`
	Query      string        `json:"query"`
}

// Search executes an FTS5 search query over the indexed session database.
func (d *DB) Search(ctx context.Context, params SearchParams) (*SearchResponse, error) {
	conn := d.Conn()
	if conn == nil {
		return nil, fmt.Errorf("database connection closed")
	}

	rawQuery := strings.TrimSpace(params.Query)
	if rawQuery == "" {
		return &SearchResponse{
			Matches:    []SearchMatch{},
			TotalCount: 0,
			HasMore:    false,
			Offset:     params.Offset,
			Limit:      params.Limit,
			Query:      rawQuery,
		}, nil
	}

	sanitizedQuery := sanitizeFTS5Query(rawQuery)
	if sanitizedQuery == "" {
		return &SearchResponse{
			Matches:    []SearchMatch{},
			TotalCount: 0,
			HasMore:    false,
			Offset:     params.Offset,
			Limit:      params.Limit,
			Query:      rawQuery,
		}, nil
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 10
	} else if limit > 50 {
		limit = 50
	}

	offset := params.Offset
	if offset < 0 {
		offset = 0
	}

	maxBytes := params.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 16 * 1024 // 16 KiB cap
	}

	// 1. Authorization check if specific SessionID is requested
	if params.SessionID != "" && params.CallerWorkspace != "" {
		var sessWorkspace string
		err := conn.QueryRowContext(ctx, "SELECT workspace FROM sessions_meta WHERE session_id = ?", params.SessionID).Scan(&sessWorkspace)
		if err == sql.ErrNoRows {
			// Session not in index yet or doesn't exist
			return nil, fmt.Errorf("session %s not found: %w", params.SessionID, session.ErrNotFound)
		} else if err != nil {
			return nil, fmt.Errorf("failed to query session metadata: %w", err)
		}

		if !isWorkspaceAuthorized(params.CallerWorkspace, sessWorkspace) {
			return nil, ErrUnauthorized
		}
	}

	// 2. Build Query
	var whereClauses []string
	var args []interface{}

	// FTS match clause
	whereClauses = append(whereClauses, "messages_fts MATCH ?")
	args = append(args, sanitizedQuery)

	if params.SessionID != "" {
		whereClauses = append(whereClauses, "f.session_id = ?")
		args = append(args, params.SessionID)
	}

	if params.Workspace != "" {
		whereClauses = append(whereClauses, "(m.workspace = ? OR m.workspace LIKE ?)")
		args = append(args, params.Workspace, params.Workspace+"/%")
	} else if params.CallerWorkspace != "" && params.SessionID == "" {
		// Restrict to caller's workspace
		whereClauses = append(whereClauses, "(m.workspace = ? OR m.workspace LIKE ?)")
		args = append(args, params.CallerWorkspace, params.CallerWorkspace+"/%")
	}

	if len(params.Roles) > 0 {
		rolePlaceholders := make([]string, len(params.Roles))
		for i, r := range params.Roles {
			rolePlaceholders[i] = "?"
			args = append(args, r)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("f.role IN (%s)", strings.Join(rolePlaceholders, ", ")))
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	// Total count query
	// #nosec G201 -- SQL where clauses use fixed parameterized placeholders with bind arguments
	countSQL := fmt.Sprintf(`
SELECT COUNT(*)
FROM messages_fts f
JOIN sessions_meta m ON f.session_id = m.session_id
WHERE %s
`, whereSQL)

	var totalCount int
	if err := conn.QueryRowContext(ctx, countSQL, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("search count query failed: %w", err)
	}

	// Select query with snippet
	// #nosec G201 -- SQL where clauses use fixed parameterized placeholders with bind arguments
	selectSQL := fmt.Sprintf(`
SELECT f.session_id, m.workspace, m.model, m.provider, f.role, f.msg_index, f.content,
       snippet(messages_fts, 3, '<b>', '</b>', '...', 24) AS match_snippet
FROM messages_fts f
JOIN sessions_meta m ON f.session_id = m.session_id
WHERE %s
ORDER BY f.rank
LIMIT ? OFFSET ?
`, whereSQL)

	selectArgs := append(args, limit+1, offset) // fetch limit+1 to check hasMore

	rows, err := conn.QueryContext(ctx, selectSQL, selectArgs...)
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var matches []SearchMatch
	currentBytes := 0
	hasMore := false

	for rows.Next() {
		if len(matches) >= limit {
			hasMore = true
			break
		}

		var m SearchMatch
		var rawSnippet sql.NullString
		if err := rows.Scan(&m.SessionID, &m.Workspace, &m.Model, &m.Provider, &m.Role, &m.MsgIndex, &m.Content, &rawSnippet); err != nil {
			return nil, fmt.Errorf("failed to scan search match: %w", err)
		}

		if rawSnippet.Valid {
			m.Snippet = rawSnippet.String
		}

		// Apply SecretDetector redaction at the read boundary
		m.Snippet = session.RedactSecrets(m.Snippet)
		m.Content = session.RedactSecrets(m.Content)

		// Byte bounding check
		matchSize := len(m.Snippet) + len(m.Content) + len(m.SessionID) + 64
		if currentBytes+matchSize > maxBytes && len(matches) > 0 {
			hasMore = true
			break
		}

		currentBytes += matchSize
		matches = append(matches, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return &SearchResponse{
		Matches:    matches,
		TotalCount: totalCount,
		HasMore:    hasMore || (offset+len(matches) < totalCount),
		Offset:     offset,
		Limit:      limit,
		Query:      rawQuery,
	}, nil
}

func isWorkspaceAuthorized(callerWorkspace, sessionWorkspace string) bool {
	if callerWorkspace == "" || callerWorkspace == "." {
		return true
	}
	cClean := filepath.Clean(callerWorkspace)
	sClean := filepath.Clean(sessionWorkspace)
	if cClean == sClean {
		return true
	}
	// Check if session is inside caller's workspace or vice versa
	rel, err := filepath.Rel(cClean, sClean)
	if err == nil && !strings.HasPrefix(rel, "..") {
		return true
	}
	return false
}

// sanitizeFTS5Query cleans raw query text into safe FTS5 terms with prefix support.
func sanitizeFTS5Query(raw string) string {
	terms := strings.FieldsFunc(raw, func(r rune) bool {
		return unicode.IsSpace(r) || r == '"' || r == '\'' || r == '(' || r == ')' || r == '*' || r == ':'
	})

	if len(terms) == 0 {
		return ""
	}

	var sanitized []string
	for _, term := range terms {
		t := strings.TrimSpace(term)
		if t == "" || strings.EqualFold(t, "AND") || strings.EqualFold(t, "OR") || strings.EqualFold(t, "NOT") {
			continue
		}
		// Prefix search for each term
		sanitized = append(sanitized, fmt.Sprintf("%q*", t))
	}

	if len(sanitized) == 0 {
		return ""
	}

	return strings.Join(sanitized, " ")
}
