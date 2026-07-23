package session

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestAcquireLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(setTestSessionsDir(t, dir), 0o755)

	lock, err := AcquireLock("test-lock")
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if lock == nil {
		t.Fatal("lock should not be nil")
	}
	lock.Release()
}

func TestAcquireLock_AlreadyLocked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(setTestSessionsDir(t, dir), 0o755)

	lock1, err := AcquireLock("locked-session")
	if err != nil {
		t.Fatal(err)
	}
	defer lock1.Release()

	_, err = AcquireLock("locked-session")
	if err == nil {
		t.Error("should fail when session is already locked")
	}
	var lockErr *SessionLockedError
	if err != nil {
		lockErr, _ = err.(*SessionLockedError)
		if lockErr == nil {
			t.Errorf("expected SessionLockedError, got %T", err)
		}
	}
}

func TestAcquireLock_StaleLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	sessDir := setTestSessionsDir(t, dir)
	_ = os.MkdirAll(sessDir, 0o755)

	// Create a stale lock (>5 min old)
	lockPath := sessDir + "/stale-session.lock"
	_ = os.WriteFile(lockPath, []byte("old"), 0o644)
	oldTime := time.Now().Add(-10 * time.Minute)
	_ = os.Chtimes(lockPath, oldTime, oldTime)

	lock, err := AcquireLock("stale-session")
	if err != nil {
		t.Fatalf("should acquire stale lock, got: %v", err)
	}
	if lock == nil {
		t.Fatal("lock should not be nil")
	}
	lock.Release()
}

func TestLockFile_Release(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(setTestSessionsDir(t, dir), 0o755)

	lock, _ := AcquireLock("release-test")
	lock.Release()

	// Should be able to acquire again after release
	lock2, err := AcquireLock("release-test")
	if err != nil {
		t.Fatalf("should acquire after release: %v", err)
	}
	lock2.Release()
}

func TestLockFile_Refresh(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(setTestSessionsDir(t, dir), 0o755)

	lock, _ := AcquireLock("refresh-test")
	defer lock.Release()

	lock.Refresh()
	// Should not panic
}

func TestLockFile_Release_Nil(t *testing.T) {
	t.Parallel()
	var lock *LockFile
	lock.Release() // should not panic
}

func TestContainsIgnoreCase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		s, substr string
		want      bool
	}{
		{"Hello World", "hello", true},
		{"Hello World", "WORLD", true},
		{"Hello", "xyz", false},
		{"", "x", false},
		{"abc", "", true},
	}
	for _, tt := range tests {
		if got := containsIgnoreCase(tt.s, tt.substr); got != tt.want {
			t.Errorf("containsIgnoreCase(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
		}
	}
}

func TestToLower(t *testing.T) {
	t.Parallel()
	if toLower("HELLO") != "hello" {
		t.Error("toLower(HELLO) should be hello")
	}
	if toLower("") != "" {
		t.Error("toLower empty")
	}
}

func TestIndexOf(t *testing.T) {
	t.Parallel()
	if indexOf("hello world", "world") != 6 {
		t.Error("indexOf should find 'world' at 6")
	}
	if indexOf("hello", "xyz") != -1 {
		t.Error("indexOf should return -1 for missing")
	}
}

func TestExtractContext(t *testing.T) {
	t.Parallel()
	content := "line1\nline2\nline3 match here\nline4\nline5"
	ctx := extractContext(content, "match", 1)
	if ctx == "" {
		t.Error("extractContext should return surrounding lines")
	}
}

func TestExtractContextMultibyteSafe(t *testing.T) {
	t.Parallel()
	// "日" is a 3-byte rune. The prefix places the ASCII query at byte 32 so
	// that idx-30 (=2) and idx+len(query)+50 (=87) both land mid-rune; a naive
	// byte cut would yield invalid UTF-8.
	content := strings.Repeat("日", 10) + "ab" + "match" + strings.Repeat("日", 20)
	ctx := extractContext(content, "match", 100)
	if !utf8.ValidString(ctx) {
		t.Errorf("extractContext returned invalid UTF-8: %q", ctx)
	}
	if !strings.Contains(ctx, "match") {
		t.Errorf("extractContext should still contain the query, got %q", ctx)
	}
	if !strings.HasPrefix(ctx, "...") || !strings.HasSuffix(ctx, "...") {
		t.Errorf("window strictly inside content should get both ellipses, got %q", ctx)
	}
}

