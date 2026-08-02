//go:build sqlite

package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	// Pure-Go SQLite driver (no CGO).
	_ "modernc.org/sqlite"
)

// testStore creates a temporary SQLiteStore for testing. Each test gets its
// own database file to avoid interference.
func testStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestCreateAndGetSession(t *testing.T) {
	store := testStore(t)

	sess := &SessionRecord{
		ID:         "sess-001",
		ProjectDir: "/home/user/project",
		Provider:   "anthropic",
		Model:      "claude-4-opus",
		Status:     "active",
		Title:      "Test Session",
	}

	if err := store.CreateSession(context.Background(), sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := store.GetSession(context.Background(), "sess-001")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	if got.ID != sess.ID {
		t.Errorf("ID = %q, want %q", got.ID, sess.ID)
	}
	if got.ProjectDir != sess.ProjectDir {
		t.Errorf("ProjectDir = %q, want %q", got.ProjectDir, sess.ProjectDir)
	}
	if got.Provider != sess.Provider {
		t.Errorf("Provider = %q, want %q", got.Provider, sess.Provider)
	}
	if got.Model != sess.Model {
		t.Errorf("Model = %q, want %q", got.Model, sess.Model)
	}
	if got.Status != "active" {
		t.Errorf("Status = %q, want %q", got.Status, "active")
	}
	if got.Title != sess.Title {
		t.Errorf("Title = %q, want %q", got.Title, sess.Title)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestGetSessionNotFound(t *testing.T) {
	store := testStore(t)

	_, err := store.GetSession(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestListSessions(t *testing.T) {
	store := testStore(t)

	// Create sessions in different projects.
	for i := 0; i < 5; i++ {
		sess := &SessionRecord{
			ID:         fmt.Sprintf("sess-%03d", i),
			ProjectDir: "/project/alpha",
			Provider:   "anthropic",
			Model:      "claude-4-opus",
		}
		if i >= 3 {
			sess.ProjectDir = "/project/beta"
		}
		if err := store.CreateSession(context.Background(), sess); err != nil {
			t.Fatalf("CreateSession %d: %v", i, err)
		}
		// Small delay so updated_at ordering is deterministic.
		time.Sleep(5 * time.Millisecond)
	}

	// List all sessions.
	all, err := store.ListSessions(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("ListSessions all: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("ListSessions all: got %d, want 5", len(all))
	}

	// List with limit.
	limited, err := store.ListSessions(context.Background(), "", 2)
	if err != nil {
		t.Fatalf("ListSessions limited: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("ListSessions limited: got %d, want 2", len(limited))
	}

	// List by project.
	alpha, err := store.ListSessions(context.Background(), "/project/alpha", 0)
	if err != nil {
		t.Fatalf("ListSessions alpha: %v", err)
	}
	if len(alpha) != 3 {
		t.Errorf("ListSessions alpha: got %d, want 3", len(alpha))
	}

	// Verify ordering: most recent first.
	if len(all) >= 2 {
		if all[0].UpdatedAt.Before(all[1].UpdatedAt) {
			t.Error("sessions should be ordered by updated_at DESC")
		}
	}
}

func TestAppendAndGetMessages(t *testing.T) {
	store := testStore(t)

	sess := &SessionRecord{
		ID:         "sess-msg-001",
		ProjectDir: "/project",
		Provider:   "anthropic",
		Model:      "claude-4-opus",
	}
	if err := store.CreateSession(context.Background(), sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Append messages.
	messages := []*MessageRecord{
		{Role: "user", Content: "Hello, can you help me?", Tokens: 10},
		{Role: "assistant", Content: "Of course! What do you need?", Tokens: 15},
		{Role: "user", Content: "Write a function to sort a list", Tokens: 12},
		{Role: "assistant", Content: "Here is a sort function...", ToolName: "file_write", ToolUseID: "tool-1", Tokens: 50},
	}

	for _, msg := range messages {
		if err := store.AppendMessage(context.Background(), "sess-msg-001", msg); err != nil {
			t.Fatalf("AppendMessage: %v", err)
		}
	}

	// Retrieve messages.
	got, err := store.GetMessages(context.Background(), "sess-msg-001")
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}

	if len(got) != 4 {
		t.Fatalf("GetMessages: got %d messages, want 4", len(got))
	}

	// Verify order and content.
	if got[0].Role != "user" || got[0].Content != "Hello, can you help me?" {
		t.Errorf("message 0: got role=%q content=%q", got[0].Role, got[0].Content)
	}
	if got[3].ToolName != "file_write" {
		t.Errorf("message 3 tool_name: got %q, want %q", got[3].ToolName, "file_write")
	}
	if got[3].ToolUseID != "tool-1" {
		t.Errorf("message 3 tool_use_id: got %q, want %q", got[3].ToolUseID, "tool-1")
	}

	// Verify session token total was updated.
	updated, err := store.GetSession(context.Background(), "sess-msg-001")
	if err != nil {
		t.Fatalf("GetSession after messages: %v", err)
	}
	expectedTokens := 10 + 15 + 12 + 50
	if updated.TotalTokens != expectedTokens {
		t.Errorf("TotalTokens = %d, want %d", updated.TotalTokens, expectedTokens)
	}
}

func TestForkSession(t *testing.T) {
	store := testStore(t)

	// Create original session with messages.
	sess := &SessionRecord{
		ID:         "original",
		ProjectDir: "/project",
		Provider:   "anthropic",
		Model:      "claude-4-opus",
		Title:      "Original Session",
	}
	if err := store.CreateSession(context.Background(), sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	msgs := []*MessageRecord{
		{Role: "user", Content: "First message", Tokens: 5},
		{Role: "assistant", Content: "First response", Tokens: 10},
		{Role: "user", Content: "Second message", Tokens: 8},
	}
	for _, msg := range msgs {
		if err := store.AppendMessage(context.Background(), "original", msg); err != nil {
			t.Fatalf("AppendMessage: %v", err)
		}
	}

	// Fork.
	if err := store.ForkSession(context.Background(), "original", "forked"); err != nil {
		t.Fatalf("ForkSession: %v", err)
	}

	// Verify fork exists.
	forked, err := store.GetSession(context.Background(), "forked")
	if err != nil {
		t.Fatalf("GetSession forked: %v", err)
	}
	if forked.ParentID != "original" {
		t.Errorf("ParentID = %q, want %q", forked.ParentID, "original")
	}
	if forked.Title != "Original Session" {
		t.Errorf("Title = %q, want %q", forked.Title, "Original Session")
	}

	// Verify forked messages.
	forkedMsgs, err := store.GetMessages(context.Background(), "forked")
	if err != nil {
		t.Fatalf("GetMessages forked: %v", err)
	}
	if len(forkedMsgs) != 3 {
		t.Fatalf("forked messages: got %d, want 3", len(forkedMsgs))
	}
	if forkedMsgs[0].Content != "First message" {
		t.Errorf("forked msg 0: got %q", forkedMsgs[0].Content)
	}

	// Verify original is unchanged.
	origMsgs, err := store.GetMessages(context.Background(), "original")
	if err != nil {
		t.Fatalf("GetMessages original: %v", err)
	}
	if len(origMsgs) != 3 {
		t.Errorf("original messages: got %d, want 3", len(origMsgs))
	}
}

func TestSearchSessions(t *testing.T) {
	store := testStore(t)

	// Create two sessions with different content.
	sess1 := &SessionRecord{
		ID: "search-1", ProjectDir: "/project", Provider: "anthropic", Model: "claude-4-opus",
	}
	sess2 := &SessionRecord{
		ID: "search-2", ProjectDir: "/project", Provider: "anthropic", Model: "claude-4-opus",
	}
	store.CreateSession(context.Background(), sess1)
	store.CreateSession(context.Background(), sess2)

	store.AppendMessage(context.Background(), "search-1", &MessageRecord{Role: "user", Content: "implement quicksort algorithm"})
	store.AppendMessage(context.Background(), "search-1", &MessageRecord{Role: "assistant", Content: "Here is a quicksort implementation in Go"})
	store.AppendMessage(context.Background(), "search-2", &MessageRecord{Role: "user", Content: "write a REST API handler"})
	store.AppendMessage(context.Background(), "search-2", &MessageRecord{Role: "assistant", Content: "Here is an HTTP handler for your API"})

	// Search for quicksort.
	results, err := store.SearchSessions(context.Background(), "quicksort")
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("search quicksort: got %d results, want 1", len(results))
	}
	if results[0].ID != "search-1" {
		t.Errorf("search result ID = %q, want %q", results[0].ID, "search-1")
	}

	// Search for API.
	results, err = store.SearchSessions(context.Background(), "API")
	if err != nil {
		t.Fatalf("SearchSessions API: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("search API: got %d results, want 1", len(results))
	}
	if results[0].ID != "search-2" {
		t.Errorf("search result ID = %q, want %q", results[0].ID, "search-2")
	}
}

func TestCompact(t *testing.T) {
	store := testStore(t)

	sess := &SessionRecord{
		ID: "compact-1", ProjectDir: "/project", Provider: "anthropic", Model: "claude-4-opus",
	}
	store.CreateSession(context.Background(), sess)

	// Add 10 messages.
	for i := 0; i < 10; i++ {
		msg := &MessageRecord{
			Role:    "user",
			Content: fmt.Sprintf("Message %d", i),
			Tokens:  100,
		}
		if err := store.AppendMessage(context.Background(), "compact-1", msg); err != nil {
			t.Fatalf("AppendMessage %d: %v", i, err)
		}
	}

	// Compact to keep last 3.
	if err := store.Compact(context.Background(), "compact-1", 3); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	// Verify only 3 messages remain.
	msgs, err := store.GetMessages(context.Background(), "compact-1")
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("after compact: got %d messages, want 3", len(msgs))
	}

	// Verify we kept the LAST 3 (messages 7, 8, 9).
	if msgs[0].Content != "Message 7" {
		t.Errorf("first remaining message: got %q, want %q", msgs[0].Content, "Message 7")
	}
	if msgs[2].Content != "Message 9" {
		t.Errorf("last remaining message: got %q, want %q", msgs[2].Content, "Message 9")
	}

	// Verify token total was recalculated.
	updated, err := store.GetSession(context.Background(), "compact-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if updated.TotalTokens != 300 { // 3 messages * 100 tokens
		t.Errorf("TotalTokens after compact = %d, want 300", updated.TotalTokens)
	}
}

func TestCompactInvalidKeepLast(t *testing.T) {
	store := testStore(t)

	sess := &SessionRecord{
		ID: "compact-invalid", ProjectDir: "/project", Provider: "anthropic", Model: "claude-4-opus",
	}
	store.CreateSession(context.Background(), sess)

	if err := store.Compact(context.Background(), "compact-invalid", 0); err == nil {
		t.Error("expected error for keepLast=0")
	}
	if err := store.Compact(context.Background(), "compact-invalid", -1); err == nil {
		t.Error("expected error for keepLast=-1")
	}
}

func TestDeleteSession(t *testing.T) {
	store := testStore(t)

	sess := &SessionRecord{
		ID: "delete-me", ProjectDir: "/project", Provider: "anthropic", Model: "claude-4-opus",
	}
	store.CreateSession(context.Background(), sess)
	store.AppendMessage(context.Background(), "delete-me", &MessageRecord{Role: "user", Content: "hello"})

	if err := store.DeleteSession(context.Background(), "delete-me"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	_, err := store.GetSession(context.Background(), "delete-me")
	if err == nil {
		t.Error("expected error after delete")
	}

	msgs, err := store.GetMessages(context.Background(), "delete-me")
	if err != nil {
		t.Fatalf("GetMessages after delete: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after delete, got %d", len(msgs))
	}
}

func TestDeleteSessionNotFound(t *testing.T) {
	store := testStore(t)

	err := store.DeleteSession(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestUpdateSession(t *testing.T) {
	store := testStore(t)

	sess := &SessionRecord{
		ID: "update-me", ProjectDir: "/project", Provider: "anthropic", Model: "claude-4-opus",
		Status: "active",
	}
	store.CreateSession(context.Background(), sess)

	err := store.UpdateSession(context.Background(), "update-me", map[string]interface{}{
		"status": "completed",
		"title":  "My Updated Session",
	})
	if err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}

	got, _ := store.GetSession(context.Background(), "update-me")
	if got.Status != "completed" {
		t.Errorf("Status = %q, want %q", got.Status, "completed")
	}
	if got.Title != "My Updated Session" {
		t.Errorf("Title = %q, want %q", got.Title, "My Updated Session")
	}
}

func TestUpdateSessionDisallowedField(t *testing.T) {
	store := testStore(t)

	sess := &SessionRecord{
		ID: "update-bad", ProjectDir: "/project", Provider: "anthropic", Model: "claude-4-opus",
	}
	store.CreateSession(context.Background(), sess)

	err := store.UpdateSession(context.Background(), "update-bad", map[string]interface{}{
		"id": "hacked",
	})
	if err == nil {
		t.Error("expected error for disallowed field")
	}
}

func TestGetSessionStats(t *testing.T) {
	store := testStore(t)

	sess := &SessionRecord{
		ID:           "stats-1",
		ProjectDir:   "/project",
		Provider:     "anthropic",
		Model:        "claude-4-opus",
		TotalCostUSD: 0.05,
	}
	store.CreateSession(context.Background(), sess)

	messages := []*MessageRecord{
		{Role: "user", Content: "hello", Tokens: 5},
		{Role: "assistant", Content: "hi there", Tokens: 10, ToolName: "greet"},
		{Role: "user", Content: "do something", Tokens: 8},
		{Role: "assistant", Content: "done", Tokens: 12, ToolName: "execute"},
		{Role: "assistant", Content: "also done", Tokens: 6},
	}
	for _, msg := range messages {
		store.AppendMessage(context.Background(), "stats-1", msg)
		time.Sleep(2 * time.Millisecond)
	}

	// Update cost manually.
	store.UpdateSession(context.Background(), "stats-1", map[string]interface{}{"total_cost_usd": 0.15})

	stats, err := store.GetSessionStats(context.Background(), "stats-1")
	if err != nil {
		t.Fatalf("GetSessionStats: %v", err)
	}

	if stats.MessageCount != 5 {
		t.Errorf("MessageCount = %d, want 5", stats.MessageCount)
	}
	if stats.ToolCalls != 2 {
		t.Errorf("ToolCalls = %d, want 2", stats.ToolCalls)
	}
	if stats.TotalTokens != 41 {
		t.Errorf("TotalTokens = %d, want 41", stats.TotalTokens)
	}
	if stats.TotalCostUSD != 0.15 {
		t.Errorf("TotalCostUSD = %f, want 0.15", stats.TotalCostUSD)
	}
	if stats.Duration <= 0 {
		t.Error("Duration should be positive")
	}
}

func TestConcurrentAccess(t *testing.T) {
	store := testStore(t)

	sess := &SessionRecord{
		ID: "concurrent", ProjectDir: "/project", Provider: "anthropic", Model: "claude-4-opus",
	}
	store.CreateSession(context.Background(), sess)

	// Run concurrent message appends.
	var wg sync.WaitGroup
	errCh := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			msg := &MessageRecord{
				Role:    "user",
				Content: fmt.Sprintf("Concurrent message %d", idx),
				Tokens:  10,
			}
			if err := store.AppendMessage(context.Background(), "concurrent", msg); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent append error: %v", err)
	}

	// All messages should be present.
	msgs, err := store.GetMessages(context.Background(), "concurrent")
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 20 {
		t.Errorf("got %d messages, want 20", len(msgs))
	}

	// Token total should be 200.
	got, _ := store.GetSession(context.Background(), "concurrent")
	if got.TotalTokens != 200 {
		t.Errorf("TotalTokens = %d, want 200", got.TotalTokens)
	}
}

func TestConcurrentReadsAndWrites(t *testing.T) {
	store := testStore(t)

	sess := &SessionRecord{
		ID: "rw-concurrent", ProjectDir: "/project", Provider: "anthropic", Model: "claude-4-opus",
	}
	store.CreateSession(context.Background(), sess)

	// Pre-populate some messages.
	for i := 0; i < 5; i++ {
		store.AppendMessage(context.Background(), "rw-concurrent", &MessageRecord{
			Role: "user", Content: fmt.Sprintf("Seed %d", i), Tokens: 1,
		})
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 30)

	// Writers.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			msg := &MessageRecord{
				Role: "assistant", Content: fmt.Sprintf("Reply %d", idx), Tokens: 2,
			}
			if err := store.AppendMessage(context.Background(), "rw-concurrent", msg); err != nil {
				errCh <- err
			}
		}(i)
	}

	// Readers.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.GetMessages(context.Background(), "rw-concurrent"); err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent rw error: %v", err)
	}
}

func TestDBCreatedOnFirstUse(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "subdir", "nested", "sessions.db")

	// The parent directories don't exist yet; NewSQLiteStore should fail
	// because sql.Open with a file path requires the parent dir.
	// However, let's test with an existing parent.
	dbPath2 := filepath.Join(dir, "sessions.db")

	// Verify file doesn't exist.
	if _, err := os.Stat(dbPath2); err == nil {
		t.Fatal("db file should not exist yet")
	}

	store, err := NewSQLiteStore(dbPath2)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	// File should now exist.
	if _, err := os.Stat(dbPath2); err != nil {
		t.Errorf("db file should exist after NewSQLiteStore: %v", err)
	}

	// Verify we can use it.
	sess := &SessionRecord{
		ID: "first-use", ProjectDir: "/project", Provider: "anthropic", Model: "claude-4-opus",
	}
	if err := store.CreateSession(context.Background(), sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Close and reopen to verify persistence.
	store.Close()

	store2, err := NewSQLiteStore(dbPath2)
	if err != nil {
		t.Fatalf("reopen NewSQLiteStore: %v", err)
	}
	defer store2.Close()

	got, err := store2.GetSession(context.Background(), "first-use")
	if err != nil {
		t.Fatalf("GetSession after reopen: %v", err)
	}
	if got.ID != "first-use" {
		t.Errorf("ID = %q, want %q", got.ID, "first-use")
	}

	_ = dbPath // suppress unused warning
}

func TestMigrationIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")

	// Open and close multiple times to verify migrations are idempotent.
	for i := 0; i < 3; i++ {
		store, err := NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("iteration %d: NewSQLiteStore: %v", i, err)
		}
		store.Close()
	}
}

func TestMessageIsErrorFlag(t *testing.T) {
	store := testStore(t)

	sess := &SessionRecord{
		ID: "error-test", ProjectDir: "/project", Provider: "anthropic", Model: "claude-4-opus",
	}
	store.CreateSession(context.Background(), sess)

	store.AppendMessage(context.Background(), "error-test", &MessageRecord{
		Role: "assistant", Content: "success response", IsError: false,
	})
	store.AppendMessage(context.Background(), "error-test", &MessageRecord{
		Role: "assistant", Content: "error: file not found", IsError: true,
	})

	msgs, _ := store.GetMessages(context.Background(), "error-test")
	if msgs[0].IsError {
		t.Error("message 0 should not be error")
	}
	if !msgs[1].IsError {
		t.Error("message 1 should be error")
	}
}
