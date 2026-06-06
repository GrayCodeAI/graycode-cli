package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	// SQLite driver (pure Go) is already a direct dependency of hawk and is
	// registered under the "sqlite" driver name.
	_ "modernc.org/sqlite"
)

// SQLTool is a built-in tool for exploring relational databases. It connects to
// a database via database/sql, runs a single query, and returns the rows
// formatted as a plain-text table.
//
// Only the SQLite driver ships with hawk today (via modernc.org/sqlite). The
// Postgres and MySQL dialects are recognized so callers get a clear error
// instead of a confusing "unknown driver" failure: pull in a driver and the
// dialect light up without further changes here.
type SQLTool struct{}

func (SQLTool) Name() string      { return "SQL" }
func (SQLTool) Aliases() []string { return []string{"sql", "sql_query"} }

func (SQLTool) Description() string {
	return "Run a SQL query against a SQLite, Postgres, or MySQL database and " +
		"return the rows as a table. Read-only by default: destructive " +
		"statements (INSERT/UPDATE/DELETE/DROP/...) are blocked unless " +
		"allow_write is set to true. Only the SQLite driver is bundled; other " +
		"dialects require their driver to be compiled in."
}

// SQLTool is read-only by default and so is low risk, but writes are possible
// when explicitly allowed; treat it as medium overall so confirmation flows
// can prompt when appropriate.
func (SQLTool) RiskLevel() string { return "medium" }

func (SQLTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"driver": map[string]interface{}{
				"type":        "string",
				"description": "Database dialect: one of sqlite, postgres, mysql. Defaults to sqlite.",
				"enum":        []string{"sqlite", "postgres", "mysql"},
			},
			"dsn": map[string]interface{}{
				"type": "string",
				"description": "Data source name / connection string. For sqlite " +
					"this is the file path (or \":memory:\").",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The SQL statement to execute.",
			},
			"allow_write": map[string]interface{}{
				"type": "boolean",
				"description": "Allow destructive / mutating statements. When false " +
					"(the default) only read-only queries are permitted.",
			},
			"max_rows": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of rows to return (default 100).",
			},
		},
		"required": []string{"dsn", "query"},
	}
}

// driverName maps a user-facing dialect to its database/sql driver name. The
// bool reports whether hawk bundles a driver for that dialect.
func sqlDriverName(dialect string) (driver string, bundled bool, err error) {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "", "sqlite", "sqlite3":
		return "sqlite", true, nil
	case "postgres", "postgresql", "pgx":
		return "postgres", false, nil
	case "mysql", "mariadb":
		return "mysql", false, nil
	default:
		return "", false, fmt.Errorf("unsupported driver %q (want sqlite, postgres, or mysql)", dialect)
	}
}

// writeVerbs are statement leading keywords that mutate state. A query whose
// first token matches one of these is rejected unless allow_write is set.
var writeVerbs = map[string]bool{
	"insert":   true,
	"update":   true,
	"delete":   true,
	"drop":     true,
	"alter":    true,
	"create":   true,
	"truncate": true,
	"replace":  true,
	"merge":    true,
	"grant":    true,
	"revoke":   true,
	"attach":   true,
	"detach":   true,
	"vacuum":   true,
	"reindex":  true,
	"pragma":   true,
}

// isReadOnlyQuery reports whether the statement is safe to run in read-only
// mode. It inspects the first meaningful keyword after stripping leading
// comments and whitespace.
func isReadOnlyQuery(query string) bool {
	first := firstSQLKeyword(query)
	if first == "" {
		return false
	}
	return !writeVerbs[first]
}

// firstSQLKeyword returns the lowercased first keyword of a statement, skipping
// leading line (--) and block (/* */) comments and whitespace.
func firstSQLKeyword(query string) string {
	s := query
	for {
		s = strings.TrimLeft(s, " \t\r\n;")
		switch {
		case strings.HasPrefix(s, "--"):
			if idx := strings.IndexByte(s, '\n'); idx >= 0 {
				s = s[idx+1:]
				continue
			}
			return ""
		case strings.HasPrefix(s, "/*"):
			if idx := strings.Index(s, "*/"); idx >= 0 {
				s = s[idx+2:]
				continue
			}
			return ""
		}
		break
	}
	// Read the leading word.
	end := 0
	for end < len(s) {
		c := s[end]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			end++
			continue
		}
		break
	}
	return strings.ToLower(s[:end])
}

func (t SQLTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Driver     string `json:"driver"`
		DSN        string `json:"dsn"`
		Query      string `json:"query"`
		AllowWrite bool   `json:"allow_write"`
		MaxRows    int    `json:"max_rows"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if strings.TrimSpace(p.DSN) == "" {
		return "", fmt.Errorf("dsn is required")
	}
	if strings.TrimSpace(p.Query) == "" {
		return "", fmt.Errorf("query is required")
	}
	if p.MaxRows <= 0 {
		p.MaxRows = 100
	}

	driver, bundled, err := sqlDriverName(p.Driver)
	if err != nil {
		return "", err
	}
	if !bundled {
		return "", fmt.Errorf("the %s driver is not compiled into this build of hawk; only sqlite is available", driver)
	}

	if !p.AllowWrite && !isReadOnlyQuery(p.Query) {
		return "", fmt.Errorf("refusing to run a destructive statement in read-only mode; set allow_write=true to override")
	}

	db, err := sql.Open(driver, p.DSN)
	if err != nil {
		return "", fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return runSQLQuery(queryCtx, db, p.Query, p.MaxRows)
}

// querier is the subset of *sql.DB used by runSQLQuery, extracted so tests can
// supply an alternative if desired.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// runSQLQuery executes the query and renders the result set as a table.
func runSQLQuery(ctx context.Context, db querier, query string, maxRows int) (string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("columns: %w", err)
	}
	if len(cols) == 0 {
		return "(statement executed; no result columns)", nil
	}

	var table [][]string
	table = append(table, cols)

	truncated := false
	count := 0
	for rows.Next() {
		if count >= maxRows {
			truncated = true
			break
		}
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", fmt.Errorf("scan: %w", err)
		}
		row := make([]string, len(cols))
		for i, v := range raw {
			row[i] = formatSQLValue(v)
		}
		table = append(table, row)
		count++
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate rows: %w", err)
	}

	out := renderSQLTable(table)
	if truncated {
		out += fmt.Sprintf("\n... (truncated at %d rows)", maxRows)
	} else {
		out += fmt.Sprintf("\n(%d row(s))", count)
	}
	return out, nil
}

// formatSQLValue renders a scanned column value as a display string.
func formatSQLValue(v any) string {
	switch val := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return string(val)
	case time.Time:
		return val.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// renderSQLTable formats rows (first row treated as the header) into an
// aligned, pipe-delimited text table.
func renderSQLTable(table [][]string) string {
	if len(table) == 0 {
		return ""
	}
	widths := make([]int, len(table[0]))
	for _, row := range table {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	var b strings.Builder
	writeRow := func(row []string) {
		for i, cell := range row {
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(cell)
			if i < len(widths) {
				b.WriteString(strings.Repeat(" ", widths[i]-len(cell)))
			}
		}
		b.WriteByte('\n')
	}

	writeRow(table[0])
	// Separator under the header.
	for i := range table[0] {
		if i > 0 {
			b.WriteString("-+-")
		}
		b.WriteString(strings.Repeat("-", widths[i]))
	}
	b.WriteByte('\n')
	for _, row := range table[1:] {
		writeRow(row)
	}
	return strings.TrimRight(b.String(), "\n")
}
