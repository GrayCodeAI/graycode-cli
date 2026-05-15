package session

import (
	"testing"
)

func TestSplitStatements(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"two statements", "CREATE TABLE t (id INT); INSERT INTO t VALUES (1);", 2},
		{"single no semicolon", "SELECT 1", 1},
		{"empty", "", 0},
		{"string with semicolon", "SELECT 'hello; world'; SELECT 2;", 2},
		{"multiple spaces", "  SELECT 1 ;  SELECT 2 ;  ", 2},
		{"only semicolons", ";;;", 0},
		{"trailing semicolon", "SELECT 1;", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stmts := splitStatements(tt.input)
			if len(stmts) != tt.want {
				t.Errorf("splitStatements(%q) = %d stmts, want %d", tt.input, len(stmts), tt.want)
			}
		})
	}
}
