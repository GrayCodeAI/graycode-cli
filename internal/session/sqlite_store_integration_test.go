package session

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestSQLiteStore_CreateAndGet(t *testing.T) {
	store := newTestStore(t)
	rec := &SessionRecord{ID: "test-001", Model: "claude-sonnet", Provider: "anthropic", ProjectDir: "/tmp", Title: "test", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.CreateSession(rec); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, err := store.GetSession("test-001")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.ID != "test-001" || got.Model != "claude-sonnet" {
		t.Errorf("got %+v", got)
	}
}

func TestSQLiteStore_GetNotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetSession("x")
	if err == nil {
		t.Error("want error")
	}
}

func TestSQLiteStore_List(t *testing.T) {
	store := newTestStore(t)
	for i := 0; i < 3; i++ {
		_ = store.CreateSession(&SessionRecord{ID: fmt.Sprintf("l-%d", i), Model: "m", ProjectDir: "/p", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	}
	ss, err := store.ListSessions("/p", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) != 3 {
		t.Errorf("len=%d want 3", len(ss))
	}
}

func TestSQLiteStore_Messages(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(&SessionRecord{ID: "m1", Model: "m", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	_ = store.AppendMessage("m1", &MessageRecord{SessionID: "m1", Role: "user", Content: "hi", CreatedAt: time.Now()})
	_ = store.AppendMessage("m1", &MessageRecord{SessionID: "m1", Role: "assistant", Content: "hello", CreatedAt: time.Now()})
	msgs, err := store.GetMessages("m1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Errorf("len=%d want 2", len(msgs))
	}
}

func TestSQLiteStore_Update(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(&SessionRecord{ID: "u1", Model: "old", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	_ = store.UpdateSession("u1", map[string]interface{}{"model": "new"})
	got, _ := store.GetSession("u1")
	if got.Model != "new" {
		t.Errorf("model=%q want new", got.Model)
	}
}

func TestSQLiteStore_Delete(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(&SessionRecord{ID: "d1", Model: "m", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	_ = store.DeleteSession("d1")
	_, err := store.GetSession("d1")
	if err == nil {
		t.Error("want error after delete")
	}
}

func TestSQLiteStore_Fork(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(&SessionRecord{ID: "orig", Model: "m", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	_ = store.AppendMessage("orig", &MessageRecord{SessionID: "orig", Role: "user", Content: "x", CreatedAt: time.Now()})
	err := store.ForkSession("orig", "fork1")
	if err != nil {
		t.Fatal(err)
	}
	msgs, _ := store.GetMessages("fork1")
	if len(msgs) != 1 {
		t.Errorf("fork msgs=%d want 1", len(msgs))
	}
}

func TestSQLiteStore_Search(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(&SessionRecord{ID: "s1", Model: "m", Title: "golang review", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	_, err := store.SearchSessions("golang")
	if err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteStore_Stats(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(&SessionRecord{ID: "st1", Model: "m", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	for i := 0; i < 3; i++ {
		_ = store.AppendMessage("st1", &MessageRecord{SessionID: "st1", Role: "user", Content: "x", CreatedAt: time.Now()})
	}
	stats, err := store.GetSessionStats("st1")
	if err != nil {
		t.Fatal(err)
	}
	if stats == nil {
		t.Fatal("nil stats")
	}
}

func TestSQLiteStore_Compact(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(&SessionRecord{ID: "c1", Model: "m", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	for i := 0; i < 10; i++ {
		_ = store.AppendMessage("c1", &MessageRecord{SessionID: "c1", Role: "user", Content: fmt.Sprintf("m%d", i), CreatedAt: time.Now()})
	}
	if err := store.Compact("c1", 3); err != nil {
		t.Fatal(err)
	}
}

func TestSplitStatements(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"two", "SELECT 1; SELECT 2;", 2},
		{"one", "SELECT 1", 1},
		{"empty", "", 0},
		{"string semicolon", "SELECT 'a;b'; SELECT 2;", 2},
		{"trigger", "CREATE TRIGGER tr AFTER INSERT ON t BEGIN INSERT INTO log VALUES (1); END; SELECT 1;", 2},
		{"nested", "CREATE TRIGGER tr AFTER INSERT ON t BEGIN UPDATE x SET y=1; INSERT INTO z VALUES (2); END;", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := splitStatements(tt.input)
			if len(got) != tt.want {
				t.Errorf("splitStatements(%q) = %d (%v), want %d", tt.input, len(got), got, tt.want)
			}
		})
	}
}