func TestExtractContextMultibyteAtStart(t *testing.T) {
	t.Parallel()
	// Match at the very start; the trailing window edge (byte 55) lands mid-rune.
	content := "match" + strings.Repeat("日", 30)
	ctx := extractContext(content, "match", 100)
	if !utf8.ValidString(ctx) {
		t.Errorf("extractContext returned invalid UTF-8: %q", ctx)
	}
	if !strings.Contains(ctx, "match") {
		t.Errorf("extractContext should contain the query, got %q", ctx)
	}
	if strings.HasPrefix(ctx, "...") {
		t.Errorf("match at start should not get a leading ellipsis, got %q", ctx)
	}
}

func TestExtractContextASCIIUnchanged(t *testing.T) {
	t.Parallel()
	content := "line1\nline2\nline3 match here\nline4\nline5"
	ctx := extractContext(content, "match", 100)
	if !utf8.ValidString(ctx) {
		t.Errorf("invalid UTF-8: %q", ctx)
	}
	if !strings.Contains(ctx, "match") {
		t.Errorf("should contain query, got %q", ctx)
	}
}

func TestContainsTag(t *testing.T) {
	t.Parallel()
	if !containsTag("#important #urgent", "important") {
		t.Error("should find 'important' tag")
	}
	if containsTag("#important #urgent", "missing") {
		t.Error("should not find 'missing' tag")
	}
}

func TestCleanOldSessions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	sessDir := setTestSessionsDir(t, dir)
	_ = os.MkdirAll(sessDir, 0o755)

	// Create old sessions
	for i := 0; i < 5; i++ {
		sess := &Session{
			ID:        fmt.Sprintf("old-%d", i),
			CreatedAt: time.Now().Add(-60 * 24 * time.Hour),
			UpdatedAt: time.Now().Add(-60 * 24 * time.Hour),
			Messages:  []Message{{Role: "user", Content: "old msg"}},
		}
		_ = Save(sess)
	}

	removed, err := CleanOldSessions(30)
	if err != nil {
		t.Fatalf("CleanOldSessions: %v", err)
	}
	_ = removed
}

func TestExportToMarkdown(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	sessDir := setTestSessionsDir(t, dir)
	_ = os.MkdirAll(sessDir, 0o755)

	sess := &Session{
		ID:        "export-test",
		Model:     "test-model",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
	}
	_ = Save(sess)

	md := ExportToMarkdown(sess)
	if md == "" {
		t.Error("markdown should not be empty")
	}
}

func TestSearchSessions_Integration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	sessDir := setTestSessionsDir(t, dir)
	_ = os.MkdirAll(sessDir, 0o755)

	sess := &Session{
		ID:        "search-int",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages: []Message{
			{Role: "user", Content: "fix the golang authentication bug"},
			{Role: "assistant", Content: "I'll look at the auth module"},
		},
	}
	_ = Save(sess)

	results, _ := SearchSessions("golang", 10)
	_ = results
}

func TestSessionLockedError(t *testing.T) {
	t.Parallel()
	err := &SessionLockedError{ID: "test-123"}
	msg := err.Error()
	if msg == "" {
		t.Error("Error() should not be empty")
	}
}

func TestAddTag(t *testing.T) {
	sess := &Session{Name: ""}
	AddTag(sess, "important")
	if sess.Name == "" {
		t.Error("AddTag should modify Name")
	}
}

func TestRemoveTag(t *testing.T) {
	sess := &Session{Name: "tag:important,tag:urgent"}
	RemoveTag(sess, "important")
	// Just verify no panic — behavior depends on implementation
}

func TestNewAutoSaver(t *testing.T) {
	saved := false
	saver := NewAutoSaver(5*time.Minute, func() { saved = true })
	if saver == nil {
		t.Fatal("NewAutoSaver returned nil")
	}
	saver.Stop()
	_ = saved
}

func TestAutoSaver_Touch(t *testing.T) {
	saver := NewAutoSaver(5*time.Minute, func() {})
	defer saver.Stop()
	saver.Touch()
}

func TestAutoSaver_Reset(t *testing.T) {
	saver := NewAutoSaver(5*time.Minute, func() {})
	defer saver.Stop()
	saver.Reset()
}
