package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
)

// openTestDB opens an in-memory sqlite database, skipping the test cleanly if no
// driver is compiled in.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("no sqlite driver available: %v", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Skipf("sqlite driver not usable: %v", err)
	}
	return db
}

func TestSQLToolSelectFormatsRows(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	stmts := []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)`,
		`INSERT INTO users (id, name, age) VALUES (1, 'alice', 30)`,
		`INSERT INTO users (id, name, age) VALUES (2, 'bob', 25)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("setup %q: %v", s, err)
		}
	}

	out, err := runSQLQuery(ctx, db, "SELECT id, name, age FROM users ORDER BY id", 100)
	if err != nil {
		t.Fatalf("runSQLQuery: %v", err)
	}

	for _, want := range []string{"id", "name", "age", "alice", "bob", "30", "25", "(2 row(s))"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestSQLToolMaxRowsTruncates(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE n (v INTEGER)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := db.ExecContext(ctx, `INSERT INTO n (v) VALUES (?)`, i); err != nil {
			t.Fatal(err)
		}
	}

	out, err := runSQLQuery(ctx, db, "SELECT v FROM n ORDER BY v", 2)
	if err != nil {
		t.Fatalf("runSQLQuery: %v", err)
	}
	if !strings.Contains(out, "truncated at 2 rows") {
		t.Errorf("expected truncation notice, got:\n%s", out)
	}
}

func TestSQLToolExecuteReadOnly(t *testing.T) {
	// Confirm a driver is present, otherwise the Execute path can't connect.
	openTestDB(t).Close()

	tool := SQLTool{}
	in, _ := json.Marshal(map[string]any{
		"driver": "sqlite",
		"dsn":    ":memory:",
		"query":  "SELECT 1 AS one, 'hi' AS greeting",
	})
	out, err := tool.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "one") || !strings.Contains(out, "hi") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestSQLToolBlocksDestructiveByDefault(t *testing.T) {
	tool := SQLTool{}
	in, _ := json.Marshal(map[string]any{
		"driver": "sqlite",
		"dsn":    ":memory:",
		"query":  "DROP TABLE users",
	})
	_, err := tool.Execute(context.Background(), in)
	if err == nil {
		t.Fatal("expected destructive statement to be blocked in read-only mode")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("expected read-only error, got: %v", err)
	}
}

func TestSQLToolUnbundledDriverErrors(t *testing.T) {
	tool := SQLTool{}
	for _, drv := range []string{"postgres", "mysql"} {
		in, _ := json.Marshal(map[string]any{
			"driver": drv,
			"dsn":    "whatever",
			"query":  "SELECT 1",
		})
		_, err := tool.Execute(context.Background(), in)
		if err == nil {
			t.Fatalf("expected error for unbundled driver %q", drv)
		}
		if !strings.Contains(err.Error(), "not compiled") {
			t.Errorf("driver %q: expected 'not compiled' error, got: %v", drv, err)
		}
	}
}

func TestIsReadOnlyQuery(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{"SELECT * FROM t", true},
		{"  select 1", true},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", true},
		{"-- a comment\nSELECT 1", true},
		{"/* block */ SELECT 1", true},
		{"INSERT INTO t VALUES (1)", false},
		{"update t set x=1", false},
		{"DELETE FROM t", false},
		{"DROP TABLE t", false},
		{"PRAGMA foreign_keys=ON", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := isReadOnlyQuery(tc.query); got != tc.want {
			t.Errorf("isReadOnlyQuery(%q) = %v, want %v", tc.query, got, tc.want)
		}
	}
}

func TestSQLDriverName(t *testing.T) {
	tests := []struct {
		in          string
		wantDriver  string
		wantBundled bool
		wantErr     bool
	}{
		{"", "sqlite", true, false},
		{"sqlite", "sqlite", true, false},
		{"sqlite3", "sqlite", true, false},
		{"postgres", "postgres", false, false},
		{"mysql", "mysql", false, false},
		{"oracle", "", false, true},
	}
	for _, tc := range tests {
		drv, bundled, err := sqlDriverName(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("sqlDriverName(%q) err=%v, wantErr=%v", tc.in, err, tc.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if drv != tc.wantDriver || bundled != tc.wantBundled {
			t.Errorf("sqlDriverName(%q) = (%q,%v), want (%q,%v)", tc.in, drv, bundled, tc.wantDriver, tc.wantBundled)
		}
	}
}
